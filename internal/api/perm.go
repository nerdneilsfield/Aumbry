package api

import (
	"context"
	"net/http"

	"github.com/jmoiron/sqlx"

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
	return loadTreeFrom(ctx, s.db)
}

// loadTreeFrom reads the tree through q, which may be a transaction.
func loadTreeFrom(ctx context.Context, q sqlx.QueryerContext) (*dirTree, error) {
	var rows []dirRec
	err := sqlx.SelectContext(ctx, q, &rows, `SELECT id, COALESCE(parent_id, '') AS parent_id, name, description,
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

// ancestors calls visit for id and then each parent up to the root, stopping when visit returns
// false. It reports whether the walk reached a root; an unknown id, a missing parent or a cycle in a
// corrupt tree end it early with false, so no caller can loop forever.
func (t *dirTree) ancestors(id string, visit func(*dirRec) bool) bool {
	seen := map[string]bool{}
	for cur := id; cur != ""; {
		d := t.dirs[cur]
		if d == nil || seen[cur] {
			return false
		}
		seen[cur] = true
		if !visit(d) {
			return true
		}
		cur = d.ParentID
	}
	return true
}

// accessible reports whether v may see directory id: no directory on the way to the root has a
// password, or the visitor unlocked one of them. It fails closed: an unknown id or a broken parent
// chain is never accessible.
func (t *dirTree) accessible(v viewer, id string) bool {
	if v.admin {
		return true
	}
	protected, open := false, false
	complete := t.ancestors(id, func(d *dirRec) bool {
		if v.unlocked[d.ID] {
			open = true
			return false
		}
		protected = protected || d.PasswordHash != ""
		return true
	})
	if open {
		return true
	}
	return complete && !protected
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
	var found *dirRec
	t.ancestors(id, func(d *dirRec) bool {
		if d.PasswordHash != "" {
			found = d
		}
		return found == nil
	})
	return found
}

// within reports whether id is ancestor itself or one of its descendants.
func (t *dirTree) within(id, ancestor string) bool {
	found := false
	t.ancestors(id, func(d *dirRec) bool {
		found = d.ID == ancestor
		return !found
	})
	return found
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
