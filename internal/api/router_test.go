package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHealthEndpoint(t *testing.T) {
	e := newEnv(t)
	resp := e.anon().do("GET", "/healthz", nil)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.status, http.StatusOK)
	}
	if got := resp.header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
}

func TestUnknownAPIPathIsJSON404NotSPA(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{"/api/v1/nope", "/api/v1/dirs/x/y/z", "/api/other"} {
		resp := e.anon().do("GET", path, nil)
		if resp.status != http.StatusNotFound || resp.json()["error"] != "not_found" {
			t.Errorf("%s: status %d body %s, want 404 not_found JSON", path, resp.status, resp.body)
		}
	}
}

func TestFrontendHandlerServesSPA(t *testing.T) {
	frontend := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>app</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("app()")},
	}
	handler := frontendHandler(frontend)
	for _, path := range []string{"/", "/d/example", "/assets/app.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("%s status = %d, want %d", path, response.Code, http.StatusOK)
		}
	}
}
