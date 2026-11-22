package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func newFakeS3Server(t *testing.T, bucket string) (*httptest.Server, *s3mem.Backend) {
	t.Helper()
	backend := s3mem.New()
	if bucket != "" {
		if err := backend.CreateBucket(bucket); err != nil {
			t.Fatal(err)
		}
	}
	faker := gofakes3.New(backend)
	srv := httptest.NewServer(faker.Server())
	t.Cleanup(srv.Close)
	return srv, backend
}

func TestS3Conformance(t *testing.T) {
	runStoreConformanceTests(t, func(t *testing.T) Store {
		srv, _ := newFakeS3Server(t, "aumbry")
		store, err := NewS3(S3Options{
			Endpoint:        srv.URL,
			Region:          "us-east-1",
			Bucket:          "aumbry",
			AccessKeyID:     "test-ak",
			SecretAccessKey: "test-sk",
			PathStyle:       true,
			ListPageSize:    2,
		})
		if err != nil {
			t.Fatal(err)
		}
		return store
	})

	t.Run("list excludes directory placeholder and temp keys", func(t *testing.T) {
		srv, backend := newFakeS3Server(t, "aumbry")
		store, err := NewS3(S3Options{
			Endpoint:        srv.URL,
			Region:          "us-east-1",
			Bucket:          "aumbry",
			AccessKeyID:     "test-ak",
			SecretAccessKey: "test-sk",
			PathStyle:       true,
			ListPageSize:    2,
		})
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if _, putErr := backend.PutObject("aumbry", "xml/", nil, strings.NewReader(""), 0, nil); putErr != nil {
			t.Fatal(putErr)
		}
		if putErr := store.Put(ctx, "/xml/.tmp-partial", strings.NewReader("tmp")); putErr != nil {
			t.Fatal(putErr)
		}
		if putErr := store.Put(ctx, "/xml/live.xml", strings.NewReader("<ok/>")); putErr != nil {
			t.Fatal(putErr)
		}
		got, err := store.List(ctx, "/xml")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, []string{"/xml/live.xml"}) {
			t.Fatalf("List(/xml) = %v, want [/xml/live.xml]", got)
		}
	})
}

func TestS3NoTrailingChecksumHeaderForR2Compat(t *testing.T) {
	var mu sync.Mutex
	var gotHeaders http.Header

	srv, _ := newFakeS3Server(t, "aumbry")
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			mu.Lock()
			gotHeaders = r.Header.Clone()
			mu.Unlock()
		}
		srv.Config.Handler.ServeHTTP(w, r)
	}))
	defer proxy.Close()

	store, err := NewS3(S3Options{
		Endpoint:        proxy.URL,
		Bucket:          "aumbry",
		AccessKeyID:     "test-ak",
		SecretAccessKey: "test-sk",
		PathStyle:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "/xml/r2.xml", strings.NewReader("<document/>")); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if sha := gotHeaders.Get("X-Amz-Content-Sha256"); strings.Contains(sha, "TRAILER") {
		t.Fatalf("X-Amz-Content-Sha256 = %q, want non-trailing checksum for R2 compatibility", sha)
	}
	if gotHeaders.Get("X-Amz-Trailer") != "" {
		t.Fatalf("unexpected X-Amz-Trailer header %q", gotHeaders.Get("X-Amz-Trailer"))
	}
}

