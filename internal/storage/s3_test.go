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

	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	if putErr := store.Put(cctx, "/xml/c.xml", strings.NewReader("<a/>")); !errors.Is(putErr, context.Canceled) {
		t.Fatalf("canceled Put = %v, want context.Canceled", putErr)
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
