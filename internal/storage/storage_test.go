package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/webdav"
)

func TestWebDAVRoundTrip(t *testing.T) {
	server := httptest.NewServer(&webdav.Handler{FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()})
	defer server.Close()
	store, err := NewWebDAV(server.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if ok, err := store.Exists(ctx, "/xml/a.xml"); err != nil || ok {
		t.Fatalf("Exists before put = %v, %v", ok, err)
	}
	if _, err := store.Open(ctx, "/xml/a.xml"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open missing = %v, want ErrNotFound", err)
	}
	if err := store.Put(ctx, "/xml/a.xml", strings.NewReader("<a/>")); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "/xml/a.xml", strings.NewReader("<b/>")); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	r, err := store.Open(ctx, "/xml/a.xml")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if string(got) != "<b/>" {
		t.Fatalf("content = %q", got)
	}
	if err := store.Delete(ctx, "/xml/a.xml"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "/xml/a.xml"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
	if ok, _ := store.Exists(ctx, "/xml/a.xml"); ok {
		t.Fatal("still exists after delete")
	}
}

func TestWebDAVList(t *testing.T) {
	server := httptest.NewServer(&webdav.Handler{FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()})
	defer server.Close()
	store, err := NewWebDAV(server.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := store.List(ctx, "/xml"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("List missing dir = %v, want ErrNotFound", err)
	}

	for _, name := range []string{"/xml/b.xml", "/xml/a.xml", "/xml/sub/c.xml"} {
		if err := store.Put(ctx, name, strings.NewReader("<x/>")); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.List(ctx, "/xml")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/xml/a.xml", "/xml/b.xml"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List(/xml) = %v, want %v", got, want)
	}
}

func TestWebDAVProbe(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		server := httptest.NewServer(&webdav.Handler{FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()})
		defer server.Close()
		store, err := NewWebDAV(server.URL, "u", "p")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Probe(ctx); err != nil {
			t.Fatalf("Probe() = %v, want nil", err)
		}
	})

	t.Run("auth failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Basic realm="dav"`)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()
		store, err := NewWebDAV(server.URL, "u", "wrong")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Probe(ctx); !errors.Is(err, ErrAuth) {
			t.Fatalf("Probe() = %v, want ErrAuth", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}))
		defer server.Close()
		defer close(release)
		store, err := NewWebDAVWithOptions(server.URL, "u", "p", WebDAVOptions{
			DialTimeout:           100 * time.Millisecond,
			ResponseHeaderTimeout: 100 * time.Millisecond,
			ProbeTimeout:          50 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Probe(ctx); !errors.Is(err, ErrTimeout) {
			t.Fatalf("Probe() = %v, want ErrTimeout", err)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := server.URL
		server.Close()
		store, err := NewWebDAV(url, "u", "p")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Probe(ctx); !errors.Is(err, ErrUnreachable) {
			t.Fatalf("Probe() = %v, want ErrUnreachable", err)
		}
	})
}

func TestWebDAVResponseHeaderTimeout(t *testing.T) {
	if defaultDialTimeout != 5*time.Second || defaultResponseHeaderTimeout != 30*time.Second || defaultProbeTimeout != 2*time.Second {
		t.Fatalf("unexpected default timeouts: dial=%v header=%v probe=%v",
			defaultDialTimeout, defaultResponseHeaderTimeout, defaultProbeTimeout)
	}

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	store, err := NewWebDAVWithOptions(server.URL, "u", "p", WebDAVOptions{
		DialTimeout:           100 * time.Millisecond,
		ResponseHeaderTimeout: 60 * time.Millisecond,
		ProbeTimeout:          50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = store.Put(context.Background(), "/xml/slow.xml", strings.NewReader("<a/>"))
	if err == nil {
		t.Fatal("Put() succeeded against hanging server, want timeout error")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("Put() took %v, expected prompt timeout", time.Since(start))
	}
}
