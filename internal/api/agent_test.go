package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func (e *env) agentDo(method, path, token string) response {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, nil)
	if token != "-" {
		tok := token
		if tok == "" {
			tok = apiToken
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return e.anon().send(req)
}

func TestAgentEndpointsRequireToken(t *testing.T) {
	e := newEnv(t)
	endpoints := [][2]string{
		{http.MethodGet, "/api/v1/agent/dirs"},
		{http.MethodGet, "/api/v1/agent/items"},
		{http.MethodDelete, "/api/v1/agent/items/nonexistent1/mounts"},
		{http.MethodDelete, "/api/v1/agent/items/nonexistent1"},
	}
	for _, ep := range endpoints {
		for _, tok := range []string{"-", "wrong"} {
			resp := e.agentDo(ep[0], ep[1], tok)
			if resp.status != http.StatusUnauthorized || resp.json()["error"] != "unauthorized" {
				t.Errorf("%s %s tok=%q = %d %s, want 401", ep[0], ep[1], tok, resp.status, resp.body)
			}
		}
	}
}

func TestAgentDirsHierarchyAndProtection(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	rootOpen := admin.createDir("", "Public", "")
	rootLocked := admin.createDir("", "Private", "secretpw")
	childInherited := admin.createDir(rootLocked, "SubNoOwnPW", "")
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Item1"}), dirs: []string{childInherited}})

	resp := e.agentDo(http.MethodGet, "/api/v1/agent/dirs", "")
	if resp.status != http.StatusOK {
		t.Fatalf("GET /api/v1/agent/dirs = %d %s", resp.status, resp.body)
	}
	if strings.Contains(string(resp.body), "password_hash") || strings.Contains(string(resp.body), "$2") {
		t.Fatalf("agent dirs leaked password hash: %s", resp.body)
	}

	var dirs []agentDir
	if err := json.Unmarshal(resp.body, &dirs); err != nil {
		t.Fatal(err)
	}
	byID := map[string]agentDir{}
	for _, d := range dirs {
		byID[d.ID] = d
	}
	if got := byID[rootOpen]; got.Path != "Public" || got.ParentID != nil || got.HasPassword || got.Protected {
		t.Errorf("rootOpen = %+v", got)
	}
	if got := byID[rootLocked]; got.Path != "Private" || !got.HasPassword || !got.Protected {
		t.Errorf("rootLocked = %+v", got)
	}
	// #43: self has no password, ancestor has password -> has_password=false, protected=true.
	sub := byID[childInherited]
	if sub.Path != "Private / SubNoOwnPW" || sub.ParentID == nil || *sub.ParentID != rootLocked || sub.HasPassword || !sub.Protected || sub.ItemCount != 1 {
		t.Errorf("childInherited = %+v", sub)
	}
}

func TestAgentListItemsScopingAndOwnership(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir1 := admin.createDir("", "D1", "")
	dir2 := admin.createDir("", "D2", "")

	idA := admin.ingestOK(ingestReq{
		token: apiToken,
		xml:   xmlDoc(docOpts{title: "Alpha Paper", summary: "UniqueSummaryOnlySecret", md: "UniqueBodyOnlySecret"}),
		key:   "paper:2611.01234",
		dirs:  []string{dir1},
	})
	idB := admin.ingestOK(ingestReq{
		token: "other",
		xml:   xmlDoc(docOpts{title: "Beta Draft", md: "beta body"}),
		key:   "repo:foo/bar",
		draft: true,
	})
	// Direct legacy NULL item
	if _, err := e.db.Exec(`INSERT INTO items
		(id, external_key, type, title, report_date, summary, tags_json, source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text, created_by)
		VALUES ('legacyitem01', 'legacy:k', 'post', 'Legacy Gamma', '2026-11-19', 'leg sum', '[]', 'a', '', '[]', '/xml/leg.xml', 'leg body', NULL)`); err != nil {
		t.Fatal(err)
	}
	_ = admin.do("PUT", "/api/v1/admin/items/legacyitem01/mounts", map[string]any{"dirs": []string{dir2}})

	// #38: key exact match returns ID & dirs and never XML content.
	respKey := e.agentDo(http.MethodGet, "/api/v1/agent/items?key=paper:2611.01234", apiToken)
	if respKey.status != http.StatusOK {
		t.Fatalf("list by key = %d %s", respKey.status, respKey.body)
	}
	if strings.Contains(string(respKey.body), "<document") || strings.Contains(string(respKey.body), "UniqueBodyOnlySecret") {
		t.Fatalf("list response leaked XML body: %s", respKey.body)
	}
	var out agentItemsResponse
	if err := json.Unmarshal(respKey.body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].ID != idA || !out.Items[0].Owned || len(out.Items[0].Dirs) != 1 || out.Items[0].Dirs[0] != dir1 {
		t.Fatalf("key list items = %+v", out.Items)
	}

	// q matches title or external_key, NEVER summary or body (PRD 7.5).
	for _, secret := range []string{"UniqueSummaryOnlySecret", "UniqueBodyOnlySecret"} {
		resp := e.agentDo(http.MethodGet, "/api/v1/agent/items?q="+secret, apiToken)
		var r agentItemsResponse
		_ = json.Unmarshal(resp.body, &r)
		if len(r.Items) != 0 {
			t.Errorf("q=%q leaked match via summary/body: %+v", secret, r.Items)
		}
	}
	// q matches external_key for idB, owned=false for Token A, true for Token B ("other").
	respQ := e.agentDo(http.MethodGet, "/api/v1/agent/items?q=foo/bar", apiToken)
	_ = json.Unmarshal(respQ.body, &out)
	if len(out.Items) != 1 || out.Items[0].ID != idB || out.Items[0].Owned {
		t.Errorf("q=foo/bar as Token A = %+v, want idB owned=false", out.Items)
	}
	respQB := e.agentDo(http.MethodGet, "/api/v1/agent/items?q=foo/bar", "other")
	_ = json.Unmarshal(respQB.body, &out)
	if len(out.Items) != 1 || out.Items[0].ID != idB || !out.Items[0].Owned {
		t.Errorf("q=foo/bar as Token B = %+v, want idB owned=true", out.Items)
	}

	// dir filter + legacy item ownership check.
	respDir := e.agentDo(http.MethodGet, "/api/v1/agent/items?dir="+dir2, apiToken)
	_ = json.Unmarshal(respDir.body, &out)
	if len(out.Items) != 1 || out.Items[0].ID != "legacyitem01" || out.Items[0].Owned {
		t.Errorf("dir filter legacy = %+v, want legacyitem01 owned=false", out.Items)
	}
}

func TestAgentUnpublishDeleteAndOwnershipEnforcement(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	anon := e.anon()
	dir := admin.createDir("", "Public", "")

	idA := admin.ingestOK(ingestReq{token: apiToken, xml: xmlDoc(docOpts{title: "Owned A", md: "tokenaunique"}), key: "k:a", dirs: []string{dir}})
	if _, err := e.db.Exec(`INSERT INTO items
		(id, external_key, type, title, report_date, summary, tags_json, source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text, created_by)
		VALUES ('legacy000001', 'k:leg', 'report', 'Legacy', '2026-11-20', 's', '[]', 'a', '', '[]', '/xml/leg.xml', 'legacytext', NULL)`); err != nil {
		t.Fatal(err)
	}
	_ = admin.do("PUT", "/api/v1/admin/items/legacy000001/mounts", map[string]any{"dirs": []string{dir}})

	// Token B cannot unpublish or delete Token A's item -> 403 not_owner; content & mounts intact.
	if resp := e.agentDo(http.MethodDelete, "/api/v1/agent/items/"+idA+"/mounts", "other"); resp.status != http.StatusForbidden || resp.json()["error"] != "not_owner" {
		t.Fatalf("Token B unpublish = %d %s, want 403 not_owner", resp.status, resp.body)
	}
	if resp := e.agentDo(http.MethodDelete, "/api/v1/agent/items/"+idA, "other"); resp.status != http.StatusForbidden || resp.json()["error"] != "not_owner" {
		t.Fatalf("Token B delete = %d %s, want 403 not_owner", resp.status, resp.body)
	}
	if st := anon.do("GET", "/api/v1/items/"+idA, nil).status; st != http.StatusOK {
		t.Fatalf("item A vanished after rejected Token B actions: %d", st)
	}

	// Legacy NULL created_by rejects every token with 403 not_owner, but admin delete works.
	for _, tok := range []string{apiToken, "other"} {
		if resp := e.agentDo(http.MethodDelete, "/api/v1/agent/items/legacy000001/mounts", tok); resp.status != http.StatusForbidden || resp.json()["error"] != "not_owner" {
			t.Errorf("legacy unpublish tok=%q = %d %s, want 403 not_owner", tok, resp.status, resp.body)
		}
		if resp := e.agentDo(http.MethodDelete, "/api/v1/agent/items/legacy000001", tok); resp.status != http.StatusForbidden || resp.json()["error"] != "not_owner" {
			t.Errorf("legacy delete tok=%q = %d %s, want 403 not_owner", tok, resp.status, resp.body)
		}
	}
	if resp := admin.do("DELETE", "/api/v1/admin/items/legacy000001", nil); resp.status != http.StatusNoContent {
		t.Fatalf("admin delete legacy = %d %s, want 204", resp.status, resp.body)
	}

	// Token A unpublish -> visitor 403, admin 200, ID preserved.
	if resp := e.agentDo(http.MethodDelete, "/api/v1/agent/items/"+idA+"/mounts", apiToken); resp.status != http.StatusNoContent {
		t.Fatalf("Token A unpublish = %d %s, want 204", resp.status, resp.body)
	}
	if st := anon.do("GET", "/api/v1/items/"+idA, nil).status; st != http.StatusForbidden {
		t.Errorf("visitor GET unpublished = %d, want 403", st)
	}
	if st := admin.do("GET", "/api/v1/items/"+idA, nil).status; st != http.StatusOK {
		t.Errorf("admin GET unpublished = %d, want 200", st)
	}

	// Token A delete -> 204, item & items_fts & storage object removed.
	if resp := e.agentDo(http.MethodDelete, "/api/v1/agent/items/"+idA, apiToken); resp.status != http.StatusNoContent {
		t.Fatalf("Token A delete = %d %s, want 204", resp.status, resp.body)
	}
	var itemCount, ftsCount int
	_ = e.db.Get(&itemCount, `SELECT count(*) FROM items WHERE id = ?`, idA)
	_ = e.db.Get(&ftsCount, `SELECT count(*) FROM items_fts WHERE item_id = ?`, idA)
	if itemCount != 0 || ftsCount != 0 || e.store.count("/xml/"+idA) != 0 {
		t.Fatalf("delete leftover: items=%d fts=%d storeXML=%d", itemCount, ftsCount, e.store.count("/xml/"+idA))
	}

	// Missing item -> 404.
	if resp := e.agentDo(http.MethodDelete, "/api/v1/agent/items/doesnotexist", apiToken); resp.status != http.StatusNotFound {
		t.Errorf("missing item delete = %d, want 404", resp.status)
	}
}

func TestAgentAndIngestShareWriteRateLimitBucket(t *testing.T) {
	e := newEnv(t)
	// Burn 120 requests against /api/v1/agent/dirs from one TCP peer IP.
	for i := range writeRateLimitPerMin {
		rec := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/agent/dirs", nil)
		req.RemoteAddr = "198.51.100.77:5555"
		req.Header.Set("Authorization", "Bearer "+apiToken)
		e.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("agent dirs #%d = %d, want 200", i+1, rec.Code)
		}
	}
	// 121st request across /api/v1/ingest from the SAME IP must hit the shared 429 bucket.
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/ingest", nil)
	req.RemoteAddr = "198.51.100.77:5555"
	req.Header.Set("Authorization", "Bearer "+apiToken)
	e.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("121st write request = %d, want 429 from shared bucket", rec.Code)
	}
}
