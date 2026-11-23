package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/webdav"

	"github.com/nerdneilsfield/aumbry/internal/api"
	"github.com/nerdneilsfield/aumbry/internal/auth"
	"github.com/nerdneilsfield/aumbry/internal/config"
	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/internal/storage"
)

func TestCheckConfigCommand(t *testing.T) {
	hashBytes, err := bcrypt.GenerateFromPassword([]byte("admin-pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	hash := string(hashBytes)
	nonExistentDataDir := filepath.Join(t.TempDir(), "should-not-be-created")
	tomlPath := filepath.Join(t.TempDir(), "server.toml")
	content := `
listen_addr = ":9090"
data_dir = "` + filepath.ToSlash(nonExistentDataDir) + `"
storage = "webdav"
api_tokens = ["amb_live_secret_1234"]
cookie_secret = "super-secret-cookie-value-at-least-32-bytes-5678"
trusted_proxies = ["10.0.0.0/8"]

[admin]
user = "admin"
password_hash = "` + hash + `"

[webdav]
url = "http://127.0.0.1:1/unreachable-not-probed"
user = "davuser"
password = "dav-password-9012"
`
	if err := os.WriteFile(tomlPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("valid TOML outputs masked config and does not touch DB or WebDAV", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cmd := newRootCmd(slog.New(slog.NewTextHandler(io.Discard, nil)))
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"check-config", "--config", tomlPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("check-config failed: %v", err)
		}
		out := stdout.String()
		for _, fullSecret := range []string{
			hash,
			"amb_live_secret_1234",
			"super-secret-cookie-value-at-least-32-bytes-5678",
			"dav-password-9012",
		} {
			if strings.Contains(out, fullSecret) {
				t.Errorf("check-config leaked full secret %q", fullSecret)
			}
		}
		for _, expected := range []string{
			"****1234",
			"****5678",
			"****9012",
			":9090",
		} {
			if !strings.Contains(out, expected) {
				t.Errorf("check-config output missing %q:\n%s", expected, out)
			}
		}
		if _, err := os.Stat(nonExistentDataDir); !os.IsNotExist(err) {
			t.Errorf("check-config must not create data_dir %s", nonExistentDataDir)
		}
	})

	t.Run("unknown key fails naming key", func(t *testing.T) {
		badPath := filepath.Join(t.TempDir(), "unknown.toml")
		badContent := strings.Replace(content, "listen_addr", "listn_addr", 1)
		if err := os.WriteFile(badPath, []byte(badContent), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := newRootCmd(slog.New(slog.NewTextHandler(io.Discard, nil)))
		cmd.SetArgs([]string{"check-config", "--config", badPath})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "listn_addr") {
			t.Fatalf("error = %v, want listn_addr named", err)
		}
	})

	t.Run("missing explicit config fails", func(t *testing.T) {
		cmd := newRootCmd(slog.New(slog.NewTextHandler(io.Discard, nil)))
		cmd.SetArgs([]string{"check-config", "--config", filepath.Join(t.TempDir(), "missing.toml")})
		if err := cmd.Execute(); err == nil {
			t.Fatal("expected error for missing explicit --config")
		}
	})
}

func setupMaintenanceFixture(t *testing.T) (configPath, dataDir, davURL string) {
	t.Helper()
	dav := httptest.NewServer(&webdav.Handler{FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()})
	t.Cleanup(dav.Close)

	dataDir = filepath.Join(t.TempDir(), "data")
	hashBytes, err := bcrypt.GenerateFromPassword([]byte("admin-pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	configPath = filepath.Join(t.TempDir(), "server.toml")
	cfg := `
data_dir = "` + filepath.ToSlash(dataDir) + `"
storage = "webdav"
api_tokens = ["amb_test_tok"]
cookie_secret = "0123456789abcdef0123456789abcdef"

[admin]
user = "admin"
password_hash = "` + string(hashBytes) + `"

[webdav]
url = "` + dav.URL + `"
user = "u"
password = "p"
`
	if err := os.WriteFile(configPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath, dataDir, dav.URL
}

func TestMaintenanceCommands(t *testing.T) {
	ctx := context.Background()
	cfgPath, dataDir, davURL := setupMaintenanceFixture(t)

	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewWebDAV(davURL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}

	liveAssetHash := strings.Repeat("a", 64)
	liveAssetPath := "/assets/" + liveAssetHash + ".svg"
	liveXMLPath := "/xml/item00000001.xml"

	orphanDBHash := strings.Repeat("b", 64)
	orphanDBAssetPath := "/assets/" + orphanDBHash + ".svg"
	untrackedAssetPath := "/assets/" + strings.Repeat("c", 64) + ".png"
	orphanXMLPath := "/xml/item00000002-" + strings.Repeat("d", 64) + ".xml"
	manualXMLReadme := "/xml/README.txt"
	manualXMLNotes := "/xml/notes.xml"
	manualAssetReadme := "/assets/README.txt"
	manualAssetImage := "/assets/manual.png"

	for path, body := range map[string]string{
		liveXMLPath:        "<document/>",
		liveAssetPath:      "<svg/>",
		orphanDBAssetPath:  "<svg>orphan-db</svg>",
		untrackedAssetPath: "untracked-upload",
		orphanXMLPath:      "<document>old</document>",
		manualXMLReadme:    "operator readme",
		manualXMLNotes:     "<notes/>",
		manualAssetReadme:  "operator asset readme",
		manualAssetImage:   "manual image",
	} {
		if err = store.Put(ctx, path, strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err = database.ExecContext(ctx, `INSERT INTO items
		(id, external_key, type, title, report_date, summary, tags_json, source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text)
		VALUES ('item00000001', NULL, 'report', 'Live', '2026-11-21', 'sum', '[]', 'agent', '', '[]', ?, 'text')`, liveXMLPath); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO assets (hash, mime_type, size_bytes, storage_path) VALUES (?, 'image/svg+xml', 6, ?)`, liveAssetHash, liveAssetPath); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO item_assets (item_id, asset_hash) VALUES ('item00000001', ?)`, liveAssetHash); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO assets (hash, mime_type, size_bytes, storage_path) VALUES (?, 'image/svg+xml', 18, ?)`, orphanDBHash, orphanDBAssetPath); err != nil {
		t.Fatal(err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}

	runCLI := func(args ...string) (string, error) {
		var out, errBuf bytes.Buffer
		cmd := newRootCmd(slog.New(slog.NewTextHandler(io.Discard, nil)))
		cmd.SetOut(&out)
		cmd.SetErr(&errBuf)
		cmd.SetArgs(append([]string{"--config", cfgPath}, args...))
		execErr := cmd.Execute()
		return out.String(), execErr
	}

	// 1. backup produces a valid snapshot that passes verify-storage when copied to a fresh data_dir.
	snapDir := t.TempDir()
	snapPath := filepath.Join(snapDir, "aumbry.db")
	if out, backupErr := runCLI("backup", "--out", snapPath); backupErr != nil || !strings.Contains(out, snapPath) {
		t.Fatalf("backup = %q, %v", out, backupErr)
	}
	snapDB, err := sqlx.Open("sqlite", snapPath)
	if err != nil {
		t.Fatal(err)
	}
	var integrity string
	if err = snapDB.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		snapDB.Close()
		t.Fatalf("snapshot integrity_check = %q, %v", integrity, err)
	}
	snapDB.Close()

	// Point another config at snapDir and run verify-storage -> OK.
	snapCfgPath := filepath.Join(t.TempDir(), "snap.toml")
	rawCfg, _ := os.ReadFile(cfgPath)
	snapCfg := strings.Replace(string(rawCfg), filepath.ToSlash(dataDir), filepath.ToSlash(snapDir), 1)
	if err = os.WriteFile(snapCfgPath, []byte(snapCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	{
		var out bytes.Buffer
		cmd := newRootCmd(slog.New(slog.NewTextHandler(io.Discard, nil)))
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--config", snapCfgPath, "verify-storage"})
		if err = cmd.Execute(); err != nil {
			t.Fatalf("verify-storage on backup failed: %v", err)
		}
	}

	// 2. While a server holds data_dir lock: gc --dry-run succeeds & leaves objects untouched; destructive gc fails.
	lock, err := db.TryLockDataDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	dryOut, err := runCLI("gc", "--dry-run")
	if err != nil {
		_ = lock.Close()
		t.Fatalf("gc --dry-run while server running failed: %v", err)
	}
	for _, want := range []string{orphanDBAssetPath, untrackedAssetPath, orphanXMLPath} {
		if !strings.Contains(dryOut, want) {
			t.Errorf("gc --dry-run missing %q:\n%s", want, dryOut)
		}
	}
	if strings.Contains(dryOut, liveAssetPath) || strings.Contains(dryOut, liveXMLPath) {
		t.Errorf("gc --dry-run listed live object:\n%s", dryOut)
	}
	for _, untouched := range []string{manualXMLReadme, manualXMLNotes, manualAssetReadme, manualAssetImage} {
		if strings.Contains(dryOut, untouched) {
			t.Errorf("gc --dry-run listed non-Aumbry object %s:\n%s", untouched, dryOut)
		}
	}
	if _, err = runCLI("gc"); err == nil {
		_ = lock.Close()
		t.Fatal("destructive gc succeeded while server lock was held, want refusal")
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}

	// 3. Offline destructive gc removes orphans + untracked remnants while preserving live objects.
	gcOut, err := runCLI("gc")
	if err != nil {
		t.Fatalf("offline gc failed: %v", err)
	}
	if !strings.Contains(gcOut, "deleted 1 xml, 2 remote assets, 1 db asset rows") {
		t.Errorf("unexpected gc output: %q", gcOut)
	}
	for _, gone := range []string{orphanDBAssetPath, untrackedAssetPath, orphanXMLPath} {
		if ok, _ := store.Exists(ctx, gone); ok {
			t.Errorf("orphan %s still exists after gc", gone)
		}
	}
	for _, kept := range []string{liveXMLPath, liveAssetPath, manualXMLReadme, manualXMLNotes, manualAssetReadme, manualAssetImage} {
		if ok, _ := store.Exists(ctx, kept); !ok {
			t.Errorf("object %s was deleted by gc", kept)
		}
	}

	// 4. verify-storage passes before deletion, and fails listing the missing asset after deleting it from WebDAV.
	if _, err := runCLI("verify-storage"); err != nil {
		t.Fatalf("verify-storage before deletion = %v", err)
	}
	if err := store.Delete(ctx, liveAssetPath); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI("verify-storage"); err == nil || !strings.Contains(err.Error(), liveAssetPath) {
		t.Fatalf("verify-storage after asset deletion = %v, want missing %s listed", err, liveAssetPath)
	}
}

const sampleTestSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12"><rect width="12" height="12" fill="#000"/></svg>`

func testXMLDoc(title string, figures ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><document version="1.1"><meta>`)
	b.WriteString(`<title>` + title + `</title><type>report</type><date>2026-11-22</date>`)
	b.WriteString(`<summary>summary of ` + title + `</summary><source-agent>test-agent</source-agent></meta>`)
	b.WriteString(`<body><section title="Main"><md><![CDATA[Body of ` + title + `]]></md>`)
	for _, fig := range figures {
		b.WriteString(`<figure src="` + fig + `" caption="fig"/>`)
	}
	b.WriteString(`</section></body></document>`)
	return b.String()
}

func pushTestItem(t *testing.T, baseURL, token, xmlBody string, assetName, assetSrc string, assetData []byte, itemID string) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	p, _ := mw.CreateFormFile("xml", "report.xml")
	_, _ = p.Write([]byte(xmlBody))
	if assetSrc != "" {
		af, _ := mw.CreateFormFile("assets[]", assetName)
		_, _ = af.Write(assetData)
		_ = mw.WriteField("asset_srcs", assetSrc)
	}
	if itemID != "" {
		_ = mw.WriteField("item_id", itemID)
	}
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/ingest", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("ingest failed: %d %s", resp.StatusCode, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func loginTestAdmin(t *testing.T, baseURL, user, pass string) []*http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	resp, err := http.Post(baseURL+"/api/v1/admin/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("admin login = %d %s", resp.StatusCode, b)
	}
	return resp.Cookies()
}

func httpGetWithCookies(t *testing.T, url string, cookies []*http.Cookie) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// clearAumbryEnv keeps AUMBRY_* variables exported on the host from overriding the test configuration.
func clearAumbryEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AUMBRY_CONFIG", "AUMBRY_LISTEN_ADDR", "AUMBRY_DATA_DIR", "AUMBRY_STORAGE",
		"AUMBRY_ADMIN_USER", "AUMBRY_ADMIN_PASSWORD_HASH", "AUMBRY_API_TOKENS",
		"AUMBRY_COOKIE_SECRET", "AUMBRY_TRUSTED_PROXIES", "AUMBRY_LOCAL_PATH",
		"AUMBRY_WEBDAV_URL", "AUMBRY_WEBDAV_USER", "AUMBRY_WEBDAV_PASS",
		"AUMBRY_S3_ENDPOINT", "AUMBRY_S3_REGION", "AUMBRY_S3_BUCKET",
		"AUMBRY_S3_ACCESS_KEY_ID", "AUMBRY_S3_SECRET_ACCESS_KEY", "AUMBRY_S3_PATH_STYLE",
		"AUMBRY_R2_ACCOUNT_ID", "AUMBRY_R2_BUCKET",
		"AUMBRY_R2_ACCESS_KEY_ID", "AUMBRY_R2_SECRET_ACCESS_KEY",
	} {
		t.Setenv(key, "")
	}
}

func TestCheckConfigAcceptsExampleTOML(t *testing.T) {
	clearAumbryEnv(t)
	examplePath := filepath.Join("..", "..", "config.example.toml")
	var stdout, stderr bytes.Buffer
	cmd := newRootCmd(slog.New(slog.NewTextHandler(io.Discard, nil)))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"check-config", "--config", examplePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("check-config config.example.toml failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "storage = 'local'") {
		t.Fatalf("check-config output missing default storage = 'local':\n%s", stdout.String())
	}
}

func TestLocalEndToEndDefaultConfig(t *testing.T) {
	clearAumbryEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	hashBytes, err := bcrypt.GenerateFromPassword([]byte("admin-pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(t.TempDir(), "default.toml")
	cfgContent := `
data_dir = "` + filepath.ToSlash(dataDir) + `"
api_tokens = ["amb_local_tok"]
cookie_secret = "0123456789abcdef0123456789abcdef"

[admin]
user = "admin"
password_hash = "` + string(hashBytes) + `"
`
	if writeErr := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	res, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.Storage != "local" {
		t.Fatalf("default Storage = %q, want local", res.Config.Storage)
	}
	store, err := openStore(res.Config, true)
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(res.Config.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()

	sessions, err := auth.New(auth.Settings{
		AdminUser: res.Config.Admin.User, AdminPasswordHash: res.Config.Admin.PasswordHash,
		APITokens: res.Config.APITokens, CookieSecret: res.Config.CookieSecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	router, err := api.NewRouter(database, store, sessions, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	code, readyBody := httpGetWithCookies(t, srv.URL+"/readyz", nil)
	if code != http.StatusOK || !strings.Contains(string(readyBody), `"storage":"ok"`) {
		t.Fatalf("/readyz = %d %s", code, readyBody)
	}

	pushed := pushTestItem(t, srv.URL, "amb_local_tok", testXMLDoc("LocalE2E", "./imgs/a.svg"), "a.svg", "./imgs/a.svg", []byte(sampleTestSVG), "")
	itemID := pushed["id"].(string)

	cookies := loginTestAdmin(t, srv.URL, "admin", "admin-pw")
	xmlCode, xmlBody := httpGetWithCookies(t, srv.URL+"/api/v1/items/"+itemID+"/content", cookies)
	if xmlCode != http.StatusOK || !strings.Contains(string(xmlBody), "Body of LocalE2E") {
		t.Fatalf("get XML = %d %s", xmlCode, xmlBody)
	}
	idx := strings.Index(string(xmlBody), "/api/v1/assets/")
	if idx < 0 {
		t.Fatalf("rewritten asset URL missing from XML: %s", xmlBody)
	}
	assetURL := string(xmlBody[idx : idx+len("/api/v1/assets/")+64])
	assetCode, assetBody := httpGetWithCookies(t, srv.URL+assetURL, cookies)
	if assetCode != http.StatusOK || string(assetBody) != sampleTestSVG {
		t.Fatalf("get asset = %d %q, want %q", assetCode, assetBody, sampleTestSVG)
	}

	objectsDir := filepath.Join(dataDir, "objects")
	for _, dir := range []string{objectsDir, filepath.Join(objectsDir, "xml"), filepath.Join(objectsDir, "assets")} {
		info, statErr := os.Stat(dir)
		if statErr != nil {
			t.Fatalf("stat %s: %v", dir, statErr)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("dir %s mode = %04o, want 0700", dir, perm)
		}
	}
	for _, sub := range []string{"xml", "assets"} {
		entries, readErr := os.ReadDir(filepath.Join(objectsDir, sub))
		if readErr != nil || len(entries) != 1 {
			t.Fatalf("%s entries = %v, %v; want 1 object", sub, len(entries), readErr)
		}
		info, infoErr := entries[0].Info()
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("object %s/%s mode = %04o, want 0600", sub, entries[0].Name(), perm)
		}
	}
}

func TestLocalMaintenanceRequiresExistingRoot(t *testing.T) {
	clearAumbryEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = database.Close()

	hashBytes, err := bcrypt.GenerateFromPassword([]byte("admin-pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	absentRoot := filepath.Join(dataDir, "objects")
	cfgPath := filepath.Join(t.TempDir(), "local.toml")
	cfgContent := `
data_dir = "` + filepath.ToSlash(dataDir) + `"
api_tokens = ["amb_local_tok"]
cookie_secret = "0123456789abcdef0123456789abcdef"

[admin]
user = "admin"
password_hash = "` + string(hashBytes) + `"
`
	if writeErr := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	for _, args := range [][]string{
		{"verify-storage"},
		{"gc", "--dry-run"},
		{"gc"},
		{"reindex", "--dry-run"},
		{"reindex"},
	} {
		cmd := newRootCmd(slog.New(slog.NewTextHandler(io.Discard, nil)))
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(append([]string{"--config", cfgPath}, args...))
		if execErr := cmd.Execute(); execErr == nil {
			t.Errorf("%v succeeded with absent objects root, want error", args)
		}
		if _, statErr := os.Stat(absentRoot); !os.IsNotExist(statErr) {
			t.Fatalf("%v created absent objects root %s", args, absentRoot)
		}
	}
}

func TestS3EndToEndAndMaintenance(t *testing.T) {
	clearAumbryEnv(t)
	backend := s3mem.New()
	if err := backend.CreateBucket("aumbry"); err != nil {
		t.Fatal(err)
	}
	s3Srv := httptest.NewServer(gofakes3.New(backend).Server())
	defer s3Srv.Close()

	dataDir := filepath.Join(t.TempDir(), "data")
	hashBytes, err := bcrypt.GenerateFromPassword([]byte("admin-pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(t.TempDir(), "s3.toml")
	cfgContent := `
data_dir = "` + filepath.ToSlash(dataDir) + `"
storage = "s3"
api_tokens = ["amb_s3_tok"]
cookie_secret = "0123456789abcdef0123456789abcdef"

[admin]
user = "admin"
password_hash = "` + string(hashBytes) + `"

[s3]
endpoint = "` + s3Srv.URL + `"
region = "us-east-1"
bucket = "aumbry"
path_style = true
access_key_id = "test-ak"
secret_access_key = "test-sk"
`
	if writeErr := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	res, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	store, err := openStore(res.Config, false)
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.New(auth.Settings{
		AdminUser: res.Config.Admin.User, AdminPasswordHash: res.Config.Admin.PasswordHash,
		APITokens: res.Config.APITokens, CookieSecret: res.Config.CookieSecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	router, err := api.NewRouter(database, store, sessions, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	// 1. Push with asset, read back, update, and verify the old XML stays until gc.
	v1 := pushTestItem(t, srv.URL, "amb_s3_tok", testXMLDoc("S3 v1", "./imgs/a.svg"), "a.svg", "./imgs/a.svg", []byte(sampleTestSVG), "")
	itemID := v1["id"].(string)

	cookies := loginTestAdmin(t, srv.URL, "admin", "admin-pw")
	code, xmlBody := httpGetWithCookies(t, srv.URL+"/api/v1/items/"+itemID+"/content", cookies)
	if code != http.StatusOK || !strings.Contains(string(xmlBody), "Body of S3 v1") {
		t.Fatalf("v1 content = %d %s", code, xmlBody)
	}
	idx := strings.Index(string(xmlBody), "/api/v1/assets/")
	assetPathSuffix := string(xmlBody[idx+len("/api/v1"):idx+len("/api/v1/assets/")+64]) + ".svg"
	assetURL := srv.URL + string(xmlBody[idx:idx+len("/api/v1/assets/")+64])
	if aCode, aBody := httpGetWithCookies(t, assetURL, cookies); aCode != http.StatusOK || string(aBody) != sampleTestSVG {
		t.Fatalf("get S3 asset = %d %q", aCode, aBody)
	}

	_ = pushTestItem(t, srv.URL, "amb_s3_tok", testXMLDoc("S3 v2", "./imgs/a.svg"), "a.svg", "./imgs/a.svg", []byte(sampleTestSVG), itemID)
	xmlObjs, err := store.List(context.Background(), "/xml")
	if err != nil || len(xmlObjs) != 2 {
		t.Fatalf("expected old and new xml objects after update, got %v, %v", xmlObjs, err)
	}

	// Push and delete another item.
	toDel := pushTestItem(t, srv.URL, "amb_s3_tok", testXMLDoc("TempItem"), "", "", nil, "")
	delID := toDel["id"].(string)
	delReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/agent/items/"+delID, nil)
	delReq.Header.Set("Authorization", "Bearer amb_s3_tok")
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete item error = %v", err)
	}
	_ = delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete item status = %d, want 204", delResp.StatusCode)
	}

	// Close DB connection from test HTTP server before running offline write maintenance commands.
	_ = database.Close()

	runCLI := func(args ...string) (string, error) {
		var out bytes.Buffer
		cmd := newRootCmd(slog.New(slog.NewTextHandler(io.Discard, nil)))
		cmd.SetOut(&out)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(append([]string{"--config", cfgPath}, args...))
		execErr := cmd.Execute()
		return out.String(), execErr
	}

	// 2. reindex runs cleanly over S3.
	if reOut, reErr := runCLI("reindex"); reErr != nil || !strings.Contains(reOut, "reindexed 1 of 1 items") {
		t.Fatalf("s3 reindex = %q, %v", reOut, reErr)
	}

	// 3. gc --dry-run lists the orphan and offline gc removes it together with the superseded v1 xml and deleted temp item xml.
	orphanXML := "/xml/" + strings.Repeat("Z", 12) + "-" + strings.Repeat("f", 64) + ".xml"
	if err := store.Put(context.Background(), orphanXML, strings.NewReader("<orphan/>")); err != nil {
		t.Fatal(err)
	}
	if dryOut, dryErr := runCLI("gc", "--dry-run"); dryErr != nil || !strings.Contains(dryOut, orphanXML) {
		t.Fatalf("s3 gc --dry-run = %q, %v; want %s", dryOut, dryErr, orphanXML)
	}
	if gcOut, gcErr := runCLI("gc"); gcErr != nil || !strings.Contains(gcOut, "deleted 3 xml") {
		t.Fatalf("s3 gc = %q, %v", gcOut, gcErr)
	}
	if ok, _ := store.Exists(context.Background(), orphanXML); ok {
		t.Fatalf("orphan %s still exists after gc", orphanXML)
	}
	if left, listErr := store.List(context.Background(), "/xml"); listErr != nil || len(left) != 1 {
		t.Fatalf("xml objects after gc = %v, %v; want only the live v2", left, listErr)
	}

	// 4. verify-storage passes, then fails listing deleted asset.
	if _, verErr := runCLI("verify-storage"); verErr != nil {
		t.Fatalf("s3 verify-storage before deletion = %v", verErr)
	}
	if err := store.Delete(context.Background(), assetPathSuffix); err != nil {
		t.Fatal(err)
	}
	if _, verErr := runCLI("verify-storage"); verErr == nil || !strings.Contains(verErr.Error(), assetPathSuffix) {
		t.Fatalf("s3 verify-storage after deletion = %v, want missing %s listed", verErr, assetPathSuffix)
	}
}

func TestV2MaintenanceProtectsRevisionsAndAudioAssets(t *testing.T) {
	ctx := context.Background()
	cfgPath, dataDir, davURL := setupMaintenanceFixture(t)

	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewWebDAV(davURL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}

	revXMLPath := "/xml/itemhist0001-" + strings.Repeat("1", 64) + ".xml"
	liveXMLPath := "/xml/itemhist0001-" + strings.Repeat("2", 64) + ".xml"
	oldAudioHash := strings.Repeat("e", 64)
	oldAudioPath := "/assets/" + oldAudioHash + ".mp3"
	orphanAudioPath := "/assets/" + strings.Repeat("f", 64) + ".m4a"

	for p, body := range map[string]string{
		revXMLPath:      "<document/>",
		liveXMLPath:     "<document/>",
		oldAudioPath:    "rev1-mp3-bytes",
		orphanAudioPath: "orphan-m4a-bytes",
	} {
		if err = store.Put(ctx, p, strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err = database.ExecContext(ctx, `INSERT INTO items
		(id, external_key, type, title, report_date, summary, tags_json, source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text)
		VALUES ('itemhist0001', NULL, 'report', 'Live', '2026-11-23', 's', '[]', 'a', '', '[]', ?, 'live')`, liveXMLPath); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO assets (hash, mime_type, size_bytes, storage_path)
		VALUES (?, 'audio/mpeg', 14, ?)`, oldAudioHash, oldAudioPath); err != nil {
		t.Fatal(err)
	}
	res, err := database.ExecContext(ctx, `INSERT INTO item_revisions
		(item_id, rev, title, summary, report_date, xml_storage_path, xml_sha256)
		VALUES ('itemhist0001', 1, 'V1', 's', '2026-11-22', ?, ?)`, revXMLPath, strings.Repeat("1", 64))
	if err != nil {
		t.Fatal(err)
	}
	revID, _ := res.LastInsertId()
	if _, err = database.ExecContext(ctx, `INSERT INTO item_revision_assets (revision_id, asset_hash) VALUES (?, ?)`, revID, oldAudioHash); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()

	runCLI := func(args ...string) (string, error) {
		var out bytes.Buffer
		cmd := newRootCmd(slog.New(slog.NewTextHandler(io.Discard, nil)))
		cmd.SetOut(&out)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(append([]string{"--config", cfgPath}, args...))
		execErr := cmd.Execute()
		return out.String(), execErr
	}

	if _, err = runCLI("verify-storage"); err != nil {
		t.Fatalf("verify-storage before delete = %v", err)
	}

	// gc preserves rev1 XML & rev1-only audio asset while removing only orphanAudioPath.
	if dryOut, dryErr := runCLI("gc", "--dry-run"); dryErr != nil ||
		strings.Contains(dryOut, revXMLPath) || strings.Contains(dryOut, oldAudioPath) || !strings.Contains(dryOut, orphanAudioPath) {
		t.Fatalf("gc --dry-run = %q, %v", dryOut, dryErr)
	}
	if gcOut, gcErr := runCLI("gc"); gcErr != nil || !strings.Contains(gcOut, "deleted 0 xml, 1 remote assets, 0 db asset rows") {
		t.Fatalf("first gc = %q, %v", gcOut, gcErr)
	}
	for _, kept := range []string{revXMLPath, liveXMLPath, oldAudioPath} {
		if ok, _ := store.Exists(ctx, kept); !ok {
			t.Fatalf("revision-referenced object %s was deleted by gc", kept)
		}
	}

	// Deleting missing revision XML fails verify-storage naming the revision path.
	if err = store.Delete(ctx, revXMLPath); err != nil {
		t.Fatal(err)
	}
	if _, err = runCLI("verify-storage"); err == nil || !strings.Contains(err.Error(), revXMLPath) {
		t.Fatalf("verify-storage with missing rev XML = %v, want %s", err, revXMLPath)
	}

	// Once rev1 is removed from SQLite, gc reclaims oldAudioPath + DB row.
	database, err = db.OpenExisting(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = database.ExecContext(ctx, `DELETE FROM item_revisions WHERE id = ?`, revID)
	_ = database.Close()

	if gcOut, gcErr := runCLI("gc"); gcErr != nil || !strings.Contains(gcOut, "deleted 0 xml, 1 remote assets, 1 db asset rows") {
		t.Fatalf("second gc after rev delete = %q, %v", gcOut, gcErr)
	}
	if ok, _ := store.Exists(ctx, oldAudioPath); ok {
		t.Fatalf("oldAudioPath still exists after revision delete + gc")
	}
}
