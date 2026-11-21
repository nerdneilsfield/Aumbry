package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/nerdneilsfield/aumbry/internal/auth"
)

type crumb struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type childDir struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Locked      bool   `json:"locked"`
}

type dirView struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	ParentID    *string       `json:"parent_id"`
	Breadcrumb  []crumb       `json:"breadcrumb"`
	Children    []childDir    `json:"children"`
	Items       []itemSummary `json:"items"`
}

// requireDir loads the tree and the directory, answering 404 or 401 itself when the caller cannot open it.
func (s *Server) requireDir(w http.ResponseWriter, r *http.Request) (*dirTree, viewer, *dirRec, bool) {
	t, err := s.loadTree(r.Context())
	if err != nil {
		internalError(w, r, err)
		return nil, viewer{}, nil, false
	}
	d := t.dirs[chi.URLParam(r, "id")]
	if d == nil {
		writeError(w, http.StatusNotFound, "not_found", "directory not found")
		return nil, viewer{}, nil, false
	}
	v := s.viewerFor(r, t)
	if !t.accessible(v, d.ID) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": "locked", "message": "this directory is password protected",
			"locked": true, "dir_name": d.Name,
		})
		return nil, viewer{}, nil, false
	}
	return t, v, d, true
}

func (s *Server) getDir(w http.ResponseWriter, r *http.Request) {
	t, v, d, ok := s.requireDir(w, r)
	if !ok {
		return
	}
	view := dirView{
		ID: d.ID, Name: d.Name, Description: d.Description,
		Breadcrumb: []crumb{}, Children: []childDir{}, Items: []itemSummary{},
	}
	// Walk up while the ancestors are still visible; a hidden ancestor ends the trail.
	for cur := d; cur != nil && t.accessible(v, cur.ID); cur = t.dirs[cur.ParentID] {
		view.Breadcrumb = append([]crumb{{ID: cur.ID, Name: cur.Name}}, view.Breadcrumb...)
	}
	if d.ParentID != "" && t.accessible(v, d.ParentID) {
		parent := d.ParentID
		view.ParentID = &parent
	}
	for _, id := range t.kids[d.ID] {
		kid := t.dirs[id]
		child := childDir{ID: kid.ID, Name: kid.Name, Description: kid.Description, Locked: !t.accessible(v, kid.ID)}
		if child.Locked {
			child.Description = ""
		}
		view.Children = append(view.Children, child)
	}
	var rows []itemRow
	err := s.db.SelectContext(r.Context(), &rows, `SELECT `+itemColumns+` FROM items i
		JOIN directory_items di ON di.item_id = i.id WHERE di.directory_id = ?
		ORDER BY i.report_date DESC, i.created_at DESC, i.id`, d.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	view.Items = summaries(rows)
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) unlockDir(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	t, err := s.loadTree(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if t.dirs[id] == nil {
		writeError(w, http.StatusNotFound, "not_found", "directory not found")
		return
	}
	// The password that guards this directory belongs to its nearest protected ancestor (or itself).
	guard := t.protectedAncestor(id)
	if guard == nil {
		writeError(w, http.StatusBadRequest, "no_password", "this directory is not password protected")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(guard.PasswordHash), []byte(body.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid_password", "incorrect password")
		return
	}
	unlocked := map[string]string{}
	for dir := range s.viewerFor(r, t).unlocked {
		unlocked[dir] = auth.Fingerprint(t.dirs[dir].PasswordHash)
	}
	unlocked[guard.ID] = auth.Fingerprint(guard.PasswordHash)
	if err := s.auth.SetUnlocked(w, r, unlocked); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
