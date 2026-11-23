package api

import (
	"encoding/json"
	"math"
	"net/http"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nerdneilsfield/aumbry/internal/buildinfo"
	"github.com/nerdneilsfield/aumbry/internal/storage"
)

func TestVersionEndpointIsMinimal(t *testing.T) {
	origVer, origCommit, origDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.Date = origVer, origCommit, origDate
	})
	buildinfo.Version = "0.2.1"
	buildinfo.Commit = "1234567abcdef"
	buildinfo.Date = "2026-11-23T13:00:00Z"

	e := newEnv(t)
	resp := e.anon().do(http.MethodGet, "/api/v1/version", nil)
	if resp.status != http.StatusOK {
		t.Fatalf("GET /api/v1/version = %d %s", resp.status, resp.body)
	}
	if cc := resp.header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
	var fields map[string]any
	if err := json.Unmarshal(resp.body, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 || fields["version"] != "0.2.1" || fields["commit"] != "1234567" || fields["build_time"] != "2026-11-23T13:00:00Z" {
		t.Fatalf("unexpected public version payload: %v", fields)
	}
}

func TestInferStorageBackend(t *testing.T) {
	localStore, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	davStore, err := storage.NewWebDAV("http://127.0.0.1:8080", "", "")
	if err != nil {
		t.Fatal(err)
	}
	s3Store, err := storage.NewS3(storage.S3Options{
		Endpoint:        "http://127.0.0.1:9000",
		Region:          "us-east-1",
		Bucket:          "aumbry",
		AccessKeyID:     "k",
		SecretAccessKey: "s",
		PathStyle:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		opt   string
		store storage.Store
		want  string
	}{
		{opt: "R2", store: s3Store, want: "r2"},
		{opt: "", store: localStore, want: "local"},
		{opt: "", store: davStore, want: "webdav"},
		{opt: "", store: s3Store, want: "s3"},
		{opt: "", store: nil, want: "unknown"},
	}
	for _, tc := range cases {
		if got := inferStorageBackend(tc.opt, tc.store); got != tc.want {
			t.Errorf("inferStorageBackend(%q, %T) = %q, want %q", tc.opt, tc.store, got, tc.want)
		}
	}
}

func TestServerStatusEndpointsAndCounts(t *testing.T) {
	e := newEnvWithOptions(t, RouterOptions{
		StorageBackend: "local",
		PublicURL:      "https://hub.example.org",
	}, nil)
	admin := e.admin()
	anon := e.anon()

	if resp := anon.do(http.MethodGet, "/api/v1/admin/status", nil); resp.status != http.StatusUnauthorized {
		t.Fatalf("unauth admin status = %d, want 401", resp.status)
	}
	if resp := e.agentDo(http.MethodGet, "/api/v1/agent/status", "-"); resp.status != http.StatusUnauthorized {
		t.Fatalf("unauth agent status = %d, want 401", resp.status)
	}
	if resp := e.agentDo(http.MethodGet, "/api/v1/agent/status", "invalid-token"); resp.status != http.StatusUnauthorized {
		t.Fatalf("invalid token agent status = %d, want 401", resp.status)
	}

	// Build directory tree: 2 public (pub, pubChild), 2 protected (prot + inherited protChild).
	pub := admin.createDir("", "Public", "")
	_ = admin.createDir(pub, "Public Child", "")
	prot := admin.createDir("", "Private", "pw-secret")
	_ = admin.createDir(prot, "Private Inherited", "")

	tokResp := admin.do(http.MethodPost, "/api/v1/admin/dirs/"+prot+"/tokens", map[string]string{"label": "rss"})
	if tokResp.status != http.StatusCreated {
		t.Fatalf("create dir token = %d %s", tokResp.status, tokResp.body)
	}
	dirTok, _ := tokResp.json()["token"].(string)

	// Published item with an image updated twice (2 revisions, 1 deduplicated asset) + 1 draft.
	pngBytes := validTestPNG(t)
	_ = admin.ingestOK(ingestReq{
		key:    "status:pub",
		xml:    xmlDoc(docOpts{title: "Status Item v1", figures: []string{"fig.png"}}),
		assets: []figure{{src: "fig.png", name: "fig.png", data: pngBytes}},
		dirs:   []string{pub},
	})
	_ = admin.ingestOK(ingestReq{
		key:    "status:pub",
		xml:    xmlDoc(docOpts{title: "Status Item v2", figures: []string{"fig.png"}}),
		assets: []figure{{src: "fig.png", name: "fig.png", data: pngBytes}},
		dirs:   []string{pub},
	})
	_ = admin.ingestOK(ingestReq{
		key:   "status:draft",
		xml:   xmlDoc(docOpts{title: "Draft Item"}),
		draft: true,
	})

	// Connect an active anonymous MCP session so live session telemetry reflects it.
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "status-test", Version: "1.0"}, nil)
	mcpSess, err := mcpClient.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             e.srv.URL + "/mcp",
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect anonymous MCP: %v", err)
	}
	t.Cleanup(func() { _ = mcpSess.Close() })

	adminResp := admin.do(http.MethodGet, "/api/v1/admin/status", nil)
	if adminResp.status != http.StatusOK {
		t.Fatalf("admin status = %d %s", adminResp.status, adminResp.body)
	}
	if cc := adminResp.header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("admin status Cache-Control = %q, want no-store", cc)
	}
	rawAdmin := string(adminResp.body)
	for _, secret := range []string{apiToken, adminPassword, cookieSecret, dirTok, "hub.example.org"} {
		if secret != "" && strings.Contains(rawAdmin, secret) {
			t.Fatalf("status payload leaked sensitive value %q: %s", secret, rawAdmin)
		}
	}

	var st ServerStatus
	if err = json.Unmarshal(adminResp.body, &st); err != nil {
		t.Fatal(err)
	}
	if st.Status != "ok" || st.Checks["sqlite"] != "ok" || st.Checks["storage"] != "ok" {
		t.Fatalf("unexpected health checks: status=%q checks=%v", st.Status, st.Checks)
	}
	if st.Runtime.NumCPU < 1 || st.Runtime.Goroutines < 1 || st.Runtime.HeapAllocBytes == 0 || st.Runtime.SysBytes < st.Runtime.HeapAllocBytes || st.Runtime.StartedAt == "" {
		t.Fatalf("unexpected runtime telemetry: %+v", st.Runtime)
	}
	if runtime.GOOS == "linux" && st.Runtime.ProcessRSSBytes == 0 {
		t.Fatalf("expected non-zero Linux process_rss_bytes, got %+v", st.Runtime)
	}
	if st.Storage.Backend != "local" || st.Storage.SQLiteBytes <= 0 {
		t.Fatalf("unexpected storage telemetry: %+v", st.Storage)
	}
	if st.Counts.Directories != 4 || st.Counts.ProtectedDirectories != 2 ||
		st.Counts.Items != 2 || st.Counts.PublishedItems != 1 || st.Counts.DraftItems != 1 ||
		st.Counts.Revisions != 2 || st.Counts.Assets != 1 || st.Counts.AssetBytes != int64(len(pngBytes)) ||
		st.Counts.DirectoryTokens != 1 {
		t.Fatalf("unexpected content counts: %+v", st.Counts)
	}
	if st.Activity.IngestLimit != ingestConcurrencyLimit || st.Activity.MCPActiveSessions != 1 || st.Activity.MCPAnonSessions != 1 {
		t.Fatalf("unexpected activity telemetry: %+v", st.Activity)
	}
	if !st.Config.AllowRemoteImages || !st.Config.PublicURLConfigured || st.Config.MaxRevisionsPerItem != defaultRouterMaxRev {
		t.Fatalf("unexpected config summary: %+v", st.Config)
	}

	agentResp := e.agentDo(http.MethodGet, "/api/v1/agent/status", apiToken)
	if agentResp.status != http.StatusOK {
		t.Fatalf("agent status = %d %s", agentResp.status, agentResp.body)
	}
	if cc := agentResp.header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("agent status Cache-Control = %q, want no-store", cc)
	}
	rawAgent := string(agentResp.body)
	if strings.Contains(rawAgent, `"counts"`) || strings.Contains(rawAgent, `"sqlite_bytes"`) {
		t.Fatalf("agent status must omit admin repository counts and sqlite_bytes: %s", rawAgent)
	}
	var agentST ServerStatus
	if err = json.Unmarshal(agentResp.body, &agentST); err != nil {
		t.Fatal(err)
	}
	if agentST.Counts != nil || agentST.Storage.SQLiteBytes != 0 || agentST.Activity.MCPActiveSessions != 1 {
		t.Fatalf("unexpected scoped agent status: %+v", agentST)
	}
}

