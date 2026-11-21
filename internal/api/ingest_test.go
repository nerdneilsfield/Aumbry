package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
)

func TestIngestRequiresToken(t *testing.T) {
	e := newEnv(t)
	c := e.anon()
	for _, token := range []string{"-", "wrong"} {
		resp := c.ingest(ingestReq{token: token, xml: xmlDoc(docOpts{title: "x"})})
		if resp.status != http.StatusUnauthorized || resp.json()["error"] != "unauthorized" {
			t.Errorf("token %q: %d %s, want 401", token, resp.status, resp.body)
		}
	}
	if resp := c.do("POST", "/api/v1/ingest", nil); resp.status != http.StatusUnauthorized {
		t.Errorf("no auth header: %d", resp.status)
	}
	if n := e.store.count("/xml/"); n != 0 {
		t.Errorf("%d xml files stored by rejected requests", n)
	}
}

func TestIngestFigureRewriteAndDedup(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "Papers", "")
	sum := sha256.Sum256([]byte(testSVG))
	hash := hex.EncodeToString(sum[:])

	resp := admin.ingest(ingestReq{
		xml:    xmlDoc(docOpts{title: "With figure", figures: []string{"./imgs/arch.svg"}, tags: []string{"A", "B"}}),
		assets: []figure{{src: "./imgs/arch.svg", name: "arch.svg", data: []byte(testSVG)}},
		dirs:   []string{dir},
	})
	if resp.status != http.StatusCreated {
		t.Fatalf("ingest: %d %s", resp.status, resp.body)
	}
	out := resp.json()
	id := out["id"].(string)
	if len(id) != 12 || out["url"] != "/r/"+id || out["type"] != "report" || out["created"] != true || out["assets"] != float64(1) {
		t.Errorf("response = %v", out)
	}
	if got := out["dirs"].([]any); len(got) != 1 || got[0] != dir {
		t.Errorf("dirs = %v", got)
	}

	// The stored XML points at the proxied asset, not at the local path.
	xml := string(admin.do("GET", "/api/v1/items/"+id+"/content", nil).body)
	if !strings.Contains(xml, `src="/api/v1/assets/`+hash+`"`) || strings.Contains(xml, "imgs/arch.svg") {
		t.Errorf("figure src not rewritten: %s", xml)
	}
	if ct := admin.do("GET", "/api/v1/items/"+id+"/content", nil).header.Get("Content-Type"); ct != "application/xml; charset=utf-8" {
		t.Errorf("content type = %q", ct)
	}
	if got := e.store.files["/assets/"+hash+".svg"]; string(got) != testSVG {
		t.Errorf("asset not stored at /assets/%s.svg", hash)
	}
	if n := e.store.count("/xml/" + id); n != 1 {
		t.Errorf("%d xml objects stored for the item, want 1", n)
	}
	if !e.store.has("/xml/"+id, xml) {
		t.Error("served xml is not what is stored for the item")
	}

	// Same bytes under another src and in another item: one stored object.
	resp = admin.ingest(ingestReq{
		xml:    xmlDoc(docOpts{title: "Second", typ: "post", figures: []string{"pics/other-name.svg"}}),
		assets: []figure{{src: "pics/other-name.svg", name: "x.svg", data: []byte(testSVG)}, {src: "unused.svg", name: "u.svg", data: []byte("ignored")}},
		dirs:   []string{dir},
	})
	if resp.status != http.StatusCreated {
		t.Fatalf("second ingest: %d %s", resp.status, resp.body)
	}
	if got := resp.json(); got["url"] != "/p/"+got["id"].(string) || got["assets"] != float64(1) {
		t.Errorf("second response = %v", got)
	}
	if n := e.store.count("/assets/"); n != 1 {
		t.Errorf("%d assets stored, want 1 (dedup, unreferenced upload ignored)", n)
	}
}

