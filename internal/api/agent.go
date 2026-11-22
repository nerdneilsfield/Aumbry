package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

type agentDigestKey struct{}

func (s *Server) requireAgentToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		digest, ok := s.auth.AuthenticateToken(r.Header.Get("Authorization"))
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="aumbry"`)
			writeError(w, http.StatusUnauthorized, "unauthorized", "a valid API token is required")
			return
		}
		ctx := context.WithValue(r.Context(), agentDigestKey{}, digest)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func agentTokenDigest(r *http.Request) string {
	digest, _ := r.Context().Value(agentDigestKey{}).(string)
	return digest
}

type agentDir struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Path        string  `json:"path"`
	ParentID    *string `json:"parent_id"`
	HasPassword bool    `json:"has_password"`
	Protected   bool    `json:"protected"`
	ItemCount   int     `json:"item_count"`
}

func (s *Server) agentDirs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, err := s.loadTree(ctx)
	if err != nil {
		internalError(w, r, err)
		return
	}
	var counts []struct {
		Dir string `db:"directory_id"`
		N   int    `db:"n"`
	}
	if err := s.db.SelectContext(ctx, &counts, `SELECT directory_id, count(*) AS n FROM directory_items GROUP BY directory_id`); err != nil {
		internalError(w, r, err)
		return
	}
	itemCount := make(map[string]int, len(counts))
	for _, c := range counts {
		itemCount[c.Dir] = c.N
	}

	out := make([]agentDir, 0, len(t.dirs))
	var walk func(parent, pathPrefix string, ancestorProtected bool)
	walk = func(parent, pathPrefix string, ancestorProtected bool) {
		for _, id := range t.kids[parent] {
			d := t.dirs[id]
			fullPath := d.Name
			if pathPrefix != "" {
				fullPath = pathPrefix + " / " + d.Name
			}
			hasPW := d.PasswordHash != ""
			protected := ancestorProtected || hasPW
			var parentPtr *string
			if d.ParentID != "" {
				p := d.ParentID
				parentPtr = &p
			}
			out = append(out, agentDir{
				ID:          d.ID,
				Name:        d.Name,
				Path:        fullPath,
				ParentID:    parentPtr,
				HasPassword: hasPW,
				Protected:   protected,
				ItemCount:   itemCount[d.ID],
			})
			walk(d.ID, fullPath, protected)
		}
	}
	walk("", "", false)
	writeJSON(w, http.StatusOK, out)
}

type agentItem struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Date        string   `json:"date"`
	ExternalKey string   `json:"external_key"`
	URL         string   `json:"url"`
	Dirs        []string `json:"dirs"`
	Owned       bool     `json:"owned"`
}

type agentItemsResponse struct {
	Items  []agentItem `json:"items"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

func (s *Server) agentListItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	digest := agentTokenDigest(r)
	params := r.URL.Query()
	limit, offset, ok := pageParams(w, r)
	if !ok {
		return
	}

	var where []string
	var args []any
	if key := strings.TrimSpace(params.Get("key")); key != "" {
		where = append(where, `i.external_key = ?`)
		args = append(args, key)
	}
	// Agent search q only matches title and external_key (never summary, plain_text or items_fts).
	for _, tok := range strings.Fields(params.Get("q")) {
		pattern := "%" + likeEscaper.Replace(tok) + "%"
		where = append(where, `(i.title LIKE ? ESCAPE '\' OR COALESCE(i.external_key, '') LIKE ? ESCAPE '\')`)
		args = append(args, pattern, pattern)
	}
	if dir := strings.TrimSpace(params.Get("dir")); dir != "" {
		where = append(where, `EXISTS (SELECT 1 FROM directory_items di WHERE di.item_id = i.id AND di.directory_id = ?)`)
		args = append(args, dir)
	}

	query := `SELECT i.id, i.type, i.title, i.report_date, COALESCE(i.external_key, '') AS external_key,
		COALESCE(i.created_by, '') AS created_by FROM items i`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY i.report_date DESC, i.created_at DESC, i.id LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	var rows []struct {
		ID          string `db:"id"`
		Type        string `db:"type"`
		Title       string `db:"title"`
		Date        string `db:"report_date"`
		ExternalKey string `db:"external_key"`
		CreatedBy   string `db:"created_by"`
	}
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		internalError(w, r, err)
		return
	}

	mountsByItem := make(map[string][]string, len(rows))
	if len(rows) > 0 {
		ids := make([]string, len(rows))
		for i, row := range rows {
			ids[i] = row.ID
		}
		mountQ, mountArgs, err := sqlx.In(`SELECT item_id, directory_id FROM directory_items WHERE item_id IN (?) ORDER BY directory_id`, ids)
		if err != nil {
			internalError(w, r, err)
			return
		}
		var mountRows []struct {
			ItemID string `db:"item_id"`
			DirID  string `db:"directory_id"`
		}
		if err := s.db.SelectContext(ctx, &mountRows, mountQ, mountArgs...); err != nil {
			internalError(w, r, err)
			return
		}
		for _, m := range mountRows {
			mountsByItem[m.ItemID] = append(mountsByItem[m.ItemID], m.DirID)
		}
	}

	items := make([]agentItem, len(rows))
	for i, row := range rows {
		prefix := "/r/"
		if row.Type == "post" {
			prefix = "/p/"
		}
		dirs := mountsByItem[row.ID]
		if dirs == nil {
			dirs = []string{}
		}
		items[i] = agentItem{
			ID:          row.ID,
			Type:        row.Type,
			Title:       row.Title,
			Date:        row.Date,
			ExternalKey: row.ExternalKey,
			URL:         prefix + row.ID,
			Dirs:        dirs,
			Owned:       row.CreatedBy != "" && row.CreatedBy == digest,
		}
	}
	writeJSON(w, http.StatusOK, agentItemsResponse{Items: items, Limit: limit, Offset: offset})
}

func (s *Server) agentUnpublishItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	digest := agentTokenDigest(r)

	var createdBy sql.NullString
	err := s.db.GetContext(ctx, &createdBy, `SELECT created_by FROM items WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !createdBy.Valid || createdBy.String != digest {
		writeError(w, http.StatusForbidden, "not_owner", "only the creating token may unpublish this item")
		return
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM directory_items WHERE item_id = ?`, id); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) agentDeleteItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	digest := agentTokenDigest(r)

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var row struct {
		XMLPath   string         `db:"xml_storage_path"`
		CreatedBy sql.NullString `db:"created_by"`
	}
	err = tx.GetContext(ctx, &row, `SELECT xml_storage_path, created_by FROM items WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !row.CreatedBy.Valid || row.CreatedBy.String != digest {
		writeError(w, http.StatusForbidden, "not_owner", "only the creating token may delete this item")
		return
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM items_fts WHERE item_id = ?`, id); err != nil {
		internalError(w, r, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id); err != nil {
		internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(w, r, err)
		return
	}
	if err := s.store.Delete(ctx, row.XMLPath); err != nil {
		s.logger.WarnContext(ctx, "delete item content from storage", "item", id, "path", row.XMLPath, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}
