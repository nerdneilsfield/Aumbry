package storage

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
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

	if ok, existsErr := store.Exists(ctx, "/xml/a.xml"); existsErr != nil || ok {
		t.Fatalf("Exists before put = %v, %v", ok, existsErr)
	}
	if _, err = store.Open(ctx, "/xml/a.xml"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open missing = %v, want ErrNotFound", err)
	}
	if err = store.Put(ctx, "/xml/a.xml", strings.NewReader("<a/>")); err != nil {
		t.Fatal(err)
	}
	if err = store.Put(ctx, "/xml/a.xml", strings.NewReader("<b/>")); err != nil {
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

	if _, err = store.List(ctx, "/xml"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("List missing dir = %v, want ErrNotFound", err)
	}

	for _, name := range []string{"/xml/b.xml", "/xml/a.xml", "/xml/sub/c.xml"} {
		if err = store.Put(ctx, name, strings.NewReader("<x/>")); err != nil {
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

func TestWebDAVIncomplete207Rejected(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		body string
	}{
		{
			name: "empty multistatus",
			body: `<?xml version="1.0" encoding="utf-8"?><D:multistatus xmlns:D="DAV:"/>`,
		},
		{
			name: "response without 200 propstat",
			body: `<?xml version="1.0" encoding="utf-8"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>/xml/a.xml</D:href><D:propstat><D:prop/><D:status>HTTP/1.1 404 Not Found</D:status></D:propstat></D:response></D:multistatus>`,
		},
		{
			name: "truncated XML body",
			body: `<?xml version="1.0" encoding="utf-8"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>/`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "PROPFIND" {
					t.Fatalf("unexpected method %s", r.Method)
				}
				w.Header().Set("Content-Type", "application/xml; charset=utf-8")
				w.WriteHeader(http.StatusMultiStatus)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			store, err := NewWebDAV(server.URL, "u", "p")
			if err != nil {
				t.Fatal(err)
			}
			if err = store.Probe(ctx); !errors.Is(err, ErrUnreachable) {
				t.Fatalf("Probe() = %v, want ErrUnreachable", err)
			}
			exists, err := store.Exists(ctx, "/xml/a.xml")
			if exists || err == nil {
				t.Fatalf("Exists() = (%v, %v), want (false, non-nil error)", exists, err)
			}
		})
	}
}

func TestWebDAVAtomicPutAndTempFiltering(t *testing.T) {
	fsys := webdav.NewMemFS()
	server := httptest.NewServer(&webdav.Handler{FileSystem: fsys, LockSystem: webdav.NewMemLS()})
	defer server.Close()

	store, err := NewWebDAV(server.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Seed a stray .tmp-* file directly in WebDAV storage and verify List hides it.
	if err = store.client.MkdirAll("/assets", 0o755); err != nil {
		t.Fatal(err)
	}
	if err = store.client.WriteStream("/assets/.tmp-leftover", strings.NewReader("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate an interrupted upload on a brand-new asset path.
	payload := bytes.Repeat([]byte("A"), 64*1024)
	broken := &failingReadSeeker{data: payload, failAt: 1024, err: errors.New("stream reset")}
	if putErr := store.Put(ctx, "/assets/atom.bin", broken); putErr == nil {
		t.Fatal("expected Put with failing reader to fail")
	}

	if ok, exErr := store.Exists(ctx, "/assets/atom.bin"); exErr != nil || ok {
		t.Fatalf("Exists(/assets/atom.bin) after failed Put = (%v, %v), want (false, nil)", ok, exErr)
	}
	listed, listErr := store.List(ctx, "/assets")
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(listed) != 0 {
		t.Fatalf("List(/assets) = %v, want empty (no .tmp-* exposed)", listed)
	}

	// Retry same path with full content succeeds cleanly.
	if err = store.Put(ctx, "/assets/atom.bin", bytes.NewReader(payload)); err != nil {
		t.Fatalf("retry Put = %v", err)
	}
	rc, err := store.Open(ctx, "/assets/atom.bin")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, payload) {
		t.Fatalf("reopened payload len=%d, want %d", len(got), len(payload))
	}

	// Simulate interrupted overwrite: existing intact object must remain readable.
	broken2 := &failingReadSeeker{data: bytes.Repeat([]byte("B"), 16*1024), failAt: 512, err: errors.New("reset")}
	if overwriteErr := store.Put(ctx, "/assets/atom.bin", broken2); overwriteErr == nil {
		t.Fatal("expected failing overwrite to error")
	}
	rc2, err := store.Open(ctx, "/assets/atom.bin")
	if err != nil {
		t.Fatal(err)
	}
	got2, _ := io.ReadAll(rc2)
	_ = rc2.Close()
	if !bytes.Equal(got2, payload) {
		t.Fatalf("existing object corrupted on failed overwrite (len=%d)", len(got2))
	}
}

func TestWebDAVOpenRangeModesAndCancellation(t *testing.T) {
	ctx := context.Background()
	payload := []byte("abcdefghijklmnopqrstuvwxyz0123456789")

	t.Run("native 206 partial content on webdav handler", func(t *testing.T) {
		server := httptest.NewServer(&webdav.Handler{FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()})
		defer server.Close()
		store, err := NewWebDAV(server.URL, "u", "p")
		if err != nil {
			t.Fatal(err)
		}
		if err = store.Put(ctx, "/assets/song.mp3", bytes.NewReader(payload)); err != nil {
			t.Fatal(err)
		}
		r, err := store.OpenRange(ctx, "/assets/song.mp3", 5, 10)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil || string(got) != "fghijk" {
			t.Fatalf("OpenRange 206 = (%q, %v), want fghijk", got, err)
		}
	})

	t.Run("server ignores Range and returns 200 OK", func(t *testing.T) {
		var sawRange atomic.Value
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawRange.Store(r.Header.Get("Range"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
		}))
		defer server.Close()

		store, err := NewWebDAV(server.URL+"/prefix", "u", "p")
		if err != nil {
			t.Fatal(err)
		}
		r, err := store.OpenRange(ctx, "/assets/song.mp3", 26, 30)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil || string(got) != "01234" {
			t.Fatalf("OpenRange 200 fallback = (%q, %v), want 01234", got, err)
		}
		if wantHeader := "bytes=26-30"; sawRange.Load() != wantHeader {
			t.Fatalf("Range header = %v, want %s", sawRange.Load(), wantHeader)
		}
	})

	t.Run("ctx canceled during 200 skip exits promptly", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			chunk := bytes.Repeat([]byte("x"), 8192)
			for range 100 {
				if _, writeErr := w.Write(chunk); writeErr != nil {
					return
				}
				if fl, ok := w.(http.Flusher); ok {
					fl.Flush()
				}
				select {
				case <-r.Context().Done():
					return
				case <-time.After(15 * time.Millisecond):
				}
			}
		}))
		defer server.Close()

		store, err := NewWebDAV(server.URL, "u", "p")
		if err != nil {
			t.Fatal(err)
		}
		reqCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err = store.OpenRange(reqCtx, "/assets/slow.mp3", 1024*1024, 1024*1024+10)
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Fatalf("OpenRange timeout error = %v, want context cancellation/deadline", err)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("OpenRange cancellation took %v, expected prompt exit", elapsed)
		}
	})
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func parseDigestHeader(raw string) map[string]string {
	out := make(map[string]string)
	rest := strings.TrimPrefix(strings.TrimSpace(raw), "Digest ")
	for _, part := range strings.Split(rest, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return out
}

func TestWebDAVDigestAuthContentAddressedNoOverwriteAndCancel(t *testing.T) {
	ctx := context.Background()

	t.Run("digest-only WebDAV authenticates Put, Open, OpenRange (200 and 206)", func(t *testing.T) {
		inner := &webdav.Handler{FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()}
		const realm = "aumbry-dav"
		const nonce = "nonce1234567890"
		const user = "davuser"
		const pass = "davpass"
		ha1 := md5Hex(user + ":" + realm + ":" + pass)

		var sawDigest atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHdr := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHdr, "Digest ") {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Digest realm="%s", nonce="%s", qop="auth"`, realm, nonce))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			parts := parseDigestHeader(authHdr)
			ha2 := md5Hex(r.Method + ":" + parts["uri"])
			want := md5Hex(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, parts["nonce"], parts["nc"], parts["cnonce"], parts["qop"], ha2))
			if parts["username"] != user || parts["response"] != want {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Digest realm="%s", nonce="%s", qop="auth", stale=true`, realm, nonce))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			sawDigest.Store(true)
			inner.ServeHTTP(w, r)
		}))
		defer server.Close()

		store, err := NewWebDAV(server.URL, user, pass)
		if err != nil {
			t.Fatal(err)
		}
		assetPath := "/assets/" + strings.Repeat("a", 64) + ".mp3"
		payload := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
		if err = store.Put(ctx, assetPath, bytes.NewReader(payload)); err != nil {
			t.Fatalf("Digest Put = %v", err)
		}

		rc206, err := store.OpenRange(ctx, assetPath, 4, 9)
		if err != nil {
			t.Fatalf("Digest OpenRange 206 = %v", err)
		}
		got206, _ := io.ReadAll(rc206)
		_ = rc206.Close()
		if string(got206) != "456789" {
			t.Fatalf("Digest OpenRange 206 = %q, want 456789", got206)
		}

		rcFull, err := store.OpenRange(ctx, assetPath, 0, int64(len(payload)-1))
		if err != nil {
			t.Fatalf("Digest OpenRange full = %v", err)
		}
		gotFull, _ := io.ReadAll(rcFull)
		_ = rcFull.Close()
		if !bytes.Equal(gotFull, payload) || !sawDigest.Load() {
			t.Fatalf("Digest full read failed (len=%d sawDigest=%v)", len(gotFull), sawDigest.Load())
		}
	})

	t.Run("content-addressed MOVE sends Overwrite: F and treats 412 existing object as dedup success", func(t *testing.T) {
		inner := &webdav.Handler{FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()}
		var mu sync.Mutex
		var sawOverwrite []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "MOVE" {
				mu.Lock()
				sawOverwrite = append(sawOverwrite, r.Header.Get("Overwrite"))
				mu.Unlock()
			}
			inner.ServeHTTP(w, r)
		}))
		defer server.Close()

		store, err := NewWebDAV(server.URL, "u", "p")
		if err != nil {
			t.Fatal(err)
		}
		assetPath := "/assets/" + strings.Repeat("b", 64) + ".mp3"
		orig := []byte("original-audio-bytes")
		if err = store.Put(ctx, assetPath, bytes.NewReader(orig)); err != nil {
			t.Fatalf("first Put = %v", err)
		}
		// Second Put on same content-addressed path triggers Overwrite: F -> 412 -> dedup success, tmp cleaned up.
		if err = store.Put(ctx, assetPath, bytes.NewReader([]byte("raced-second-bytes"))); err != nil {
			t.Fatalf("duplicate content-addressed Put = %v, want nil dedup success", err)
		}
		if len(sawOverwrite) != 2 || sawOverwrite[0] != "F" || sawOverwrite[1] != "F" {
			t.Fatalf("MOVE Overwrite headers = %v, want [F F]", sawOverwrite)
		}
		rc, err := store.Open(ctx, assetPath)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(rc)
		_ = rc.Close()
		if !bytes.Equal(got, orig) {
			t.Fatalf("content changed on Overwrite:F MOVE = %q, want %q", got, orig)
		}
		list, _ := store.List(ctx, "/assets")
		if len(list) != 1 || list[0] != assetPath {
			t.Fatalf("List(/assets) = %v, want [%s]", list, assetPath)
		}
	})

	t.Run("stalled WebDAV PUT respects ctx cancel promptly and removes temp file", func(t *testing.T) {
		inner := &webdav.Handler{FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()}
		putStarted := make(chan struct{}, 1)
		release := make(chan struct{})
		var deleteCount atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, ".tmp-") {
				inner.ServeHTTP(w, r)
				select {
				case putStarted <- struct{}{}:
				default:
				}
				select {
				case <-release:
				case <-r.Context().Done():
				}
				return
			}
			if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, ".tmp-") {
				deleteCount.Add(1)
			}
			inner.ServeHTTP(w, r)
		}))
		defer server.Close()
		defer close(release)

		store, err := NewWebDAV(server.URL, "u", "p")
		if err != nil {
			t.Fatal(err)
		}
		reqCtx, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
		defer cancel()
		start := time.Now()
		err = store.Put(reqCtx, "/assets/"+strings.Repeat("c", 64)+".mp3", bytes.NewReader([]byte("abc")))
		if err == nil {
			t.Fatal("expected stalled Put to fail on ctx timeout")
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("stalled Put took %v, expected prompt cancel", elapsed)
		}
		if deleteCount.Load() == 0 {
			t.Fatalf("expected temp file cleanup DELETE after canceled Put")
		}
	})
}
