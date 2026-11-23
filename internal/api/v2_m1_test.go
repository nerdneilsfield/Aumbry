package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/abema/go-mp4"
)

func buildTestWAV(secs int) []byte {
	const sampleRate = 8000
	const channels = 1
	const bitsPerSample = 16
	byteRate := sampleRate * channels * (bitsPerSample / 8)
	dataLen := secs * byteRate
	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataLen))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels*(bitsPerSample/8)))
	_ = binary.Write(buf, binary.LittleEndian, uint16(bitsPerSample))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataLen))
	buf.Write(make([]byte, dataLen))
	return buf.Bytes()
}

func buildTestMP3(frames int) []byte {
	const frameSize = 417
	frame := make([]byte, frameSize)
	frame[0] = 0xFF
	frame[1] = 0xFB
	frame[2] = 0x90
	frame[3] = 0xC0
	buf := &bytes.Buffer{}
	buf.Write([]byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 10})
	buf.Write(make([]byte, 10))
	for range frames {
		buf.Write(frame)
	}
	return buf.Bytes()
}

func buildTestMP4(t *testing.T, handlerType [4]byte, timescale, duration uint32) []byte {
	t.Helper()
	p := filepath.Join(t.TempDir(), "track.m4a")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	w := mp4.NewWriter(f)
	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeFtyp()})
	_, _ = mp4.Marshal(w, &mp4.Ftyp{
		MajorBrand: [4]byte{'M', '4', 'A', ' '},
		CompatibleBrands: []mp4.CompatibleBrandElem{
			{CompatibleBrand: [4]byte{'M', '4', 'A', ' '}},
			{CompatibleBrand: [4]byte{'i', 's', 'o', 'm'}},
		},
	}, mp4.Context{})
	_, _ = w.EndBox()

	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMoov()})
	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMvhd()})
	_, _ = mp4.Marshal(w, &mp4.Mvhd{Timescale: timescale, DurationV0: duration, Rate: 0x00010000, Volume: 0x0100, NextTrackID: 2}, mp4.Context{})
	_, _ = w.EndBox()
	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeTrak()})
	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMdia()})
	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMdhd()})
	_, _ = mp4.Marshal(w, &mp4.Mdhd{Timescale: timescale, DurationV0: duration}, mp4.Context{})
	_, _ = w.EndBox()
	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeHdlr()})
	_, _ = mp4.Marshal(w, &mp4.Hdlr{HandlerType: handlerType, Name: "Handler"}, mp4.Context{})
	_, _ = w.EndBox()
	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMinf()})
	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeStbl()})
	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeStsd()})
	stsdBody := make([]byte, 4+4+36)
	binary.BigEndian.PutUint32(stsdBody[4:8], 1)
	binary.BigEndian.PutUint32(stsdBody[8:12], 36)
	copy(stsdBody[12:16], []byte("mp4a"))
	_, _ = w.Write(stsdBody)
	_, _ = w.EndBox()
	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeStsz()})
	stszBody := make([]byte, 16)
	binary.BigEndian.PutUint32(stszBody[8:12], 1)
	binary.BigEndian.PutUint32(stszBody[12:16], 7)
	_, _ = w.Write(stszBody)
	_, _ = w.EndBox()
	_, _ = w.EndBox()
	_, _ = w.EndBox()
	_, _ = w.EndBox()
	_, _ = w.EndBox()
	_, _ = w.EndBox()
	_, _ = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMdat()})
	_, _ = w.Write([]byte("payload"))
	_, _ = w.EndBox()
	_, _ = f.Seek(0, io.SeekStart)
	raw, _ := io.ReadAll(f)
	_ = f.Close()
	return raw
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func xmlDoc12(title, date, summary, body string, tags ...string) string {
	if date == "" {
		date = "2026-11-23"
	}
	if summary == "" {
		summary = "summary of " + title
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><document version="1.2"><meta>`)
	fmt.Fprintf(&b, "<title>%s</title><type>report</type><date>%s</date><summary>%s</summary><source-agent>v2-agent</source-agent>", title, date, summary)
	if len(tags) > 0 {
		b.WriteString("<tags>")
		for _, tg := range tags {
			fmt.Fprintf(&b, "<tag>%s</tag>", tg)
		}
		b.WriteString("</tags>")
	}
	b.WriteString("</meta><body>")
	b.WriteString(body)
	b.WriteString("</body></document>")
	return b.String()
}

type syncedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncedBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncedBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestV2BacklinksReferencedByAndPermissions(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	anon := e.anon()

	pubDir := admin.createDir("", "Public", "")
	privDir := admin.createDir("", "Secret", "pw-secret")

	detailB := admin.ingestOK(ingestReq{
		xml:  xmlDoc12("Target Detail B", "2026-11-20", "", `<md>Target B</md>`),
		dirs: []string{pubDir},
	})
	digestPub := admin.ingestOK(ingestReq{
		key:  "digest:pub",
		xml:  xmlDoc12("Public Digest", "2026-11-22", "", fmt.Sprintf(`<news-item title="N" detail-id="%s">Sum</news-item><paper-card title="P" url="https://example.org" related-id="%s">Rel</paper-card>`, detailB, detailB)),
		dirs: []string{pubDir},
	})
	digestPriv := admin.ingestOK(ingestReq{
		key:  "digest:priv",
		xml:  xmlDoc12("Secret Briefing", "2026-11-23", "Sensitive brief summary", fmt.Sprintf(`<news-item title="S" detail-id="%s">Secret</news-item>`, detailB)),
		dirs: []string{privDir},
	})

	// Anonymous viewer sees only Public Digest with merged link_kinds, and ZERO leak of Secret Briefing.
	anonRes := anon.do("GET", "/api/v1/items/"+detailB, nil)
	if anonRes.status != http.StatusOK {
		t.Fatalf("GET item = %d %s", anonRes.status, anonRes.body)
	}
	if strings.Contains(string(anonRes.body), digestPriv) || strings.Contains(string(anonRes.body), "Secret Briefing") || strings.Contains(string(anonRes.body), "Sensitive brief summary") {
		t.Fatalf("locked digest leaked in referenced_by: %s", anonRes.body)
	}
	refs := anonRes.json()["referenced_by"].([]any)
	if len(refs) != 1 {
		t.Fatalf("anon referenced_by len = %d, want 1 (%s)", len(refs), anonRes.body)
	}
	ref0 := refs[0].(map[string]any)
	if ref0["id"] != digestPub || len(ref0["link_kinds"].([]any)) != 2 {
		t.Fatalf("unexpected ref0 = %+v", ref0)
	}

	// Unlocked visitor sees Secret Briefing first (2026-11-23 > 2026-11-22), then Public Digest.
	visitor := e.anon()
	if uResp := visitor.do("POST", "/api/v1/dirs/"+privDir+"/unlock", map[string]string{"password": "pw-secret"}); uResp.status != http.StatusOK {
		t.Fatalf("unlock failed: %d %s", uResp.status, uResp.body)
	}
	visRefs := visitor.do("GET", "/api/v1/items/"+detailB, nil).json()["referenced_by"].([]any)
	if len(visRefs) != 2 || visRefs[0].(map[string]any)["id"] != digestPriv || visRefs[1].(map[string]any)["id"] != digestPub {
		t.Fatalf("unlocked referenced_by = %+v", visRefs)
	}

	// Editing digestPub to drop reference removes it from referenced_by; unpublishing digestPriv hides it.
	admin.ingestOK(ingestReq{key: "digest:pub", xml: xmlDoc12("Public Digest", "2026-11-22", "", `<md>No link now</md>`)})
	if resp := e.agentDo(http.MethodDelete, "/api/v1/agent/items/"+digestPriv+"/mounts", apiToken); resp.status != http.StatusNoContent {
		t.Fatalf("unpublish digestPriv = %d %s", resp.status, resp.body)
	}
	afterRefs := visitor.do("GET", "/api/v1/items/"+detailB, nil).json()["referenced_by"].([]any)
	if len(afterRefs) != 0 {
		t.Fatalf("expected 0 referenced_by after unpublish & link removal, got %+v", afterRefs)
	}
}

func TestV2RevisionsHistoryB1PrivacyAndAdminDelete(t *testing.T) {
	maxRevs := 3
	e := newEnvWithOptions(t, RouterOptions{MaxRevisionsPerItem: &maxRevs}, nil)
	admin := e.admin()
	anon := e.anon()

	pubDir := admin.createDir("", "Pub", "")
	privDir := admin.createDir("", "Priv", "pw-priv")

	privImg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"><rect width="4" height="4" fill="#f00"/></svg>`)
	privImgHash := sha256Hex(privImg)

	// 1. Draft updates create 0 revisions.
	admin.ingestOK(ingestReq{key: "hist:item", draft: true, xml: xmlDoc12("Draft 1", "", "", `<md>d1</md>`)})
	id := admin.ingestOK(ingestReq{key: "hist:item", draft: true, xml: xmlDoc12("Draft 2", "", "", `<md>d2</md>`)})
	if got := admin.do("GET", "/api/v1/items/"+id, nil).json()["revision_count"].(float64); got != 0 {
		t.Fatalf("draft revision_count = %v, want 0", got)
	}

	// 2. First publish in privDir with private image creates rev=1 (mounts=[privDir]).
	v1XML := xmlDoc12("Secret V1", "2026-11-21", "Private V1 summary", `<md>Secret V1 body</md><figure src="./secret.svg"/>`)
	admin.ingestOK(ingestReq{
		key: "hist:item", xml: v1XML, dirs: []string{privDir},
		assets: []figure{{src: "secret.svg", name: "secret.svg", data: privImg}},
	})
	// Idempotent re-push of exact same content must NOT create rev=2.
	admin.ingestOK(ingestReq{
		key: "hist:item", xml: v1XML, dirs: []string{privDir},
		assets: []figure{{src: "secret.svg", name: "secret.svg", data: privImg}},
	})
	if revResp := admin.do("GET", "/api/v1/items/"+id+"/revisions", nil).json(); len(revResp["revisions"].([]any)) != 1 {
		t.Fatalf("expected 1 rev after idempotent push, got %+v", revResp)
	}

	// 3. Path: unpublish -> --draft overwrite (sanitized V2) -> admin PUT /mounts to pubDir -> creates rev=2!
	if resp := e.agentDo(http.MethodDelete, "/api/v1/agent/items/"+id+"/mounts", apiToken); resp.status != http.StatusNoContent {
		t.Fatalf("unpublish = %d %s", resp.status, resp.body)
	}
	v2XML := xmlDoc12("Public V2", "2026-11-22", "Clean summary", `<md>Clean V2 body</md>`)
	admin.ingestOK(ingestReq{key: "hist:item", draft: true, xml: v2XML})
	if putResp := admin.do("PUT", "/api/v1/admin/items/"+id+"/mounts", map[string]any{"dirs": []string{pubDir}}); putResp.status != http.StatusNoContent {
		t.Fatalf("remount = %d %s", putResp.status, putResp.body)
	}

	// B1 Check: anonymous reads V2 (current_rev=2), listRevisions hides V1, rev/1/content -> 403, secret.svg -> 403!
	itemAnon := anon.do("GET", "/api/v1/items/"+id, nil)
	if itemAnon.status != http.StatusOK || itemAnon.json()["current_rev"].(float64) != 2 || itemAnon.json()["revision_count"].(float64) != 1 {
		t.Fatalf("anon item = %d %s", itemAnon.status, itemAnon.body)
	}
	anonRevs := anon.do("GET", "/api/v1/items/"+id+"/revisions", nil).json()["revisions"].([]any)
	if len(anonRevs) != 1 || anonRevs[0].(map[string]any)["rev"].(float64) != 2 {
		t.Fatalf("anon revisions leaked rev 1: %+v", anonRevs)
	}
	if st := anon.do("GET", "/api/v1/items/"+id+"/revisions/1/content", nil).status; st != http.StatusForbidden {
		t.Fatalf("anon rev 1 content = %d, want 403", st)
	}
	if st := anon.do("GET", "/api/v1/assets/"+privImgHash, nil).status; st != http.StatusForbidden {
		t.Fatalf("anon historical private image = %d, want 403", st)
	}
	if resp := anon.do("GET", "/api/v1/items/"+id+"/revisions/2/content", nil); resp.status != http.StatusOK || !strings.Contains(string(resp.body), "Clean V2 body") {
		t.Fatalf("anon rev 2 content = %d %s", resp.status, resp.body)
	}

	// Unlocked visitor who has privDir unlocked CAN see rev=1 and secret.svg!
	vis := e.anon()
	if st := vis.do("POST", "/api/v1/dirs/"+privDir+"/unlock", map[string]string{"password": "pw-priv"}).status; st != http.StatusOK {
		t.Fatalf("unlock = %d", st)
	}
	if len(vis.do("GET", "/api/v1/items/"+id+"/revisions", nil).json()["revisions"].([]any)) != 2 {
		t.Fatal("unlocked visitor expected 2 visible revisions")
	}
	if st := vis.do("GET", "/api/v1/items/"+id+"/revisions/1/content", nil).status; st != http.StatusOK {
		t.Fatalf("unlocked rev 1 content = %d", st)
	}
	if st := vis.do("GET", "/api/v1/assets/"+privImgHash, nil).status; st != http.StatusOK {
		t.Fatalf("unlocked rev 1 image = %d", st)
	}

	// Unpublishing current item blocks reading rev=2 too (B1 Gate 1).
	_ = admin.do("PUT", "/api/v1/admin/items/"+id+"/mounts", map[string]any{"dirs": []string{}})
	if st := anon.do("GET", "/api/v1/items/"+id+"/revisions/2/content", nil).status; st != http.StatusForbidden {
		t.Fatalf("unpublished item rev 2 = %d, want 403", st)
	}
	if st := vis.do("GET", "/api/v1/assets/"+privImgHash, nil).status; st != http.StatusForbidden {
		t.Fatalf("unpublished item historical asset = %d, want 403", st)
	}
	_ = admin.do("PUT", "/api/v1/admin/items/"+id+"/mounts", map[string]any{"dirs": []string{pubDir}})

	// Deleting current rev=2 -> 409; deleting rev=1 -> 204!
	if st := admin.do("DELETE", "/api/v1/admin/items/"+id+"/revisions/2", nil).status; st != http.StatusConflict {
		t.Fatalf("delete current rev = %d, want 409", st)
	}
	if st := admin.do("DELETE", "/api/v1/admin/items/"+id+"/revisions/1", nil).status; st != http.StatusNoContent {
		t.Fatalf("delete old rev 1 = %d, want 204", st)
	}
	if st := admin.do("GET", "/api/v1/items/"+id+"/revisions/1/content", nil).status; st != http.StatusNotFound {
		t.Fatalf("deleted rev 1 = %d, want 404", st)
	}

	// Push updates to trigger FIFO pruning (max_per_item = 3).
	for i := 3; i <= 5; i++ {
		admin.ingestOK(ingestReq{key: "hist:item", xml: xmlDoc12(fmt.Sprintf("V%d", i), "", "", fmt.Sprintf(`<md>body-%d</md>`, i))})
	}
	allRevs := admin.do("GET", "/api/v1/items/"+id+"/revisions", nil).json()["revisions"].([]any)
	if len(allRevs) != 3 || allRevs[0].(map[string]any)["rev"].(float64) != 5 || allRevs[2].(map[string]any)["rev"].(float64) != 3 {
		t.Fatalf("after FIFO pruning, revisions = %+v", allRevs)
	}
}

