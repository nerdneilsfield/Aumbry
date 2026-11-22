// Package api provides the HTTP router and handlers.
package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/go-chi/httprate"
	"github.com/jmoiron/sqlx"
	"github.com/unrolled/secure"

	"github.com/nerdneilsfield/aumbry/internal/auth"
	"github.com/nerdneilsfield/aumbry/internal/storage"
	"github.com/nerdneilsfield/aumbry/internal/webembed"
)

const (
	ingestConcurrencyLimit = 4
	writeRateLimitPerMin   = 120
)

// RouterOptions configures proxy trust and readiness probing behavior on the HTTP router.
type RouterOptions struct {
	TrustedProxies []netip.Prefix
	ReadyCacheTTL  time.Duration
	ReadyTimeout   time.Duration
}

// Server holds the dependencies shared by all handlers.
type Server struct {
	db     *sqlx.DB
	store  storage.Store
	auth   *auth.Sessions
	logger *slog.Logger
	ready  *ReadinessChecker
}

// NewRouter creates the API routes and embedded frontend handler.
func NewRouter(database *sqlx.DB, store storage.Store, sessions *auth.Sessions, logger *slog.Logger, opts ...RouterOptions) (http.Handler, error) {
	frontend, err := webembed.Files()
	if err != nil {
		return nil, err
	}
	var opt RouterOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	s := &Server{
		db:     database,
		store:  store,
		auth:   sessions,
		logger: logger,
		ready:  NewReadinessChecker(database, store, logger, opt.ReadyCacheTTL, opt.ReadyTimeout),
	}

	logSchema := httplog.SchemaECS.Concise(true)
	logSchema.RequestRemoteIP = httplog.SchemaECS.RequestRemoteIP

	router := chi.NewRouter()
	router.Use(clientIPMiddleware(opt.TrustedProxies))
	router.Use(securityHeaders())
	router.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        logSchema,
		RecoverPanics: true,
		Skip:          func(r *http.Request, _ int) bool { return r.URL.Path == "/healthz" },
	}))

	router.Get("/healthz", health)
	router.Get("/readyz", s.readyz)
	router.Route("/api/v1", func(r chi.Router) {
		writeLimit := writeLimiter()
		r.Group(func(r chi.Router) {
			r.Use(writeLimit)
			r.With(middleware.Throttle(ingestConcurrencyLimit)).Post("/ingest", s.ingest)
			r.Route("/agent", func(r chi.Router) {
				r.Use(s.requireAgentToken)
				r.Get("/dirs", s.agentDirs)
				r.Get("/items", s.agentListItems)
				r.Delete("/items/{id}/mounts", s.agentUnpublishItem)
				r.Delete("/items/{id}", s.agentDeleteItem)
			})
		})

		r.Get("/dirs/{id}", s.getDir)
		r.With(attemptLimiter()).Post("/dirs/{id}/unlock", s.unlockDir)
		r.Get("/dirs/{id}/search", s.searchDir)
		r.Get("/items/{id}", s.getItem)
		r.Get("/items/{id}/content", s.getItemContent)
		r.Get("/assets/{hash}", s.getAsset)

		r.With(attemptLimiter()).Post("/admin/login", s.adminLogin)
		r.Post("/admin/logout", s.adminLogout)
		r.Group(func(r chi.Router) {
			r.Use(s.requireAdmin)
			r.Get("/admin/me", s.adminMe)
			r.Get("/admin/tree", s.adminTree)
			r.Post("/admin/dirs", s.adminCreateDir)
			r.Patch("/admin/dirs/{id}", s.adminUpdateDir)
			r.Delete("/admin/dirs/{id}", s.adminDeleteDir)
			r.Put("/admin/items/{id}/mounts", s.adminSetMounts)
			r.Delete("/admin/items/{id}", s.adminDeleteItem)
			r.Get("/admin/search", s.adminSearch)
		})
	})

	spa := frontendHandler(frontend)
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "not_found", "no such API endpoint")
			return
		}
		spa.ServeHTTP(w, r)
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})
	return router, nil
}

func securityHeaders() func(http.Handler) http.Handler {
	return secure.New(secure.Options{
		FrameDeny:          true,
		ContentTypeNosniff: true,
		ReferrerPolicy:     "no-referrer",
	}).Handler
}

func clientIPKey(r *http.Request) (string, error) {
	return httprate.CanonicalizeIP(middleware.GetClientIP(r.Context())), nil
}

// attemptLimiter allows 10 requests per minute per client IP for password-checking endpoints.
func attemptLimiter() func(http.Handler) http.Handler {
	return httprate.LimitBy(10, time.Minute,
		clientIPKey,
		httprate.WithLimitHandler(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusTooManyRequests, "too_many_attempts", "too many attempts, try again later")
		}),
	)
}

// writeLimiter allows 120 requests per minute per client IP across write/agent endpoints.
func writeLimiter() func(http.Handler) http.Handler {
	return httprate.LimitBy(writeRateLimitPerMin, time.Minute,
		clientIPKey,
		httprate.WithLimitHandler(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusTooManyRequests, "too_many_requests", "rate limit exceeded, try again later")
		}),
	)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func frontendHandler(frontend fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || !fileExists(frontend, name) {
			name = "index.html"
		}
		if !fileExists(frontend, name) {
			http.Error(w, "frontend build is not embedded", http.StatusServiceUnavailable)
			return
		}
		http.ServeFileFS(w, r, frontend, name)
	})
}

func fileExists(filesystem fs.FS, name string) bool {
	info, err := fs.Stat(filesystem, name)
	return err == nil && !info.IsDir()
}
