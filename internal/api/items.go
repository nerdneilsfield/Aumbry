package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"github.com/nerdneilsfield/aumbry/internal/storage"
)

// itemRow is the list-view projection of an item.
type itemRow struct {
	ID          string `db:"id"`
	Type        string `db:"type"`
	Title       string `db:"title"`
	Date        string `db:"report_date"`
	Summary     string `db:"summary"`
	TagsJSON    string `db:"tags_json"`
	SourceAgent string `db:"source_agent"`
}

const itemColumns = `i.id, i.type, i.title, i.report_date, i.summary, i.tags_json, i.source_agent`

type itemSummary struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Date        string   `json:"date"`
	Summary     string   `json:"summary"`
	Tags        []string `json:"tags"`
	SourceAgent string   `json:"source_agent"`
}

func parseTags(raw string) []string {
	tags := []string{}
	_ = json.Unmarshal([]byte(raw), &tags)
	if tags == nil {
		tags = []string{}
	}
	return tags
}

func (row itemRow) summary() itemSummary {
	return itemSummary{
		ID: row.ID, Type: row.Type, Title: row.Title, Date: row.Date, Summary: row.Summary,
		Tags: parseTags(row.TagsJSON), SourceAgent: row.SourceAgent,
	}
}

func summaries(rows []itemRow) []itemSummary {
	out := make([]itemSummary, len(rows))
	for i, row := range rows {
		out[i] = row.summary()
	}
	return out
}

type dirRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type itemDetail struct {
	itemSummary
	CanonicalURL string         `json:"canonical_url"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
	Dirs         []dirRef       `json:"dirs"`
	Related      []itemSummary  `json:"related"`
	Neighbors    *itemNeighbors `json:"neighbors"`
}

// itemMounts returns the directories item id is mounted in.
func itemMounts(ctx context.Context, q sqlx.QueryerContext, id string) ([]string, error) {
	var dirs []string
	err := sqlx.SelectContext(ctx, q, &dirs, `SELECT directory_id FROM directory_items WHERE item_id = ? ORDER BY directory_id`, id)
	return dirs, err
}

// canViewItem reports whether v may read item id; the item must exist.
func (s *Server) canViewItem(ctx context.Context, t *dirTree, v viewer, id string) (bool, error) {
	if v.admin {
		return true, nil
	}
	mounts, err := itemMounts(ctx, s.db, id)
	if err != nil {
		return false, err
	}
	return len(t.visibleDirs(v, mounts)) > 0, nil
}

// itemExists reports whether an item with this id exists.
func (s *Server) itemExists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.db.GetContext(ctx, &n, `SELECT count(*) FROM items WHERE id = ?`, id)
	return n > 0, err
}

// authorizeItem answers 404 or 403 itself and returns false when the caller may not read the item.
func (s *Server) authorizeItem(w http.ResponseWriter, r *http.Request, id string) bool {
	ctx := r.Context()
	exists, err := s.itemExists(ctx, id)
	if err != nil {
		internalError(w, r, err)
		return false
	}
	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return false
	}
	t, err := s.loadTree(ctx)
	if err != nil {
		internalError(w, r, err)
		return false
	}
	ok, err := s.canViewItem(ctx, t, s.viewerFor(r, t), id)
	if err != nil {
		internalError(w, r, err)
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden", "this item is not accessible")
		return false
	}
	return true
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	var row struct {
		itemRow
		CanonicalURL string `db:"canonical_url"`
		RelatedJSON  string `db:"related_ids_json"`
		CreatedAt    string `db:"created_at"`
		UpdatedAt    string `db:"updated_at"`
	}
	err := s.db.GetContext(ctx, &row, `SELECT `+itemColumns+`, i.canonical_url, i.related_ids_json,
		strftime('%Y-%m-%dT%H:%M:%SZ', i.created_at) AS created_at,
		strftime('%Y-%m-%dT%H:%M:%SZ', i.updated_at) AS updated_at
		FROM items i WHERE i.id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	t, err := s.loadTree(ctx)
	if err != nil {
		internalError(w, r, err)
		return
	}
	v := s.viewerFor(r, t)
	mounts, err := itemMounts(ctx, s.db, id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	visible := t.visibleDirs(v, mounts)
	if len(visible) == 0 && !v.admin {
		writeError(w, http.StatusForbidden, "forbidden", "this item is not accessible")
		return
	}
	detail := itemDetail{
		itemSummary:  row.summary(),
		CanonicalURL: row.CanonicalURL,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
		Dirs:         make([]dirRef, 0, len(visible)),
		Related:      []itemSummary{},
	}
	for _, d := range visible {
		// A mount whose directory is missing from the loaded tree has no name to show.
		if rec := t.dirs[d]; rec != nil {
			detail.Dirs = append(detail.Dirs, dirRef{ID: d, Name: rec.Name})
		}
	}
	detail.Related, err = s.relatedItems(ctx, t, v, row.RelatedJSON)
	if err != nil {
		internalError(w, r, err)
		return
	}
	// ?dir= names the directory the reader came from; it only chooses among mounts the caller may open.
	detail.Neighbors, err = s.neighborsFor(ctx, t, visible, r.URL.Query().Get("dir"), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// relatedItems resolves related_ids_json to the summaries v may see, keeping the document order.
func (s *Server) relatedItems(ctx context.Context, t *dirTree, v viewer, relatedJSON string) ([]itemSummary, error) {
	ids := parseTags(relatedJSON)
	out := []itemSummary{}
	if len(ids) == 0 {
		return out, nil
	}
	query, args, err := sqlx.In(`SELECT `+itemColumns+` FROM items i WHERE i.id IN (?)`, ids)
	if err != nil {
		return nil, err
	}
	var rows []itemRow
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	byID := make(map[string]itemRow, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	for _, id := range ids {
		row, ok := byID[id]
		if !ok {
			continue
		}
		if ok, err := s.canViewItem(ctx, t, v, id); err != nil {
			return nil, err
		} else if ok {
			out = append(out, row.summary())
		}
	}
	return out, nil
}

func (s *Server) getItemContent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !s.authorizeItem(w, r, id) {
		return
	}
	var path string
	if err := s.db.GetContext(r.Context(), &path, `SELECT xml_storage_path FROM items WHERE id = ?`, id); err != nil {
		internalError(w, r, err)
		return
	}
	body, err := s.store.Open(r.Context(), path)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "item content is missing from storage")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Tenant-written XML opened directly must not run scripts (XHTML/SVG elements) in the app origin.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = io.Copy(w, body)
}
