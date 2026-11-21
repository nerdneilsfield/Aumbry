package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestAdminEndpointsRequireLogin(t *testing.T) {
	e := newEnv(t)
	v := e.anon()
	for _, ep := range []struct{ method, path string }{
		{"GET", "/api/v1/admin/me"},
		{"GET", "/api/v1/admin/tree"},
		{"POST", "/api/v1/admin/dirs"},
		{"PATCH", "/api/v1/admin/dirs/x"},
		{"DELETE", "/api/v1/admin/dirs/x"},
		{"PUT", "/api/v1/admin/items/x/mounts"},
		{"DELETE", "/api/v1/admin/items/x"},
		{"GET", "/api/v1/admin/search"},
	} {
		if got := v.do(ep.method, ep.path, map[string]any{}).status; got != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", ep.method, ep.path, got)
		}
	}
	// An API token is not an admin session.
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/admin/me", nil)
	req.Header.Set("Authorization", "Bearer "+apiToken)
	if got := v.send(req).status; got != http.StatusUnauthorized {
		t.Errorf("API token as admin = %d", got)
	}
}

func TestAdminLoginLogout(t *testing.T) {
	e := newEnv(t)
	c := e.anon()
	for name, creds := range map[string]map[string]string{
		"wrong password": {"username": adminUser, "password": "nope"},
		"wrong user":     {"username": "root", "password": adminPassword},
		"empty":          {"username": "", "password": ""},
	} {
		resp := c.do("POST", "/api/v1/admin/login", creds)
		if resp.status != http.StatusUnauthorized || resp.json()["error"] != "invalid_credentials" {
			t.Errorf("%s: %d %s", name, resp.status, resp.body)
		}
	}
	resp := c.do("POST", "/api/v1/admin/login", map[string]string{"username": adminUser, "password": adminPassword})
	if resp.status != http.StatusOK || resp.json()["ok"] != true {
		t.Fatalf("login: %d %s", resp.status, resp.body)
	}
	set := resp.header.Get("Set-Cookie")
	for _, attr := range []string{"HttpOnly", "SameSite=Lax", "Path=/", "Max-Age=604800"} {
		if !strings.Contains(set, attr) {
			t.Errorf("Set-Cookie %q lacks %s", set, attr)
		}
	}
	if got := c.do("GET", "/api/v1/admin/me", nil).status; got != http.StatusOK {
		t.Errorf("me after login = %d", got)
	}
	if got := c.do("POST", "/api/v1/admin/logout", nil).status; got != http.StatusNoContent {
		t.Errorf("logout = %d", got)
	}
	if got := c.do("GET", "/api/v1/admin/me", nil).status; got != http.StatusUnauthorized {
		t.Errorf("me after logout = %d", got)
	}
}

