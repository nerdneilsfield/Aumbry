package config

import (
	"strings"
	"testing"
)

func TestLoadRejectsMissingRequiredSettings(t *testing.T) {
	for _, key := range []string{
		"AUMBRY_DATA_DIR", "AUMBRY_ADMIN_USER", "AUMBRY_ADMIN_PASSWORD_HASH",
		"AUMBRY_API_TOKENS", "AUMBRY_COOKIE_SECRET", "AUMBRY_WEBDAV_URL",
		"AUMBRY_WEBDAV_USER", "AUMBRY_WEBDAV_PASS",
	} {
		t.Setenv(key, "")
	}
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "AUMBRY_DATA_DIR") {
		t.Fatalf("Load() error = %v, want missing required settings", err)
	}
}

func TestLoadAcceptsValidConfig(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("AUMBRY_API_TOKENS", " first-token, second-token ")
	t.Setenv("AUMBRY_LISTEN_ADDR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("ListenAddr = %q, want default :8080", cfg.ListenAddr)
	}
	if len(cfg.APITokens) != 2 || cfg.APITokens[0] != "first-token" || cfg.APITokens[1] != "second-token" {
		t.Fatalf("APITokens = %#v, want trimmed tokens", cfg.APITokens)
	}
}

func TestLoadRejectsShortCookieSecret(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("AUMBRY_COOKIE_SECRET", "short")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("Load() error = %v, want cookie secret length error", err)
	}
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("AUMBRY_DATA_DIR", t.TempDir())
	t.Setenv("AUMBRY_ADMIN_USER", "admin")
	t.Setenv("AUMBRY_ADMIN_PASSWORD_HASH", "hash")
	t.Setenv("AUMBRY_API_TOKENS", "token")
	t.Setenv("AUMBRY_COOKIE_SECRET", "01234567890123456789012345678901")
	t.Setenv("AUMBRY_WEBDAV_URL", "https://webdav.example.invalid")
	t.Setenv("AUMBRY_WEBDAV_USER", "user")
	t.Setenv("AUMBRY_WEBDAV_PASS", "pass")
}
