// Package api provides the HTTP router and handlers.
package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/go-chi/httprate"
	"github.com/jmoiron/sqlx"

	"github.com/nerdneilsfield/aumbry/internal/auth"
	"github.com/nerdneilsfield/aumbry/internal/storage"
	"github.com/nerdneilsfield/aumbry/internal/webembed"
)

// Server holds the dependencies shared by all handlers.
type Server struct {
	db     *sqlx.DB
	store  storage.Store
	auth   *auth.Sessions
	logger *slog.Logger
}

// NewRouter creates the API routes and embedded frontend handler.
func NewRouter(database *sqlx.DB, store storage.Store, sessions *auth.Sessions, logger *slog.Logger) (http.Handler, error) {
	frontend, err := webembed.Files()
	if err != nil {
		return nil, err
	}
	s := &Server{db: database, store: store, auth: sessions, logger: logger}

	router := chi.NewRouter()
	// The client IP is the TCP peer. Behind a reverse proxy, replace this with the matching
	// middleware.ClientIPFrom* for your proxy; never trust forwarding headers by default.
	router.Use(middleware.ClientIPFromRemoteAddr)
	router.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        httplog.SchemaECS.Concise(true),
		RecoverPanics: true,
		Skip:          func(r *http.Request, _ int) bool { return r.URL.Path == "/healthz" },
	}))

	router.Get("/healthz", health)
	router.Route("/api/v1", func(r chi.Router) {
		r.Post("/ingest", s.ingest)

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
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})
	return router, nil
}

// attemptLimiter allows 10 requests per minute per client IP for password-checking endpoints.
func attemptLimiter() func(http.Handler) http.Handler {
	return httprate.LimitBy(10, time.Minute,
		func(r *http.Request) (string, error) {
			return httprate.CanonicalizeIP(middleware.GetClientIP(r.Context())), nil
		},
		httprate.WithLimitHandler(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusTooManyRequests, "too_many_attempts", "too many attempts, try again later")
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
