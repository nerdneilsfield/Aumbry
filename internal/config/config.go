// Package config loads and validates server configuration from TOML files and the environment.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/crypto/bcrypt"
)

const defaultListenAddr = ":8080"

var defaultConfigPaths = []string{"./aumbry.toml", "/etc/aumbry/config.toml"}

// AdminConfig holds administrator credentials.
type AdminConfig struct {
	User         string `toml:"user" env:"AUMBRY_ADMIN_USER"`
	PasswordHash string `toml:"password_hash" env:"AUMBRY_ADMIN_PASSWORD_HASH"`
}

// WebDAVConfig holds private WebDAV object storage settings.
type WebDAVConfig struct {
	URL      string `toml:"url" env:"AUMBRY_WEBDAV_URL"`
	User     string `toml:"user" env:"AUMBRY_WEBDAV_USER"`
	Password string `toml:"password" env:"AUMBRY_WEBDAV_PASS"`
}

// Config contains the server's runtime settings.
type Config struct {
	ListenAddr           string         `toml:"listen_addr" env:"AUMBRY_LISTEN_ADDR"`
	DataDir              string         `toml:"data_dir" env:"AUMBRY_DATA_DIR"`
	Admin                AdminConfig    `toml:"admin"`
	APITokens            []string       `toml:"api_tokens" env:"AUMBRY_API_TOKENS" envSeparator:","`
	CookieSecret         string         `toml:"cookie_secret" env:"AUMBRY_COOKIE_SECRET"`
	WebDAV               WebDAVConfig   `toml:"webdav"`
	TrustedProxies       []string       `toml:"trusted_proxies" env:"AUMBRY_TRUSTED_PROXIES" envSeparator:","`
	TrustedProxyPrefixes []netip.Prefix `toml:"-" env:"-"`
}

// LoadResult holds the resolved configuration along with file discovery metadata.
type LoadResult struct {
	Config   Config
	FilePath string
	Warnings []string
}

// verbatim lists the environment secrets that are used exactly as given; every other value is trimmed.
var verbatim = map[string]bool{
	"AUMBRY_COOKIE_SECRET": true,
	"AUMBRY_WEBDAV_PASS":   true,
}

// Load reads and validates configuration from an optional TOML file and environment overrides.
func Load(configPath string) (LoadResult, error) {
	return load(configPath, defaultConfigPaths, os.Environ())
}

func load(configPath string, defaultPaths []string, environ []string) (LoadResult, error) {
	environment := buildEnvironment(environ)

	filePath, explicit, err := resolveConfigPath(configPath, environment["AUMBRY_CONFIG"], defaultPaths)
	if err != nil {
		return LoadResult{}, err
	}

	var result LoadResult
	if filePath != "" {
		info, err := os.Stat(filePath)
		if err != nil {
			if explicit {
				return LoadResult{}, fmt.Errorf("config file %s: %w", filePath, err)
			}
			return LoadResult{}, fmt.Errorf("stat config file %s: %w", filePath, err)
		}
		if info.IsDir() {
			return LoadResult{}, fmt.Errorf("config path %s is a directory", filePath)
		}
		if info.Mode().Perm()&0o044 != 0 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("config file %s is readable by group or others (mode %04o); consider chmod 600", filePath, info.Mode().Perm()))
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return LoadResult{}, fmt.Errorf("read config file %s: %w", filePath, err)
		}
		if err := decodeStrictTOML(filePath, data, &result.Config); err != nil {
			return LoadResult{}, err
		}
		result.FilePath = filePath
	}

	if err := env.ParseWithOptions(&result.Config, env.Options{Environment: environment}); err != nil {
		return LoadResult{}, fmt.Errorf("parse environment configuration: %w", err)
	}

	normalizeAndApplyDefaults(&result.Config)
	if err := validateConfig(&result.Config); err != nil {
		return LoadResult{}, err
	}
	return result, nil
}

