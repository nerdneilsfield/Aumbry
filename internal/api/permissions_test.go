package api

import (
	"net/http"
	"strings"
	"testing"
)

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>`

// fixture is the directory tree used by the permission tests:
//
//	pub (public)
//	  pubLocked (own password "inner-pw")
//	prot (password "secret")
//	  protInner (no password of its own)
//	other (password "other-pw")
type fixture struct {
	admin                                *client
	pub, pubLocked, prot, protInner, oth string
	itemPub, itemPubLocked, itemProt     string
	itemProtInner, itemBoth              string
	assetHash                            string // figure of itemProt
}

func newFixture(t *testing.T, e *env) fixture {
	t.Helper()
	f := fixture{admin: e.admin()}
	f.pub = f.admin.createDir("", "Public", "")
	f.pubLocked = f.admin.createDir(f.pub, "PublicLocked", "inner-pw")
	f.prot = f.admin.createDir("", "Protected", "secret")
	f.protInner = f.admin.createDir(f.prot, "ProtectedInner", "")
	f.oth = f.admin.createDir("", "Other", "other-pw")

	ingest := func(title string, dirs ...string) string {
		return f.admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: title, md: "zebra " + title}), dirs: dirs})
	}
	f.itemPub = ingest("Alpha public", f.pub)
	f.itemPubLocked = ingest("Bravo hidden", f.pubLocked)
	f.itemProtInner = ingest("Delta inner", f.protInner)
	f.itemBoth = ingest("Echo shared", f.prot, f.pub)
	f.itemProt = f.admin.ingestOK(ingestReq{
		xml:    xmlDoc(docOpts{title: "Charlie secret", md: "zebra Charlie", figures: []string{"./a.svg"}}),
		assets: []figure{{src: "./a.svg", name: "a.svg", data: []byte(testSVG)}},
		dirs:   []string{f.prot},
	})
	f.assetHash = hashOf(t, f.admin, f.itemProt)
	return f
}

// hashOf extracts the asset hash referenced by an item's rewritten XML.
func hashOf(t *testing.T, c *client, item string) string {
	t.Helper()
	body := string(c.do("GET", "/api/v1/items/"+item+"/content", nil).body)
	_, rest, ok := strings.Cut(body, "/api/v1/assets/")
	if !ok || len(rest) < 64 {
		t.Fatalf("no asset url in %s", body)
	}
	return rest[:64]
}

func ids(t *testing.T, list any) []string {
	t.Helper()
	var out []string
	for _, v := range list.([]any) {
		out = append(out, v.(map[string]any)["id"].(string))
	}
	return out
}

func sameSet(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
}

func TestVisitorWithoutUnlocks(t *testing.T) {
	e := newEnv(t)
	f := newFixture(t, e)
	v := e.anon()

	// A public directory lists its items; the protected child shows up by name only, as locked.
	resp := v.do("GET", "/api/v1/dirs/"+f.pub, nil)
	if resp.status != http.StatusOK {
		t.Fatalf("public dir: %d %s", resp.status, resp.body)
	}
	dir := resp.json()
	if got := ids(t, dir["items"]); !sameSet(got, f.itemPub, f.itemBoth) {
		t.Errorf("public dir items = %v, want Alpha and Echo only", got)
	}
	children := dir["children"].([]any)
	if len(children) != 1 {
		t.Fatalf("children = %v", children)
	}
	child := children[0].(map[string]any)
	if child["id"] != f.pubLocked || child["name"] != "PublicLocked" || child["locked"] != true {
		t.Errorf("locked child = %v", child)
	}
	if dir["parent_id"] != nil {
		t.Errorf("root parent_id = %v, want null", dir["parent_id"])
	}

	// Protected directories answer 401 with the lock marker and the name.
	for _, id := range []string{f.prot, f.protInner, f.pubLocked, f.oth} {
		resp := v.do("GET", "/api/v1/dirs/"+id, nil)
		body := resp.json()
		if resp.status != http.StatusUnauthorized || body["error"] != "locked" || body["locked"] != true || body["dir_name"] == "" {
			t.Errorf("dir %s: %d %s, want 401 locked", id, resp.status, resp.body)
		}
		if resp := v.do("GET", "/api/v1/dirs/"+id+"/search?q=zebra", nil); resp.status != http.StatusUnauthorized {
			t.Errorf("search in locked dir %s: %d", id, resp.status)
		}
	}
	if resp := v.do("GET", "/api/v1/dirs/nonexistent1", nil); resp.status != http.StatusNotFound || resp.json()["error"] != "not_found" {
		t.Errorf("missing dir: %d %s", resp.status, resp.body)
	}

	// The protected child under the public parent does not leak through search of the parent.
	search := v.do("GET", "/api/v1/dirs/"+f.pub+"/search?q=zebra", nil).json()
	if got := ids(t, search["results"]); !sameSet(got, f.itemPub, f.itemBoth) {
		t.Errorf("parent search = %v, want Alpha and Echo only (Bravo is in a locked child)", got)
	}

	// Leaves: 403 unless some mount is open to the caller.
	for item, want := range map[string]int{
		f.itemPub: 200, f.itemBoth: 200, f.itemPubLocked: 403, f.itemProt: 403, f.itemProtInner: 403,
	} {
		if got := v.do("GET", "/api/v1/items/"+item, nil).status; got != want {
			t.Errorf("GET item %s = %d, want %d", item, got, want)
		}
		if got := v.do("GET", "/api/v1/items/"+item+"/content", nil).status; got != want {
			t.Errorf("GET content %s = %d, want %d", item, got, want)
		}
	}
	if resp := v.do("GET", "/api/v1/items/"+f.itemProt, nil); resp.json()["error"] != "forbidden" {
		t.Errorf("leaf 403 body = %s", resp.body)
	}
	if resp := v.do("GET", "/api/v1/items/nonexistent1", nil); resp.status != http.StatusNotFound {
		t.Errorf("missing item: %d", resp.status)
	}

	// Echo is also mounted in the protected tree; the visitor only learns about the open mount.
	detail := v.do("GET", "/api/v1/items/"+f.itemBoth, nil).json()
	if got := ids(t, detail["dirs"]); !sameSet(got, f.pub) {
		t.Errorf("item dirs for visitor = %v, want only the public mount", got)
	}
	if got := ids(t, f.admin.do("GET", "/api/v1/items/"+f.itemBoth, nil).json()["dirs"]); !sameSet(got, f.pub, f.prot) {
		t.Errorf("item dirs for admin = %v", got)
	}

	// Assets follow the items that reference them.
	if got := v.do("GET", "/api/v1/assets/"+f.assetHash, nil).status; got != http.StatusForbidden {
		t.Errorf("asset of a protected item = %d, want 403", got)
	}
	if got := f.admin.do("GET", "/api/v1/assets/"+f.assetHash, nil).status; got != http.StatusOK {
		t.Errorf("asset as admin = %d, want 200", got)
	}
	if got := v.do("GET", "/api/v1/assets/"+strings.Repeat("0", 64), nil).status; got != http.StatusNotFound {
		t.Errorf("unknown asset = %d, want 404", got)
	}
	if got := v.do("GET", "/api/v1/assets/not-a-hash", nil).status; got != http.StatusNotFound {
		t.Errorf("malformed asset hash = %d, want 404", got)
	}

	// Admin reaches everything.
	for _, item := range []string{f.itemProt, f.itemPubLocked, f.itemProtInner} {
		if got := f.admin.do("GET", "/api/v1/items/"+item+"/content", nil).status; got != http.StatusOK {
			t.Errorf("admin content %s = %d", item, got)
		}
	}
	if got := f.admin.do("GET", "/api/v1/dirs/"+f.prot, nil).status; got != http.StatusOK {
		t.Errorf("admin dir = %d", got)
	}
}

func TestUnlockSessions(t *testing.T) {
	e := newEnv(t)
	f := newFixture(t, e)
	v := e.anon()
	unlock := func(dir, pw string) response {
		return v.do("POST", "/api/v1/dirs/"+dir+"/unlock", map[string]string{"password": pw})
	}

	if resp := unlock(f.prot, "wrong"); resp.status != http.StatusUnauthorized || resp.json()["error"] != "invalid_password" {
		t.Errorf("wrong password: %d %s", resp.status, resp.body)
	}
	if resp := unlock(f.pub, "x"); resp.status != http.StatusBadRequest || resp.json()["error"] != "no_password" {
		t.Errorf("unprotected dir: %d %s", resp.status, resp.body)
	}
	if resp := unlock("nonexistent1", "x"); resp.status != http.StatusNotFound {
		t.Errorf("missing dir: %d", resp.status)
	}
	if got := v.do("GET", "/api/v1/dirs/"+f.prot, nil).status; got != http.StatusUnauthorized {
		t.Fatalf("still locked after failed attempts, got %d", got)
	}

	// An inner link of a protected subtree is unlocked with the nearest protected ancestor's password.
	if resp := unlock(f.protInner, "secret"); resp.status != http.StatusOK || resp.json()["ok"] != true {
		t.Fatalf("unlock inner dir with parent password: %d %s", resp.status, resp.body)
	}
	cookies := v.http.Jar.Cookies(mustURL(e.srv.URL))
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v", cookies)
	}
	set := unlockSetCookie(t, e, f, "secret")
	for _, attr := range []string{"HttpOnly", "SameSite=Lax", "Path=/", "Max-Age=2592000"} {
		if !strings.Contains(set, attr) {
			t.Errorf("Set-Cookie %q lacks %s", set, attr)
		}
	}
	if strings.Contains(set, "Secure") {
		t.Errorf("Secure set on plain http: %q", set)
	}
	if !strings.Contains(unlockSetCookieTLS(t, e, f), "Secure") {
		t.Error("Secure not set behind X-Forwarded-Proto=https")
	}

	// The whole protected subtree is open now: dir, inner dir, items, search and assets.
	for _, id := range []string{f.prot, f.protInner} {
		if got := v.do("GET", "/api/v1/dirs/"+id, nil).status; got != http.StatusOK {
			t.Errorf("dir %s = %d, want 200", id, got)
		}
	}
	for _, item := range []string{f.itemProt, f.itemProtInner, f.itemBoth} {
		if got := v.do("GET", "/api/v1/items/"+item+"/content", nil).status; got != http.StatusOK {
			t.Errorf("content %s = %d, want 200", item, got)
		}
	}
	if got := v.do("GET", "/api/v1/assets/"+f.assetHash, nil).status; got != http.StatusOK {
		t.Errorf("asset = %d, want 200 after unlock", got)
	}
	search := v.do("GET", "/api/v1/dirs/"+f.prot+"/search?q=zebra", nil).json()
	if got := ids(t, search["results"]); !sameSet(got, f.itemProt, f.itemProtInner, f.itemBoth) {
		t.Errorf("search in unlocked tree = %v", got)
	}
	// Other protected trees stay closed, and so does the locked child of the public tree.
	for _, id := range []string{f.oth, f.pubLocked} {
		if got := v.do("GET", "/api/v1/dirs/"+id, nil).status; got != http.StatusUnauthorized {
			t.Errorf("dir %s = %d, want 401", id, got)
		}
	}
	if got := v.do("GET", "/api/v1/items/"+f.itemPubLocked, nil).status; got != http.StatusForbidden {
		t.Errorf("item in other locked dir = %d, want 403", got)
	}

	// Unlocking a second tree keeps the first; the parent listing now reveals the unlocked child's items.
	if resp := unlock(f.pubLocked, "inner-pw"); resp.status != http.StatusOK {
		t.Fatalf("unlock pubLocked: %d %s", resp.status, resp.body)
	}
	if got := v.do("GET", "/api/v1/dirs/"+f.prot, nil).status; got != http.StatusOK {
		t.Errorf("first unlock lost after second: %d", got)
	}
	search = v.do("GET", "/api/v1/dirs/"+f.pub+"/search?q=zebra", nil).json()
	if got := ids(t, search["results"]); !sameSet(got, f.itemPub, f.itemBoth, f.itemPubLocked) {
		t.Errorf("parent search after unlocking the child = %v", got)
	}
}

func unlockSetCookie(t *testing.T, e *env, f fixture, pw string) string {
	t.Helper()
	resp := e.anon().do("POST", "/api/v1/dirs/"+f.prot+"/unlock", map[string]string{"password": pw})
	return strings.Join(resp.header.Values("Set-Cookie"), "\n")
}

func unlockSetCookieTLS(t *testing.T, e *env, f fixture) string {
	t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/dirs/"+f.prot+"/unlock", strings.NewReader(`{"password":"secret"}`))
	req.Header.Set("X-Forwarded-Proto", "https")
	resp := e.anon().send(req)
	return strings.Join(resp.header.Values("Set-Cookie"), "\n")
}

func TestCookieForgeryAndTamper(t *testing.T) {
	e := newEnv(t)
	f := newFixture(t, e)
	v := e.anon()
	if resp := v.do("POST", "/api/v1/dirs/"+f.prot+"/unlock", map[string]string{"password": "secret"}); resp.status != http.StatusOK {
		t.Fatal(resp.body)
	}
	good := v.rawCookie("aumbry_unlocked")
	if good == "" {
		t.Fatal("no visitor cookie issued")
	}
	status := func(cookie string) int {
		c := e.anon()
		c.cookie = cookie
		return c.do("GET", "/api/v1/dirs/"+f.prot, nil).status
	}
	if got := status("aumbry_unlocked=" + good); got != http.StatusOK {
		t.Fatalf("genuine cookie = %d, want 200", got)
	}

	flipped := []byte(good)
	flipped[len(flipped)/2] ^= 1 // still a base64 alphabet character, different content
	forgeries := map[string]string{
		"tampered":        string(flipped),
		"garbage":         "not-a-cookie",
		"plain json":      `{"` + f.prot + `":"x"}`,
		"truncated":       good[:len(good)-4],
		"other cookie":    "", // filled below: an admin cookie replayed as the visitor cookie
		"wrong signature": good[:len(good)-1] + "A",
	}
	admin := f.admin.rawCookie("aumbry_admin")
	forgeries["other cookie"] = admin
	for name, value := range forgeries {
		if got := status("aumbry_unlocked=" + value); got != http.StatusUnauthorized {
			t.Errorf("%s visitor cookie = %d, want 401", name, got)
		}
	}

	// Admin cookies cannot be forged or borrowed from the visitor cookie either.
	for name, value := range map[string]string{"literal": "true", "visitor cookie": good, "empty": ""} {
		c := e.anon()
		c.cookie = "aumbry_admin=" + value
		if got := c.do("GET", "/api/v1/admin/me", nil).status; got != http.StatusUnauthorized {
			t.Errorf("%s admin cookie = %d, want 401", name, got)
		}
	}
	if got := f.admin.do("GET", "/api/v1/admin/me", nil); got.status != http.StatusOK || got.json()["admin"] != true {
		t.Errorf("genuine admin cookie: %d %s", got.status, got.body)
	}
}

func TestChangingPasswordRevokesUnlocks(t *testing.T) {
	e := newEnv(t)
	f := newFixture(t, e)
	v := e.anon()
	v.do("POST", "/api/v1/dirs/"+f.prot+"/unlock", map[string]string{"password": "secret"})
	if got := v.do("GET", "/api/v1/dirs/"+f.prot, nil).status; got != http.StatusOK {
		t.Fatalf("unlocked dir = %d", got)
	}
	if got := f.admin.do("PATCH", "/api/v1/admin/dirs/"+f.prot, map[string]any{"password": "new-secret"}).status; got != http.StatusNoContent {
		t.Fatalf("patch password = %d", got)
	}
	if got := v.do("GET", "/api/v1/dirs/"+f.prot, nil).status; got != http.StatusUnauthorized {
		t.Errorf("old session after password change = %d, want 401", got)
	}
	if got := v.do("POST", "/api/v1/dirs/"+f.prot+"/unlock", map[string]string{"password": "new-secret"}).status; got != http.StatusOK {
		t.Errorf("unlock with new password = %d", got)
	}
	// Clearing the password makes the directory public.
	f.admin.do("PATCH", "/api/v1/admin/dirs/"+f.prot, map[string]any{"password": ""})
	if got := e.anon().do("GET", "/api/v1/dirs/"+f.prot, nil).status; got != http.StatusOK {
		t.Errorf("directory with cleared password = %d, want 200", got)
	}
}

func TestAssetAccessFollowsMounts(t *testing.T) {
	e := newEnv(t)
	f := newFixture(t, e)
	v := e.anon()
	url := "/api/v1/assets/" + f.assetHash
	if got := v.do("GET", url, nil).status; got != http.StatusForbidden {
		t.Fatalf("before mounting publicly = %d, want 403", got)
	}
	if got := f.admin.do("PUT", "/api/v1/admin/items/"+f.itemProt+"/mounts", map[string]any{"dirs": []string{f.prot, f.pub}}).status; got != http.StatusNoContent {
		t.Fatalf("PUT mounts = %d", got)
	}
	resp := v.do("GET", url, nil)
	if resp.status != http.StatusOK || string(resp.body) != testSVG {
		t.Fatalf("after mounting publicly = %d %q, want the SVG", resp.status, resp.body)
	}
	for header, want := range map[string]string{
		"Content-Type":            "image/svg+xml",
		"Cache-Control":           "private, max-age=86400",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
	} {
		if got := resp.header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	// Unmounting everything makes it admin-only again.
	f.admin.do("PUT", "/api/v1/admin/items/"+f.itemProt+"/mounts", map[string]any{"dirs": []string{}})
	if got := v.do("GET", url, nil).status; got != http.StatusForbidden {
		t.Errorf("unmounted asset = %d, want 403", got)
	}
}

func TestRelatedItemsOnlyListAccessible(t *testing.T) {
	e := newEnv(t)
	f := newFixture(t, e)
	post := f.admin.ingestOK(ingestReq{
		xml:  xmlDoc(docOpts{title: "Index", related: []string{f.itemPub, f.itemProt, "ZZZZZZZZZZZZ"}}),
		dirs: []string{f.pub},
	})
	if got := ids(t, e.anon().do("GET", "/api/v1/items/"+post, nil).json()["related"]); !sameSet(got, f.itemPub) {
		t.Errorf("visitor related = %v, want only the public item", got)
	}
	if got := ids(t, f.admin.do("GET", "/api/v1/items/"+post, nil).json()["related"]); !sameSet(got, f.itemPub, f.itemProt) {
		t.Errorf("admin related = %v", got)
	}
}

func TestBreadcrumbStopsAtHiddenAncestors(t *testing.T) {
	e := newEnv(t)
	f := newFixture(t, e)
	v := e.anon()
	v.do("POST", "/api/v1/dirs/"+f.protInner+"/unlock", map[string]string{"password": "secret"})
	// Everything on the way up is the unlocked, protected root itself.
	dir := v.do("GET", "/api/v1/dirs/"+f.protInner, nil).json()
	if got := ids(t, dir["breadcrumb"]); len(got) != 2 || got[0] != f.prot || got[1] != f.protInner {
		t.Errorf("breadcrumb = %v, want root to self", got)
	}
	if dir["parent_id"] != f.prot {
		t.Errorf("parent_id = %v", dir["parent_id"])
	}

	// Unlocking only the child of a public parent: the public parent is visible, so it is in the trail.
	v2 := e.anon()
	v2.do("POST", "/api/v1/dirs/"+f.pubLocked+"/unlock", map[string]string{"password": "inner-pw"})
	dir = v2.do("GET", "/api/v1/dirs/"+f.pubLocked, nil).json()
	if got := ids(t, dir["breadcrumb"]); len(got) != 2 || got[0] != f.pub {
		t.Errorf("breadcrumb = %v", got)
	}
}
