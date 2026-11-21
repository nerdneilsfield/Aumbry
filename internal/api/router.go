// Package api provides the HTTP router and handlers.
package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/nerdneilsfield/aumbry/internal/webembed"
)

// NewRouter creates the API routes and embedded frontend handler.
func NewRouter() (http.Handler, error) {
	frontend, err := webembed.Files()
	if err != nil {
		return nil, err
	}
	router := chi.NewRouter()
	router.Get("/healthz", health)
	router.Get("/api/v1/*", notImplemented)
	router.Post("/api/v1/*", notImplemented)
	router.Put("/api/v1/*", notImplemented)
	router.Patch("/api/v1/*", notImplemented)
	router.Delete("/api/v1/*", notImplemented)
	router.Handle("/*", frontendHandler(frontend))
	return router, nil
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error":   "not_implemented",
		"message": "this API is not implemented yet",
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
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
