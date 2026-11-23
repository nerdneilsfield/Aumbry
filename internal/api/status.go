package api

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"runtime"
	"runtime/metrics"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/nerdneilsfield/aumbry/internal/buildinfo"
	"github.com/nerdneilsfield/aumbry/internal/storage"
)

const (
	cpuSampleMinWindow = 0.5
	cpuBootMinWindow   = 0.05
)

type cpuSampler struct {
	mu      sync.Mutex
	bootAt  time.Time
	bootCPU float64
	prevAt  time.Time
	prevCPU float64
	lastPct float64
}

func newCPUSampler() *cpuSampler {
	now := time.Now()
	cur := readProcessCPUSeconds()
	return &cpuSampler{
		bootAt:  now,
		bootCPU: cur,
		prevAt:  now,
		prevCPU: cur,
	}
}

func readProcessCPUSeconds() float64 {
	sample := []metrics.Sample{{Name: "/cpu/classes/total:cpu-seconds"}}
	metrics.Read(sample)
	if sample[0].Value.Kind() != metrics.KindFloat64 {
		return 0
	}
	v := sample[0].Value.Float64()
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return v
}

func clampRoundedCPUPercent(dCPU, dt float64) float64 {
	if dt <= 0 || dCPU <= 0 {
		return 0
	}
	maxPct := float64(max(1, runtime.NumCPU())) * 100.0
	pct := (dCPU / dt) * 100.0
	if pct < 0 {
		pct = 0
	} else if pct > maxPct {
		pct = maxPct
	}
	return math.Round(pct*10) / 10
}

func (c *cpuSampler) sample() (totalSeconds, percent float64) {
	now := time.Now()
	cur := readProcessCPUSeconds()
	total := math.Round(cur*100) / 100

	c.mu.Lock()
	defer c.mu.Unlock()

	dt := now.Sub(c.prevAt).Seconds()
	if dt >= cpuSampleMinWindow {
		pct := clampRoundedCPUPercent(cur-c.prevCPU, dt)
		c.prevAt = now
		c.prevCPU = cur
		c.lastPct = pct
		return total, pct
	}
	if c.lastPct == 0 {
		if bootDT := now.Sub(c.bootAt).Seconds(); bootDT > cpuBootMinWindow {
			return total, clampRoundedCPUPercent(cur-c.bootCPU, bootDT)
		}
	}
	return total, c.lastPct
}

// RuntimeStatus captures pure-Go runtime CPU, memory, goroutine, and GC telemetry.
type RuntimeStatus struct {
	GoVersion       string  `json:"go_version"`
	GOOS            string  `json:"goos"`
	GOARCH          string  `json:"goarch"`
	NumCPU          int     `json:"num_cpu"`
	Goroutines      int     `json:"goroutines"`
	UptimeSeconds   int64   `json:"uptime_seconds"`
	StartedAt       string  `json:"started_at"`
	CPUSecondsTotal float64 `json:"cpu_seconds_total"`
	CPUPercent      float64 `json:"cpu_percent"`
	HeapAllocBytes  uint64  `json:"heap_alloc_bytes"`
	HeapInuseBytes  uint64  `json:"heap_inuse_bytes"`
	SysBytes        uint64  `json:"sys_bytes"`
	GCCount         uint32  `json:"gc_count"`
	ProcessRSSBytes uint64  `json:"process_rss_bytes,omitempty"`
}

// StorageStatus identifies the active object storage backend type and SQLite footprint.
type StorageStatus struct {
	Backend     string `json:"backend"`
	SQLiteBytes int64  `json:"sqlite_bytes"`
}

// ContentCounts summarizes stored repository entities without exposing item titles or secret values.
type ContentCounts struct {
	Directories          int   `json:"directories"`
	ProtectedDirectories int   `json:"protected_directories"`
	Items                int   `json:"items"`
	PublishedItems       int   `json:"published_items"`
	DraftItems           int   `json:"draft_items"`
	Revisions            int   `json:"revisions"`
	Assets               int   `json:"assets"`
	AssetBytes           int64 `json:"asset_bytes"`
	DirectoryTokens      int   `json:"directory_tokens"`
}

// ActivityStatus reports live in-flight ingest and MCP session concurrency.
type ActivityStatus struct {
	IngestInFlight       int `json:"ingest_in_flight"`
	IngestLimit          int `json:"ingest_limit"`
	MCPActiveSessions    int `json:"mcp_active_sessions"`
	MCPAnonSessions      int `json:"mcp_anon_sessions"`
	MCPAnonPostsInFlight int `json:"mcp_anon_posts_in_flight"`
}

// ConfigStatus summarizes non-secret server operational limits.
type ConfigStatus struct {
	MaxIngestBodyBytes  int64 `json:"max_ingest_body_bytes"`
	MaxRevisionsPerItem int   `json:"max_revisions_per_item"`
	AllowRemoteImages   bool  `json:"allow_remote_images"`
	PublicURLConfigured bool  `json:"public_url_configured"`
	TrustedProxiesCount int   `json:"trusted_proxies_count"`
}

// ServerStatus is the payload returned by GET /api/v1/admin/status and GET /api/v1/agent/status.
type ServerStatus struct {
	Status   string            `json:"status"`
	Checks   map[string]string `json:"checks"`
	Build    buildinfo.Info    `json:"build"`
	Runtime  RuntimeStatus     `json:"runtime"`
	Storage  StorageStatus     `json:"storage"`
	Counts   ContentCounts     `json:"counts"`
	Activity ActivityStatus    `json:"activity"`
	Config   ConfigStatus      `json:"config"`
}

