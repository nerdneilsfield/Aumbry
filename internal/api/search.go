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

	"github.com/nerdneilsfield/aumbry/internal/cjk"
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
	Snippet   string `db:"snippet"`
	PlainText string `db:"plain_text"`
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
	tag       string
	from, to  string
	unmounted bool
	limit     int
	offset    int
}

// likeEscaper escapes LIKE wildcards; the pattern is used with ESCAPE '\'.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// bm25Weights are the bm25() column weights shared by items_fts and items_fts_cjk, in table column
// order (item_id, title, summary, tags, plain_text): title 10 > tags 5 > summary 3 > plain_text 1.
// item_id is UNINDEXED and never contributes, so its weight is irrelevant.
const bm25Weights = `0.0, 10.0, 3.0, 5.0, 1.0`

// queryTerms is a search text split by the index that answers each part.
type queryTerms struct {
	trigram []string // FTS5 phrases for items_fts (non-CJK segments of 3 or more runes)
	cjk     []string // FTS5 phrases of bigrams for items_fts_cjk (CJK segments of 2 or more runes)
	short   []string // searched with LIKE: non-CJK segments under 3 runes and single CJK characters
	mark    []string // original text of the FTS-matched segments, highlighted in the snippet
}

// ftsPhrase quotes s as one FTS5 phrase so user input can never be parsed as FTS syntax.
func ftsPhrase(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// splitTerms splits every whitespace-separated word into CJK and other segments (internal/cjk) and
// assigns each segment to the index that can answer it. All segments are ANDed.
func splitTerms(text string) queryTerms {
	var t queryTerms
	for _, word := range strings.Fields(text) {
		for _, seg := range cjk.Segments(word) {
			n := utf8.RuneCountInString(seg.Text)
			switch {
			case seg.CJK && n >= 2:
				t.cjk = append(t.cjk, ftsPhrase(cjk.Bigrams(seg.Text)))
				t.mark = append(t.mark, seg.Text)
			case seg.CJK || n < 3:
				t.short = append(t.short, seg.Text)
			default:
				t.trigram = append(t.trigram, ftsPhrase(seg.Text))
				t.mark = append(t.mark, seg.Text)
			}
		}
	}
	return t
}

// builtSearch is a ready-to-run search statement. mark is non-empty when the snippet must be built
// in Go from the original text (see snippet.go) because items_fts_cjk, which stores expanded text,
// takes part in the query.
type builtSearch struct {
	sql  string
	args []any
	mark []string
}

// buildSearch assembles the SQL for q. Terms the trigram index (items_fts) or the bigram index
// (items_fts_cjk) can answer become MATCH conditions on the FTS tables, which then also order the
// results by bm25 (the sum when both tables take part); the rest are LIKE conditions.
func buildSearch(q searchQuery) (builtSearch, error) {
	t := splitTerms(q.text)
	from := `items i`
	columns := itemColumns + `, '' AS snippet, '' AS plain_text`
	var colArgs, args []any
	var where []string
	order := `i.report_date DESC, i.created_at DESC, i.id`
	var built builtSearch

	var ranks []string
	if len(t.trigram) > 0 {
		from = `items_fts JOIN items i ON i.id = items_fts.item_id`
		where = append(where, `items_fts MATCH ?`)
		args = append(args, strings.Join(t.trigram, " "))
		ranks = append(ranks, `bm25(items_fts, `+bm25Weights+`)`)
	}
	if len(t.cjk) > 0 {
		if len(t.trigram) > 0 {
			from += ` JOIN items_fts_cjk ON items_fts_cjk.item_id = i.id`
		} else {
			from = `items_fts_cjk JOIN items i ON i.id = items_fts_cjk.item_id`
		}
		where = append(where, `items_fts_cjk MATCH ?`)
		args = append(args, strings.Join(t.cjk, " "))
		ranks = append(ranks, `bm25(items_fts_cjk, `+bm25Weights+`)`)
		built.mark = t.mark
		columns = itemColumns + `, '' AS snippet, i.plain_text AS plain_text`
	} else if len(t.trigram) > 0 {
		columns = itemColumns + `, snippet(items_fts, -1, ?, ?, '…', 24) AS snippet, '' AS plain_text`
		colArgs = []any{markStart, markEnd}
	}
	if len(ranks) > 0 {
		order = strings.Join(ranks, ` + `) + `, ` + order
	}
	for _, tok := range t.short {
		pattern := "%" + likeEscaper.Replace(tok) + "%"
		where = append(where, `(i.title LIKE ? ESCAPE '\' OR i.summary LIKE ? ESCAPE '\' OR i.plain_text LIKE ? ESCAPE '\' OR i.tags_json LIKE ? ESCAPE '\')`)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	if q.dirIDs != nil {
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
	if q.tag != "" {
		where = append(where, `EXISTS (SELECT 1 FROM json_each(i.tags_json) WHERE json_each.value = ?)`)
		args = append(args, q.tag)
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
	var err error
	built.sql, built.args, err = sqlx.In(query, all...)
	return built, err
}

func (s *Server) search(ctx context.Context, q searchQuery) ([]searchResult, error) {
	if q.dirIDs != nil && len(q.dirIDs) == 0 {
		return []searchResult{}, nil
	}
	built, err := buildSearch(q)
	if err != nil {
		return nil, err
	}
	var rows []searchRow
	if err := s.db.SelectContext(ctx, &rows, built.sql, built.args...); err != nil {
		return nil, err
	}
	results := make([]searchResult, len(rows))
	for i, row := range rows {
		snippet := row.Snippet
		if len(built.mark) > 0 {
			snippet = originalSnippet([]string{row.PlainText, row.Summary, row.Title}, built.mark)
		}
		results[i] = searchResult{itemSummary: row.summary(), Snippet: renderSnippet(snippet)}
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
	params := r.URL.Query()
	q := searchQuery{
		text: params.Get("q"),
		typ:  strings.TrimSpace(params.Get("type")),
		tag:  strings.TrimSpace(params.Get("tag")),
	}
	if q.typ != "" && q.typ != "report" && q.typ != "post" {
		writeError(w, http.StatusBadRequest, "invalid_request", "type must be report or post")
		return
	}
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