func TestMCPServerInfoTool(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	prot := admin.createDir("", "Protected", "secret-pw")
	tokResp := admin.do(http.MethodPost, "/api/v1/admin/dirs/"+prot+"/tokens", map[string]string{"label": "mcp"})
	if tokResp.status != http.StatusCreated {
		t.Fatalf("create dir token = %d %s", tokResp.status, tokResp.body)
	}
	dirTok, _ := tokResp.json()["token"].(string)

	connectWithBearer := func(token string) *mcp.ClientSession {
		hdr := make(http.Header)
		if token != "" {
			hdr.Set("Authorization", "Bearer "+token)
		}
		rt := &hostInjectRT{base: http.DefaultTransport, headers: hdr}
		c := mcp.NewClient(&mcp.Implementation{Name: "info-client", Version: "1.0"}, nil)
		sess, err := c.Connect(t.Context(), &mcp.StreamableClientTransport{
			Endpoint:             e.srv.URL + "/mcp",
			HTTPClient:           &http.Client{Transport: rt},
			DisableStandaloneSSE: true,
		}, nil)
		if err != nil {
			t.Fatalf("mcp Connect: %v", err)
		}
		t.Cleanup(func() { _ = sess.Close() })
		return sess
	}

	toStrings := func(raw any) []string {
		arr, _ := raw.([]any)
		out := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}

	anonOut, rawAnon, isErr := callMCPJSON(t, connectWithBearer(""), "server_info", map[string]any{})
	if isErr {
		t.Fatalf("anon server_info failed: %s", rawAnon)
	}
	if anonOut["caller_role"] != "anonymous" || anonOut["can_push"] != false {
		t.Fatalf("anon server_info = %v", anonOut)
	}
	if _, hasHealth := anonOut["health"]; hasHealth {
		t.Fatalf("anonymous server_info must omit health: %s", rawAnon)
	}
	if anonTools := toStrings(anonOut["tools"]); slices.Contains(anonTools, "push_item") || !slices.Contains(anonTools, "server_info") {
		t.Fatalf("unexpected anon tools: %v", anonTools)
	}

	dirOut, rawDir, isErr := callMCPJSON(t, connectWithBearer(dirTok), "server_info", map[string]any{})
	if isErr {
		t.Fatalf("dir_token server_info failed: %s", rawDir)
	}
	if dirOut["caller_role"] != "dir_token" || dirOut["can_push"] != false {
		t.Fatalf("dir_token server_info = %v", dirOut)
	}
	if _, hasHealth := dirOut["health"]; hasHealth {
		t.Fatalf("dir_token server_info must omit health: %s", rawDir)
	}
	if dirTools := toStrings(dirOut["tools"]); slices.Contains(dirTools, "push_item") {
		t.Fatalf("dir_token tools must omit push_item: %v", dirTools)
	}

	apiOut, rawAPI, isErr := callMCPJSON(t, connectWithBearer(apiToken), "server_info", map[string]any{})
	if isErr {
		t.Fatalf("api_token server_info failed: %s", rawAPI)
	}
	if apiOut["caller_role"] != "api_token" || apiOut["can_push"] != true {
		t.Fatalf("api_token server_info = %v", apiOut)
	}
	apiTools := toStrings(apiOut["tools"])
	if !slices.Contains(apiTools, "push_item") || !slices.Contains(apiTools, "server_info") {
		t.Fatalf("api_token tools missing expected entries: %v", apiTools)
	}
	health, _ := apiOut["health"].(map[string]any)
	if health == nil || health["status"] != "ok" {
		t.Fatalf("api_token server_info missing ok health: %s", rawAPI)
	}
	limits, _ := apiOut["limits"].(map[string]any)
	if limits == nil || int64(limits["max_xml_bytes"].(float64)) != int64(maxXMLSize) {
		t.Fatalf("unexpected server_info limits: %v", apiOut["limits"])
	}
}

