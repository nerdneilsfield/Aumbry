package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nerdneilsfield/aumbry/internal/buildinfo"
	"github.com/nerdneilsfield/aumbry/internal/client"
)

func TestRootVersionFlagAndVerboseShorthand(t *testing.T) {
	origVer, origCommit, origDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.Date = origVer, origCommit, origDate
	})
	buildinfo.Version = "0.2.1"
	buildinfo.Commit = "1234567abcdef"
	buildinfo.Date = "2026-11-23T12:30:00Z"

	out := mustRun(t, "--version")
	if !strings.Contains(out, "aumbry 0.2.1\ncommit: 1234567\nbuilt: 2026-11-23T12:30:00Z\n") {
		t.Fatalf("--version output = %q", out)
	}

	// -v remains bound to --verbose, not --version.
	outV := mustRun(t, "-v", "version")
	if !strings.Contains(outV, "aumbry 0.2.1\n") {
		t.Fatalf("-v version output = %q", outV)
	}
}

func TestStatusCommand(t *testing.T) {
	payload := client.ServerStatus{
		Status: "ok",
		Checks: map[string]string{"sqlite": "ok", "storage": "ok"},
		Build: client.BuildInfo{
			Version:   "0.2.1",
			Commit:    "abcdef1",
			BuildTime: "2026-11-23T12:30:00Z",
		},
		Runtime: client.RuntimeStatus{
			GoVersion:       "go1.27.4",
			GOOS:            "linux",
			GOARCH:          "amd64",
			NumCPU:          8,
			Goroutines:      14,
			UptimeSeconds:   3725,
			StartedAt:       "2026-11-23T11:27:55Z",
			CPUSecondsTotal: 1.25,
			CPUPercent:      3.5,
			HeapAllocBytes:  4 * 1024 * 1024,
			HeapInuseBytes:  6 * 1024 * 1024,
			SysBytes:        32 * 1024 * 1024,
			GCCount:         5,
			ProcessRSSBytes: 24 * 1024 * 1024,
		},
		Storage: client.StorageStatus{
			Backend:     "local",
			SQLiteBytes: 128 * 1024,
		},
		Counts: &client.ContentCounts{
			Directories:          5,
			ProtectedDirectories: 2,
			Items:                9,
			PublishedItems:       7,
			DraftItems:           2,
			Revisions:            14,
			Assets:               6,
			AssetBytes:           3 * 1024 * 1024,
			DirectoryTokens:      1,
		},
		Activity: client.ActivityStatus{
			IngestInFlight:       1,
			IngestLimit:          4,
			MCPActiveSessions:    3,
			MCPAnonSessions:      2,
			MCPAnonPostsInFlight: 0,
		},
		Config: client.ConfigStatus{
			MaxIngestBodyBytes:  200 * 1024 * 1024,
			MaxRevisionsPerItem: 20,
			AllowRemoteImages:   true,
			PublicURLConfigured: true,
			TrustedProxiesCount: 1,
		},
	}

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/status" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	isolate(t)
	if _, err := runCLI(t, "status"); err == nil {
		t.Fatal("expected error when no server is configured")
	}

	mustRun(t, "config", "set", "--server", srv.URL, "--token", "amb_status_tok")

	origVer, origCommit, origDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.Date = origVer, origCommit, origDate
	})
	buildinfo.Version = "0.2.0"
	buildinfo.Commit = "5ba6832"
	buildinfo.Date = "2026-11-23T12:18:00Z"

	textOut := mustRun(t, "status")
	if gotAuth != "Bearer amb_status_tok" {
		t.Fatalf("Authorization header = %q, want Bearer amb_status_tok", gotAuth)
	}
	for _, want := range []string{
		"Health:    ok (sqlite=ok storage=ok)",
		"ServerVer: v0.2.1 (commit abcdef1, built 2026-11-23T12:30:00Z)",
		"CLIVer:    v0.2.0 (commit 5ba6832, built 2026-11-23T12:18:00Z)",
		"Note:      CLI (v0.2.0) and server (v0.2.1) versions differ",
		"CPU:       3.5% across 8 cores (1.25s total)",
		"Memory:    rss 24.0 MB (heap 4.00 MB alloc / 6.00 MB in-use / 32.0 MB sys)",
		"Storage:   local (sqlite 128 KB)",
		"Content:   dirs=5 (2 protected), items=9 (7 published, 2 drafts), revisions=14, assets=6 (3.00 MB), dir_tokens=1",
		"Activity:  ingest=1/4, mcp_sessions=3 (anon=2, anon_posts=0)",
		"Limits:    max_ingest=200 MB, max_revisions=20, remote_images=true, public_url=true, trusted_proxies=1",
	} {
		if !strings.Contains(textOut, want) {
			t.Errorf("status text output missing %q:\n%s", want, textOut)
		}
	}

	// Equal versions suppress mismatch note (ignoring leading v prefix).
	buildinfo.Version = "v0.2.1"
	sameOut := mustRun(t, "status")
	if strings.Contains(sameOut, "versions differ") {
		t.Errorf("unexpected version mismatch note when both are v0.2.1:\n%s", sameOut)
	}

	// --json returns formatted JSON matching ServerStatus.
	var jsonBuf bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&jsonBuf)
	root.SetArgs([]string{"status", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var decoded client.ServerStatus
	if err := json.Unmarshal(jsonBuf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode status --json: %v\n%s", err, jsonBuf.String())
	}
	if decoded.Build.Commit != "abcdef1" || decoded.Counts == nil || decoded.Counts.Items != 9 || decoded.Activity.MCPActiveSessions != 3 {
		t.Fatalf("unexpected decoded status --json: %+v", decoded)
	}

	// Agent status without admin counts/sqlite_bytes omits the Content line cleanly,
	// and configured URL userinfo/query/fragment are stripped from stdout and error messages.
	payload.Counts = nil
	payload.Storage.SQLiteBytes = 0

	parsedSrv, _ := url.Parse(srv.URL)
	dirtyURL := "http://alice:supersecret@" + parsedSrv.Host + "?token=queryleak#frag"
	mustRun(t, "config", "set", "--server", dirtyURL, "--token", "amb_status_tok")

	scopedOut := mustRun(t, "status")
	if strings.Contains(scopedOut, "Content:") || strings.Contains(scopedOut, "sqlite ") {
		t.Errorf("scoped agent status unexpectedly printed Content or sqlite size:\n%s", scopedOut)
	}
	for _, secret := range []string{"alice", "supersecret", "queryleak", "#frag"} {
		if strings.Contains(scopedOut, secret) {
			t.Errorf("status stdout leaked credential fragment %q:\n%s", secret, scopedOut)
		}
	}

	// Simulate unreachable dirty server URL and ensure error output never prints userinfo/query/fragment.
	srv.Close()
	_, connErr := runCLI(t, "status")
	if connErr == nil {
		t.Fatal("expected connection error against closed server")
	}
	for _, secret := range []string{"alice", "supersecret", "queryleak", "#frag"} {
		if strings.Contains(connErr.Error(), secret) {
			t.Errorf("status error leaked credential fragment %q: %v", secret, connErr)
		}
	}
}