func TestAdminDirectoryManagement(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()

	root := admin.createDir("", "Root", "pw")
	child := admin.createDir(root, "Child", "")
	grand := admin.createDir(child, "Grandchild", "")
	if resp := admin.do("POST", "/api/v1/admin/dirs", map[string]any{"name": "  "}); resp.status != http.StatusBadRequest {
		t.Errorf("empty name = %d", resp.status)
	}
	if resp := admin.do("POST", "/api/v1/admin/dirs", map[string]any{"name": "x", "parent_id": "nosuchdir123"}); resp.status != http.StatusBadRequest {
		t.Errorf("unknown parent = %d", resp.status)
	}
	item := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "I"}), dirs: []string{grand}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Loose"})})

	tree := admin.do("GET", "/api/v1/admin/tree", nil).json()
	if tree["unmounted_count"] != float64(1) {
		t.Errorf("unmounted_count = %v", tree["unmounted_count"])
	}
	top := tree["tree"].([]any)
	if len(top) != 1 {
		t.Fatalf("tree = %v", tree)
	}
	node := top[0].(map[string]any)
	if node["id"] != root || node["has_password"] != true || node["item_count"] != float64(0) {
		t.Errorf("root node = %v", node)
	}
	leaf := node["children"].([]any)[0].(map[string]any)["children"].([]any)[0].(map[string]any)
	if leaf["id"] != grand || leaf["has_password"] != false || leaf["item_count"] != float64(1) {
		t.Errorf("leaf node = %v", leaf)
	}

	patch := func(id string, body map[string]any) int {
		return admin.do("PATCH", "/api/v1/admin/dirs/"+id, body).status
	}
	// Cycles are rejected.
	if got := patch(root, map[string]any{"parent_id": grand}); got != http.StatusBadRequest {
		t.Errorf("move into descendant = %d, want 400", got)
	}
	if got := patch(root, map[string]any{"parent_id": root}); got != http.StatusBadRequest {
		t.Errorf("move into itself = %d, want 400", got)
	}
	if got := patch(root, map[string]any{"parent_id": "nosuchdir123"}); got != http.StatusBadRequest {
		t.Errorf("move below unknown = %d, want 400", got)
	}
	if got := patch("nosuchdir123", map[string]any{"name": "x"}); got != http.StatusNotFound {
		t.Errorf("patch unknown = %d, want 404", got)
	}
	if got := patch(root, map[string]any{"bogus": 1}); got != http.StatusBadRequest {
		t.Errorf("unknown field = %d, want 400", got)
	}

	// Absent fields stay; explicit null moves to the root.
	if got := patch(grand, map[string]any{"name": "Renamed", "description": "d", "sort_order": 5}); got != http.StatusNoContent {
		t.Fatalf("patch = %d", got)
	}
	view := admin.do("GET", "/api/v1/dirs/"+grand, nil).json()
	if view["name"] != "Renamed" || view["description"] != "d" || view["parent_id"] != child {
		t.Errorf("after rename: %v", view)
	}
	if got := patch(grand, map[string]any{"parent_id": nil}); got != http.StatusNoContent {
		t.Fatalf("move to root = %d", got)
	}
	if got := admin.do("GET", "/api/v1/dirs/"+grand, nil).json()["parent_id"]; got != nil {
		t.Errorf("parent_id after move to root = %v", got)
	}
	// Moving out of the protected tree makes the item public.
	if got := e.anon().do("GET", "/api/v1/items/"+item, nil).status; got != http.StatusOK {
		t.Errorf("item in a directory moved to the root = %d, want 200", got)
	}
	if got := patch(grand, map[string]any{"parent_id": child}); got != http.StatusNoContent {
		t.Fatalf("move back = %d", got)
	}
	if got := e.anon().do("GET", "/api/v1/items/"+item, nil).status; got != http.StatusForbidden {
		t.Errorf("item under the protected tree again = %d, want 403", got)
	}

	// Deleting a directory cascades to its subtree and keeps the items.
	if got := admin.do("DELETE", "/api/v1/admin/dirs/"+root, nil).status; got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	if got := admin.do("GET", "/api/v1/dirs/"+grand, nil).status; got != http.StatusNotFound {
		t.Errorf("subtree after delete = %d, want 404", got)
	}
	if got := admin.do("GET", "/api/v1/items/"+item, nil).status; got != http.StatusOK {
		t.Errorf("item after its directory was deleted = %d, want 200", got)
	}
	if got := admin.do("GET", "/api/v1/admin/tree", nil).json()["unmounted_count"]; got != float64(2) {
		t.Errorf("unmounted_count after delete = %v, want 2", got)
	}
	if got := admin.do("DELETE", "/api/v1/admin/dirs/"+root, nil).status; got != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", got)
	}
}

func TestAdminSetMounts(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	a, b := admin.createDir("", "A", ""), admin.createDir("", "B", "")
	item := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "I"}), dirs: []string{a}})
	put := func(id string, body map[string]any) int {
		return admin.do("PUT", "/api/v1/admin/items/"+id+"/mounts", body).status
	}
	if got := put(item, map[string]any{"dirs": []string{b, "nosuchdir123"}}); got != http.StatusBadRequest {
		t.Errorf("unknown dir = %d, want 400", got)
	}
	if got := put("nosuchitem12", map[string]any{"dirs": []string{b}}); got != http.StatusNotFound {
		t.Errorf("unknown item = %d, want 404", got)
	}
	if got := put(item, map[string]any{}); got != http.StatusBadRequest {
		t.Errorf("missing dirs = %d, want 400", got)
	}
	if got := put(item, map[string]any{"dirs": []string{b, b}}); got != http.StatusNoContent {
		t.Fatalf("replace = %d", got)
	}
	if got := ids(t, admin.do("GET", "/api/v1/items/"+item, nil).json()["dirs"]); !sameSet(got, b) {
		t.Errorf("mounts = %v, want only %s", got, b)
	}
	if got := ids(t, admin.do("GET", "/api/v1/dirs/"+a, nil).json()["items"]); len(got) != 0 {
		t.Errorf("old directory still lists the item: %v", got)
	}
}

func TestPasswordAttemptsAreRateLimited(t *testing.T) {
	for name, attempt := range map[string]func(*env, *client) response{
		"admin login": func(_ *env, c *client) response {
			return c.do("POST", "/api/v1/admin/login", map[string]string{"username": adminUser, "password": "wrong"})
		},
		"directory unlock": func(e *env, c *client) response {
			return c.do("POST", "/api/v1/dirs/"+e.dir+"/unlock", map[string]string{"password": "wrong"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			if name == "directory unlock" { // the setup login must not eat the admin login budget
				e.dir = e.admin().createDir("", "Locked", "pw")
			}
			c := e.anon()
			for i := 1; i <= 10; i++ {
				if got := attempt(e, c).status; got != http.StatusUnauthorized {
					t.Fatalf("attempt %d = %d, want 401", i, got)
				}
			}
			resp := attempt(e, c)
			if resp.status != http.StatusTooManyRequests || resp.json()["error"] != "too_many_attempts" {
				t.Fatalf("attempt 11 = %d %s, want 429 too_many_attempts", resp.status, resp.body)
			}
			if resp.header.Get("Retry-After") == "" {
				t.Error("429 without Retry-After")
			}
		})
	}
}