func TestStatusHandlesDegradedDBGracefully(t *testing.T) {
	e := newEnv(t)
	if err := e.db.Close(); err != nil {
		t.Fatal(err)
	}
	resp := e.agentDo(http.MethodGet, "/api/v1/agent/status", apiToken)
	if resp.status != http.StatusOK {
		t.Fatalf("degraded status should return 200 JSON instead of 500, got %d %s", resp.status, resp.body)
	}
	if resp.json()["status"] != "degraded" {
		t.Fatalf("status = %v, want degraded", resp.json()["status"])
	}
}

func TestCPUSamplerIdleIsNearZero(t *testing.T) {
	before := readProcessCPUSeconds()
	if before <= 0 {
		t.Fatalf("readProcessCPUSeconds() = %v, want > 0", before)
	}
	s := newCPUSampler()
	time.Sleep(600 * time.Millisecond)
	total, pct := s.sample()
	if total < math.Floor(before*100)/100 {
		t.Fatalf("total CPU seconds regressed: got %v, before %v", total, before)
	}
	if pct >= 50.0 {
		t.Fatalf("idle cpu_percent = %.1f%%, want < 50%% (must track consumed CPU, not GOMAXPROCS wall budget)", pct)
	}
	_, quickPct := s.sample()
	if quickPct != pct {
		t.Fatalf("follow-up cpu_percent = %.1f%%, want %.1f%%", quickPct, pct)
	}
}

func TestStatusBusySQLiteDegradesQuickly(t *testing.T) {
	e := newEnvWithOptions(t, RouterOptions{ReadyTimeout: 100 * time.Millisecond}, nil)
	admin := e.admin()

	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	start := time.Now()
	resp := admin.do(http.MethodGet, "/api/v1/admin/status", nil)
	elapsed := time.Since(start)
	if resp.status != http.StatusOK {
		t.Fatalf("busy-sqlite admin status = %d %s", resp.status, resp.body)
	}
	if elapsed >= 1500*time.Millisecond {
		t.Fatalf("busy-sqlite admin status took %v, expected fast degraded return", elapsed)
	}
	var st ServerStatus
	if err = json.Unmarshal(resp.body, &st); err != nil {
		t.Fatal(err)
	}
	if st.Status != "degraded" || st.Checks["sqlite"] != "timeout" || st.Counts == nil {
		t.Fatalf("unexpected busy-sqlite status payload: %+v", st)
	}
}