func TestV2PreMigrationZeroRevItemPreservesOldRevisionOnUpdate(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "Open", "")

	// Seed a legacy pre-v0.1.4 published item with 0 item_revisions rows.
	legacyXML := xmlDoc12("Legacy V1", "2026-11-18", "Old Sum", `<md>Legacy body</md>`)
	legacyPath := "/xml/legacyitem01-" + sha256Hex([]byte(legacyXML)) + ".xml"
	_ = e.store.Put(t.Context(), legacyPath, strings.NewReader(legacyXML))
	if _, err := e.db.Exec(`INSERT INTO items
		(id, external_key, type, title, report_date, summary, tags_json, source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text)
		VALUES ('legacyitem01', 'legacy:0rev', 'report', 'Legacy V1', '2026-11-18', 'Old Sum', '[]', 'a', '', '[]', ?, 'Legacy body')`, legacyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`INSERT INTO directory_items (directory_id, item_id) VALUES (?, 'legacyitem01')`, dir); err != nil {
		t.Fatal(err)
	}

	// Push an update directly before running reindex: rev=1 must snapshot Legacy V1 and rev=2 the new version.
	admin.ingestOK(ingestReq{key: "legacy:0rev", xml: xmlDoc12("New V2", "2026-11-23", "New Sum", `<md>New body</md>`)})
	revs := admin.do("GET", "/api/v1/items/legacyitem01/revisions", nil).json()["revisions"].([]any)
	if len(revs) != 2 || revs[0].(map[string]any)["title"] != "New V2" || revs[1].(map[string]any)["title"] != "Legacy V1" {
		t.Fatalf("expected rev 1 preserved as Legacy V1 and rev 2 New V2, got %+v", revs)
	}
}

