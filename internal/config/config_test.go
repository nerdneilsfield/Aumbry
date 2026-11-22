package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func testBcryptHash(t *testing.T) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}

func TestLoadRejectsMissingRequiredSettings(t *testing.T) {
	for _, key := range []string{
		"AUMBRY_CONFIG",
		"AUMBRY_DATA_DIR", "AUMBRY_ADMIN_USER", "AUMBRY_ADMIN_PASSWORD_HASH",
		"AUMBRY_API_TOKENS", "AUMBRY_COOKIE_SECRET", "AUMBRY_WEBDAV_URL",
		"AUMBRY_WEBDAV_USER", "AUMBRY_WEBDAV_PASS",
	} {
		t.Setenv(key, "")
	}
	_, err := load("", []string{filepath.Join(t.TempDir(), "absent.toml")}, os.Environ())
	if err == nil {
		t.Fatal("load() succeeded, want missing required settings")
	}
	// Every missing setting is reported at once with both TOML key and env name.
	for _, pair := range [][2]string{
		{"data_dir", "AUMBRY_DATA_DIR"},
		{"admin.user", "AUMBRY_ADMIN_USER"},
		{"admin.password_hash", "AUMBRY_ADMIN_PASSWORD_HASH"},
		{"api_tokens", "AUMBRY_API_TOKENS"},
		{"cookie_secret", "AUMBRY_COOKIE_SECRET"},
		{"webdav.url", "AUMBRY_WEBDAV_URL"},
		{"webdav.user", "AUMBRY_WEBDAV_USER"},
		{"webdav.password", "AUMBRY_WEBDAV_PASS"},
	} {
		if !strings.Contains(err.Error(), pair[0]) || !strings.Contains(err.Error(), pair[1]) {
			t.Errorf("load() error = %v, want it to name %s and %s", err, pair[0], pair[1])
		}
	}
}

func TestLoadTreatsWhitespaceOnlyAsMissing(t *testing.T) {
	for key, envKey := range map[string]string{
		"AUMBRY_ADMIN_USER": "   ",
		"AUMBRY_API_TOKENS": " , ,",
	} {
		t.Run(key, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv(key, envKey)
			if _, err := load("", nil, os.Environ()); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("load() error = %v, want %s rejected", err, key)
			}
		})
	}
}

