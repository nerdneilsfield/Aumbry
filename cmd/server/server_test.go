package main

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
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