func TestIngestUpsertByExternalKey(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	first := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Before edit", md: "oldtext"}), key: "daily:2026-11-20"})
	if first.status != http.StatusCreated {
		t.Fatalf("first: %d %s", first.status, first.body)
	}
	id := first.json()["id"].(string)

	second := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "After edit", md: "newtext"}), key: "daily:2026-11-20"})
	if second.status != http.StatusOK || second.json()["id"] != id || second.json()["created"] != false {
		t.Fatalf("second: %d %s, want 200 with the same id", second.status, second.body)
	}
	if n := countItems(t, admin); n != 1 {
		t.Errorf("%d items, want 1", n)
	}
	if got := admin.do("GET", "/api/v1/items/"+id, nil).json()["title"]; got != "After edit" {
		t.Errorf("title = %v", got)
	}
	// The search index follows the update.
	search := func(q string) int {
		return len(admin.do("GET", "/api/v1/admin/search?q="+q, nil).json()["results"].([]any))
	}
	if search("newtext") != 1 || search("oldtext") != 0 {
		t.Error("items_fts not replaced on update")
	}

	// Updating by item_id works as well; an unknown id is a 404.
	if resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "By id"}), itemID: id}); resp.status != http.StatusOK || resp.json()["id"] != id {
		t.Errorf("update by item_id: %d %s", resp.status, resp.body)
	}
	if resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "x"}), itemID: "doesnotexist"}); resp.status != http.StatusNotFound {
		t.Errorf("unknown item_id: %d", resp.status)
	}

	// Pointing another item at a key that is taken conflicts.
	other := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Other"}), key: "other-key"})
	if resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Other"}), itemID: other, key: "daily:2026-11-20"}); resp.status != http.StatusConflict {
		t.Errorf("duplicate external_key: %d %s, want 409", resp.status, resp.body)
	}
}

func countItems(t *testing.T, admin *client) int {
	t.Helper()
	return len(admin.do("GET", "/api/v1/admin/search?limit=100", nil).json()["results"].([]any))
}

func TestIngestAddsMountsNeverRemoves(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	a, b := admin.createDir("", "A", ""), admin.createDir("", "B", "")
	id := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Doc"}), key: "k", dirs: []string{a}})

	resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Doc v2"}), key: "k", dirs: []string{b}, commaDir: true})
	if resp.status != http.StatusOK {
		t.Fatalf("re-ingest: %d %s", resp.status, resp.body)
	}
	if got := resp.json()["dirs"].([]any); len(got) != 2 {
		t.Errorf("dirs after re-ingest = %v, want both", got)
	}
	// Comma separated, repeated and without any dirs: existing mounts survive.
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Doc v3"}), key: "k"})
	detail := admin.do("GET", "/api/v1/items/"+id, nil).json()
	if got := ids(t, detail["dirs"]); !sameSet(got, a, b) {
		t.Errorf("mounts = %v, want %s and %s", got, a, b)
	}
	resp = admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Doc v4"}), key: "k", dirs: []string{a, b}, commaDir: true})
	if got := resp.json()["dirs"].([]any); len(got) != 2 {
		t.Errorf("comma separated dirs: %v", got)
	}
}

func TestIngestRejections(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "D", "")

	t.Run("invalid xml answers 422 with the validation result", func(t *testing.T) {
		for name, xml := range map[string]string{
			"malformed":     `<document version="1.0"><meta>`,
			"missing title": strings.Replace(xmlDoc(docOpts{title: "t"}), "<title>t</title>", "", 1),
			"missing asset": xmlDoc(docOpts{title: "t", figures: []string{"./nope.png"}}),
			"bad type":      strings.Replace(xmlDoc(docOpts{title: "t"}), "<type>report</type>", "<type>memo</type>", 1),
		} {
			resp := admin.ingest(ingestReq{xml: xml, dirs: []string{dir}})
			body := resp.json()
			errs, _ := body["errors"].([]any)
			if resp.status != http.StatusUnprocessableEntity || body["valid"] != false || len(errs) == 0 {
				t.Errorf("%s: %d %s, want 422 valid:false with errors", name, resp.status, resp.body)
			}
		}
		resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "t", figures: []string{"./nope.png"}})})
		if !strings.Contains(string(resp.body), "MISSING_ASSET") {
			t.Errorf("missing figure not reported as MISSING_ASSET: %s", resp.body)
		}
		if n := countItems(t, admin); n != 0 {
			t.Errorf("%d items stored from rejected documents", n)
		}
		if n := e.store.count("/xml/"); n != 0 {
			t.Errorf("%d xml files stored from rejected documents", n)
		}
	})

	t.Run("figure that is not an image is rejected", func(t *testing.T) {
		resp := admin.ingest(ingestReq{
			xml:    xmlDoc(docOpts{title: "t", figures: []string{"./fake.png"}}),
			assets: []figure{{src: "./fake.png", name: "fake.png", data: []byte("just some text, not a picture")}},
		})
		if resp.status != http.StatusUnprocessableEntity || !strings.Contains(string(resp.body), "INVALID_ASSET") {
			t.Errorf("%d %s", resp.status, resp.body)
		}
	})

	t.Run("unknown directory", func(t *testing.T) {
		resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "t"}), dirs: []string{dir, "nosuchdir123"}})
		if resp.status != http.StatusBadRequest || !strings.Contains(string(resp.body), "nosuchdir123") {
			t.Errorf("%d %s", resp.status, resp.body)
		}
	})

	t.Run("asset_srcs must pair with the files", func(t *testing.T) {
		resp := admin.ingest(ingestReq{
			xml:     xmlDoc(docOpts{title: "t", figures: []string{"./a.svg"}}),
			assets:  []figure{{src: "./a.svg", name: "a.svg", data: []byte(testSVG)}},
			rawSrcs: []string{},
		})
		if resp.status != http.StatusBadRequest {
			t.Errorf("missing asset_srcs: %d %s", resp.status, resp.body)
		}
		resp = admin.ingest(ingestReq{
			xml:     xmlDoc(docOpts{title: "t"}),
			assets:  []figure{{src: "x", name: "a.svg", data: []byte(testSVG)}},
			rawSrcs: []string{"../escape.svg"},
		})
		if resp.status != http.StatusBadRequest {
			t.Errorf("path escaping asset_srcs: %d %s", resp.status, resp.body)
		}
	})
}

