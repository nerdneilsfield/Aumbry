package api

import (
	"context"
	"net/http"

	"github.com/nerdneilsfield/aumbry/internal/auth"
)

// dirRec is one directory row.
type dirRec struct {
	ID           string `db:"id"`
	ParentID     string `db:"parent_id"` // empty for a root directory
	Name         string `db:"name"`
	Description  string `db:"description"`
	PasswordHash string `db:"password_hash"` // empty when the directory has no password
	SortOrder    int    `db:"sort_order"`
}

// dirTree is the whole (small) directory table held in memory for one request.
type dirTree struct {
	dirs map[string]*dirRec
	kids map[string][]string // parent id ("" for roots) -> child ids ordered by sort_order, name
}

func (s *Server) loadTree(ctx context.Context) (*dirTree, error) {
	var rows []dirRec
	err := s.db.SelectContext(ctx, &rows, `SELECT id, COALESCE(parent_id, '') AS parent_id, name, description,
		COALESCE(password_hash, '') AS password_hash, sort_order
		FROM directories ORDER BY sort_order, name, id`)
	if err != nil {
		return nil, err
	}
	t := &dirTree{dirs: make(map[string]*dirRec, len(rows)), kids: map[string][]string{}}
	for i := range rows {
		d := &rows[i]
		t.dirs[d.ID] = d
		t.kids[d.ParentID] = append(t.kids[d.ParentID], d.ID)
	}
	return t, nil
}

// viewer is the caller's authorization state.
type viewer struct {
	admin    bool
	unlocked map[string]bool // directories whose password the visitor supplied and is still valid
}

// viewerFor builds the viewer from the request cookies. Unlocked entries only count while the
// directory still has the password they were issued for.
func (s *Server) viewerFor(r *http.Request, t *dirTree) viewer {
	if s.auth.IsAdmin(r) {
		return viewer{admin: true}
	}
	v := viewer{unlocked: map[string]bool{}}
	for id, fp := range s.auth.Unlocked(r) {
		if d, ok := t.dirs[id]; ok && d.PasswordHash != "" && auth.Fingerprint(d.PasswordHash) == fp {
			v.unlocked[id] = true
		}
	}
	return v
}

// accessible reports whether v may see directory id: no directory on the way to the root has a
// password, or the visitor unlocked one of them.
func (t *dirTree) accessible(v viewer, id string) bool {
	if v.admin {
		return true
	}
	protected := false
	for cur := t.dirs[id]; cur != nil; cur = t.dirs[cur.ParentID] {
		if v.unlocked[cur.ID] {
			return true
		}
		protected = protected || cur.PasswordHash != ""
	}
	return !protected
}

// accessibleSet returns every directory v may see.
func (t *dirTree) accessibleSet(v viewer) map[string]bool {
	set := make(map[string]bool, len(t.dirs))
	var walk func(id string, open, locked bool)
	walk = func(id string, open, locked bool) {
		d := t.dirs[id]
		open = open || v.unlocked[id]
		locked = locked || d.PasswordHash != ""
		if v.admin || open || !locked {
			set[id] = true
		}
		for _, kid := range t.kids[id] {
			walk(kid, open, locked)
		}
	}
	for _, root := range t.kids[""] {
		walk(root, false, false)
	}
	return set
}

// subtree returns id and all its descendants.
func (t *dirTree) subtree(id string) []string {
	var out []string
	var walk func(string)
	walk = func(cur string) {
		out = append(out, cur)
		for _, kid := range t.kids[cur] {
			walk(kid)
		}
	}
	walk(id)
	return out
}

// protectedAncestor returns the nearest directory with a password on the way from id to the root, id included.
func (t *dirTree) protectedAncestor(id string) *dirRec {
	for cur := t.dirs[id]; cur != nil; cur = t.dirs[cur.ParentID] {
		if cur.PasswordHash != "" {
			return cur
		}
	}
	return nil
}

// within reports whether id is ancestor itself or one of its descendants.
func (t *dirTree) within(id, ancestor string) bool {
	for cur := t.dirs[id]; cur != nil; cur = t.dirs[cur.ParentID] {
		if cur.ID == ancestor {
			return true
		}
	}
	return false
}

// visibleDirs returns the subset of dirIDs v may see.
func (t *dirTree) visibleDirs(v viewer, dirIDs []string) []string {
	var out []string
	for _, id := range dirIDs {
		if t.accessible(v, id) {
			out = append(out, id)
		}
	}
	return out
}
