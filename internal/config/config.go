// Package config loads and validates server configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Config contains the server's runtime settings.
type Config struct {
	ListenAddr        string
	DataDir           string
	AdminUser         string
	AdminPasswordHash string
	APITokens         []string
	CookieSecret      string
	WebDAVURL         string
	WebDAVUser        string
	WebDAVPassword    string
}

// Load reads and validates the required AUMBRY_* environment variables.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:        envOr("AUMBRY_LISTEN_ADDR", ":8080"),
		DataDir:           strings.TrimSpace(os.Getenv("AUMBRY_DATA_DIR")),
		AdminUser:         strings.TrimSpace(os.Getenv("AUMBRY_ADMIN_USER")),
		AdminPasswordHash: strings.TrimSpace(os.Getenv("AUMBRY_ADMIN_PASSWORD_HASH")),
		APITokens:         splitTokens(os.Getenv("AUMBRY_API_TOKENS")),
		CookieSecret:      os.Getenv("AUMBRY_COOKIE_SECRET"),
		WebDAVURL:         strings.TrimSpace(os.Getenv("AUMBRY_WEBDAV_URL")),
		WebDAVUser:        strings.TrimSpace(os.Getenv("AUMBRY_WEBDAV_USER")),
		WebDAVPassword:    os.Getenv("AUMBRY_WEBDAV_PASS"),
	}

	var missing []string
	required := []struct {
		key   string
		value string
	}{
		{"AUMBRY_DATA_DIR", cfg.DataDir},
		{"AUMBRY_ADMIN_USER", cfg.AdminUser},
		{"AUMBRY_ADMIN_PASSWORD_HASH", cfg.AdminPasswordHash},
		{"AUMBRY_COOKIE_SECRET", cfg.CookieSecret},
		{"AUMBRY_WEBDAV_URL", cfg.WebDAVURL},
		{"AUMBRY_WEBDAV_USER", cfg.WebDAVUser},
		{"AUMBRY_WEBDAV_PASS", cfg.WebDAVPassword},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			missing = append(missing, field.key)
		}
	}
	if len(cfg.APITokens) == 0 {
		missing = append(missing, "AUMBRY_API_TOKENS")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	if len(cfg.CookieSecret) < 32 {
		return Config{}, errors.New("AUMBRY_COOKIE_SECRET must contain at least 32 bytes")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitTokens(value string) []string {
	var tokens []string
	for _, token := range strings.Split(value, ",") {
		if token = strings.TrimSpace(token); token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}