func TestS3ResponseHeaderTimeout(t *testing.T) {
	if defaultDialTimeout != 5*time.Second || defaultResponseHeaderTimeout != 30*time.Second || defaultProbeTimeout != 2*time.Second {
		t.Fatalf("unexpected default timeouts: dial=%v header=%v probe=%v",
			defaultDialTimeout, defaultResponseHeaderTimeout, defaultProbeTimeout)
	}

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	store, err := NewS3(S3Options{
		Endpoint:              srv.URL,
		Region:                "us-east-1",
		Bucket:                "aumbry",
		AccessKeyID:           "test-ak",
		SecretAccessKey:       "test-sk",
		PathStyle:             true,
		DialTimeout:           100 * time.Millisecond,
		ResponseHeaderTimeout: 60 * time.Millisecond,
		ProbeTimeout:          50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	err = store.Put(context.Background(), "/xml/slow.xml", strings.NewReader("<a/>"))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("Put() against hanging server = %v, want ErrTimeout", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("Put() took %v, expected prompt timeout", time.Since(start))
	}

	if err := store.Probe(context.Background()); !errors.Is(err, ErrTimeout) {
		t.Fatalf("Probe() against hanging server = %v, want ErrTimeout", err)
	}
}

func TestS3ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "head-")
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			<-r.Context().Done()
			return
		}
		// The server only notices a closed connection after the request body has been read.
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer srv.Close()

	store, err := NewS3(S3Options{
		Endpoint:        srv.URL,
		Region:          "us-east-1",
		Bucket:          "aumbry",
		AccessKeyID:     "test-ak",
		SecretAccessKey: "test-sk",
		PathStyle:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cancel while the request is in flight, so the request itself must honor ctx.
	cctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	started := time.Now()
	if putErr := store.Put(cctx, "/xml/c.xml", strings.NewReader("<a/>")); !errors.Is(putErr, context.Canceled) {
		t.Fatalf("canceled Put = %v, want context.Canceled", putErr)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("canceled Put returned after %v, want shortly after cancel", elapsed)
	}

	streamCtx, streamCancel := context.WithCancel(context.Background())
	r, err := store.Open(streamCtx, "/xml/stream.xml")
	if err != nil {
		streamCancel()
		t.Fatalf("Open stream failed: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(r, buf); err != nil {
		streamCancel()
		_ = r.Close()
		t.Fatalf("initial ReadFull = %v", err)
	}
	streamCancel()
	if _, err := io.ReadAll(r); err == nil {
		_ = r.Close()
		t.Fatal("ReadAll succeeded after canceling stream context")
	}
	_ = r.Close()
}

func TestS3ErrorMappingAndProbe(t *testing.T) {
	ctx := context.Background()

	t.Run("403 forbidden maps to ErrAuth including Open and Exists", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			if r.Method != http.MethodHead {
				_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
			}
		}))
		defer srv.Close()

		store, err := NewS3(S3Options{
			Endpoint:        srv.URL,
			Region:          "us-east-1",
			Bucket:          "aumbry",
			AccessKeyID:     "bad-ak",
			SecretAccessKey: "bad-sk",
			PathStyle:       true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Probe(ctx); !errors.Is(err, ErrAuth) {
			t.Fatalf("Probe() 403 = %v, want ErrAuth", err)
		}
		if _, err := store.Open(ctx, "/xml/missing-without-listbucket.xml"); !errors.Is(err, ErrAuth) {
			t.Fatalf("Open() 403 = %v, want ErrAuth (not ErrNotFound)", err)
		}
		if ok, err := store.Exists(ctx, "/xml/missing.xml"); ok || !errors.Is(err, ErrAuth) {
			t.Fatalf("Exists() 403 = (%v, %v), want (false, ErrAuth)", ok, err)
		}
	})

	t.Run("missing bucket maps to ErrUnreachable on Probe and ops", func(t *testing.T) {
		srv, _ := newFakeS3Server(t, "")
		store, err := NewS3(S3Options{
			Endpoint:        srv.URL,
			Region:          "us-east-1",
			Bucket:          "absent-bucket",
			AccessKeyID:     "test-ak",
			SecretAccessKey: "test-sk",
			PathStyle:       true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Probe(ctx); !errors.Is(err, ErrUnreachable) {
			t.Fatalf("Probe() missing bucket = %v, want ErrUnreachable", err)
		}
		if _, err := store.Open(ctx, "/xml/a.xml"); !errors.Is(err, ErrUnreachable) {
			t.Fatalf("Open() missing bucket = %v, want ErrUnreachable (not ErrNotFound)", err)
		}
		if _, err := store.List(ctx, "/xml"); !errors.Is(err, ErrUnreachable) {
			t.Fatalf("List() missing bucket = %v, want ErrUnreachable", err)
		}
	})

	t.Run("unreachable endpoint maps to ErrUnreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		closedURL := srv.URL
		srv.Close()

		store, err := NewS3(S3Options{
			Endpoint:        closedURL,
			Region:          "us-east-1",
			Bucket:          "aumbry",
			AccessKeyID:     "test-ak",
			SecretAccessKey: "test-sk",
			PathStyle:       true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Probe(ctx); !errors.Is(err, ErrUnreachable) {
			t.Fatalf("Probe() closed = %v, want ErrUnreachable", err)
		}
	})
}

// A service that echoes the Authorization header in its error body must not leak it through the returned error,
// and a 404 on a listing is a failure rather than an empty directory.
func TestS3ErrorsDoNotEchoBackendText(t *testing.T) {
	const leak = "AKIALEAKEDKEYID"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<Error><Code>SignatureDoesNotMatch</Code><Message>bad `+r.Header.Get("Authorization")+`</Message></Error>`)
	}))
	t.Cleanup(srv.Close)
	store, err := NewS3(S3Options{Endpoint: srv.URL, Region: "us-east-1", Bucket: "b", AccessKeyID: leak, SecretAccessKey: "sk", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Open(context.Background(), "/xml/a.xml")
	if !errors.Is(err, ErrAuth) || strings.Contains(err.Error(), leak) || strings.Contains(err.Error(), "Credential") {
		t.Errorf("Open error = %v, want ErrAuth without the access key id", err)
	}
	if perr := store.Probe(context.Background()); !errors.Is(perr, ErrAuth) || strings.Contains(perr.Error(), leak) {
		t.Errorf("Probe error = %v, want ErrAuth without the access key id", perr)
	}
	if _, lerr := store.List(context.Background(), "/xml"); !errors.Is(lerr, ErrUnreachable) {
		t.Errorf("List on a 404 = %v, want ErrUnreachable", lerr)
	}
}