func TestV2AudioIngestStreamingRangeAndValidation(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	anon := e.anon()
	dir := admin.createDir("", "AudioDir", "pw-audio")

	wavData := buildTestWAV(2)
	mp3Data := buildTestMP3(40)
	m4aData := buildTestMP4(t, [4]byte{'s', 'o', 'u', 'n'}, 44100, 132300)
	vidOnly := buildTestMP4(t, [4]byte{'v', 'i', 'd', 'e'}, 90000, 270000)

	// Reject video-only mp4 renamed to .m4a.
	badResp := admin.ingest(ingestReq{
		xml:    xmlDoc12("Fake M4A", "", "", `<audio src="./fake.m4a" title="Fake"/>`),
		dirs:   []string{dir},
		assets: []figure{{src: "fake.m4a", name: "fake.m4a", data: vidOnly}},
	})
	if badResp.status != http.StatusUnprocessableEntity {
		t.Fatalf("video-only .m4a = %d %s, want 422", badResp.status, badResp.body)
	}

	// Reject corrupt wav.
	badWav := admin.ingest(ingestReq{
		xml:    xmlDoc12("Bad WAV", "", "", `<audio src="./bad.wav" title="Bad"/>`),
		dirs:   []string{dir},
		assets: []figure{{src: "bad.wav", name: "bad.wav", data: []byte("RIFF\x20\x00\x00\x00WAVE")}},
	})
	if badWav.status != http.StatusUnprocessableEntity {
		t.Fatalf("corrupt .wav = %d %s, want 422", badWav.status, badWav.body)
	}

	// Push valid mp3, wav, and m4a item; first audio (wav) becomes primary_audio_hash.
	doc := xmlDoc12("Audio Multi", "2026-11-23", "Audio summary",
		`<section id="sec1" title="Topic"><md>Notes</md></section>`+
			`<audio id="ep1" src="./unindexedname.wav" title="Episode WAV">`+
			`<chapter start="00:00:00" title="ChapIntro" target="#s-sec1"/>`+
			`<chapter start="00:00:00" title="ChapSameSecond"/>`+
			`<transcript><![CDATA[TranscriptSecretPhrase]]></transcript>`+
			`</audio>`+
			`<audio src="./ep2.mp3" title="Episode MP3"/>`+
			`<audio src="./ep3.m4a" title="Episode M4A"/>`)
	itemID := admin.ingestOK(ingestReq{
		xml:  doc,
		dirs: []string{dir},
		assets: []figure{
			{src: "unindexedname.wav", name: "unindexedname.wav", data: wavData},
			{src: "ep2.mp3", name: "ep2.mp3", data: mp3Data},
			{src: "ep3.m4a", name: "ep3.m4a", data: m4aData},
		},
	})

	wavHash := sha256Hex(wavData)
	mp3Hash := sha256Hex(mp3Data)
	m4aHash := sha256Hex(m4aData)

	// Verify DB durations & primary_audio projection.
	for h, wantDur := range map[string]int{wavHash: 2, mp3Hash: 1, m4aHash: 3} {
		var dur int
		if err := e.db.Get(&dur, `SELECT duration_seconds FROM assets WHERE hash = ?`, h); err != nil || dur != wantDur {
			t.Fatalf("asset %s duration = (%d, %v), want %d", h, dur, err, wantDur)
		}
	}
	var pHash, pCh string
	if err := e.db.Get(&pHash, `SELECT primary_audio_hash FROM items WHERE id = ?`, itemID); err != nil || pHash != wavHash {
		t.Fatalf("primary_audio_hash = (%q, %v), want %q", pHash, err, wavHash)
	}
	if err := e.db.Get(&pCh, `SELECT primary_audio_chapters_json FROM items WHERE id = ?`, itemID); err != nil || !strings.Contains(pCh, "ChapIntro") {
		t.Fatalf("chapters json = (%q, %v)", pCh, err)
	}

	// Transcript is searchable; audio filename ("unindexedname") is NOT indexed.
	if res := admin.do("GET", "/api/v1/admin/search?q=TranscriptSecretPhrase", nil).json()["results"].([]any); len(res) != 1 {
		t.Fatalf("transcript search len = %d, want 1", len(res))
	}
	if res := admin.do("GET", "/api/v1/admin/search?q=unindexedname", nil).json()["results"].([]any); len(res) != 0 {
		t.Fatalf("audio filename matched search unexpectedly: %+v", res)
	}

	// Range / HEAD tests on unlocked visitor.
	vis := e.anon()
	_ = vis.do("POST", "/api/v1/dirs/"+dir+"/unlock", map[string]string{"password": "pw-audio"})
	assetURL := "/api/v1/assets/" + wavHash

	// Unauthenticated still 403.
	if st := anon.do("GET", assetURL, nil).status; st != http.StatusForbidden {
		t.Fatalf("anon audio GET = %d, want 403", st)
	}

	// HEAD -> 200 with Content-Length & Accept-Ranges: bytes and empty body.
	headReq, _ := http.NewRequest(http.MethodHead, e.srv.URL+assetURL, nil)
	headResp := vis.send(headReq)
	if headResp.status != http.StatusOK || len(headResp.body) != 0 ||
		headResp.header.Get("Accept-Ranges") != "bytes" ||
		headResp.header.Get("Content-Length") != fmt.Sprintf("%d", len(wavData)) {
		t.Fatalf("HEAD = %d len=%d headers=%+v", headResp.status, len(headResp.body), headResp.header)
	}

	// Range prefix bytes=0-1023 -> 206 Partial Content.
	r1, _ := http.NewRequest(http.MethodGet, e.srv.URL+assetURL, nil)
	r1.Header.Set("Range", "bytes=0-1023")
	res1 := vis.send(r1)
	if res1.status != http.StatusPartialContent || !bytes.Equal(res1.body, wavData[:1024]) ||
		res1.header.Get("Content-Range") != fmt.Sprintf("bytes 0-1023/%d", len(wavData)) {
		t.Fatalf("prefix range = %d len=%d content-range=%q", res1.status, len(res1.body), res1.header.Get("Content-Range"))
	}

	// Suffix range bytes=-512 -> 206.
	r2, _ := http.NewRequest(http.MethodGet, e.srv.URL+assetURL, nil)
	r2.Header.Set("Range", "bytes=-512")
	res2 := vis.send(r2)
	if res2.status != http.StatusPartialContent || !bytes.Equal(res2.body, wavData[len(wavData)-512:]) {
		t.Fatalf("suffix range = %d len=%d", res2.status, len(res2.body))
	}

	// Out-of-bounds range -> 416.
	r3, _ := http.NewRequest(http.MethodGet, e.srv.URL+assetURL, nil)
	r3.Header.Set("Range", fmt.Sprintf("bytes=%d-", len(wavData)+100))
	if res3 := vis.send(r3); res3.status != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("OOB range = %d, want 416", res3.status)
	}

	// Multi-range bytes=0-3, 10-15 -> falls back to full 200 OK stream in 1 OpenRange call (no multipart/byteranges amplification).
	callsBefore := e.store.openRangeCalls.Load()
	r4, _ := http.NewRequest(http.MethodGet, e.srv.URL+assetURL, nil)
	r4.Header.Set("Range", "bytes=0-3, 10-15")
	res4 := vis.send(r4)
	if res4.status != http.StatusOK || strings.Contains(res4.header.Get("Content-Type"), "multipart/byteranges") ||
		!bytes.Equal(res4.body, wavData) || e.store.openRangeCalls.Load()-callsBefore != 1 {
		t.Fatalf("multi-range fallback = %d type=%q len=%d want len=%d openRange=%d", res4.status, res4.header.Get("Content-Type"), len(res4.body), len(wavData), e.store.openRangeCalls.Load()-callsBefore)
	}

	// Full GET -> 200 + full body + Content-Length.
	full := vis.do("GET", assetURL, nil)
	if full.status != http.StatusOK || !bytes.Equal(full.body, wavData) ||
		full.header.Get("Content-Length") != fmt.Sprintf("%d", len(wavData)) {
		t.Fatalf("full GET = %d len=%d want %d", full.status, len(full.body), len(wavData))
	}
}

