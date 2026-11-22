package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/webdav"

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
