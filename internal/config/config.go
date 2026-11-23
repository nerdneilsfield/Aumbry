// Package config loads and validates server configuration from TOML files and the environment.
package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/nerdneilsfield/aumbry/internal/storage"
)

const (
	defaultListenAddr          = ":8080"
	defaultStorage             = "local"
	defaultMaxRevisionsPerItem = 20
	defaultMaxIngestBodyMB     = 200
	minIngestBodyMB            = 20
	bcryptAlphabet             = "./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

var (
	defaultConfigPaths = []string{"./aumbry.toml", "/etc/aumbry/config.toml"}
	bcryptEncoding     = base64.NewEncoding(bcryptAlphabet).WithPadding(base64.NoPadding)
	r2AccountIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
)

// AdminConfig holds administrator credentials.
type AdminConfig struct {
	User         string `toml:"user" env:"AUMBRY_ADMIN_USER"`
	PasswordHash string `toml:"password_hash" env:"AUMBRY_ADMIN_PASSWORD_HASH"`
}

// LocalStorageConfig holds local directory object storage settings.
type LocalStorageConfig struct {
	Path string `toml:"path" env:"AUMBRY_LOCAL_PATH"`
}

// WebDAVConfig holds private WebDAV object storage settings.
type WebDAVConfig struct {
	URL      string `toml:"url" env:"AUMBRY_WEBDAV_URL"`
	User     string `toml:"user" env:"AUMBRY_WEBDAV_USER"`
	Password string `toml:"password" env:"AUMBRY_WEBDAV_PASS"`
}

// S3Config holds S3-compatible object storage settings.
type S3Config struct {
	Endpoint        string `toml:"endpoint" env:"AUMBRY_S3_ENDPOINT"`
	Region          string `toml:"region" env:"AUMBRY_S3_REGION"`
	Bucket          string `toml:"bucket" env:"AUMBRY_S3_BUCKET"`
	AccessKeyID     string `toml:"access_key_id" env:"AUMBRY_S3_ACCESS_KEY_ID"`
	SecretAccessKey string `toml:"secret_access_key" env:"AUMBRY_S3_SECRET_ACCESS_KEY"`
	PathStyle       bool   `toml:"path_style" env:"AUMBRY_S3_PATH_STYLE"`
}

// R2Config holds Cloudflare R2 object storage settings.
type R2Config struct {
	AccountID       string `toml:"account_id" env:"AUMBRY_R2_ACCOUNT_ID"`
	Bucket          string `toml:"bucket" env:"AUMBRY_R2_BUCKET"`
	AccessKeyID     string `toml:"access_key_id" env:"AUMBRY_R2_ACCESS_KEY_ID"`
	SecretAccessKey string `toml:"secret_access_key" env:"AUMBRY_R2_SECRET_ACCESS_KEY"`
}

// RevisionsConfig holds item revision history retention settings.
type RevisionsConfig struct {
	MaxPerItem *int `toml:"max_per_item" env:"AUMBRY_REVISIONS_MAX_PER_ITEM"`
}

// IngestConfig holds ingest request and remote image localization limits.
type IngestConfig struct {
	MaxBodyMB         int   `toml:"max_body_mb" env:"AUMBRY_INGEST_MAX_BODY_MB"`
	AllowRemoteImages *bool `toml:"allow_remote_images" env:"AUMBRY_INGEST_ALLOW_REMOTE_IMAGES"`
}

// Config contains the server's runtime settings.
type Config struct {
	ListenAddr           string             `toml:"listen_addr" env:"AUMBRY_LISTEN_ADDR"`
	PublicURL            string             `toml:"public_url" env:"AUMBRY_PUBLIC_URL"`
	DataDir              string             `toml:"data_dir" env:"AUMBRY_DATA_DIR"`
	Storage              string             `toml:"storage" env:"AUMBRY_STORAGE"`
	Admin                AdminConfig        `toml:"admin"`
	APITokens            []string           `toml:"api_tokens" env:"AUMBRY_API_TOKENS" envSeparator:","`
	CookieSecret         string             `toml:"cookie_secret" env:"AUMBRY_COOKIE_SECRET"`
	TrustedProxies       []string           `toml:"trusted_proxies" env:"AUMBRY_TRUSTED_PROXIES" envSeparator:","`
	Revisions            RevisionsConfig    `toml:"revisions"`
	Ingest               IngestConfig       `toml:"ingest"`
	Local                LocalStorageConfig `toml:"local"`
	WebDAV               WebDAVConfig       `toml:"webdav"`
	S3                   S3Config           `toml:"s3"`
	R2                   R2Config           `toml:"r2"`
	TrustedProxyPrefixes []netip.Prefix     `toml:"-" env:"-"`
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

	normalizeStrings(&result.Config)
	if err := validateAndApplyDefaults(&result.Config); err != nil {
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

func normalizeStrings(cfg *Config) {
	cfg.ListenAddr = strings.TrimSpace(cfg.ListenAddr)
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
	}
	cfg.PublicURL = strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/")
	cfg.DataDir = strings.TrimSpace(cfg.DataDir)
	cfg.Storage = strings.TrimSpace(cfg.Storage)
	cfg.Admin.User = strings.TrimSpace(cfg.Admin.User)
	cfg.Admin.PasswordHash = strings.TrimSpace(cfg.Admin.PasswordHash)
	cfg.Local.Path = strings.TrimSpace(cfg.Local.Path)
	cfg.WebDAV.URL = strings.TrimSpace(cfg.WebDAV.URL)
	cfg.WebDAV.User = strings.TrimSpace(cfg.WebDAV.User)
	cfg.S3.Endpoint = strings.TrimSpace(cfg.S3.Endpoint)
	cfg.S3.Region = strings.TrimSpace(cfg.S3.Region)
	cfg.S3.Bucket = strings.TrimSpace(cfg.S3.Bucket)
	cfg.S3.AccessKeyID = strings.TrimSpace(cfg.S3.AccessKeyID)
	cfg.S3.SecretAccessKey = strings.TrimSpace(cfg.S3.SecretAccessKey)
	cfg.R2.AccountID = strings.TrimSpace(cfg.R2.AccountID)
	cfg.R2.Bucket = strings.TrimSpace(cfg.R2.Bucket)
	cfg.R2.AccessKeyID = strings.TrimSpace(cfg.R2.AccessKeyID)
	cfg.R2.SecretAccessKey = strings.TrimSpace(cfg.R2.SecretAccessKey)

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

func hasNonLocalBackendConfig(cfg *Config) bool {
	if cfg.WebDAV.URL != "" || cfg.WebDAV.User != "" || cfg.WebDAV.Password != "" {
		return true
	}
	if cfg.S3.Endpoint != "" || cfg.S3.Region != "" || cfg.S3.Bucket != "" ||
		cfg.S3.AccessKeyID != "" || cfg.S3.SecretAccessKey != "" || cfg.S3.PathStyle {
		return true
	}
	if cfg.R2.AccountID != "" || cfg.R2.Bucket != "" ||
		cfg.R2.AccessKeyID != "" || cfg.R2.SecretAccessKey != "" {
		return true
	}
	return false
}

func validateAndApplyDefaults(cfg *Config) error {
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

	var errs []error
	if cfg.Storage == "" {
		if hasNonLocalBackendConfig(cfg) {
			errs = append(errs, errors.New("storage (AUMBRY_STORAGE) must be set when webdav, s3, or r2 settings are provided"))
		} else {
			cfg.Storage = defaultStorage
		}
	}

	switch cfg.Storage {
	case "local":
		if cfg.Local.Path == "" && cfg.DataDir != "" {
			cfg.Local.Path = filepath.Join(cfg.DataDir, "objects")
		}
	case "webdav":
		checkReq(cfg.WebDAV.URL, "webdav.url (AUMBRY_WEBDAV_URL)")
		checkReq(cfg.WebDAV.User, "webdav.user (AUMBRY_WEBDAV_USER)")
		checkReq(cfg.WebDAV.Password, "webdav.password (AUMBRY_WEBDAV_PASS)")
		if cfg.WebDAV.URL != "" {
			if u, err := url.Parse(cfg.WebDAV.URL); err != nil || u.Scheme == "" || u.Host == "" {
				errs = append(errs, errors.New("webdav.url (AUMBRY_WEBDAV_URL) must be an absolute URL with scheme and host"))
			}
		}
	case "s3":
		if cfg.S3.Endpoint != "" {
			if err := storage.ValidateS3Endpoint(cfg.S3.Endpoint); err != nil {
				errs = append(errs, fmt.Errorf("s3.endpoint (AUMBRY_S3_ENDPOINT) %w", err))
			}
			if cfg.S3.Region == "" {
				cfg.S3.Region = "auto"
			}
		} else {
			checkReq(cfg.S3.Region, "s3.region (AUMBRY_S3_REGION)")
		}
		checkReq(cfg.S3.Bucket, "s3.bucket (AUMBRY_S3_BUCKET)")
		checkReq(cfg.S3.AccessKeyID, "s3.access_key_id (AUMBRY_S3_ACCESS_KEY_ID)")
		checkReq(cfg.S3.SecretAccessKey, "s3.secret_access_key (AUMBRY_S3_SECRET_ACCESS_KEY)")
	case "r2":
		checkReq(cfg.R2.AccountID, "r2.account_id (AUMBRY_R2_ACCOUNT_ID)")
		checkReq(cfg.R2.Bucket, "r2.bucket (AUMBRY_R2_BUCKET)")
		checkReq(cfg.R2.AccessKeyID, "r2.access_key_id (AUMBRY_R2_ACCESS_KEY_ID)")
		checkReq(cfg.R2.SecretAccessKey, "r2.secret_access_key (AUMBRY_R2_SECRET_ACCESS_KEY)")
		if cfg.R2.AccountID != "" && !r2AccountIDPattern.MatchString(cfg.R2.AccountID) {
			errs = append(errs, errors.New("r2.account_id (AUMBRY_R2_ACCOUNT_ID) must be a 32-digit hexadecimal account ID"))
		}
	case "":
		// Already recorded missing-storage error above.
	default:
		errs = append(errs, fmt.Errorf("storage (AUMBRY_STORAGE) must be one of local, webdav, s3, r2; got %q", cfg.Storage))
	}

	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", ")))
	}
	if cfg.CookieSecret != "" && len(cfg.CookieSecret) < 32 {
		errs = append(errs, errors.New("cookie_secret (AUMBRY_COOKIE_SECRET) must contain at least 32 bytes"))
	}
	if cfg.Admin.PasswordHash != "" {
		if err := validateBcryptHash(cfg.Admin.PasswordHash); err != nil {
			errs = append(errs, fmt.Errorf("admin.password_hash (AUMBRY_ADMIN_PASSWORD_HASH) is not a valid bcrypt hash: %w", err))
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

	if cfg.PublicURL != "" {
		if err := ValidatePublicURL(cfg.PublicURL); err != nil {
			errs = append(errs, fmt.Errorf("public_url (AUMBRY_PUBLIC_URL) %w", err))
		}
	}
	if cfg.Revisions.MaxPerItem == nil {
		v := defaultMaxRevisionsPerItem
		cfg.Revisions.MaxPerItem = &v
	} else if *cfg.Revisions.MaxPerItem < 0 {
		errs = append(errs, fmt.Errorf("revisions.max_per_item (AUMBRY_REVISIONS_MAX_PER_ITEM) must be >= 0, got %d", *cfg.Revisions.MaxPerItem))
	}
	if cfg.Ingest.MaxBodyMB <= 0 {
		cfg.Ingest.MaxBodyMB = defaultMaxIngestBodyMB
	} else if cfg.Ingest.MaxBodyMB < minIngestBodyMB {
		errs = append(errs, fmt.Errorf("ingest.max_body_mb (AUMBRY_INGEST_MAX_BODY_MB) must be at least %d MiB when set, got %d", minIngestBodyMB, cfg.Ingest.MaxBodyMB))
	}
	if cfg.Ingest.AllowRemoteImages == nil {
		v := true
		cfg.Ingest.AllowRemoteImages = &v
	}

	return errors.Join(errs...)
}

// ValidatePublicURL verifies that public_url uses http(s)://host[:port] with no path, query, fragment, or userinfo.
func ValidatePublicURL(raw string) error {
	return storage.ValidateS3Endpoint(raw)
}

// ResolvedMaxRevisionsPerItem returns the configured per-item revision retention cap (0 means unlimited).
func (c Config) ResolvedMaxRevisionsPerItem() int {
	if c.Revisions.MaxPerItem == nil {
		return defaultMaxRevisionsPerItem
	}
	return *c.Revisions.MaxPerItem
}

// ResolvedMaxIngestBodyBytes returns the configured maximum ingest request size in bytes.
func (c Config) ResolvedMaxIngestBodyBytes() int64 {
	mb := c.Ingest.MaxBodyMB
	if mb <= 0 {
		mb = defaultMaxIngestBodyMB
	}
	return int64(mb) * 1024 * 1024
}

// ResolvedAllowRemoteImages reports whether server-side fetching and localization of remote
// <figure src="http(s)://..."> images is enabled (default true).
func (c Config) ResolvedAllowRemoteImages() bool {
	if c.Ingest.AllowRemoteImages == nil {
		return true
	}
	return *c.Ingest.AllowRemoteImages
}

// ResolvedLocalPath returns the effective local object root directory.
func (c Config) ResolvedLocalPath() string {
	if p := strings.TrimSpace(c.Local.Path); p != "" {
		return p
	}
	if d := strings.TrimSpace(c.DataDir); d != "" {
		return filepath.Join(d, "objects")
	}
	return ""
}

// ResolvedS3 resolves effective S3 settings for the s3 and r2 storage backends.
func (c Config) ResolvedS3() S3Config {
	if c.Storage == "r2" {
		return S3Config{
			Endpoint:        fmt.Sprintf("https://%s.r2.cloudflarestorage.com", strings.ToLower(c.R2.AccountID)),
			Region:          "auto",
			Bucket:          c.R2.Bucket,
			AccessKeyID:     c.R2.AccessKeyID,
			SecretAccessKey: c.R2.SecretAccessKey,
			PathStyle:       false,
		}
	}
	out := c.S3
	if out.Endpoint != "" && out.Region == "" {
		out.Region = "auto"
	}
	return out
}

func validateBcryptHash(hash string) error {
	if _, err := bcrypt.Cost([]byte(hash)); err != nil {
		return err
	}
	prefixLen := 0
	switch {
	case strings.HasPrefix(hash, "$2$"):
		prefixLen = 6 // $2$XX$
	case strings.HasPrefix(hash, "$2a$"),
		strings.HasPrefix(hash, "$2b$"),
		strings.HasPrefix(hash, "$2x$"),
		strings.HasPrefix(hash, "$2y$"):
		prefixLen = 7 // $2y$XX$
	default:
		return errors.New("unsupported bcrypt version prefix")
	}
	if len(hash) < prefixLen+53 || hash[prefixLen-1] != '$' {
		return errors.New("malformed bcrypt cost delimiter or length")
	}
	body := hash[prefixLen:]
	if len(body) != 53 {
		return fmt.Errorf("expected 53 encoded salt+digest characters, got %d", len(body))
	}
	salt, err := bcryptEncoding.DecodeString(body[:22])
	if err != nil || len(salt) != 16 {
		return errors.New("invalid bcrypt base64 salt")
	}
	digest, err := bcryptEncoding.DecodeString(body[22:])
	if err != nil || len(digest) != 23 {
		return errors.New("invalid bcrypt base64 digest")
	}
	return nil
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
	masked.S3.AccessKeyID = maskSecret(c.S3.AccessKeyID)
	masked.S3.SecretAccessKey = maskSecret(c.S3.SecretAccessKey)
	masked.R2.AccessKeyID = maskSecret(c.R2.AccessKeyID)
	masked.R2.SecretAccessKey = maskSecret(c.R2.SecretAccessKey)

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
