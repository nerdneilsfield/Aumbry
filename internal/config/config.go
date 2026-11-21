// Package config loads and validates server configuration from the environment.
package config

import (
	"errors"
	"os"
	"strings"

	"github.com/caarlos0/env/v11"
)

// Config contains the server's runtime settings.
type Config struct {
	ListenAddr        string   `env:"AUMBRY_LISTEN_ADDR" envDefault:":8080"`
	DataDir           string   `env:"AUMBRY_DATA_DIR,required,notEmpty"`
	AdminUser         string   `env:"AUMBRY_ADMIN_USER,required,notEmpty"`
	AdminPasswordHash string   `env:"AUMBRY_ADMIN_PASSWORD_HASH,required,notEmpty"`
	APITokens         []string `env:"AUMBRY_API_TOKENS,required,notEmpty" envSeparator:","`
	CookieSecret      string   `env:"AUMBRY_COOKIE_SECRET,required,notEmpty"`
	WebDAVURL         string   `env:"AUMBRY_WEBDAV_URL,required,notEmpty"`
	WebDAVUser        string   `env:"AUMBRY_WEBDAV_USER,required,notEmpty"`
	WebDAVPassword    string   `env:"AUMBRY_WEBDAV_PASS,required,notEmpty"`
}

// verbatim lists the secrets that are used exactly as given; every other value is trimmed.
var verbatim = map[string]bool{"AUMBRY_COOKIE_SECRET": true, "AUMBRY_WEBDAV_PASS": true}

// Load reads and validates the required AUMBRY_* environment variables.
func Load() (Config, error) {
	environment := env.ToMap(os.Environ())
	for key, value := range environment {
		if !verbatim[key] {
			environment[key] = strings.TrimSpace(value)
		}
	}
	cfg, err := env.ParseAsWithOptions[Config](env.Options{Environment: environment})
	if err != nil {
		return Config{}, err
	}

	tokens := cfg.APITokens[:0]
	for _, token := range cfg.APITokens {
		if token = strings.TrimSpace(token); token != "" {
			tokens = append(tokens, token)
		}
	}
	if cfg.APITokens = tokens; len(tokens) == 0 {
		return Config{}, errors.New(`required environment variable "AUMBRY_API_TOKENS" has no tokens`)
	}
	if len(cfg.CookieSecret) < 32 {
		return Config{}, errors.New("AUMBRY_COOKIE_SECRET must contain at least 32 bytes")
	}
	return cfg, nil
}
