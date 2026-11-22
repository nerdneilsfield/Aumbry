package api

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nerdneilsfield/aumbry/internal/auth"
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
			"malformed":     `<document version="1.1"><meta>`,
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
	if !e.store.has("/xml/"+id, before) {
		t.Errorf("old xml object missing from storage after failed update")
	}
	if got := admin.do("GET", "/api/v1/items/"+id, nil).json()["title"]; got != "Original" {
		t.Errorf("title after a failed update = %v", got)
	}

	// A successful update serves the new content; the superseded object stays until offline gc.
	if resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Replacement", md: "newtext"}), itemID: id}); resp.status != http.StatusOK {
		t.Fatalf("update = %d %s", resp.status, resp.body)
	}
	after := string(admin.do("GET", "/api/v1/items/"+id+"/content", nil).body)
	if !strings.Contains(after, "newtext") || e.store.count("/xml/"+id) != 2 || !e.store.has("/xml/"+id, after) || !e.store.has("/xml/"+id, before) {
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

func TestIngestFigureRewriteAndVersionCheck(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "Figs", "")

	sum := sha256.Sum256([]byte(testSVG))
	wantAssetURL := "/api/v1/assets/" + hex.EncodeToString(sum[:])

	// #31: <figure> inside <step>, <tab>, and <figure-group> all rewrite on ingest.
	figuresXML := `<document version="1.1">
  <meta><title>Figs</title><type>report</type><date>2026-11-20</date><summary>S</summary><source-agent>a</source-agent></meta>
  <body>
    <steps><step title="S1"><figure src="./s.svg"/></step></steps>
    <tabs><tab title="T1"><figure src="./t.svg"/></tab><tab title="T2"><md>ok</md></tab></tabs>
    <figure-group cols="2"><figure src="./g1.svg"/><figure src="./g2.svg"/></figure-group>
  </body>
</document>`
	resp := admin.ingest(ingestReq{
		xml: figuresXML,
		assets: []figure{
			{src: "./s.svg", name: "s.svg", data: []byte(testSVG)},
			{src: "./t.svg", name: "t.svg", data: []byte(testSVG)},
			{src: "./g1.svg", name: "g1.svg", data: []byte(testSVG)},
			{src: "./g2.svg", name: "g2.svg", data: []byte(testSVG)},
		},
		dirs: []string{dir},
	})
	if resp.status != http.StatusCreated {
		t.Fatalf("ingest figures: %d %s", resp.status, resp.body)
	}
	id := resp.json()["id"].(string)
	content := string(admin.do("GET", "/api/v1/items/"+id+"/content", nil).body)
	if gotCount := strings.Count(content, wantAssetURL); gotCount != 4 {
		t.Fatalf("expected 4 rewritten %s paths, got %d\n%s", wantAssetURL, gotCount, content)
	}

	// Only version="1.1" is accepted: 422 UNSUPPORTED_VERSION with a hint.
	oldVersion := strings.Replace(xmlDoc(docOpts{title: "T"}), `version="1.1"`, `version="1.0"`, 1)
	rej := admin.ingest(ingestReq{xml: oldVersion})
	if rej.status != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 on version 1.0, got %d %s", rej.status, rej.body)
	}
	errs := rej.json()["errors"].([]any)
	first := errs[0].(map[string]any)
	if first["code"] != "UNSUPPORTED_VERSION" || first["hint"] == nil || first["hint"] == "" {
		t.Fatalf("expected UNSUPPORTED_VERSION with hint, got %v", first)
	}
}

