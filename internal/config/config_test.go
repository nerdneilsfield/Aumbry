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
	clearConfigEnv(t)
	t.Run("default local missing core required settings", func(t *testing.T) {
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
		} {
			if !strings.Contains(err.Error(), pair[0]) || !strings.Contains(err.Error(), pair[1]) {
				t.Errorf("load() error = %v, want it to name %s and %s", err, pair[0], pair[1])
			}
		}
		if strings.Contains(err.Error(), "webdav.") {
			t.Errorf("default local storage should not require webdav fields: %v", err)
		}
	})

	t.Run("webdav backend lists all missing webdav fields", func(t *testing.T) {
		clearConfigEnv(t)
		setBaseEnvironment(t)
		t.Setenv("AUMBRY_STORAGE", "webdav")
		_, err := load("", []string{filepath.Join(t.TempDir(), "absent.toml")}, os.Environ())
		if err == nil {
			t.Fatal("load() succeeded, want missing webdav settings")
		}
		for _, pair := range [][2]string{
			{"webdav.url", "AUMBRY_WEBDAV_URL"},
			{"webdav.user", "AUMBRY_WEBDAV_USER"},
			{"webdav.password", "AUMBRY_WEBDAV_PASS"},
		} {
			if !strings.Contains(err.Error(), pair[0]) || !strings.Contains(err.Error(), pair[1]) {
				t.Errorf("load() error = %v, want it to name %s and %s", err, pair[0], pair[1])
			}
		}
	})

	t.Run("s3 backend without endpoint lists all missing s3 fields", func(t *testing.T) {
		clearConfigEnv(t)
		setBaseEnvironment(t)
		t.Setenv("AUMBRY_STORAGE", "s3")
		_, err := load("", []string{filepath.Join(t.TempDir(), "absent.toml")}, os.Environ())
		if err == nil {
			t.Fatal("load() succeeded, want missing s3 settings")
		}
		for _, pair := range [][2]string{
			{"s3.region", "AUMBRY_S3_REGION"},
			{"s3.bucket", "AUMBRY_S3_BUCKET"},
			{"s3.access_key_id", "AUMBRY_S3_ACCESS_KEY_ID"},
			{"s3.secret_access_key", "AUMBRY_S3_SECRET_ACCESS_KEY"},
		} {
			if !strings.Contains(err.Error(), pair[0]) || !strings.Contains(err.Error(), pair[1]) {
				t.Errorf("load() error = %v, want it to name %s and %s", err, pair[0], pair[1])
			}
		}
	})

	t.Run("r2 backend lists all missing r2 fields", func(t *testing.T) {
		clearConfigEnv(t)
		setBaseEnvironment(t)
		t.Setenv("AUMBRY_STORAGE", "r2")
		_, err := load("", []string{filepath.Join(t.TempDir(), "absent.toml")}, os.Environ())
		if err == nil {
			t.Fatal("load() succeeded, want missing r2 settings")
		}
		for _, pair := range [][2]string{
			{"r2.account_id", "AUMBRY_R2_ACCOUNT_ID"},
			{"r2.bucket", "AUMBRY_R2_BUCKET"},
			{"r2.access_key_id", "AUMBRY_R2_ACCESS_KEY_ID"},
			{"r2.secret_access_key", "AUMBRY_R2_SECRET_ACCESS_KEY"},
		} {
			if !strings.Contains(err.Error(), pair[0]) || !strings.Contains(err.Error(), pair[1]) {
				t.Errorf("load() error = %v, want it to name %s and %s", err, pair[0], pair[1])
			}
		}
	})
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
storage = "webdav"
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
storage = "webdav"
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
storage = "webdav"
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
		for _, bad := range []string{
			"not-bcrypt",
			"$2y$10$" + strings.Repeat("!", 53),
			"$2y$10$" + strings.Repeat("a", 52),
			"$2y$10$" + strings.Repeat("a", 54),
			"$1$10$" + strings.Repeat("a", 53),
			"$2z$10$" + strings.Repeat("a", 53),
			"$2y$10X" + strings.Repeat("a", 53),
		} {
			setRequiredEnvironment(t)
			t.Setenv("AUMBRY_ADMIN_PASSWORD_HASH", bad)
			if _, err := load("", nil, os.Environ()); err == nil || !strings.Contains(err.Error(), "bcrypt") {
				t.Fatalf("hash %q: load() error = %v, want bcrypt error", bad, err)
			}
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

	t.Run("unset storage with non-local backend config fails", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			env  map[string]string
		}{
			{"webdav url", map[string]string{"AUMBRY_WEBDAV_URL": "https://dav.example.invalid"}},
			{"s3 bucket", map[string]string{"AUMBRY_S3_BUCKET": "aumbry"}},
			{"s3 path_style", map[string]string{"AUMBRY_S3_PATH_STYLE": "true"}},
			{"r2 account_id", map[string]string{"AUMBRY_R2_ACCOUNT_ID": "0123456789abcdef0123456789abcdef"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				clearConfigEnv(t)
				setBaseEnvironment(t)
				for k, v := range tc.env {
					t.Setenv(k, v)
				}
				if _, err := load("", nil, os.Environ()); err == nil || !strings.Contains(err.Error(), "AUMBRY_STORAGE") {
					t.Fatalf("load() error = %v, want storage hint", err)
				}
			})
		}
	})

	t.Run("default storage is local and defaults objects dir", func(t *testing.T) {
		clearConfigEnv(t)
		dataDir := t.TempDir()
		setBaseEnvironment(t)
		t.Setenv("AUMBRY_DATA_DIR", dataDir)

		res, err := load("", nil, os.Environ())
		if err != nil {
			t.Fatalf("load() default local error = %v", err)
		}
		if res.Config.Storage != "local" {
			t.Fatalf("Storage = %q, want local", res.Config.Storage)
		}
		wantPath := filepath.Join(dataDir, "objects")
		if res.Config.ResolvedLocalPath() != wantPath || res.Config.Local.Path != wantPath {
			t.Fatalf("local path = (%q, %q), want %q", res.Config.Local.Path, res.Config.ResolvedLocalPath(), wantPath)
		}
	})

	t.Run("invalid r2 account_id fails and valid r2 resolves endpoint and auto region", func(t *testing.T) {
		for _, bad := range []string{"0123456789abcdef0123456789abcde", "0123456789abcdef0123456789abcdef0", "0123456789abcdef0123456789abcdeg", "0123456789abcdef-123456789abcdef"} {
			clearConfigEnv(t)
			setBaseEnvironment(t)
			t.Setenv("AUMBRY_STORAGE", "r2")
			t.Setenv("AUMBRY_R2_ACCOUNT_ID", bad)
			t.Setenv("AUMBRY_R2_BUCKET", "aumbry")
			t.Setenv("AUMBRY_R2_ACCESS_KEY_ID", "ak_r2_123456")
			t.Setenv("AUMBRY_R2_SECRET_ACCESS_KEY", "sk_r2_secret_7890")
			if _, err := load("", nil, os.Environ()); err == nil || !strings.Contains(err.Error(), "r2.account_id") {
				t.Fatalf("account_id %q: load() error = %v, want r2.account_id error", bad, err)
			}
		}

		clearConfigEnv(t)
		setBaseEnvironment(t)
		t.Setenv("AUMBRY_STORAGE", "r2")
		t.Setenv("AUMBRY_R2_ACCOUNT_ID", "0123456789ABCDEF0123456789abcdef")
		t.Setenv("AUMBRY_R2_BUCKET", "aumbry")
		t.Setenv("AUMBRY_R2_ACCESS_KEY_ID", "ak_r2_123456")
		t.Setenv("AUMBRY_R2_SECRET_ACCESS_KEY", "sk_r2_secret_7890")

		res, err := load("", nil, os.Environ())
		if err != nil {
			t.Fatalf("valid r2 config failed: %v", err)
		}
		resolved := res.Config.ResolvedS3()
		if resolved.Endpoint != "https://0123456789abcdef0123456789abcdef.r2.cloudflarestorage.com" || resolved.Region != "auto" || resolved.PathStyle {
			t.Fatalf("ResolvedS3() = %+v", resolved)
		}
		masked, err := res.Config.FormatMasked()
		if err != nil {
			t.Fatal(err)
		}
		for _, full := range []string{"ak_r2_123456", "sk_r2_secret_7890"} {
			if strings.Contains(masked, full) {
				t.Errorf("FormatMasked leaked R2 credential %q", full)
			}
		}
		for _, tail := range []string{"****3456", "****7890"} {
			if !strings.Contains(masked, tail) {
				t.Errorf("FormatMasked missing R2 mask %q in:\n%s", tail, masked)
			}
		}
	})

	t.Run("s3 endpoint validation, default auto region, and masking", func(t *testing.T) {
		for _, badEndpoint := range []string{
			"http://minio:9000/sub",
			"http://minio:9000/",
			"http://minio:9000?foo=bar",
			"http://user:pass@minio:9000",
			"ftp://minio:9000",
			"minio:9000",
		} {
			clearConfigEnv(t)
			setBaseEnvironment(t)
			t.Setenv("AUMBRY_STORAGE", "s3")
			t.Setenv("AUMBRY_S3_ENDPOINT", badEndpoint)
			t.Setenv("AUMBRY_S3_BUCKET", "aumbry")
			t.Setenv("AUMBRY_S3_ACCESS_KEY_ID", "ak_s3_1111")
			t.Setenv("AUMBRY_S3_SECRET_ACCESS_KEY", "sk_s3_2222")
			if _, err := load("", nil, os.Environ()); err == nil || !strings.Contains(err.Error(), "s3.endpoint") {
				t.Fatalf("endpoint %q: load() error = %v, want s3.endpoint error", badEndpoint, err)
			}
		}

		clearConfigEnv(t)
		setBaseEnvironment(t)
		t.Setenv("AUMBRY_STORAGE", "s3")
		t.Setenv("AUMBRY_S3_ENDPOINT", "http://minio:9000")
		t.Setenv("AUMBRY_S3_BUCKET", "aumbry")
		t.Setenv("AUMBRY_S3_ACCESS_KEY_ID", "ak_s3_1111")
		t.Setenv("AUMBRY_S3_SECRET_ACCESS_KEY", "sk_s3_2222")
		res, err := load("", nil, os.Environ())
		if err != nil {
			t.Fatalf("s3 with endpoint and omitted region failed: %v", err)
		}
		if res.Config.S3.Region != "auto" || res.Config.ResolvedS3().Region != "auto" {
			t.Fatalf("Region = %q, want auto", res.Config.S3.Region)
		}
		masked, err := res.Config.FormatMasked()
		if err != nil {
			t.Fatal(err)
		}
		for _, full := range []string{"ak_s3_1111", "sk_s3_2222"} {
			if strings.Contains(masked, full) {
				t.Errorf("FormatMasked leaked S3 credential %q", full)
			}
		}
		for _, tail := range []string{"****1111", "****2222"} {
			if !strings.Contains(masked, tail) {
				t.Errorf("FormatMasked missing S3 mask %q in:\n%s", tail, masked)
			}
		}
	})

	t.Run("public_url, revisions.max_per_item, and ingest.max_body_mb defaults and validation", func(t *testing.T) {
		clearConfigEnv(t)
		setBaseEnvironment(t)
		res, err := load("", nil, os.Environ())
		if err != nil {
			t.Fatalf("defaults load() = %v", err)
		}
		if res.Config.PublicURL != "" || res.Config.ResolvedMaxRevisionsPerItem() != 20 ||
			res.Config.ResolvedMaxIngestBodyBytes() != 200*1024*1024 || !res.Config.ResolvedAllowRemoteImages() {
			t.Fatalf("unexpected defaults: %+v", res.Config)
		}

		// Trailing slash trimmed on valid public_url; zero max_per_item and 20MB min ingest allowed.
		clearConfigEnv(t)
		setBaseEnvironment(t)
		t.Setenv("AUMBRY_PUBLIC_URL", "https://reports.example.com/")
		t.Setenv("AUMBRY_REVISIONS_MAX_PER_ITEM", "0")
		t.Setenv("AUMBRY_INGEST_MAX_BODY_MB", "20")
		t.Setenv("AUMBRY_INGEST_ALLOW_REMOTE_IMAGES", "false")
		res, err = load("", nil, os.Environ())
		if err != nil {
			t.Fatalf("valid overrides load() = %v", err)
		}
		if res.Config.ResolvedAllowRemoteImages() {
			t.Fatal("ResolvedAllowRemoteImages() = true, want false from env override")
		}
		if res.Config.PublicURL != "https://reports.example.com" {
			t.Fatalf("PublicURL = %q, want trimmed URL", res.Config.PublicURL)
		}
		if res.Config.ResolvedMaxRevisionsPerItem() != 0 {
			t.Fatalf("MaxRevisions = %d, want 0 (unlimited)", res.Config.ResolvedMaxRevisionsPerItem())
		}
		if res.Config.ResolvedMaxIngestBodyBytes() != 20*1024*1024 {
			t.Fatalf("MaxIngestBodyBytes = %d, want 20MiB", res.Config.ResolvedMaxIngestBodyBytes())
		}

		// Invalid public_url rejected.
		for _, badURL := range []string{
			"https://example.com/sub",
			"https://example.com?q=1",
			"https://example.com#frag",
			"https://user:pass@example.com",
			"ftp://example.com",
		} {
			clearConfigEnv(t)
			setBaseEnvironment(t)
			t.Setenv("AUMBRY_PUBLIC_URL", badURL)
			if _, err = load("", nil, os.Environ()); err == nil || !strings.Contains(err.Error(), "public_url") {
				t.Fatalf("public_url %q: err = %v, want public_url error", badURL, err)
			}
		}

		// Negative revisions.max_per_item rejected via env and TOML.
		clearConfigEnv(t)
		setBaseEnvironment(t)
		t.Setenv("AUMBRY_REVISIONS_MAX_PER_ITEM", "-1")
		if _, err = load("", nil, os.Environ()); err == nil || !strings.Contains(err.Error(), "revisions.max_per_item") {
			t.Fatalf("negative max_per_item err = %v", err)
		}

		// ingest.max_body_mb 1..19 rejected.
		clearConfigEnv(t)
		setBaseEnvironment(t)
		t.Setenv("AUMBRY_INGEST_MAX_BODY_MB", "19")
		if _, err = load("", nil, os.Environ()); err == nil || !strings.Contains(err.Error(), "ingest.max_body_mb") {
			t.Fatalf("19 MiB max_body_mb err = %v", err)
		}

		// TOML overrides for zero and default fallback.
		clearConfigEnv(t)
		setBaseEnvironment(t)
		cfgFile := filepath.Join(t.TempDir(), "aumbry.toml")
		tomlBody := "public_url = \"http://localhost:8080/\"\n[revisions]\nmax_per_item = 0\n[ingest]\nmax_body_mb = 500\nallow_remote_images = false\n"
		if writeErr := os.WriteFile(cfgFile, []byte(tomlBody), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		res, err = load(cfgFile, nil, os.Environ())
		if err != nil {
			t.Fatalf("TOML load() = %v", err)
		}
		if res.Config.PublicURL != "http://localhost:8080" || res.Config.ResolvedMaxRevisionsPerItem() != 0 ||
			res.Config.ResolvedMaxIngestBodyBytes() != 500*1024*1024 || res.Config.ResolvedAllowRemoteImages() {
			t.Fatalf("unexpected TOML config: %+v", res.Config)
		}
	})
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AUMBRY_CONFIG", "AUMBRY_LISTEN_ADDR", "AUMBRY_PUBLIC_URL", "AUMBRY_DATA_DIR", "AUMBRY_STORAGE",
		"AUMBRY_ADMIN_USER", "AUMBRY_ADMIN_PASSWORD_HASH", "AUMBRY_API_TOKENS",
		"AUMBRY_COOKIE_SECRET", "AUMBRY_TRUSTED_PROXIES", "AUMBRY_REVISIONS_MAX_PER_ITEM",
		"AUMBRY_INGEST_MAX_BODY_MB", "AUMBRY_INGEST_ALLOW_REMOTE_IMAGES", "AUMBRY_LOCAL_PATH",
		"AUMBRY_WEBDAV_URL", "AUMBRY_WEBDAV_USER", "AUMBRY_WEBDAV_PASS",
		"AUMBRY_S3_ENDPOINT", "AUMBRY_S3_REGION", "AUMBRY_S3_BUCKET",
		"AUMBRY_S3_ACCESS_KEY_ID", "AUMBRY_S3_SECRET_ACCESS_KEY", "AUMBRY_S3_PATH_STYLE",
		"AUMBRY_R2_ACCOUNT_ID", "AUMBRY_R2_BUCKET",
		"AUMBRY_R2_ACCESS_KEY_ID", "AUMBRY_R2_SECRET_ACCESS_KEY",
	} {
		t.Setenv(key, "")
	}
}

func setBaseEnvironment(t *testing.T) {
	t.Helper()
	clearConfigEnv(t)
	t.Setenv("AUMBRY_DATA_DIR", t.TempDir())
	t.Setenv("AUMBRY_ADMIN_USER", "admin")
	t.Setenv("AUMBRY_ADMIN_PASSWORD_HASH", testBcryptHash(t))
	t.Setenv("AUMBRY_API_TOKENS", "token")
	t.Setenv("AUMBRY_COOKIE_SECRET", "01234567890123456789012345678901")
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	setBaseEnvironment(t)
	t.Setenv("AUMBRY_STORAGE", "webdav")
	t.Setenv("AUMBRY_WEBDAV_URL", "https://webdav.example.invalid")
	t.Setenv("AUMBRY_WEBDAV_USER", "user")
	t.Setenv("AUMBRY_WEBDAV_PASS", "pass")
}