func TestAdminDeleteItemCleansUp(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "D", "")
	id := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Doomed", md: "uniqueword"}), dirs: []string{dir}})
	if resp := e.anon().do("DELETE", "/api/v1/admin/items/"+id, nil); resp.status != http.StatusUnauthorized {
		t.Fatalf("delete without admin = %d", resp.status)
	}
	if resp := admin.do("DELETE", "/api/v1/admin/items/"+id, nil); resp.status != http.StatusNoContent {
		t.Fatalf("delete: %d %s", resp.status, resp.body)
	}
	if got := admin.do("GET", "/api/v1/items/"+id, nil).status; got != http.StatusNotFound {
		t.Errorf("deleted item = %d, want 404", got)
	}
	if got := len(admin.do("GET", "/api/v1/admin/search?q=uniqueword", nil).json()["results"].([]any)); got != 0 {
		t.Errorf("deleted item still searchable")
	}
	if n := e.store.count("/xml/"); n != 0 {
		t.Errorf("xml file not removed from storage")
	}
	if got := admin.do("DELETE", "/api/v1/admin/items/"+id, nil).status; got != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", got)
	}
}

// A failed update must leave the item exactly as it was: same content served, same object stored.
func TestFailedUpdateKeepsOldContent(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	id := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Original", md: "oldtext"})})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Holder"}), key: "taken"})
	before := string(admin.do("GET", "/api/v1/items/"+id+"/content", nil).body)

	resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Replacement", md: "newtext"}), itemID: id, key: "taken"})
	if resp.status != http.StatusConflict {
		t.Fatalf("update with a taken external_key = %d %s, want 409", resp.status, resp.body)
	}
	if after := string(admin.do("GET", "/api/v1/items/"+id+"/content", nil).body); after != before || !strings.Contains(after, "oldtext") {
		t.Errorf("content after a failed update changed:\n%s", after)
	}
	if n := e.store.count("/xml/" + id); n != 1 || !e.store.has("/xml/"+id, before) {
		t.Errorf("storage holds %d objects for the item, want only the old one", n)
	}
	if got := admin.do("GET", "/api/v1/items/"+id, nil).json()["title"]; got != "Original" {
		t.Errorf("title after a failed update = %v", got)
	}

	// A successful update serves the new content and removes the superseded object.
	if resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Replacement", md: "newtext"}), itemID: id}); resp.status != http.StatusOK {
		t.Fatalf("update = %d %s", resp.status, resp.body)
	}
	after := string(admin.do("GET", "/api/v1/items/"+id+"/content", nil).body)
	if !strings.Contains(after, "newtext") || e.store.count("/xml/"+id) != 1 || !e.store.has("/xml/"+id, after) {
		t.Errorf("after a successful update: %d objects, content %s", e.store.count("/xml/"+id), after)
	}
	// Re-sending identical content keeps the object that is live.
	if resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Replacement", md: "newtext"}), itemID: id}); resp.status != http.StatusOK {
		t.Fatalf("identical update = %d %s", resp.status, resp.body)
	}
	if got := string(admin.do("GET", "/api/v1/items/"+id+"/content", nil).body); got != after {
		t.Errorf("content after an identical update changed")
	}
}

func TestContentIsServedSandboxed(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	id := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "T"})})
	resp := admin.do("GET", "/api/v1/items/"+id+"/content", nil)
	if got := resp.header.Get("Content-Security-Policy"); got != "default-src 'none'; sandbox" {
		t.Errorf("Content-Security-Policy = %q, want default-src 'none'; sandbox", got)
	}
}