func TestV2LargeMultipartSpillsToDiskAndSucceeds(t *testing.T) {
	// Test >32 MiB upload (exceeds formMemory = 32 MiB) using a ~33 MiB valid WAV streamed over io.Pipe.
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "Big", "")

	// 8000 Hz * 2 bytes/sec = 16000 B/s; 2100 seconds = 33,600,044 bytes (~32.04 MiB).
	wavData := buildTestWAV(2100)
	wavHash := sha256Hex(wavData)

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		xmlPart, _ := mw.CreateFormFile("xml", "big.xml")
		_, _ = xmlPart.Write([]byte(xmlDoc12("Big Audio", "", "", `<audio src="./big.wav" title="Long Ep"/>`)))
		_ = mw.WriteField("asset_srcs", "big.wav")
		_ = mw.WriteField("dirs", dir)
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="assets[]"; filename="big.wav"`)
		p, _ := mw.CreatePart(h)
		_, _ = p.Write(wavData)
		_ = pw.CloseWithError(mw.Close())
	}()

	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/api/v1/ingest", pr)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("large upload = %d %s, want 201", resp.StatusCode, body)
	}
	if _, ok := e.store.files["/assets/"+wavHash+".wav"]; !ok {
		t.Fatalf("large wav not stored in object store")
	}
}

type rssParseDoc struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel struct {
		Title          string `xml:"title"`
		Link           string `xml:"link"`
		Language       string `xml:"language"`
		ItunesExplicit string `xml:"explicit"`
		Items          []struct {
			Title       string   `xml:"title"`
			Link        string   `xml:"link"`
			GUID        string   `xml:"guid"`
			PubDate     string   `xml:"pubDate"`
			Creator     string   `xml:"creator"`
			Categories  []string `xml:"category"`
			Description string   `xml:"description"`
			Enclosure   *struct {
				URL    string `xml:"url,attr"`
				Length int64  `xml:"length,attr"`
				Type   string `xml:"type,attr"`
			} `xml:"enclosure"`
			Duration string `xml:"duration"`
		} `xml:"item"`
	} `xml:"channel"`
}

func TestV2FeedsRSSPodcastDirTokensA1AndLogRedaction(t *testing.T) {
	var logBuf syncedBuffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	e := newEnvWithOptions(t, RouterOptions{PublicURL: "https://hub.example.org"}, logger)
	admin := e.admin()
	anon := e.anon()

	pubParent := admin.createDir("", "OpenRoot", "")
	pubChild := admin.createDir(pubParent, "OpenChild", "")
	protD := admin.createDir("", "TeamD", "pw-d")
	protDU := admin.createDir(protD, "SubOpen", "")
	protC := admin.createDir(protD, "LockedChildC", "pw-c")

	wav := buildTestWAV(2)
	wavHash := sha256Hex(wav)
	nullDurWav := buildTestWAV(4)
	nullDurWavHash := sha256Hex(nullDurWav)
	cAudio := buildTestWAV(1)
	cAudioHash := sha256Hex(cAudio)

	// Public items: one mounted in both pubParent and pubChild (must dedup via DISTINCT),
	// one audio item with NULL duration_seconds to verify <itunes:duration> omission, and one text-only item.
	textID := admin.ingestOK(ingestReq{
		xml:  xmlDoc12("R&amp;D Briefing", "2026-11-22", "A &amp; B Summary", `<md>Text only</md>`, "ai", "ml"),
		dirs: []string{pubParent, pubChild},
	})
	nullDurAudioID := admin.ingestOK(ingestReq{
		xml:    xmlDoc12("Podcast NullDur", "2026-11-23", "Listen &amp; Learn", `<audio src="./ep.wav" title="Ep"><chapter start="00:00:00" title="Part A &amp; B"/></audio>`),
		dirs:   []string{pubParent},
		assets: []figure{{src: "ep.wav", name: "ep.wav", data: nullDurWav}},
	})
	if _, err := e.db.Exec(`UPDATE assets SET duration_seconds = NULL WHERE hash = ?`, nullDurWavHash); err != nil {
		t.Fatal(err)
	}

	// Protected tree D + C items.
	dID := admin.ingestOK(ingestReq{
		xml:    xmlDoc12("D Episode", "2026-11-23", "D summary", `<audio src="./d.wav" title="D"/>`),
		dirs:   []string{protDU},
		assets: []figure{{src: "d.wav", name: "d.wav", data: wav}},
	})
	cID := admin.ingestOK(ingestReq{
		xml:    xmlDoc12("Secret C Only", "2026-11-23", "C secret", `<audio src="./c.wav" title="C"/>`),
		dirs:   []string{protC},
		assets: []figure{{src: "c.wav", name: "c.wav", data: cAudio}},
	})
	_ = dID

	// 1. Public vs protected directory metadata flag (`protected`) & Public RSS/Podcast checks.
	if pubDir := anon.do("GET", "/api/v1/dirs/"+pubParent, nil); pubDir.status != http.StatusOK || pubDir.json()["protected"] != false {
		t.Fatalf("public dir protected = %#v (status %d), want false", pubDir.json()["protected"], pubDir.status)
	}
	if protDir := admin.do("GET", "/api/v1/dirs/"+protD, nil); protDir.status != http.StatusOK || protDir.json()["protected"] != true {
		t.Fatalf("protected dir protected = %#v (status %d), want true", protDir.json()["protected"], protDir.status)
	}
	if protSubDir := admin.do("GET", "/api/v1/dirs/"+protDU, nil); protSubDir.status != http.StatusOK || protSubDir.json()["protected"] != true {
		t.Fatalf("protected child dir protected = %#v (status %d), want true", protSubDir.json()["protected"], protSubDir.status)
	}

	rssResp := anon.do("GET", "/feeds/"+pubParent+"/rss.xml", nil)
	if rssResp.status != http.StatusOK || !strings.Contains(rssResp.header.Get("Content-Type"), "application/rss+xml") {
		t.Fatalf("public rss = %d %s", rssResp.status, rssResp.body)
	}
	if !bytes.Contains(rssResp.body, []byte("R&amp;D Briefing")) {
		t.Fatalf("rss missing XML-escaped title R&amp;D Briefing: %s", rssResp.body)
	}
	var parsedRSS rssParseDoc
	if err := xml.Unmarshal(rssResp.body, &parsedRSS); err != nil {
		t.Fatalf("unmarshal rss: %v\n%s", err, rssResp.body)
	}
	if len(parsedRSS.Channel.Items) != 2 || parsedRSS.Channel.Link != "https://hub.example.org/d/"+pubParent {
		t.Fatalf("unexpected rss channel: %+v", parsedRSS.Channel)
	}
	if parsedRSS.Channel.Items[0].GUID != "aumbry:item:"+nullDurAudioID || parsedRSS.Channel.Items[1].GUID != "aumbry:item:"+textID {
		t.Fatalf("rss item GUIDs = %+v", parsedRSS.Channel.Items)
	}
	if parsedRSS.Channel.Items[1].Creator != "v2-agent" || len(parsedRSS.Channel.Items[1].Categories) != 2 {
		t.Fatalf("rss creator/categories = %+v", parsedRSS.Channel.Items[1])
	}
	if parsedRSS.Channel.Items[0].Enclosure == nil || parsedRSS.Channel.Items[0].Enclosure.URL != "https://hub.example.org/api/v1/assets/"+nullDurWavHash ||
		parsedRSS.Channel.Items[0].Enclosure.Length != int64(len(nullDurWav)) {
		t.Fatalf("public enclosure = %+v", parsedRSS.Channel.Items[0].Enclosure)
	}

	podResp := anon.do("GET", "/feeds/"+pubParent+"/podcast.xml", nil)
	var parsedPod rssParseDoc
	if err := xml.Unmarshal(podResp.body, &parsedPod); err != nil {
		t.Fatalf("unmarshal podcast: %v\n%s", err, podResp.body)
	}
	if len(parsedPod.Channel.Items) != 1 || parsedPod.Channel.Items[0].Duration != "" ||
		!strings.Contains(parsedPod.Channel.Items[0].Description, "00:00:00 — Part A & B") {
		t.Fatalf("unexpected podcast channel: %+v", parsedPod.Channel)
	}

	// 2. Protected feeds reject anonymous AND admin/visitor cookies (Simplification #2).
	if st := anon.do("GET", "/feeds/"+protD+"/rss.xml", nil).status; st != http.StatusUnauthorized {
		t.Fatalf("anon protected feed = %d, want 401", st)
	}
	if st := admin.do("GET", "/feeds/"+protD+"/rss.xml", nil).status; st != http.StatusUnauthorized {
		t.Fatalf("admin cookie protected feed = %d, want 401 (no cookie leakage)", st)
	}

	// 3. Issue tokens on D and C -> A1 boundary checks.
	tokDResp := admin.do("POST", "/api/v1/admin/dirs/"+protD+"/tokens", map[string]string{"label": "reader-d"})
	if tokDResp.status != http.StatusCreated || tokDResp.header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("create token D = %d cc=%q %s", tokDResp.status, tokDResp.header.Get("Cache-Control"), tokDResp.body)
	}
	tokDID := tokDResp.json()["id"].(string)
	tokD := tokDResp.json()["token"].(string)

	tokCResp := admin.do("POST", "/api/v1/admin/dirs/"+protC+"/tokens", map[string]string{"label": "reader-c"})
	tokC := tokCResp.json()["token"].(string)

	// Token D accesses D feed & SubOpen item, does NOT contain Secret C Only, and sets private, no-store.
	dFeed := anon.do("GET", "/feeds/"+protD+"/podcast.xml?token="+tokD, nil)
	if dFeed.status != http.StatusOK || dFeed.header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("D feed = %d cc=%q %s", dFeed.status, dFeed.header.Get("Cache-Control"), dFeed.body)
	}
	if strings.Contains(string(dFeed.body), cID) || strings.Contains(string(dFeed.body), "Secret C Only") {
		t.Fatalf("Token D feed penetrated locked child C: %s", dFeed.body)
	}
	wantEnc := fmt.Sprintf("https://hub.example.org/feeds/%s/assets/%s?token=%s", protD, wavHash, tokD)
	if !strings.Contains(string(dFeed.body), wantEnc) {
		t.Fatalf("D feed enclosure missing expected proxy URL %q in:\n%s", wantEnc, dFeed.body)
	}

	// Token D accesses D's audio asset (200 & Range 206 + no-store), but CANNOT access C's feed or C's exclusive asset!
	dAsset := anon.do("GET", "/feeds/"+protD+"/assets/"+wavHash+"?token="+tokD, nil)
	if dAsset.status != http.StatusOK || dAsset.header.Get("Cache-Control") != "private, no-store" || !bytes.Equal(dAsset.body, wav) {
		t.Fatalf("D feed asset = %d cc=%q len=%d", dAsset.status, dAsset.header.Get("Cache-Control"), len(dAsset.body))
	}
	rangeReq, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/feeds/"+protD+"/assets/"+wavHash+"?token="+tokD, nil)
	rangeReq.Header.Set("Range", "bytes=0-15")
	if rRes := anon.send(rangeReq); rRes.status != http.StatusPartialContent || !bytes.Equal(rRes.body, wav[:16]) {
		t.Fatalf("D feed asset range = %d len=%d", rRes.status, len(rRes.body))
	}
	if st := anon.do("GET", "/feeds/"+protC+"/rss.xml?token="+tokD, nil).status; st != http.StatusUnauthorized {
		t.Fatalf("Token D on dir C = %d, want 401", st)
	}
	if st := anon.do("GET", "/feeds/"+protD+"/assets/"+cAudioHash+"?token="+tokD, nil).status; st != http.StatusForbidden {
		t.Fatalf("Token D on C-exclusive asset = %d, want 403", st)
	}

	// Token C reads C, rejected on D.
	if cFeed := anon.do("GET", "/feeds/"+protC+"/rss.xml?token="+tokC, nil); cFeed.status != http.StatusOK || !strings.Contains(string(cFeed.body), cID) {
		t.Fatalf("Token C on C = %d %s", cFeed.status, cFeed.body)
	}
	if st := anon.do("GET", "/feeds/"+protD+"/rss.xml?token="+tokC, nil).status; st != http.StatusUnauthorized {
		t.Fatalf("Token C on D = %d, want 401", st)
	}

	// Changing D's visitor password does NOT revoke Token D (A1 decoupled credentials).
	_ = admin.do("PATCH", "/api/v1/admin/dirs/"+protD, map[string]any{"password": "new-password"})
	if st := anon.do("GET", "/feeds/"+protD+"/rss.xml?token="+tokD, nil).status; st != http.StatusOK {
		t.Fatalf("Token D invalidated by directory password change = %d, want 200", st)
	}

	// Rotate Token D -> old tokD immediately 401, new token succeeds.
	rotResp := admin.do("POST", "/api/v1/admin/dirs/"+protD+"/tokens/"+tokDID+"/rotate", nil)
	if rotResp.status != http.StatusOK || rotResp.header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("rotate = %d cc=%q %s", rotResp.status, rotResp.header.Get("Cache-Control"), rotResp.body)
	}
	newTokD := rotResp.json()["token"].(string)
	if st := anon.do("GET", "/feeds/"+protD+"/rss.xml?token="+tokD, nil).status; st != http.StatusUnauthorized {
		t.Fatalf("rotated-out token = %d, want 401", st)
	}
	if st := anon.do("GET", "/feeds/"+protD+"/rss.xml?token="+newTokD, nil).status; st != http.StatusOK {
		t.Fatalf("rotated-in token = %d, want 200", st)
	}

	// Delete token -> immediate 401.
	if st := admin.do("DELETE", "/api/v1/admin/dirs/"+protD+"/tokens/"+tokDID, nil).status; st != http.StatusNoContent {
		t.Fatalf("delete token = %d, want 204", st)
	}
	if st := anon.do("GET", "/feeds/"+protD+"/rss.xml?token="+newTokD, nil).status; st != http.StatusUnauthorized {
		t.Fatalf("deleted token = %d, want 401", st)
	}

	// List tokens contains only remaining token (C or empty on D) and no plaintext secret.
	listD := admin.do("GET", "/api/v1/admin/dirs/"+protD+"/tokens", nil)
	if listD.status != http.StatusOK || len(listD.json()["tokens"].([]any)) != 0 {
		t.Fatalf("list D tokens = %d %s", listD.status, listD.body)
	}

	// Assert ZERO occurrences of any plaintext ad_ token in the entire httplog output buffer!
	logs := logBuf.String()
	for _, secret := range []string{tokD, tokC, newTokD} {
		if strings.Contains(logs, secret) {
			t.Fatalf("plaintext directory token %q leaked into HTTP logs:\n%s", secret, logs)
		}
	}
	if !strings.Contains(logs, "REDACTED") {
		t.Fatalf("expected REDACTED marker in HTTP logs:\n%s", logs)
	}
}

func TestV2AssetStreamingEdgeCases(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	anon := e.anon()

	protD := admin.createDir("", "ProtectedStream", "pwStream")
	wav1 := buildTestWAV(1)
	sum := sha256.Sum256(wav1)
	hash1 := hex.EncodeToString(sum[:])

	admin.ingestOK(ingestReq{
		xml:    xmlDoc12("Stream Item", "", "", `<audio src="./ep1.wav" title="Ep1"/>`),
		dirs:   []string{protD},
		assets: []figure{{src: "ep1.wav", name: "ep1.wav", data: wav1}},
	})

	tokResp := admin.do(http.MethodPost, "/api/v1/admin/dirs/"+protD+"/tokens", map[string]string{"label": "stream-reader"})
	if tokResp.status != http.StatusCreated {
		t.Fatalf("create token = %d %s", tokResp.status, tokResp.body)
	}
	tokD := tokResp.json()["token"].(string)

	// 1. Out-of-bounds Range on tokenized feed asset must still preserve Cache-Control: private, no-store on 416.
	req416, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/feeds/"+protD+"/assets/"+hash1+"?token="+tokD, nil)
	req416.Header.Set("Range", "bytes=999999-1000000")
	res416 := anon.send(req416)
	if res416.status != http.StatusRequestedRangeNotSatisfiable || res416.header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("feed OOB range = %d Cache-Control=%q body=%s", res416.status, res416.header.Get("Cache-Control"), res416.body)
	}

	var storagePath string
	if err := e.db.Get(&storagePath, `SELECT storage_path FROM assets WHERE hash = ?`, hash1); err != nil {
		t.Fatalf("select storage_path: %v", err)
	}

	// 2. Truncated object (DB size_bytes=200 while object is 40 bytes): must fail fast with 1 OpenRange call and never loop.
	e.store.mu.Lock()
	e.store.files[storagePath] = append([]byte(nil), wav1[:40]...)
	e.store.mu.Unlock()
	if _, err := e.db.Exec(`UPDATE assets SET size_bytes = 200 WHERE hash = ?`, hash1); err != nil {
		t.Fatalf("update size_bytes: %v", err)
	}

	c0 := e.store.openRangeCalls.Load()
	reqTruncRange, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/api/v1/assets/"+hash1, nil)
	reqTruncRange.Header.Set("Range", "bytes=100-150")
	resTruncRange := admin.send(reqTruncRange)
	if resTruncRange.status != http.StatusPartialContent || len(resTruncRange.body) != 0 || e.store.openRangeCalls.Load()-c0 != 1 {
		t.Fatalf("truncated past-EOF range = %d len=%d openRange=%d", resTruncRange.status, len(resTruncRange.body), e.store.openRangeCalls.Load()-c0)
	}

	c1 := e.store.openRangeCalls.Load()
	resTruncFull := admin.do(http.MethodGet, "/api/v1/assets/"+hash1, nil)
	if resTruncFull.status != http.StatusOK || len(resTruncFull.body) != 40 || e.store.openRangeCalls.Load()-c1 != 1 {
		t.Fatalf("truncated full GET = %d len=%d openRange=%d", resTruncFull.status, len(resTruncFull.body), e.store.openRangeCalls.Load()-c1)
	}

	// 3. Missing storage object: HEAD reports 200 with DB Content-Length; GET returns 404 JSON before committing headers.
	e.store.mu.Lock()
	delete(e.store.files, storagePath)
	e.store.mu.Unlock()

	reqHead, _ := http.NewRequest(http.MethodHead, e.srv.URL+"/api/v1/assets/"+hash1, nil)
	resHead := admin.send(reqHead)
	if resHead.status != http.StatusOK || resHead.header.Get("Content-Length") != "200" {
		t.Fatalf("HEAD missing object = %d cl=%q", resHead.status, resHead.header.Get("Content-Length"))
	}
	resMissing := admin.do(http.MethodGet, "/api/v1/assets/"+hash1, nil)
	if resMissing.status != http.StatusNotFound || resMissing.json()["error"] != "not_found" {
		t.Fatalf("GET missing object = %d %s", resMissing.status, resMissing.body)
	}
}

func TestV2PublicFeedCacheControlAndCyclicDirToken(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	anon := e.anon()

	pub := admin.createDir("", "PublicFeedNoURL", "")
	admin.ingestOK(ingestReq{
		xml:  xmlDoc12("Pub Feed Item", "", "", `<section title="S"><md>Hello feed</md></section>`),
		dirs: []string{pub},
	})
	pubRes := anon.do(http.MethodGet, "/feeds/"+pub+"/rss.xml", nil)
	if pubRes.status != http.StatusOK || pubRes.header.Get("Cache-Control") != "private, max-age=300" {
		t.Fatalf("public feed without PublicURL = %d cc=%q", pubRes.status, pubRes.header.Get("Cache-Control"))
	}

	protD := admin.createDir("", "CyclicD", "pwD")
	tokResp := admin.do(http.MethodPost, "/api/v1/admin/dirs/"+protD+"/tokens", map[string]string{"label": "cyc"})
	if tokResp.status != http.StatusCreated {
		t.Fatalf("create token = %d %s", tokResp.status, tokResp.body)
	}
	tokD := tokResp.json()["token"].(string)

	// Introduce self-cycle in SQLite: tokenized feed requests must fail closed (403) without stack overflow.
	if _, err := e.db.Exec(`UPDATE directories SET parent_id = ? WHERE id = ?`, protD, protD); err != nil {
		t.Fatalf("create dir cycle: %v", err)
	}
	if res := anon.do(http.MethodGet, "/feeds/"+protD+"/rss.xml?token="+tokD, nil); res.status != http.StatusForbidden {
		t.Fatalf("cyclic RSS feed = %d %s, want 403", res.status, res.body)
	}
	if res := anon.do(http.MethodGet, "/feeds/"+protD+"/podcast.xml?token="+tokD, nil); res.status != http.StatusForbidden {
		t.Fatalf("cyclic Podcast feed = %d %s, want 403", res.status, res.body)
	}
}

func TestV2RevisionMountsLastRevAndLegacyUpgrade(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	anon := e.anon()

	protP := admin.createDir("", "ProtectedP", "pwP")
	protP2 := admin.createDir("", "ProtectedP2", "pwP2")
	pubQ := admin.createDir("", "PublicQ", "")

	// 1. Moving unchanged item from P -> Q merges Q into the latest revision's mounts.
	id1 := admin.ingestOK(ingestReq{
		xml:  xmlDoc12("MoveUnchanged", "", "", `<section title="S"><md>SameBody</md></section>`),
		dirs: []string{protP},
	})
	if st := admin.do(http.MethodPut, "/api/v1/admin/items/"+id1+"/mounts", map[string]any{"dirs": []string{pubQ}}).status; st != http.StatusOK && st != http.StatusNoContent {
		t.Fatalf("remount unchanged = %d", st)
	}
	item1 := anon.do(http.MethodGet, "/api/v1/items/"+id1, nil)
	if item1.status != http.StatusOK || item1.json()["revision_count"].(float64) != 1 || item1.json()["current_rev"].(float64) != 1 {
		t.Fatalf("anon item1 after move = %d %s", item1.status, item1.body)
	}
	revs1 := anon.do(http.MethodGet, "/api/v1/items/"+id1+"/revisions", nil)
	revs1List := revs1.json()["revisions"].([]any)
	if revs1.status != http.StatusOK || len(revs1List) != 1 || revs1List[0].(map[string]any)["rev"].(float64) != 1 ||
		!revs1List[0].(map[string]any)["is_current"].(bool) {
		t.Fatalf("anon item1 revisions = %d %s", revs1.status, revs1.body)
	}

	// 2. Clearing P's password must NOT expose previously protected v1 or its exclusive asset after v2 published to Q.
	secWav := buildTestWAV(3)
	secSum := sha256.Sum256(secWav)
	secHash := hex.EncodeToString(secSum[:])
	id2 := admin.ingestOK(ingestReq{
		xml:    xmlDoc12("Secret V1", "", "", `<section title="S"><md>PRIVATE_V1_BODY</md></section><audio src="./sec.wav" title="Sec"/>`),
		dirs:   []string{protP},
		assets: []figure{{src: "sec.wav", name: "sec.wav", data: secWav}},
	})
	admin.ingestOK(ingestReq{
		itemID: id2,
		xml:    xmlDoc12("Public V2", "", "", `<section title="S"><md>PUBLIC_V2_BODY</md></section>`),
		dirs:   []string{pubQ},
	})
	if st := admin.do(http.MethodPatch, "/api/v1/admin/dirs/"+protP, map[string]string{"password": ""}).status; st != http.StatusNoContent && st != http.StatusOK {
		t.Fatalf("clear password P = %d", st)
	}
	item2Anon := anon.do(http.MethodGet, "/api/v1/items/"+id2, nil)
	if item2Anon.status != http.StatusOK || item2Anon.json()["revision_count"].(float64) != 1 || item2Anon.json()["current_rev"].(float64) != 2 {
		t.Fatalf("anon item2 after clear password P = %d %s", item2Anon.status, item2Anon.body)
	}
	revs2Anon := anon.do(http.MethodGet, "/api/v1/items/"+id2+"/revisions", nil)
	revs2List := revs2Anon.json()["revisions"].([]any)
	if len(revs2List) != 1 || revs2List[0].(map[string]any)["rev"].(float64) != 2 {
		t.Fatalf("anon item2 revisions = %s", revs2Anon.body)
	}
	if st := anon.do(http.MethodGet, "/api/v1/items/"+id2+"/revisions/1/content", nil).status; st != http.StatusForbidden {
		t.Fatalf("anon old v1 content after clear password = %d, want 403", st)
	}
	if st := anon.do(http.MethodGet, "/api/v1/assets/"+secHash, nil).status; st != http.StatusForbidden {
		t.Fatalf("anon old exclusive asset after clear password = %d, want 403", st)
	}
	if resAdminV1 := admin.do(http.MethodGet, "/api/v1/items/"+id2+"/revisions/1/content", nil); resAdminV1.status != http.StatusOK ||
		!strings.Contains(string(resAdminV1.body), "PRIVATE_V1_BODY") {
		t.Fatalf("admin old v1 content = %d %s", resAdminV1.status, resAdminV1.body)
	}

	// 3. is_current matches delete eligibility after draft push, and deleting max rev keeps next rev monotonically increasing.
	id3 := admin.ingestOK(ingestReq{
		xml:  xmlDoc12("RevMonotonic1", "", "", `<section title="S"><md>V1_BODY</md></section>`),
		dirs: []string{pubQ},
	})
	admin.ingestOK(ingestReq{
		itemID: id3,
		xml:    xmlDoc12("RevMonotonic2", "", "", `<section title="S"><md>V2_BODY</md></section>`),
		dirs:   []string{pubQ},
	})
	unpubID3, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/api/v1/agent/items/"+id3+"/mounts", nil)
	unpubID3.Header.Set("Authorization", "Bearer "+apiToken)
	if res := anon.send(unpubID3); res.status != http.StatusOK && res.status != http.StatusNoContent {
		t.Fatalf("unpublish id3 before draft = %d %s", res.status, res.body)
	}
	admin.ingestOK(ingestReq{
		itemID: id3,
		draft:  true,
		xml:    xmlDoc12("RevMonotonicDraft", "", "", `<section title="S"><md>V3_DRAFT_BODY</md></section>`),
	})
	revs3Admin := admin.do(http.MethodGet, "/api/v1/items/"+id3+"/revisions", nil)
	revs3List := revs3Admin.json()["revisions"].([]any)
	if len(revs3List) != 2 || revs3List[0].(map[string]any)["rev"].(float64) != 2 || revs3List[0].(map[string]any)["is_current"].(bool) {
		t.Fatalf("expected rev 2 with is_current=false after draft overwrite: %s", revs3Admin.body)
	}
	if st := admin.do(http.MethodDelete, "/api/v1/admin/items/"+id3+"/revisions/2", nil).status; st != http.StatusNoContent {
		t.Fatalf("delete non-current max rev 2 = %d, want 204", st)
	}
	admin.ingestOK(ingestReq{
		itemID: id3,
		xml:    xmlDoc12("RevMonotonic4", "", "", `<section title="S"><md>V4_PUB_BODY</md></section>`),
		dirs:   []string{pubQ},
	})
	revs3After := admin.do(http.MethodGet, "/api/v1/items/"+id3+"/revisions", nil).json()["revisions"].([]any)
	if top := revs3After[0].(map[string]any); top["rev"].(float64) != 3 || !top["is_current"].(bool) {
		t.Fatalf("expected monotonic rev 3 with is_current=true after deleting rev 2, got %+v", revs3After)
	}

	// 4. Pre-migration 0-rev item in protected P2: unpublish -> draft v2 -> mount to Q preserves v1 under P2 (was_protected=1).
	id4 := admin.ingestOK(ingestReq{
		xml:  xmlDoc12("LegacyItem", "", "", `<section title="S"><md>LEGACY_SECRET_V1</md></section>`),
		dirs: []string{protP2},
	})
	if _, err := e.db.Exec(`DELETE FROM item_revision_mounts WHERE revision_id IN (SELECT id FROM item_revisions WHERE item_id = ?)`, id4); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`DELETE FROM item_revisions WHERE item_id = ?`, id4); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE items SET last_rev = 0 WHERE id = ?`, id4); err != nil {
		t.Fatal(err)
	}

	unpubReq, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/api/v1/agent/items/"+id4+"/mounts", nil)
	unpubReq.Header.Set("Authorization", "Bearer "+apiToken)
	if res := anon.send(unpubReq); res.status != http.StatusOK && res.status != http.StatusNoContent {
		t.Fatalf("agent unpublish legacy item = %d %s", res.status, res.body)
	}
	admin.ingestOK(ingestReq{
		itemID: id4,
		draft:  true,
		xml:    xmlDoc12("LegacySanitized", "", "", `<section title="S"><md>SANITIZED_DRAFT_V2</md></section>`),
	})
	if st := admin.do(http.MethodPut, "/api/v1/admin/items/"+id4+"/mounts", map[string]any{"dirs": []string{pubQ}}).status; st != http.StatusOK && st != http.StatusNoContent {
		t.Fatalf("remount sanitized legacy item = %d", st)
	}
	if st := admin.do(http.MethodPatch, "/api/v1/admin/dirs/"+protP2, map[string]string{"password": ""}).status; st != http.StatusNoContent && st != http.StatusOK {
		t.Fatalf("clear password P2 = %d", st)
	}

	revs4Admin := admin.do(http.MethodGet, "/api/v1/items/"+id4+"/revisions", nil).json()["revisions"].([]any)
	if len(revs4Admin) != 2 || revs4Admin[0].(map[string]any)["rev"].(float64) != 2 || revs4Admin[1].(map[string]any)["rev"].(float64) != 1 {
		t.Fatalf("expected legacy upgrade revs [2,1], got %+v", revs4Admin)
	}
	if resV1 := admin.do(http.MethodGet, "/api/v1/items/"+id4+"/revisions/1/content", nil); resV1.status != http.StatusOK ||
		!strings.Contains(string(resV1.body), "LEGACY_SECRET_V1") {
		t.Fatalf("expected preserved legacy baseline v1, got %d %s", resV1.status, resV1.body)
	}
	if st := anon.do(http.MethodGet, "/api/v1/items/"+id4+"/revisions/1/content", nil).status; st != http.StatusForbidden {
		t.Fatalf("anon access to preserved protected v1 after password clear = %d, want 403", st)
	}
	revs4Anon := anon.do(http.MethodGet, "/api/v1/items/"+id4+"/revisions", nil).json()["revisions"].([]any)
	if len(revs4Anon) != 1 || revs4Anon[0].(map[string]any)["rev"].(float64) != 2 {
		t.Fatalf("expected anon to see only [rev 2] on legacy upgrade, got %+v", revs4Anon)
	}
}