func buildEnvironment(environ []string) map[string]string {
	raw := env.ToMap(environ)
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		if verbatim[key] {
			out[key] = value
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func resolveConfigPath(flagPath, envPath string, defaultPaths []string) (path string, explicit bool, err error) {
	if p := strings.TrimSpace(flagPath); p != "" {
		return p, true, nil
	}
	if p := strings.TrimSpace(envPath); p != "" {
		return p, true, nil
	}
	for _, candidate := range defaultPaths {
		_, statErr := os.Stat(candidate)
		if statErr == nil {
			return candidate, false, nil
		}
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		return "", false, fmt.Errorf("stat config file %s: %w", candidate, statErr)
	}
	return "", false, nil
}

func decodeStrictTOML(filePath string, data []byte, dst *Config) error {
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var strictErr *toml.StrictMissingError
		if errors.As(err, &strictErr) {
			keys := make([]string, 0, len(strictErr.Errors))
			for _, de := range strictErr.Errors {
				keys = append(keys, strings.Join(de.Key(), "."))
			}
			return fmt.Errorf("unknown configuration key(s) in %s: %s", filePath, strings.Join(keys, ", "))
		}
		return fmt.Errorf("parse config file %s: %w", filePath, err)
	}
	return nil
}

func normalizeAndApplyDefaults(cfg *Config) {
	cfg.ListenAddr = strings.TrimSpace(cfg.ListenAddr)
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
	}
	cfg.DataDir = strings.TrimSpace(cfg.DataDir)
	cfg.Admin.User = strings.TrimSpace(cfg.Admin.User)
	cfg.Admin.PasswordHash = strings.TrimSpace(cfg.Admin.PasswordHash)
	cfg.WebDAV.URL = strings.TrimSpace(cfg.WebDAV.URL)
	cfg.WebDAV.User = strings.TrimSpace(cfg.WebDAV.User)

	cfg.APITokens = compactTrimmed(cfg.APITokens)
	cfg.TrustedProxies = compactTrimmed(cfg.TrustedProxies)
}

func compactTrimmed(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func validateConfig(cfg *Config) error {
	var missing []string
	checkReq := func(val, label string) {
		if val == "" {
			missing = append(missing, label)
		}
	}
	checkReq(cfg.DataDir, "data_dir (AUMBRY_DATA_DIR)")
	checkReq(cfg.Admin.User, "admin.user (AUMBRY_ADMIN_USER)")
	checkReq(cfg.Admin.PasswordHash, "admin.password_hash (AUMBRY_ADMIN_PASSWORD_HASH)")
	if len(cfg.APITokens) == 0 {
		missing = append(missing, "api_tokens (AUMBRY_API_TOKENS)")
	}
	checkReq(cfg.CookieSecret, "cookie_secret (AUMBRY_COOKIE_SECRET)")
	checkReq(cfg.WebDAV.URL, "webdav.url (AUMBRY_WEBDAV_URL)")
	checkReq(cfg.WebDAV.User, "webdav.user (AUMBRY_WEBDAV_USER)")
	checkReq(cfg.WebDAV.Password, "webdav.password (AUMBRY_WEBDAV_PASS)")

	var errs []error
	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", ")))
	}
	if cfg.CookieSecret != "" && len(cfg.CookieSecret) < 32 {
		errs = append(errs, errors.New("cookie_secret (AUMBRY_COOKIE_SECRET) must contain at least 32 bytes"))
	}
	if cfg.Admin.PasswordHash != "" {
		if _, err := bcrypt.Cost([]byte(cfg.Admin.PasswordHash)); err != nil {
			errs = append(errs, fmt.Errorf("admin.password_hash (AUMBRY_ADMIN_PASSWORD_HASH) is not a valid bcrypt hash: %w", err))
		}
	}
	if cfg.WebDAV.URL != "" {
		if u, err := url.Parse(cfg.WebDAV.URL); err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, errors.New("webdav.url (AUMBRY_WEBDAV_URL) must be an absolute URL with scheme and host"))
		}
	}

	prefixes := make([]netip.Prefix, 0, len(cfg.TrustedProxies))
	for i, entry := range cfg.TrustedProxies {
		prefix, err := parseTrustedProxy(entry)
		if err != nil {
			errs = append(errs, fmt.Errorf("trusted_proxies[%d] (%q): %w", i, entry, err))
			continue
		}
		prefixes = append(prefixes, prefix)
	}
	cfg.TrustedProxyPrefixes = prefixes
	return errors.Join(errs...)
}

func parseTrustedProxy(raw string) (netip.Prefix, error) {
	if strings.Contains(raw, "/") {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return netip.Prefix{}, err
		}
		if p.Addr().Is4In6() && p.Bits() >= 96 {
			return netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96).Masked(), nil
		}
		return p.Masked(), nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, err
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// Masked returns a copy of Config with secrets replaced by masks preserving at most the last 4 characters.
func (c Config) Masked() Config {
	masked := c
	masked.Admin.PasswordHash = maskSecret(c.Admin.PasswordHash)
	masked.CookieSecret = maskSecret(c.CookieSecret)
	masked.WebDAV.Password = maskSecret(c.WebDAV.Password)

	tokens := make([]string, len(c.APITokens))
	for i, t := range c.APITokens {
		tokens[i] = maskSecret(t)
	}
	masked.APITokens = tokens

	proxies := make([]string, len(c.TrustedProxies))
	copy(proxies, c.TrustedProxies)
	masked.TrustedProxies = proxies
	masked.TrustedProxyPrefixes = nil
	return masked
}

// FormatMasked formats the masked configuration as TOML.
func (c Config) FormatMasked() (string, error) {
	out, err := toml.Marshal(c.Masked())
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}