func TestIngestExternalKeyValidationDraftCreateOnlyAndCreatedBy(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	anon := e.anon()
	dir := admin.createDir("", "Public", "")

	// external_key validation (whitespace-only, >200 runes, control char -> 400 invalid_request).
	for name, val := range map[string]string{
		"whitespace only": "   \t  ",
		"too long":        strings.Repeat("x", 201),
		"control char":    "bad\x07key",
		"invalid utf-8":   "bad\xffkey",
	} {
		k := val
		resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "BadKey"}), rawKey: &k})
		if resp.status != http.StatusBadRequest || resp.json()["error"] != "invalid_request" {
			t.Errorf("%s: got %d %s, want 400 invalid_request", name, resp.status, resp.body)
		}
	}
	// Exact 200 runes with surrounding spaces succeeds.
	paddedKey := "  " + strings.Repeat("k", 200) + "  "
	okResp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "MaxKey"}), rawKey: &paddedKey})
	if okResp.status != http.StatusCreated {
		t.Fatalf("200-rune padded key = %d %s", okResp.status, okResp.body)
	}

	// #22 & #42: draft lifecycle.
	draftResp := admin.ingest(ingestReq{
		xml:   xmlDoc(docOpts{title: "Draft R1", md: "round1"}),
		key:   "paper:draft1",
		draft: true,
	})
	if draftResp.status != http.StatusCreated {
		t.Fatalf("create draft = %d %s", draftResp.status, draftResp.body)
	}
	dj := draftResp.json()
	draftID := dj["id"].(string)
	if dj["draft"] != true || dj["published"] != false || len(dj["dirs"].([]any)) != 0 || dj["created"] != true {
		t.Fatalf("unexpected draft response: %v", dj)
	}
	if st := anon.do("GET", "/api/v1/items/"+draftID, nil).status; st != http.StatusForbidden {
		t.Errorf("visitor GET draft item = %d, want 403", st)
	}
	if st := admin.do("GET", "/api/v1/items/"+draftID, nil).status; st != http.StatusOK {
		t.Errorf("admin GET draft item = %d, want 200", st)
	}

	// Repeated draft update on unpublished item succeeds.
	draftUpd := admin.ingest(ingestReq{
		xml:   xmlDoc(docOpts{title: "Draft R2", md: "round2"}),
		key:   "paper:draft1",
		draft: true,
	})
	if draftUpd.status != http.StatusOK || draftUpd.json()["id"] != draftID || draftUpd.json()["published"] != false {
		t.Fatalf("update draft = %d %s", draftUpd.status, draftUpd.body)
	}

	// Draft + dirs conflicts -> 400 invalid_request.
	if resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "X"}), dirs: []string{dir}, draft: true}); resp.status != http.StatusBadRequest {
		t.Errorf("draft+dirs = %d %s, want 400", resp.status, resp.body)
	}

	// Publish draft with --dir keeps same ID and becomes visitor-visible.
	pubResp := admin.ingest(ingestReq{
		xml:  xmlDoc(docOpts{title: "Published R2", md: "round2"}),
		key:  "paper:draft1",
		dirs: []string{dir},
	})
	if pubResp.status != http.StatusOK {
		t.Fatalf("publish draft = %d %s", pubResp.status, pubResp.body)
	}
	pj := pubResp.json()
	if pj["id"] != draftID || pj["published"] != true || pj["draft"] != false {
		t.Fatalf("publish response = %v", pj)
	}
	if st := anon.do("GET", "/api/v1/items/"+draftID, nil).status; st != http.StatusOK {
		t.Errorf("visitor GET published item = %d, want 200", st)
	}

	// #42: draft against published item -> 409 already_published; content & mounts & store untouched.
	beforeXML := string(admin.do("GET", "/api/v1/items/"+draftID+"/content", nil).body)
	beforeObjCount := e.store.count("/xml/" + draftID)
	rejDraft := admin.ingest(ingestReq{
		xml:   xmlDoc(docOpts{title: "Overwrite Attempt", md: "secret"}),
		key:   "paper:draft1",
		draft: true,
	})
	if rejDraft.status != http.StatusConflict || rejDraft.json()["error"] != "already_published" {
		t.Fatalf("draft on published = %d %s, want 409 already_published", rejDraft.status, rejDraft.body)
	}
	if afterXML := string(admin.do("GET", "/api/v1/items/"+draftID+"/content", nil).body); afterXML != beforeXML {
		t.Errorf("published content changed after 409 already_published")
	}
	if e.store.count("/xml/"+draftID) != beforeObjCount {
		t.Errorf("storage objects changed after 409 already_published")
	}

	// #41: create_only conflicts & parameter checks.
	if resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "X"}), itemID: draftID, createOnly: true}); resp.status != http.StatusBadRequest {
		t.Errorf("create_only+item_id = %d %s, want 400", resp.status, resp.body)
	}
	if resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Dup"}), key: "paper:draft1", createOnly: true}); resp.status != http.StatusConflict || resp.json()["error"] != "key_exists" {
		t.Errorf("create_only existing key = %d %s, want 409 key_exists", resp.status, resp.body)
	}

	// #41 concurrent create_only race decided atomically by UNIQUE constraint.
	var readyWG, releaseWG sync.WaitGroup
	readyWG.Add(2)
	releaseWG.Add(1)
	var barrierOnce sync.Once
	e.store.onPut = func() {
		readyWG.Done()
		readyWG.Wait()
		barrierOnce.Do(func() { releaseWG.Done() })
		releaseWG.Wait()
	}
	results := make(chan response, 2)
	for i := range 2 {
		go func(idx int) {
			results <- e.anon().ingest(ingestReq{
				xml:        xmlDoc(docOpts{title: "Race", md: string(rune('A' + idx))}),
				key:        "concurrent:key",
				createOnly: true,
			})
		}(i)
	}
	r1, r2 := <-results, <-results
	e.store.onPut = nil
	if r1.status > r2.status {
		r1, r2 = r2, r1
	}
	if r1.status != http.StatusCreated || r2.status != http.StatusConflict || r2.json()["error"] != "key_exists" {
		t.Fatalf("concurrent create_only = (%d %s) and (%d %s), want 201 and 409 key_exists", r1.status, r1.body, r2.status, r2.body)
	}

	// #39a: created_by stores full SHA-256 hex digest (no plaintext) and preserves creator on update.
	tokAID := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "OwnedByA"}), key: "own:1"})
	var createdBy string
	if err := e.db.Get(&createdBy, `SELECT created_by FROM items WHERE id = ?`, tokAID); err != nil {
		t.Fatal(err)
	}
	if createdBy != auth.TokenDigest(apiToken) || strings.Contains(createdBy, apiToken) {
		t.Fatalf("created_by = %q, want SHA-256 digest %q without plaintext", createdBy, auth.TokenDigest(apiToken))
	}
	// Updating with second token ("other") preserves original created_by.
	updResp := admin.ingest(ingestReq{token: "other", xml: xmlDoc(docOpts{title: "UpdatedByOther"}), key: "own:1"})
	if updResp.status != http.StatusOK {
		t.Fatalf("update by other token = %d %s", updResp.status, updResp.body)
	}
	var createdByAfter string
	if err := e.db.Get(&createdByAfter, `SELECT created_by FROM items WHERE id = ?`, tokAID); err != nil {
		t.Fatal(err)
	}
	if createdByAfter != createdBy {
		t.Fatalf("created_by overwritten on update: %q != %q", createdByAfter, createdBy)
	}

	// #56 unit level: NULL created_by remains NULL after update via external_key.
	if _, err := e.db.Exec(`INSERT INTO items
		(id, external_key, type, title, report_date, summary, tags_json, source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text, created_by)
		VALUES ('noowner00003', 'noowner:stay', 'report', 'Ownerless', '2026-11-20', 'S', '[]', 'a', '', '[]', '/xml/l.xml', 'ownerless', NULL)`); err != nil {
		t.Fatal(err)
	}
	updOwnerless := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Ownerless Updated"}), key: "noowner:stay"})
	if updOwnerless.status != http.StatusOK {
		t.Fatalf("update ownerless = %d %s", updOwnerless.status, updOwnerless.body)
	}
	var noOwner sql.NullString
	if err := e.db.Get(&noOwner, `SELECT created_by FROM items WHERE id = 'noowner00003'`); err != nil {
		t.Fatal(err)
	}
	if noOwner.Valid {
		t.Fatalf("created_by became %q, want NULL", noOwner.String)
	}
}

