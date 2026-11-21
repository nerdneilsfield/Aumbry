package api

import (
	"net/http"
	"sync"
	"testing"
)

func testTree(recs ...dirRec) *dirTree {
	t := &dirTree{dirs: map[string]*dirRec{}, kids: map[string][]string{}}
	for i := range recs {
		t.dirs[recs[i].ID] = &recs[i]
		t.kids[recs[i].ParentID] = append(t.kids[recs[i].ParentID], recs[i].ID)
	}
	return t
}

// The tree is untrusted data: whatever shape it has, access fails closed and walks terminate.
func TestAccessFailsClosedOnCorruptTree(t *testing.T) {
	tree := testTree(
		dirRec{ID: "open"},
		dirRec{ID: "orphan", ParentID: "gone"}, // parent missing from the tree
		dirRec{ID: "a", ParentID: "b"},         // a and b form a cycle
		dirRec{ID: "b", ParentID: "a"},
	)
	visitor := viewer{unlocked: map[string]bool{}}
	if !tree.accessible(visitor, "open") {
		t.Error("an unprotected root must stay accessible")
	}
	for _, id := range []string{"unknown", "orphan", "a", "b"} {
		if tree.accessible(visitor, id) {
			t.Errorf("accessible(%q) = true for a visitor, want false", id)
		}
	}
	if !tree.accessible(viewer{admin: true}, "unknown") {
		t.Error("admin keeps access")
	}
	// None of these may hang.
	_ = tree.protectedAncestor("a")
	if tree.within("a", "open") {
		t.Error("within across a cycle = true")
	}
}

// Many concurrent moves that would form a cycle if they raced must never produce one.
func TestConcurrentDirMovesNeverCreateACycle(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	a := admin.createDir("", "A", "")
	b := admin.createDir("", "B", "")

	var wg sync.WaitGroup
	for range 20 {
		for _, move := range [][2]string{{a, b}, {b, a}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				resp := admin.do("PATCH", "/api/v1/admin/dirs/"+move[0], map[string]any{"parent_id": move[1]})
				if resp.status != http.StatusNoContent && resp.status != http.StatusBadRequest {
					t.Errorf("move = %d %s", resp.status, resp.body)
				}
			}()
		}
	}
	wg.Wait()

	// Every directory must still hang below a root; a cycle would leave it unreachable.
	reachable := 0
	var walk func(nodes []any)
	walk = func(nodes []any) {
		for _, n := range nodes {
			reachable++
			walk(n.(map[string]any)["children"].([]any))
		}
	}
	walk(admin.do("GET", "/api/v1/admin/tree", nil).json()["tree"].([]any))
	if reachable != 2 {
		t.Fatalf("%d of 2 directories reachable from the roots: a cycle was created", reachable)
	}
}
