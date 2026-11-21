package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHealthEndpoint(t *testing.T) {
	response := httptest.NewRecorder()
	router, err := NewRouter()
	if err != nil {
		t.Fatal(err)
	}
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
}

func TestUnimplementedAPIIsExplicit(t *testing.T) {
	response := httptest.NewRecorder()
	router, err := NewRouter()
	if err != nil {
		t.Fatal(err)
	}
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/dirs/example", nil))

	if response.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotImplemented)
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
