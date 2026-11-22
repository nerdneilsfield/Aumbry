package api

import (
	"net/http"
	"testing"
)

// neighborsOf fetches an item as c and returns the dir id plus the prev/next titles ("" when absent).
// ok is false when the response carries no neighbors object.
func neighborsOf(t *testing.T, c *client, id, hint string) (dir, prev, next string, ok bool) {
	t.Helper()
	path := "/api/v1/items/" + id
	if hint != "" {
		path += "?dir=" + hint
	}
	resp := c.do("GET", path, nil)
	if resp.status != http.StatusOK {
		t.Fatalf("GET %s = %d %s", path, resp.status, resp.body)
	}
	n, present := resp.json()["neighbors"].(map[string]any)
	if !present {
		return "", "", "", false
	}
	title := func(key string) string {
		if m, isMap := n[key].(map[string]any); isMap {
			return m["title"].(string)
		}
		return ""
	}
	return n["dir"].(map[string]any)["id"].(string), title("prev"), title("next"), true
}

func TestItemNeighbors(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "Daily", "")
	other := admin.createDir("", "Other", "")
	old := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Old", date: "2026-11-01"}), dirs: []string{dir}})
	mid := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Mid", date: "2026-11-10"}), dirs: []string{dir}})
	newest := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "New", date: "2026-11-20"}), dirs: []string{dir}})
	// Same date as Mid, but in another directory: never a neighbor of the Daily items.
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Elsewhere", date: "2026-11-10"}), dirs: []string{other}})
	// Mounted nowhere: has no neighbors at all.
	loose := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Loose", date: "2026-11-10"})})

	v := e.anon()
	for _, tc := range []struct {
		name, id, prev, next string
	}{
		{"first has no previous", old, "", "Mid"},
		{"middle has both", mid, "Old", "New"},
		{"last has no next", newest, "Mid", ""},
	} {
		got, prev, next, ok := neighborsOf(t, v, tc.id, "")
		if !ok || got != dir || prev != tc.prev || next != tc.next {
			t.Errorf("%s: dir=%q prev=%q next=%q ok=%v, want dir=%q prev=%q next=%q", tc.name, got, prev, next, ok, dir, tc.prev, tc.next)
		}
	}
	if _, _, _, ok := neighborsOf(t, admin, loose, ""); ok {
		t.Error("an unmounted item must have null neighbors")
	}
}

func TestItemNeighborsSameDateFollowsListingOrder(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "Daily", "")
	for _, title := range []string{"A", "B", "C"} {
		admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: title, date: "2026-11-10"}), dirs: []string{dir}})
	}
	var listed []string
	var listedIDs []string
	items := e.anon().do("GET", "/api/v1/dirs/"+dir, nil).json()["items"].([]any)
	for _, it := range items {
		m := it.(map[string]any)
		listed = append(listed, m["title"].(string))
		listedIDs = append(listedIDs, m["id"].(string))
	}
	// The listing is newest first, so the entry below an item is its previous one.
	for i, id := range listedIDs {
		_, prev, next, _ := neighborsOf(t, e.anon(), id, "")
		var wantPrev, wantNext string
		if i+1 < len(listed) {
			wantPrev = listed[i+1]
		}
		if i > 0 {
			wantNext = listed[i-1]
		}
		if prev != wantPrev || next != wantNext {
			t.Errorf("%s: prev=%q next=%q, want prev=%q next=%q", listed[i], prev, next, wantPrev, wantNext)
		}
	}
}

func TestItemNeighborsMultipleMounts(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	pub := admin.createDir("", "Public", "")
	prot := admin.createDir("", "Protected", "secret")
	// shared is mounted in both; the other items each live in one.
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Pub Before", date: "2026-11-01"}), dirs: []string{pub}})
	shared := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Shared", date: "2026-11-10"}), dirs: []string{pub, prot}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Prot After Secret", date: "2026-11-15"}), dirs: []string{prot}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Prot Before Secret", date: "2026-11-05"}), dirs: []string{prot}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Pub After", date: "2026-11-20"}), dirs: []string{pub}})

	// A locked visitor only ever sees the public mount, whatever ?dir= asks for.
	locked := e.anon()
	for _, hint := range []string{"", pub, prot, "no-such-dir"} {
		dir, prev, next, ok := neighborsOf(t, locked, shared, hint)
		if !ok || dir != pub || prev != "Pub Before" || next != "Pub After" {
			t.Errorf("locked visitor, dir=%q: got dir=%q prev=%q next=%q ok=%v, want the public neighbors", hint, dir, prev, next, ok)
		}
	}

	// After unlocking, ?dir= selects the mount; without a hint the first visible mount is used.
	v := e.anon()
	if resp := v.do("POST", "/api/v1/dirs/"+prot+"/unlock", map[string]string{"password": "secret"}); resp.status != http.StatusOK {
		t.Fatalf("unlock: %d %s", resp.status, resp.body)
	}
	dir, prev, next, _ := neighborsOf(t, v, shared, prot)
	if dir != prot || prev != "Prot Before Secret" || next != "Prot After Secret" {
		t.Errorf("unlocked, dir=prot: got dir=%q prev=%q next=%q", dir, prev, next)
	}
	dir, prev, next, _ = neighborsOf(t, v, shared, pub)
	if dir != pub || prev != "Pub Before" || next != "Pub After" {
		t.Errorf("unlocked, dir=pub: got dir=%q prev=%q next=%q", dir, prev, next)
	}
	if dir, _, _, _ = neighborsOf(t, v, shared, ""); dir != pub && dir != prot {
		t.Errorf("unlocked, no hint: dir=%q is not one of the mounts", dir)
	}
}

func TestItemNeighborsInProtectedDirectory(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	prot := admin.createDir("", "Protected", "secret")
	first := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "First", date: "2026-11-01"}), dirs: []string{prot}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Second", date: "2026-11-02"}), dirs: []string{prot}})

	// The item itself is already refused, so no neighbor can leak through it.
	locked := e.anon()
	for _, hint := range []string{"", prot} {
		path := "/api/v1/items/" + first
		if hint != "" {
			path += "?dir=" + hint
		}
		if resp := locked.do("GET", path, nil); resp.status != http.StatusForbidden {
			t.Errorf("locked visitor GET %s = %d, want 403: %s", path, resp.status, resp.body)
		}
	}

	v := e.anon()
	v.do("POST", "/api/v1/dirs/"+prot+"/unlock", map[string]string{"password": "secret"})
	if dir, prev, next, ok := neighborsOf(t, v, first, ""); !ok || dir != prot || prev != "" || next != "Second" {
		t.Errorf("unlocked: dir=%q prev=%q next=%q ok=%v", dir, prev, next, ok)
	}
}

func TestItemNeighborsSkipHiddenSubdirectories(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	parent := admin.createDir("", "Parent", "")
	child := admin.createDir(parent, "Child", "secret")
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "In Child", date: "2026-11-05"}), dirs: []string{child}})
	a := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "A", date: "2026-11-01"}), dirs: []string{parent}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "B", date: "2026-11-10"}), dirs: []string{parent}})

	// Only items mounted directly in the directory count, so the child's item is not between A and B.
	if _, prev, next, _ := neighborsOf(t, e.anon(), a, ""); prev != "" || next != "B" {
		t.Errorf("prev=%q next=%q, want prev empty and next B", prev, next)
	}
}
