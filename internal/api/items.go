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

type referencingItem struct {
	itemSummary
	LinkKinds []string `json:"link_kinds"`
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
	CanonicalURL  string            `json:"canonical_url"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
	Dirs          []dirRef          `json:"dirs"`
	Related       []itemSummary     `json:"related"`
	ReferencedBy  []referencingItem `json:"referenced_by"`
	RevisionCount int               `json:"revision_count"`
	CurrentRev    int               `json:"current_rev"`
	Neighbors     *itemNeighbors    `json:"neighbors"`
}

// itemMounts returns the directories item id is mounted in.
func itemMounts(ctx context.Context, q sqlx.QueryerContext, id string) ([]string, error) {
	var dirs []string
	err := sqlx.SelectContext(ctx, q, &dirs, `SELECT directory_id FROM directory_items WHERE item_id = ? ORDER BY directory_id`, id)
	return dirs, err
}

func canViewItemFrom(ctx context.Context, q sqlx.QueryerContext, t *dirTree, v viewer, id string) (bool, error) {
	if v.admin {
		return true, nil
	}
	mounts, err := itemMounts(ctx, q, id)
	if err != nil {
		return false, err
	}
	return len(t.visibleDirs(v, mounts)) > 0, nil
}

func itemExistsFrom(ctx context.Context, q sqlx.QueryerContext, id string) (bool, error) {
	var n int
	err := sqlx.GetContext(ctx, q, &n, `SELECT count(*) FROM items WHERE id = ?`, id)
	return n > 0, err
}

// itemExists reports whether an item with this id exists.
func (s *Server) itemExists(ctx context.Context, id string) (bool, error) {
	return itemExistsFrom(ctx, s.db, id)
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var row struct {
		itemRow
		CanonicalURL string `db:"canonical_url"`
		RelatedJSON  string `db:"related_ids_json"`
		CreatedAt    string `db:"created_at"`
		UpdatedAt    string `db:"updated_at"`
	}
	err = sqlx.GetContext(ctx, tx, &row, `SELECT `+itemColumns+`, i.canonical_url, i.related_ids_json,
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
	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		internalError(w, r, err)
		return
	}
	v := s.viewerFor(r, t)
	mounts, err := itemMounts(ctx, tx, id)
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
		ReferencedBy: []referencingItem{},
	}
	for _, d := range visible {
		// A mount whose directory is missing from the loaded tree has no name to show.
		if rec := t.dirs[d]; rec != nil {
			detail.Dirs = append(detail.Dirs, dirRef{ID: d, Name: rec.Name})
		}
	}
	detail.Related, err = relatedItemsFrom(ctx, tx, t, v, row.RelatedJSON)
	if err != nil {
		internalError(w, r, err)
		return
	}
	detail.ReferencedBy, err = referencedByFrom(ctx, tx, t, v, id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	detail.RevisionCount, detail.CurrentRev, err = revisionStatsFrom(ctx, tx, t, v, id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	// ?dir= names the directory the reader came from; it only chooses among mounts the caller may open.
	detail.Neighbors, err = neighborsForFrom(ctx, tx, t, visible, r.URL.Query().Get("dir"), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	_ = tx.Rollback()

	writeJSON(w, http.StatusOK, detail)
}

// relatedItemsFrom resolves related_ids_json to the summaries v may see, keeping the document order.
func relatedItemsFrom(ctx context.Context, q sqlx.QueryerContext, t *dirTree, v viewer, relatedJSON string) ([]itemSummary, error) {
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
	if err = sqlx.SelectContext(ctx, q, &rows, query, args...); err != nil {
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
		if visible, err := canViewItemFrom(ctx, q, t, v, id); err != nil {
			return nil, err
		} else if visible {
			out = append(out, row.summary())
		}
	}
	return out, nil
}

// referencedByFrom returns published items that cite targetID via item_links and are visible to v.
func referencedByFrom(ctx context.Context, q sqlx.QueryerContext, t *dirTree, v viewer, targetID string) ([]referencingItem, error) {
	var rows []struct {
		itemRow
		Kind string `db:"kind"`
	}
	err := sqlx.SelectContext(ctx, q, &rows, `SELECT `+itemColumns+`, il.kind
		FROM item_links il
		JOIN items i ON i.id = il.source_item_id
		WHERE il.target_item_id = ?
		  AND EXISTS (SELECT 1 FROM directory_items di WHERE di.item_id = i.id)
		ORDER BY i.report_date DESC, i.created_at DESC, i.id ASC, il.kind ASC`, targetID)
	if err != nil {
		return nil, err
	}
	out := []referencingItem{}
	indexByID := make(map[string]int)
	visCache := make(map[string]bool)

	for _, row := range rows {
		visible, cached := visCache[row.ID]
		if !cached {
			visible, err = canViewItemFrom(ctx, q, t, v, row.ID)
			if err != nil {
				return nil, err
			}
			visCache[row.ID] = visible
		}
		if !visible {
			continue
		}
		if idx, seen := indexByID[row.ID]; seen {
			out[idx].LinkKinds = append(out[idx].LinkKinds, row.Kind)
			continue
		}
		indexByID[row.ID] = len(out)
		out = append(out, referencingItem{
			itemSummary: row.summary(),
			LinkKinds:   []string{row.Kind},
		})
	}
	return out, nil
}

func (s *Server) getItemContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var xmlPath string
	err = sqlx.GetContext(ctx, tx, &xmlPath, `SELECT xml_storage_path FROM items WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		internalError(w, r, err)
		return
	}
	ok, err := canViewItemFrom(ctx, tx, t, s.viewerFor(r, t), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden", "this item is not accessible")
		return
	}
	_ = tx.Rollback()

	body, err := s.store.Open(ctx, xmlPath)
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