func TestLoadEnvOnlyValidConfig(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("AUMBRY_API_TOKENS", " first-token, second-token ")
	t.Setenv("AUMBRY_LISTEN_ADDR", "")

	res, err := load("", []string{filepath.Join(t.TempDir(), "nope.toml")}, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if res.FilePath != "" {
		t.Fatalf("FilePath = %q, want empty for env-only mode", res.FilePath)
	}
	if res.Config.ListenAddr != ":8080" {
		t.Fatalf("ListenAddr = %q, want default :8080", res.Config.ListenAddr)
	}
	if len(res.Config.APITokens) != 2 || res.Config.APITokens[0] != "first-token" || res.Config.APITokens[1] != "second-token" {
		t.Fatalf("APITokens = %#v, want trimmed tokens", res.Config.APITokens)
	}
}

func TestLoadTOMLAndEnvOverride(t *testing.T) {
	hash := testBcryptHash(t)
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "aumbry.toml")
	content := `
listen_addr = ":9090"
data_dir = "` + filepath.ToSlash(filepath.Join(dir, "data")) + `"
api_tokens = ["amb_toml_secret_1111"]
cookie_secret = "toml-cookie-secret-at-least-32-bytes-9999"
trusted_proxies = ["10.0.0.0/8", "192.0.2.5"]

[admin]
user = "tomladmin"
password_hash = "` + hash + `"

[webdav]
url = "https://dav.example.invalid/toml"
user = "tomluser"
password = "toml-dav-password-8888"
`
	if err := os.WriteFile(tomlPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("TOML only preserves listen_addr and masks secrets", func(t *testing.T) {
		res, err := load(tomlPath, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.FilePath != tomlPath {
			t.Fatalf("FilePath = %q, want %q", res.FilePath, tomlPath)
		}
		if len(res.Warnings) != 0 {
			t.Fatalf("Warnings = %v, want none for 0600 file", res.Warnings)
		}
		if res.Config.ListenAddr != ":9090" {
			t.Fatalf("ListenAddr = %q, want :9090 from TOML", res.Config.ListenAddr)
		}
		if len(res.Config.TrustedProxyPrefixes) != 2 {
			t.Fatalf("TrustedProxyPrefixes = %v, want 2 prefixes", res.Config.TrustedProxyPrefixes)
		}
		if res.Config.TrustedProxyPrefixes[1].String() != "192.0.2.5/32" {
			t.Fatalf("single IP normalized = %s, want 192.0.2.5/32", res.Config.TrustedProxyPrefixes[1])
		}

		masked, err := res.Config.FormatMasked()
		if err != nil {
			t.Fatal(err)
		}
		for _, fullSecret := range []string{
			hash,
			"amb_toml_secret_1111",
			"toml-cookie-secret-at-least-32-bytes-9999",
			"toml-dav-password-8888",
		} {
			if strings.Contains(masked, fullSecret) {
				t.Errorf("FormatMasked() leaked full secret %q", fullSecret)
			}
		}
		for _, tail := range []string{
			"****" + hash[len(hash)-4:],
			"****1111",
			"****9999",
			"****8888",
		} {
			if !strings.Contains(masked, tail) {
				t.Errorf("FormatMasked() missing masked suffix %q in:\n%s", tail, masked)
			}
		}
	})

	t.Run("env overrides TOML keys", func(t *testing.T) {
		environ := []string{
			"AUMBRY_LISTEN_ADDR=:7070",
			"AUMBRY_ADMIN_USER=envadmin",
			"AUMBRY_API_TOKENS=amb_env_1,amb_env_2",
			"AUMBRY_TRUSTED_PROXIES=127.0.0.1, ::1",
		}
		res, err := load(tomlPath, nil, environ)
		if err != nil {
			t.Fatal(err)
		}
		if res.Config.ListenAddr != ":7070" {
			t.Errorf("ListenAddr = %q, want :7070 from env", res.Config.ListenAddr)
		}
		if res.Config.Admin.User != "envadmin" {
			t.Errorf("Admin.User = %q, want envadmin from env", res.Config.Admin.User)
		}
		if len(res.Config.APITokens) != 2 || res.Config.APITokens[0] != "amb_env_1" {
			t.Errorf("APITokens = %v, want env override", res.Config.APITokens)
		}
		if len(res.Config.TrustedProxyPrefixes) != 2 || res.Config.TrustedProxyPrefixes[1].String() != "::1/128" {
			t.Errorf("TrustedProxyPrefixes = %v, want env override with ::1/128", res.Config.TrustedProxyPrefixes)
		}
		if res.Config.WebDAV.User != "tomluser" {
			t.Errorf("WebDAV.User = %q, want tomluser kept", res.Config.WebDAV.User)
		}
	})
}

func TestLoadRejectsUnknownTOMLKey(t *testing.T) {
	hash := testBcryptHash(t)
	path := filepath.Join(t.TempDir(), "bad.toml")
	content := `
listn_addr = ":8080"
data_dir = "/tmp/data"
api_tokens = ["tok"]
cookie_secret = "01234567890123456789012345678901"

[admin]
user = "admin"
password_hash = "` + hash + `"

[webdav]
url = "https://dav.example.invalid"
user = "u"
password = "p"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := load(path, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "listn_addr") {
		t.Fatalf("load() error = %v, want unknown key listn_addr named", err)
	}
}

func TestLoadExplicitMissingPathFailsImplicitFallsBack(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")

	if _, err := load(missing, nil, nil); err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("explicit --config missing error = %v, want path error", err)
	}

	setRequiredEnvironment(t)
	if _, err := load("", nil, append(os.Environ(), "AUMBRY_CONFIG="+missing)); err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("explicit AUMBRY_CONFIG missing error = %v, want path error", err)
	}

	res, err := load("", []string{missing, filepath.Join(t.TempDir(), "also-missing.toml")}, os.Environ())
	if err != nil {
		t.Fatalf("implicit missing paths should fall back to env, got %v", err)
	}
	if res.FilePath != "" {
		t.Fatalf("FilePath = %q, want empty", res.FilePath)
	}
}

func TestLoadPermissionWarning(t *testing.T) {
	hash := testBcryptHash(t)
	path := filepath.Join(t.TempDir(), "world.toml")
	content := `
data_dir = "/tmp/data"
api_tokens = ["tok"]
cookie_secret = "01234567890123456789012345678901"
[admin]
user = "admin"
password_hash = "` + hash + `"
[webdav]
url = "https://dav.example.invalid"
user = "u"
password = "p"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Ensure actual permission isn't masked below 0644 by umask in test.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := load(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected permission warning for 0644 config file")
	}
}

func TestLoadValidationErrors(t *testing.T) {
	t.Run("short cookie secret", func(t *testing.T) {
		setRequiredEnvironment(t)
		t.Setenv("AUMBRY_COOKIE_SECRET", "short")
		if _, err := load("", nil, os.Environ()); err == nil || !strings.Contains(err.Error(), "32 bytes") {
			t.Fatalf("load() error = %v, want 32 bytes error", err)
		}
	})

	t.Run("invalid bcrypt hash", func(t *testing.T) {
		setRequiredEnvironment(t)
		t.Setenv("AUMBRY_ADMIN_PASSWORD_HASH", "not-bcrypt")
		if _, err := load("", nil, os.Environ()); err == nil || !strings.Contains(err.Error(), "bcrypt") {
			t.Fatalf("load() error = %v, want bcrypt error", err)
		}
	})

	t.Run("invalid trusted proxy", func(t *testing.T) {
		setRequiredEnvironment(t)
		t.Setenv("AUMBRY_TRUSTED_PROXIES", "10.0.0.0/8,not-an-ip")
		err := func() error {
			_, e := load("", nil, os.Environ())
			return e
		}()
		if err == nil || !strings.Contains(err.Error(), "trusted_proxies[1]") || !strings.Contains(err.Error(), "not-an-ip") {
			t.Fatalf("load() error = %v, want trusted_proxies[1] error", err)
		}
	})
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("AUMBRY_CONFIG", "")
	t.Setenv("AUMBRY_LISTEN_ADDR", "")
	t.Setenv("AUMBRY_TRUSTED_PROXIES", "")
	t.Setenv("AUMBRY_DATA_DIR", t.TempDir())
	t.Setenv("AUMBRY_ADMIN_USER", "admin")
	t.Setenv("AUMBRY_ADMIN_PASSWORD_HASH", testBcryptHash(t))
	t.Setenv("AUMBRY_API_TOKENS", "token")
	t.Setenv("AUMBRY_COOKIE_SECRET", "01234567890123456789012345678901")
	t.Setenv("AUMBRY_WEBDAV_URL", "https://webdav.example.invalid")
	t.Setenv("AUMBRY_WEBDAV_USER", "user")
	t.Setenv("AUMBRY_WEBDAV_PASS", "pass")
}
