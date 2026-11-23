package buildinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestShortCommit(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "none"},
		{"  ", "none"},
		{"none", "none"},
		{"5ba6832", "5ba6832"},
		{"5ba6832d3d90301d70dce14eabd75256fb52b5a4", "5ba6832"},
		{"5ba6832d3d90301d70dce14eabd75256fb52b5a4-dirty", "5ba6832-dirty"},
		{"-dirty", "none"},
	}
	for _, tc := range cases {
		if got := ShortCommit(tc.in); got != tc.want {
			t.Errorf("ShortCommit(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCurrentAndFormat(t *testing.T) {
	origVer, origCommit, origDate := Version, Commit, Date
	t.Cleanup(func() {
		Version, Commit, Date = origVer, origCommit, origDate
	})

	Version = "0.2.0"
	Commit = "5ba6832d3d90301d70dce14eabd75256fb52b5a4"
	Date = "2026-11-23T12:18:00Z"

	info := Current()
	if info.Version != "0.2.0" || info.Commit != "5ba6832" || info.BuildTime != "2026-11-23T12:18:00Z" {
		t.Fatalf("Current() = %+v", info)
	}

	out := Format("aumbry-server")
	for _, want := range []string{
		"aumbry-server 0.2.0\n",
		"commit: 5ba6832\n",
		"built: 2026-11-23T12:18:00Z\n",
		"go: " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH + "\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Format output missing %q in:\n%s", want, out)
		}
	}

	Version, Commit, Date = " ", " ", " "
	def := Current()
	if def.Version != "dev" || def.BuildTime != "unknown" || def.Commit == "" {
		t.Fatalf("default Current() = %+v", def)
	}
}