func TestIngestConcurrentPublishAndDraftDoesNotDeleteLiveXML(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "Public", "")

	itemID := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Initial Draft", md: "v1"}), key: "race:draft", draft: true})
	sameXML := xmlDoc(docOpts{title: "Updated V2", md: "shared v2 payload"})

	draftInPut := make(chan struct{})
	releaseDraft := make(chan struct{})
	var paused atomic.Bool
	e.store.onPutPath = func(name string) {
		if !strings.HasPrefix(name, "/xml/"+itemID+"-") {
			return
		}
		if paused.CompareAndSwap(false, true) {
			close(draftInPut)
			<-releaseDraft
		}
	}

	draftCh := make(chan response, 1)
	go func() {
		draftCh <- admin.ingest(ingestReq{xml: sameXML, key: "race:draft", draft: true})
	}()

	<-draftInPut
	pubResp := admin.ingest(ingestReq{xml: sameXML, key: "race:draft", dirs: []string{dir}})
	if pubResp.status != http.StatusOK {
		close(releaseDraft)
		t.Fatalf("publish = %d %s", pubResp.status, pubResp.body)
	}

	close(releaseDraft)
	draftResp := <-draftCh
	e.store.onPutPath = nil
	if draftResp.status != http.StatusConflict || draftResp.json()["error"] != "already_published" {
		t.Fatalf("draft = %d %s, want 409 already_published", draftResp.status, draftResp.body)
	}

	served := admin.do("GET", "/api/v1/items/"+itemID+"/content", nil)
	if served.status != http.StatusOK || !strings.Contains(string(served.body), "shared v2 payload") {
		t.Fatalf("live item content after 409 draft = %d %s, want 200 with shared v2 payload", served.status, served.body)
	}
}

func TestCreateOnlyChecksExistingKeyBeforeStorage(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Existing"}), key: "exists:failstore"})

	e.store.mu.Lock()
	e.store.existsErr = errors.New("webdav down")
	e.store.putErr = errors.New("webdav down")
	e.store.mu.Unlock()

	resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Dup"}), key: "exists:failstore", createOnly: true})
	if resp.status != http.StatusConflict || resp.json()["error"] != "key_exists" {
		t.Fatalf("create_only with existing key while WebDAV down = %d %s, want 409 key_exists", resp.status, resp.body)
	}
}

func TestIngestExternalKeyRulesApplyToUpdates(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()

	id := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Keyed"}), key: "keyed:1"})
	for name, key := range map[string]string{
		"too long":     strings.Repeat("w", 201),
		"control char": "bad\x07key",
	} {
		resp := admin.ingest(ingestReq{xml: xmlDoc(docOpts{title: "Rekeyed"}), itemID: id, key: key})
		if resp.status != http.StatusBadRequest || resp.json()["error"] != "invalid_request" {
			t.Errorf("update with %s key = %d %s, want 400 invalid_request", name, resp.status, resp.body)
		}
	}
}
