package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"golang.org/x/crypto/bcrypt"
)

const idLength = 12

func newID() (string, error) { return gonanoid.New(idLength) }

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.auth.IsAdmin(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "admin login required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if !s.auth.CheckAdmin(body.Username, body.Password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	if err := s.auth.LoginAdmin(w, r); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.LogoutAdmin(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminMe(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"admin": true})
}

type dirNode struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	HasPassword bool      `json:"has_password"`
	ItemCount   int       `json:"item_count"`
	SortOrder   int       `json:"sort_order"`
	Children    []dirNode `json:"children"`
}

func (s *Server) adminTree(w http.ResponseWriter, r *http.Request) {
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
	itemCount := map[string]int{}
	for _, c := range counts {
		itemCount[c.Dir] = c.N
	}
	var unmounted int
	if err := s.db.GetContext(ctx, &unmounted, `SELECT count(*) FROM items i
		WHERE NOT EXISTS (SELECT 1 FROM directory_items di WHERE di.item_id = i.id)`); err != nil {
		internalError(w, r, err)
		return
	}
	var build func(parent string) []dirNode
	build = func(parent string) []dirNode {
		nodes := []dirNode{}
		for _, id := range t.kids[parent] {
			d := t.dirs[id]
			nodes = append(nodes, dirNode{
				ID: d.ID, Name: d.Name, Description: d.Description, HasPassword: d.PasswordHash != "",
				ItemCount: itemCount[d.ID], SortOrder: d.SortOrder, Children: build(d.ID),
			})
		}
		return nodes
	}
	writeJSON(w, http.StatusOK, map[string]any{"tree": build(""), "unmounted_count": unmounted})
}

// hashDirPassword answers 400 itself when the password cannot be hashed.
func hashDirPassword(w http.ResponseWriter, password string) (string, bool) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid password: "+err.Error())
		return "", false
	}
	return string(hash), true
}

func (s *Server) adminCreateDir(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ParentID    *string `json:"parent_id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Password    string  `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	var parent any
	if body.ParentID != nil && *body.ParentID != "" {
		exists, err := s.dirExists(r.Context(), *body.ParentID)
		if err != nil {
			internalError(w, r, err)
			return
		}
		if !exists {
			writeError(w, http.StatusBadRequest, "parent_not_found", "parent directory does not exist")
			return
		}
		parent = *body.ParentID
	}
	var hash any
	if body.Password != "" {
		h, ok := hashDirPassword(w, body.Password)
		if !ok {
			return
		}
		hash = h
	}
	id, err := newID()
	if err != nil {
		internalError(w, r, err)
		return
	}
	_, err = s.db.ExecContext(r.Context(),
		`INSERT INTO directories (id, parent_id, name, description, password_hash) VALUES (?, ?, ?, ?, ?)`,
		id, parent, name, body.Description, hash)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) dirExists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.db.GetContext(ctx, &n, `SELECT count(*) FROM directories WHERE id = ?`, id)
	return n > 0, err
}

func (s *Server) adminUpdateDir(w http.ResponseWriter, r *http.Request) {
	// Decode into raw fields so an explicit JSON null (move to root) differs from an absent field.
	var fields map[string]json.RawMessage
	if !decodeJSON(w, r, &fields) {
		return
	}
	id := chi.URLParam(r, "id")
	exists, err := s.dirExists(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "directory not found")
		return
	}

	var sets []string
	var args []any
	var moveTo *string // set when parent_id is present and not null; checked against the tree in the transaction
	for key, raw := range fields {
		switch key {
		case "name":
			var name string
			if json.Unmarshal(raw, &name) != nil || strings.TrimSpace(name) == "" {
				writeError(w, http.StatusBadRequest, "invalid_request", "name must be a non-empty string")
				return
			}
			sets, args = append(sets, "name = ?"), append(args, strings.TrimSpace(name))
		case "description":
			var description string
			if json.Unmarshal(raw, &description) != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "description must be a string")
				return
			}
			sets, args = append(sets, "description = ?"), append(args, description)
		case "sort_order":
			var order int
			if json.Unmarshal(raw, &order) != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "sort_order must be an integer")
				return
			}
			sets, args = append(sets, "sort_order = ?"), append(args, order)
		case "password":
			var password string
			if json.Unmarshal(raw, &password) != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "password must be a string")
				return
			}
			if password == "" {
				sets = append(sets, "password_hash = NULL")
				break
			}
			hash, ok := hashDirPassword(w, password)
			if !ok {
				return
			}
			sets, args = append(sets, "password_hash = ?"), append(args, hash)
		case "parent_id":
			var parent *string
			if json.Unmarshal(raw, &parent) != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "parent_id must be a string or null")
				return
			}
			if parent == nil || *parent == "" {
				sets = append(sets, "parent_id = NULL")
				break
			}
			moveTo = parent
			sets, args = append(sets, "parent_id = ?"), append(args, *parent)
		default:
			writeError(w, http.StatusBadRequest, "invalid_request", "unknown field "+key)
			return
		}
	}
	if len(sets) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Read the tree, check for a cycle and update in one transaction. The pool has a single
	// connection, so concurrent moves run one after the other and cannot create a cycle together.
	ctx := r.Context()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if moveTo != nil {
		t, err := loadTreeFrom(ctx, tx)
		if err != nil {
			internalError(w, r, err)
			return
		}
		if t.dirs[*moveTo] == nil {
			writeError(w, http.StatusBadRequest, "parent_not_found", "parent directory does not exist")
			return
		}
		if t.within(*moveTo, id) {
			writeError(w, http.StatusBadRequest, "invalid_parent", "a directory cannot be moved into itself or its descendants")
			return
		}
	}
	query := `UPDATE directories SET ` + strings.Join(sets, ", ") + `, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	if _, err := tx.ExecContext(ctx, query, append(args, id)...); err != nil {
		internalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminDeleteDir(w http.ResponseWriter, r *http.Request) {
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM directories WHERE id = ?`, chi.URLParam(r, "id"))
	if err != nil {
		internalError(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "not_found", "directory not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminSetMounts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dirs *[]string `json:"dirs"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Dirs == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "dirs is required")
		return
	}
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	exists, err := s.itemExists(ctx, id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return
	}
	dirs := dedupe(*body.Dirs)
	missing, err := s.missingDirs(ctx, dirs)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if len(missing) > 0 {
		writeError(w, http.StatusBadRequest, "unknown_dirs", "unknown directories: "+strings.Join(missing, ", "))
		return
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM directory_items WHERE item_id = ?`, id); err != nil {
		internalError(w, r, err)
		return
	}
	for _, dir := range dirs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO directory_items (directory_id, item_id) VALUES (?, ?)`, dir, id); err != nil {
			internalError(w, r, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminDeleteItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	var path string
	err := s.db.GetContext(ctx, &path, `SELECT xml_storage_path FROM items WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
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
	if err := s.store.Delete(ctx, path); err != nil {
		s.logger.WarnContext(ctx, "delete item content from storage", "item", id, "path", path, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}
