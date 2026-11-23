package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"

	"github.com/nerdneilsfield/aumbry/internal/api"
	"github.com/nerdneilsfield/aumbry/internal/auth"
	"github.com/nerdneilsfield/aumbry/internal/client"
	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/internal/storage"
)

type cliMemStore struct {
	mu    sync.Mutex
	files map[string][]byte
}

func (m *cliMemStore) Put(_ context.Context, name string, body io.ReadSeeker) error {
	data, err := io.ReadAll(body)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[name] = data
	return err
}

func (m *cliMemStore) Open(_ context.Context, name string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[name]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *cliMemStore) OpenRange(ctx context.Context, name string, start, end int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if start < 0 || end < start {
		return nil, fmt.Errorf("invalid range [%d, %d]", start, end)
	}
	m.mu.Lock()
	data, ok := m.files[name]
	m.mu.Unlock()
	if !ok {
		return nil, storage.ErrNotFound
	}
	if start >= int64(len(data)) {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	hi := min(end+1, int64(len(data)))
	return io.NopCloser(bytes.NewReader(data[start:hi])), nil
}

func (m *cliMemStore) Exists(_ context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.files[name]
	return ok, nil
}

func (m *cliMemStore) Delete(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, name)
	return nil
}

func (m *cliMemStore) List(_ context.Context, dir string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for k := range m.files {
		if strings.HasPrefix(k, dir) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out, nil
}

func (m *cliMemStore) Probe(context.Context) error { return nil }

func startTestServer(t *testing.T) (*httptest.Server, *sqlx.DB, string, string) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	hash, err := bcrypt.GenerateFromPassword([]byte("admin-pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	const tok = "amb_cli_test_token"
	sessions, err := auth.New(auth.Settings{
		AdminUser:         "admin",
		AdminPasswordHash: string(hash),
		APITokens:         []string{tok},
		CookieSecret:      "0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &cliMemStore{files: map[string][]byte{}}
	router, err := api.NewRouter(database, store, sessions, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	// Log in as admin to create directories: Open, Locked, and Locked/Child.
	anonClient := srv.Client()
	loginReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/admin/login", strings.NewReader(`{"username":"admin","password":"admin-pw"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := anonClient.Do(loginReq)
	if err != nil || loginResp.StatusCode != http.StatusOK {
		t.Fatalf("admin login failed: %v %v", loginResp, err)
	}
	cookies := loginResp.Cookies()
	_ = loginResp.Body.Close()

	createDir := func(parent, name, pw string) string {
		bodyMap := map[string]any{"name": name}
		if parent != "" {
			bodyMap["parent_id"] = parent
		}
		if pw != "" {
			bodyMap["password"] = pw
		}
		payload, _ := json.Marshal(bodyMap)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/admin/dirs", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		for _, ck := range cookies {
			req.AddCookie(ck)
		}
		resp, err := anonClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("create dir %s failed: %v %v", name, resp, err)
		}
		defer resp.Body.Close()
		var res map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&res)
		return res["id"]
	}

	openDir := createDir("", "Public", "")
	lockedDir := createDir("", "Secret", "dir-pw")
	_ = createDir(lockedDir, "Nested", "")
	return srv, database, openDir, tok
}

func TestCLIAgentEndToEndWorkflows(t *testing.T) {
	isolate(t)
	srv, database, openDir, tok := startTestServer(t)
	t.Setenv("AUMBRY_SERVER", srv.URL)
	t.Setenv("AUMBRY_TOKEN", tok)

	// #24 + #43: aumbry dirs JSON and tree formats; child inherits protected:true while has_password:false.
	dirsOut := mustRun(t, "dirs")
	var dirs []client.AgentDir
	if err := json.Unmarshal([]byte(dirsOut), &dirs); err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 3 {
		t.Fatalf("dirs count = %d, want 3", len(dirs))
	}
	var foundInherited bool
	for _, d := range dirs {
		if d.Name == "Nested" && !d.HasPassword && d.Protected && d.Path == "Secret / Nested" {
			foundInherited = true
		}
	}
	if !foundInherited {
		t.Fatalf("expected Nested dir has_password=false protected=true: %+v", dirs)
	}
	treeOut := mustRun(t, "dirs", "--format", "tree")
	if !strings.Contains(treeOut, "  - Nested") || !strings.Contains(treeOut, "protected") {
		t.Fatalf("unexpected tree output:\n%s", treeOut)
	}
	if _, err := runCLI(t, "dirs", "--format", "xml"); err == nil {
		t.Fatal("expected unsupported format error on dirs")
	}

	// #21: Incremental push across 3 rounds with stages (round 1 = single stage) via same --key.
	tmpDir := t.TempDir()
	docPath := filepath.Join(tmpDir, "paper.xml")
	writePaper := func(stagesBody string) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<document version="1.1">
  <meta><title>Iterative Paper</title><type>post</type><date>2026-11-20</date><summary>S</summary><source-agent>a</source-agent></meta>
  <body><stages id="read">` + stagesBody + `</stages></body>
</document>`
		if err := os.WriteFile(docPath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	writePaper(`<stage id="r1" title="Round 1"><md>Part 1</md></stage>`)
	// #40: dry-run checks server/token/dir without creating item.
	dryOut := mustRun(t, "push", docPath, "--key", "paper:2611.01234", "--dir", openDir, "--dry-run")
	if !strings.Contains(dryOut, `"dry_run": true`) {
		t.Fatalf("dry-run output = %s", dryOut)
	}
	var itemCount int
	_ = database.Get(&itemCount, `SELECT count(*) FROM items`)
	if itemCount != 0 {
		t.Fatalf("dry-run created %d items", itemCount)
	}
	if _, err := runCLI(t, "push", docPath, "--dir", "NO_SUCH_DIR", "--dry-run"); err == nil {
		t.Fatal("expected dry-run failure on unknown dir")
	}

	// #22: first push as --draft, then publish with --dir, then append r2 and r3.
	draftOut := mustRun(t, "push", docPath, "--key", "paper:2611.01234", "--draft")
	var draftRes map[string]any
	_ = json.Unmarshal([]byte(draftOut), &draftRes)
	itemID := draftRes["id"].(string)
	if draftRes["created"] != true || draftRes["draft"] != true || draftRes["published"] != false || len(draftRes["dirs"].([]any)) != 0 {
		t.Fatalf("draft res = %v", draftRes)
	}

	writePaper(`<stage id="r1" title="Round 1"><md>Part 1</md></stage><stage id="r2" title="Round 2"><md>Part 2</md></stage>`)
	r2Out := mustRun(t, "push", docPath, "--key", "paper:2611.01234", "--dir", openDir)
	var r2Res map[string]any
	_ = json.Unmarshal([]byte(r2Out), &r2Res)
	if r2Res["id"] != itemID || r2Res["created"] != false || r2Res["published"] != true || r2Res["draft"] != false {
		t.Fatalf("round 2 res = %v", r2Res)
	}

	writePaper(`<stage id="r1" title="Round 1"><md>Part 1</md></stage><stage id="r2" title="Round 2"><md>Part 2</md></stage><stage id="r3" title="Round 3"><md>Part 3</md></stage>`)
	r3Out := mustRun(t, "push", docPath, "--key", "paper:2611.01234", "--dir", openDir)
	var r3Res map[string]any
	_ = json.Unmarshal([]byte(r3Out), &r3Res)
	if r3Res["id"] != itemID || r3Res["created"] != false {
		t.Fatalf("round 3 res = %v", r3Res)
	}

	// #42: --draft against already published item fails cleanly.
	if _, err := runCLI(t, "push", docPath, "--key", "paper:2611.01234", "--draft"); err == nil {
		t.Fatal("expected already_published failure")
	}

	// #41: --create-only against existing key fails 409 key_exists.
	if _, err := runCLI(t, "push", docPath, "--key", "paper:2611.01234", "--create-only"); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("create-only on existing key = %v", err)
	}

	// #38: aumbry list --key.
	listOut := mustRun(t, "list", "--key", "paper:2611.01234")
	if !strings.Contains(listOut, itemID) || strings.Contains(listOut, "<document") {
		t.Fatalf("list output = %s", listOut)
	}

	// #39: aumbry unpublish, then delete without --yes (must refuse locally), then delete --yes.
	unpubOut := mustRun(t, "unpublish", itemID)
	if !strings.Contains(unpubOut, `"published": false`) {
		t.Fatalf("unpublish output = %s", unpubOut)
	}
	if _, err := runCLI(t, "delete", itemID); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("delete without --yes = %v", err)
	}
	delOut := mustRun(t, "delete", itemID, "--yes")
	if !strings.Contains(delOut, `"deleted": true`) {
		t.Fatalf("delete output = %s", delOut)
	}
	var ftsCount int
	_ = database.Get(&ftsCount, `SELECT count(*) FROM items_fts WHERE item_id = ?`, itemID)
	if ftsCount != 0 {
		t.Fatalf("items_fts left %d rows after delete", ftsCount)
	}

	// Also test delete without --yes refuses without touching network at all.
	t.Setenv("AUMBRY_SERVER", "")
	if _, err := runCLI(t, "delete", "any-id"); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("delete without --yes and no server = %v", err)
	}

	// #21 part 2: incremental push appending top-level <section> blocks under same --key.
	t.Setenv("AUMBRY_SERVER", srv.URL)
	secPath := filepath.Join(tmpDir, "sections.xml")
	writeSections := func(sections string) {
		xml := `<document version="1.1"><meta><title>Incremental Sections</title><type>report</type><date>2026-11-20</date><summary>S</summary><source-agent>a</source-agent></meta><body>` + sections + `</body></document>`
		if err := os.WriteFile(secPath, []byte(xml), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeSections(`<section title="Round 1"><md>One</md></section>`)
	s1Out := mustRun(t, "push", secPath, "--key", "report:secs", "--dir", openDir)
	var s1Res map[string]any
	_ = json.Unmarshal([]byte(s1Out), &s1Res)
	writeSections(`<section title="Round 1"><md>One</md></section><section title="Round 2"><md>Two</md></section>`)
	s2Out := mustRun(t, "push", secPath, "--key", "report:secs", "--dir", openDir)
	var s2Res map[string]any
	_ = json.Unmarshal([]byte(s2Out), &s2Res)
	if s1Res["id"] != s2Res["id"] || s1Res["created"] != true || s2Res["created"] != false {
		t.Fatalf("incremental section push failed: s1=%v s2=%v", s1Res, s2Res)
	}

	// Push a v1.2 document containing an <audio> block end-to-end via CLI.
	audioDir := filepath.Join(t.TempDir(), "podcast-report")
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wav := makeMinimalWAVForCLI(1)
	if err := os.WriteFile(filepath.Join(audioDir, "ep.wav"), wav, 0o600); err != nil {
		t.Fatal(err)
	}
	audioXMLPath := filepath.Join(audioDir, "report.xml")
	audioXML := `<document version="1.2"><meta><title>CLI Audio</title><type>report</type><date>2026-11-23</date><summary>Audio CLI</summary><source-agent>cli</source-agent></meta><body><audio id="a1" src="./ep.wav" title="Episode One"><chapter start="00:00:00" title="Start"/><transcript>Spoken via CLI</transcript></audio></body></document>`
	if err := os.WriteFile(audioXMLPath, []byte(audioXML), 0o600); err != nil {
		t.Fatal(err)
	}
	valOut := mustRun(t, "validate", audioXMLPath)
	if !strings.Contains(valOut, `"valid": true`) {
		t.Fatalf("validate v1.2 audio = %s", valOut)
	}
	pushAudioOut := mustRun(t, "push", audioXMLPath, "--dir", openDir)
	var pushAudioRes map[string]any
	_ = json.Unmarshal([]byte(pushAudioOut), &pushAudioRes)
	if pushAudioRes["created"] != true || pushAudioRes["assets"].(float64) != 1 || pushAudioRes["published"] != true {
		t.Fatalf("push v1.2 audio res = %s", pushAudioOut)
	}
}

func makeMinimalWAVForCLI(secs int) []byte {
	const sampleRate = 8000
	const byteRate = sampleRate * 2
	dataLen := secs * byteRate
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	buf.Write([]byte{byte(36 + dataLen), byte((36 + dataLen) >> 8), byte((36 + dataLen) >> 16), byte((36 + dataLen) >> 24)})
	buf.WriteString("WAVEfmt ")
	buf.Write([]byte{16, 0, 0, 0, 1, 0, 1, 0})
	buf.Write([]byte{0x40, 0x1F, 0, 0, 0x80, 0x3E, 0, 0, 2, 0, 16, 0})
	buf.WriteString("data")
	buf.Write([]byte{byte(dataLen), byte(dataLen >> 8), byte(dataLen >> 16), byte(dataLen >> 24)})
	buf.Write(make([]byte, dataLen))
	return buf.Bytes()
}