func inferStorageBackend(optName string, store storage.Store) string {
	if trimmed := strings.ToLower(strings.TrimSpace(optName)); trimmed != "" {
		return trimmed
	}
	switch store.(type) {
	case *storage.Local:
		return "local"
	case *storage.WebDAV:
		return "webdav"
	case *storage.S3:
		return "s3"
	default:
		return "unknown"
	}
}

func (s *Server) collectDBStats(ctx context.Context) (sqliteBytes int64, counts ContentCounts, err error) {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, ContentCounts{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var pageCount, pageSize int64
	if err = sqlx.GetContext(ctx, tx, &pageCount, `PRAGMA page_count`); err != nil {
		return 0, ContentCounts{}, err
	}
	if err = sqlx.GetContext(ctx, tx, &pageSize, `PRAGMA page_size`); err != nil {
		return 0, ContentCounts{}, err
	}
	sqliteBytes = max(int64(0), pageCount*pageSize)

	tree, err := loadTreeFrom(ctx, tx)
	if err != nil {
		return 0, ContentCounts{}, err
	}
	counts.Directories = len(tree.dirs)
	for id := range tree.dirs {
		if !tree.isPublic(id) {
			counts.ProtectedDirectories++
		}
	}

	if err = sqlx.GetContext(ctx, tx, &counts.Items, `SELECT COUNT(*) FROM items`); err != nil {
		return 0, ContentCounts{}, err
	}
	if err = sqlx.GetContext(ctx, tx, &counts.PublishedItems, `SELECT COUNT(DISTINCT item_id) FROM directory_items`); err != nil {
		return 0, ContentCounts{}, err
	}
	counts.DraftItems = max(0, counts.Items-counts.PublishedItems)

	if err = sqlx.GetContext(ctx, tx, &counts.Revisions, `SELECT COUNT(*) FROM item_revisions`); err != nil {
		return 0, ContentCounts{}, err
	}
	var assetRow struct {
		Count int   `db:"cnt"`
		Bytes int64 `db:"total_bytes"`
	}
	if err = sqlx.GetContext(ctx, tx, &assetRow, `SELECT COUNT(*) AS cnt, COALESCE(SUM(size_bytes), 0) AS total_bytes FROM assets`); err != nil {
		return 0, ContentCounts{}, err
	}
	counts.Assets = assetRow.Count
	counts.AssetBytes = assetRow.Bytes

	if err = sqlx.GetContext(ctx, tx, &counts.DirectoryTokens, `SELECT COUNT(*) FROM directory_tokens`); err != nil {
		return 0, ContentCounts{}, err
	}
	return sqliteBytes, counts, nil
}

func (s *Server) collectServerStatus(ctx context.Context) ServerStatus {
	rep := s.ready.Check(ctx)
	status := "ok"
	if !rep.Ready {
		status = "degraded"
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	var cpuTotal, cpuPct float64
	if s.cpu != nil {
		cpuTotal, cpuPct = s.cpu.sample()
	}

	uptime := int64(0)
	startedAt := ""
	if !s.startedAt.IsZero() {
		uptime = max(int64(0), int64(time.Since(s.startedAt).Seconds()))
		startedAt = s.startedAt.UTC().Format(time.RFC3339)
	}

	var mcpStats mcpSessionStats
	if s.mcpTracker != nil {
		mcpStats = s.mcpTracker.stats()
	}

	sqliteBytes, counts, dbErr := s.collectDBStats(ctx)
	if dbErr != nil {
		status = "degraded"
		if s.logger != nil {
			s.logger.Warn("collect server status db stats failed", "error", sanitizeError(dbErr))
		}
	}

	return ServerStatus{
		Status: status,
		Checks: rep.Checks,
		Build:  buildinfo.Current(),
		Runtime: RuntimeStatus{
			GoVersion:       runtime.Version(),
			GOOS:            runtime.GOOS,
			GOARCH:          runtime.GOARCH,
			NumCPU:          runtime.NumCPU(),
			Goroutines:      runtime.NumGoroutine(),
			UptimeSeconds:   uptime,
			StartedAt:       startedAt,
			CPUSecondsTotal: cpuTotal,
			CPUPercent:      cpuPct,
			HeapAllocBytes:  ms.HeapAlloc,
			HeapInuseBytes:  ms.HeapInuse,
			SysBytes:        ms.Sys,
			GCCount:         ms.NumGC,
			ProcessRSSBytes: readProcessRSSBytes(),
		},
		Storage: StorageStatus{
			Backend:     inferStorageBackend(s.storageBackend, s.store),
			SQLiteBytes: sqliteBytes,
		},
		Counts: counts,
		Activity: ActivityStatus{
			IngestInFlight:       len(s.ingestSem),
			IngestLimit:          cap(s.ingestSem),
			MCPActiveSessions:    mcpStats.activeSessions,
			MCPAnonSessions:      mcpStats.anonSessions,
			MCPAnonPostsInFlight: mcpStats.anonPostsInFlight,
		},
		Config: ConfigStatus{
			MaxIngestBodyBytes:  s.maxIngestBody,
			MaxRevisionsPerItem: s.maxRevisions,
			AllowRemoteImages:   s.allowRemoteImages,
			PublicURLConfigured: s.publicURL != "",
			TrustedProxiesCount: len(s.trustedProxies),
		},
	}
}

func (s *Server) serverStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.collectServerStatus(r.Context()))
}
