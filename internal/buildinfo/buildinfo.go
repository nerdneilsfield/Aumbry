// Package buildinfo carries release metadata injected with -ldflags and Go VCS build info.
package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

const shortCommitLen = 7

var (
	// Version is the application version.
	Version = "dev"
	// Commit is the source revision.
	Commit = "none"
	// Date is the build timestamp.
	Date = "unknown"
)

// Info describes build metadata exposed by CLI commands, HTTP /api/v1/version, and UI footers.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// Current returns normalized build metadata, shortening commit hashes and falling back
// to Go embedded VCS metadata when Commit was not injected via -ldflags.
func Current() Info {
	ver := strings.TrimSpace(Version)
	if ver == "" {
		ver = "dev"
	}
	commit := strings.TrimSpace(Commit)
	if commit == "" || commit == "none" {
		if vcs := vcsCommitFromBuildInfo(); vcs != "" {
			commit = vcs
		}
	}
	commit = ShortCommit(commit)
	built := strings.TrimSpace(Date)
	if built == "" {
		built = "unknown"
	}
	return Info{
		Version:   ver,
		Commit:    commit,
		BuildTime: built,
	}
}

// ShortCommit returns at most 7 characters of a commit hash while preserving a trailing "-dirty" suffix.
func ShortCommit(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "none"
	}
	base, dirty := strings.CutSuffix(s, "-dirty")
	if base == "" {
		return "none"
	}
	if len(base) > shortCommitLen {
		base = base[:shortCommitLen]
	}
	if dirty {
		return base + "-dirty"
	}
	return base
}

// Format returns the multi-line human-readable version block for a binary name.
func Format(binary string) string {
	info := Current()
	return fmt.Sprintf(
		"%s %s\ncommit: %s\nbuilt: %s\ngo: %s %s/%s\n",
		binary, info.Version, info.Commit, info.BuildTime,
		runtime.Version(), runtime.GOOS, runtime.GOARCH,
	)
}

func vcsCommitFromBuildInfo() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev string
	var dirty bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = strings.TrimSpace(s.Value)
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return ""
	}
	if dirty {
		return rev + "-dirty"
	}
	return rev
}
