package storage

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

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
