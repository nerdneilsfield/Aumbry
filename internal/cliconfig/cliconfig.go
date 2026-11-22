// Package cliconfig reads and writes the aumbry CLI configuration file
// (~/.config/aumbry/config.yaml) and resolves the effective profile.
package cliconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Environment variables that override the selected profile.
const (
	EnvServer = "AUMBRY_SERVER"
	EnvToken  = "AUMBRY_TOKEN"
	EnvDirs   = "AUMBRY_DIRS"
)

// DefaultProfile is the profile name used when none has been chosen yet.
const DefaultProfile = "default"

// Profile is one named set of connection settings.
type Profile struct {
	Server      string   `yaml:"server,omitempty"`
	Token       string   `yaml:"token,omitempty"`
	DefaultDirs []string `yaml:"default_dirs,omitempty"`
}

// File is the on-disk configuration document.
type File struct {
	ActiveProfile string             `yaml:"active_profile,omitempty"`
	Profiles      map[string]Profile `yaml:"profiles,omitempty"`
}

// Path returns the config file location: $XDG_CONFIG_HOME/aumbry/config.yaml,
// or ~/.config/aumbry/config.yaml (on every OS) when XDG_CONFIG_HOME is unset.
func Path() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate config directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "aumbry", "config.yaml"), nil
}

// Load reads the config file. A missing file yields an empty File.
func Load() (File, error) {
	path, err := Path()
	if err != nil {
		return File{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var file File
	if err := yaml.Unmarshal(data, &file); err != nil {
		return File{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return file, nil
}

// Save writes the config file with mode 0600, creating its directory with mode 0700.
func Save(file File) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	// WriteFile does not change the mode of a file that already exists.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("restrict config permissions: %w", err)
	}
	return nil
}

// Resolve returns the effective profile and its name. The profile is chosen by
// name (the --profile flag), else active_profile; AUMBRY_SERVER, AUMBRY_TOKEN and
// AUMBRY_DIRS then override the file's values. An explicitly named or active
// profile that does not exist is an error; with neither set, an empty profile is
// used so that the environment alone can configure the CLI.
func (f File) Resolve(name string) (string, Profile, error) {
	if name == "" {
		name = f.ActiveProfile
	}
	var profile Profile
	if name == "" {
		name = DefaultProfile
	} else if p, ok := f.Profiles[name]; ok {
		profile = p
	} else {
		return "", Profile{}, fmt.Errorf("unknown profile %q: create it with `aumbry config set --profile %s`", name, name)
	}
	profile.DefaultDirs = append([]string(nil), profile.DefaultDirs...)
	if v := strings.TrimSpace(os.Getenv(EnvServer)); v != "" {
		profile.Server = v
	}
	if v := strings.TrimSpace(os.Getenv(EnvToken)); v != "" {
		profile.Token = v
	}
	if v := strings.TrimSpace(os.Getenv(EnvDirs)); v != "" {
		profile.DefaultDirs = splitList(v)
	}
	return name, profile, nil
}

// MaskToken hides a token except for its last four characters.
func MaskToken(token string) string {
	if len(token) <= 4 {
		if token == "" {
			return ""
		}
		return "****"
	}
	return "****" + token[len(token)-4:]
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
