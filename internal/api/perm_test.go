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

func TestAccessibleByDirTokenSubtreeAndCorruptTrees(t *testing.T) {
	tree := testTree(
		dirRec{ID: "parent"},
		dirRec{ID: "sibling"},
		dirRec{ID: "D", ParentID: "parent", PasswordHash: "pwD"},
		dirRec{ID: "U", ParentID: "D"},
		dirRec{ID: "C", ParentID: "D", PasswordHash: "pwC"},
		dirRec{ID: "CU", ParentID: "C"},
	)

	// Token on D grants D and unprotected child U, but NEVER penetrates C or CU, nor reaches parent/sibling.
	for _, wantTrue := range []string{"D", "U"} {
		if !tree.accessibleByDirToken("D", wantTrue) {
			t.Errorf("accessibleByDirToken(D, %q) = false, want true", wantTrue)
		}
	}
	for _, wantFalse := range []string{"", "unknown", "parent", "sibling", "C", "CU"} {
		if tree.accessibleByDirToken("D", wantFalse) {
			t.Errorf("accessibleByDirToken(D, %q) = true, want false", wantFalse)
		}
	}

	// Token on C grants C and CU, nothing outside.
	for _, wantTrue := range []string{"C", "CU"} {
		if !tree.accessibleByDirToken("C", wantTrue) {
			t.Errorf("accessibleByDirToken(C, %q) = false, want true", wantTrue)
		}
	}
	for _, wantFalse := range []string{"D", "U", "parent", "sibling"} {
		if tree.accessibleByDirToken("C", wantFalse) {
			t.Errorf("accessibleByDirToken(C, %q) = true, want false", wantFalse)
		}
	}

	// Broken parent chain or cycle above D must still fail closed even when accessing D or U.
	corrupt := testTree(
		dirRec{ID: "orphanD", ParentID: "missingParent", PasswordHash: "pw"},
		dirRec{ID: "orphanU", ParentID: "orphanD"},
		dirRec{ID: "cycA", ParentID: "cycB", PasswordHash: "pw"},
		dirRec{ID: "cycB", ParentID: "cycA"},
		dirRec{ID: "cycChild", ParentID: "cycA"},
	)
	for _, pair := range [][2]string{
		{"orphanD", "orphanD"},
		{"orphanD", "orphanU"},
		{"cycA", "cycA"},
		{"cycA", "cycChild"},
	} {
		if corrupt.accessibleByDirToken(pair[0], pair[1]) {
			t.Errorf("corrupt tree accessibleByDirToken(%q, %q) = true, want false", pair[0], pair[1])
		}
	}

	// Cyclic directory trees must never cause unbounded recursion / stack overflow in subtree helpers.
	if got := corrupt.subtreeVisibleByDirToken("cycA"); len(got) != 0 {
		t.Errorf("corrupt subtreeVisibleByDirToken(cycA) = %v, want empty", got)
	}
	if got := corrupt.publicSubtree("cycA"); len(got) != 0 {
		t.Errorf("corrupt publicSubtree(cycA) = %v, want empty", got)
	}
	cyclicSubtree := testTree(
		dirRec{ID: "cycRoot", PasswordHash: "pw"},
		dirRec{ID: "cyc1", ParentID: "cycRoot"},
		dirRec{ID: "cyc2", ParentID: "cyc1"},
	)
	cyclicSubtree.kids["cyc2"] = append(cyclicSubtree.kids["cyc2"], "cyc1")
	if got := cyclicSubtree.subtreeVisibleByDirToken("cycRoot"); len(got) != 3 {
		t.Errorf("cyclic descendant subtreeVisibleByDirToken = %v, want 3 unique dirs", got)
	}
}
