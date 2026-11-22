package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/singleflight"

	"github.com/nerdneilsfield/aumbry/internal/storage"
)

const (
	defaultReadyCacheTTL = 5 * time.Second
	defaultReadyTimeout  = 2 * time.Second
)

var (
	urlUserInfoPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/@\s]+@`)
	sigQueryPattern    = regexp.MustCompile(`(?i)((?:X-Amz-)?(?:Signature|Credential)=)[^&\s",]+`)
)

// ReadinessReport holds the result of probing SQLite and object storage.
type ReadinessReport struct {
	Ready  bool
	Checks map[string]string
}

// ReadinessChecker checks SQLite and object storage health with bounded timeouts,
// request coalescing, and a short cache on storage probes.
type ReadinessChecker struct {
	db       *sqlx.DB
	store    storage.Store
	logger   *slog.Logger
	cacheTTL time.Duration
	timeout  time.Duration

	sf singleflight.Group

	mu            sync.Mutex
	hasCache      bool
	cachedStorage string
	cachedAt      time.Time
}

// NewReadinessChecker constructs a ReadinessChecker using default 5s storage cache TTL
// and 2s probe timeout when non-positive durations are supplied.
func NewReadinessChecker(database *sqlx.DB, store storage.Store, logger *slog.Logger, cacheTTL, timeout time.Duration) *ReadinessChecker {
	if cacheTTL <= 0 {
		cacheTTL = defaultReadyCacheTTL
	}
	if timeout <= 0 {
		timeout = defaultReadyTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ReadinessChecker{
		db:       database,
		store:    store,
		logger:   logger,
		cacheTTL: cacheTTL,
		timeout:  timeout,
	}
}

// Check runs both dependency checks concurrently and returns the aggregated status.
func (c *ReadinessChecker) Check(ctx context.Context) ReadinessReport {
	var (
		sqliteStatus  string
		storageStatus string
		wg            sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		sqliteStatus = c.checkSQLite(ctx)
	}()
	go func() {
		defer wg.Done()
		storageStatus = c.checkStorage(ctx)
	}()
	wg.Wait()

	return ReadinessReport{
		Ready: sqliteStatus == "ok" && storageStatus == "ok",
		Checks: map[string]string{
			"sqlite":  sqliteStatus,
			"storage": storageStatus,
		},
	}
}

func (c *ReadinessChecker) checkSQLite(ctx context.Context) string {
	ch := c.sf.DoChan("sqlite", func() (any, error) {
		probeCtx, cancel := context.WithTimeout(context.Background(), c.timeout)
		defer cancel()
		err := c.db.PingContext(probeCtx)
		if err == nil {
			return "ok", nil
		}
		category := "unreachable"
		if errors.Is(err, context.DeadlineExceeded) {
			category = "timeout"
		}
		c.logger.Warn("readiness check failed", "component", "sqlite", "category", category, "error", sanitizeError(err))
		return category, nil
	})
	select {
	case res := <-ch:
		status, _ := res.Val.(string)
		if status == "" {
			return "unreachable"
		}
		return status
	case <-ctx.Done():
		return "timeout"
	}
}

func (c *ReadinessChecker) checkStorage(ctx context.Context) string {
	c.mu.Lock()
	if c.hasCache && time.Since(c.cachedAt) < c.cacheTTL {
		status := c.cachedStorage
		c.mu.Unlock()
		return status
	}
	c.mu.Unlock()

	ch := c.sf.DoChan("storage", func() (any, error) {
		c.mu.Lock()
		if c.hasCache && time.Since(c.cachedAt) < c.cacheTTL {
			status := c.cachedStorage
			c.mu.Unlock()
			return status, nil
		}
		c.mu.Unlock()

		probeCtx, cancel := context.WithTimeout(context.Background(), c.timeout)
		defer cancel()
		err := c.store.Probe(probeCtx)
		status := classifyStorageProbeError(err)
		if err != nil {
			c.logger.Warn("readiness check failed", "component", "storage", "category", status, "error", sanitizeError(err))
		}

		c.mu.Lock()
		c.cachedStorage = status
		c.cachedAt = time.Now()
		c.hasCache = true
		c.mu.Unlock()
		return status, nil
	})
	select {
	case res := <-ch:
		status, _ := res.Val.(string)
		if status == "" {
			return "unreachable"
		}
		return status
	case <-ctx.Done():
		return "timeout"
	}
}

func classifyStorageProbeError(err error) string {
	if err == nil {
		return "ok"
	}
	switch {
	case errors.Is(err, storage.ErrAuth):
		return "auth"
	case errors.Is(err, storage.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "unreachable"
	}
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	s := urlUserInfoPattern.ReplaceAllString(err.Error(), "${1}***@")
	return sigQueryPattern.ReplaceAllString(s, "${1}***")
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	rep := s.ready.Check(r.Context())
	status := http.StatusOK
	label := "ok"
	if !rep.Ready {
		status = http.StatusServiceUnavailable
		label = "unavailable"
	}
	writeJSON(w, status, map[string]any{
		"status": label,
		"checks": rep.Checks,
	})
}
