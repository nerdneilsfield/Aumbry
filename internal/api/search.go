package api

import (
	"context"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
)

const (
	defaultLimit = 20
	maxLimit     = 100

	// Control characters cannot occur in XML text, so they safely delimit the highlighted
	// spans until the snippet is HTML-escaped.
	markStart = "\x01"
	markEnd   = "\x02"
)

type searchResult struct {
	itemSummary
	Snippet string `json:"snippet"`
}

type searchRow struct {
	itemRow
	Snippet string `db:"snippet"`
}

type searchResponse struct {
	Results []searchResult `json:"results"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
}

// searchQuery describes one search. dirIDs restricts results to items mounted in any of them
// (nil means no restriction, an empty non-nil slice matches nothing).
type searchQuery struct {
	text      string
	dirIDs    []string
	typ       string
	from, to  string
	unmounted bool
	limit     int
	offset    int
}

// likeEscaper escapes LIKE wildcards; the pattern is used with ESCAPE '\'.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// splitTerms separates the query into tokens usable with the trigram index (3 or more runes,
// quoted as FTS5 phrases so user input can never be parsed as FTS syntax) and short tokens,
// which the trigram index cannot match and are searched with LIKE instead.
func splitTerms(text string) (match string, short []string) {
	var phrases []string
	for _, tok := range strings.Fields(text) {
		if utf8.RuneCountInString(tok) < 3 {
			short = append(short, tok)
			continue
		}
		phrases = append(phrases, `"`+strings.ReplaceAll(tok, `"`, `""`)+`"`)
	}
	return strings.Join(phrases, " "), short
}

func (s *Server) search(ctx context.Context, q searchQuery) ([]searchResult, error) {
	match, short := splitTerms(q.text)
	from := `items i`
	columns := itemColumns + `, '' AS snippet`
	var colArgs, args []any
	var where []string
	order := `i.report_date DESC, i.created_at DESC, i.id`

	if match != "" {
		from = `items_fts JOIN items i ON i.id = items_fts.item_id`
		columns = itemColumns + `, snippet(items_fts, -1, ?, ?, '…', 24) AS snippet`
		colArgs = []any{markStart, markEnd}
		where = append(where, `items_fts MATCH ?`)
		args = append(args, match)
		order = `bm25(items_fts), ` + order
	}
	for _, tok := range short {
		pattern := "%" + likeEscaper.Replace(tok) + "%"
		where = append(where, `(i.title LIKE ? ESCAPE '\' OR i.summary LIKE ? ESCAPE '\' OR i.plain_text LIKE ? ESCAPE '\' OR i.tags_json LIKE ? ESCAPE '\')`)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	if q.dirIDs != nil {
		if len(q.dirIDs) == 0 {
			return []searchResult{}, nil
		}
		where = append(where, `i.id IN (SELECT item_id FROM directory_items WHERE directory_id IN (?))`)
		args = append(args, q.dirIDs)
	}
	if q.unmounted {
		where = append(where, `NOT EXISTS (SELECT 1 FROM directory_items di WHERE di.item_id = i.id)`)
	}
	if q.typ != "" {
		where = append(where, `i.type = ?`)
		args = append(args, q.typ)
	}
	if q.from != "" {
		where = append(where, `i.report_date >= ?`)
		args = append(args, q.from)
	}
	if q.to != "" {
		where = append(where, `i.report_date <= ?`)
		args = append(args, q.to)
	}

	query := `SELECT ` + columns + ` FROM ` + from
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	all := append(append(colArgs, args...), q.limit, q.offset)
	query, all, err := sqlx.In(query, all...)
	if err != nil {
		return nil, err
	}
	var rows []searchRow
	if err := s.db.SelectContext(ctx, &rows, query, all...); err != nil {
		return nil, err
	}
	results := make([]searchResult, len(rows))
	for i, row := range rows {
		results[i] = searchResult{itemSummary: row.summary(), Snippet: renderSnippet(row.Snippet)}
	}
	return results, nil
}

// renderSnippet HTML-escapes a snippet and turns the control-character markers into <mark>
// tags, so the client may insert the result as HTML.
func renderSnippet(raw string) string {
	escaped := html.EscapeString(raw)
	escaped = strings.ReplaceAll(escaped, markStart, "<mark>")
	return strings.ReplaceAll(escaped, markEnd, "</mark>")
}

// pageParams parses limit and offset; ok is false after a 400 has been written.
func pageParams(w http.ResponseWriter, r *http.Request) (limit, offset int, ok bool) {
	limit, offset = defaultLimit, 0
	for name, dst := range map[string]*int{"limit": &limit, "offset": &offset} {
		raw := r.URL.Query().Get(name)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || (name == "limit" && n < 1) {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid "+name)
			return 0, 0, false
		}
		*dst = n
	}
	return min(limit, maxLimit), offset, true
}

func writeResults(w http.ResponseWriter, results []searchResult, q searchQuery) {
	writeJSON(w, http.StatusOK, searchResponse{Results: results, Limit: q.limit, Offset: q.offset})
}

func (s *Server) searchDir(w http.ResponseWriter, r *http.Request) {
	t, v, d, ok := s.requireDir(w, r)
	if !ok {
		return
	}
	q := searchQuery{text: r.URL.Query().Get("q")}
	if q.limit, q.offset, ok = pageParams(w, r); !ok {
		return
	}
	// Only directories the caller may open: a protected child under an open parent stays hidden.
	q.dirIDs = t.visibleDirs(v, t.subtree(d.ID))
	results, err := s.search(r.Context(), q)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeResults(w, results, q)
}

func (s *Server) adminSearch(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	q := searchQuery{
		text: params.Get("q"), typ: params.Get("type"), from: params.Get("from"), to: params.Get("to"),
		unmounted: params.Get("unmounted") == "1",
	}
	if q.typ != "" && q.typ != "report" && q.typ != "post" {
		writeError(w, http.StatusBadRequest, "invalid_request", "type must be report or post")
		return
	}
	for _, date := range []string{q.from, q.to} {
		if _, err := time.Parse(time.DateOnly, date); date != "" && err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "from and to must be YYYY-MM-DD")
			return
		}
	}
	if dir := params.Get("dir"); dir != "" {
		q.dirIDs = []string{dir}
	}
	var ok bool
	if q.limit, q.offset, ok = pageParams(w, r); !ok {
		return
	}
	results, err := s.search(r.Context(), q)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeResults(w, results, q)
}
