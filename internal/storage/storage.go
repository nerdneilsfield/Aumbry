// Package storage is the private object store; all reads go through the server.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"time"

	"github.com/studio-b12/gowebdav"
)

const (
	defaultDialTimeout           = 5 * time.Second
	defaultResponseHeaderTimeout = 30 * time.Second
	defaultProbeTimeout          = 2 * time.Second
)

// Sentinel errors returned by storage operations and readiness probes.
var (
	ErrNotFound    = errors.New("storage: object not found")
	ErrAuth        = errors.New("storage: authentication failed")
	ErrTimeout     = errors.New("storage: request timed out")
	ErrUnreachable = errors.New("storage: unreachable")
)

// Store abstracts private object storage. Paths are absolute, slash separated, e.g. /xml/{id}.xml.
type Store interface {
	Put(ctx context.Context, name string, body io.Reader) error
	Open(ctx context.Context, name string) (io.ReadCloser, error)
	Exists(ctx context.Context, name string) (bool, error)
	Delete(ctx context.Context, name string) error
	List(ctx context.Context, dir string) ([]string, error)
	Probe(ctx context.Context) error
}

// WebDAVOptions configures transport and readiness timeouts for the WebDAV client.
type WebDAVOptions struct {
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	ProbeTimeout          time.Duration
}

// WebDAV stores objects on a WebDAV server.
type WebDAV struct {
	client      *gowebdav.Client
	probeClient *gowebdav.Client
}

// NewWebDAV connects to the WebDAV root at rawURL using default transport timeouts.
func NewWebDAV(rawURL, user, password string) (*WebDAV, error) {
	return NewWebDAVWithOptions(rawURL, user, password, WebDAVOptions{})
}

// NewWebDAVWithOptions connects to the WebDAV root at rawURL with explicit timeout settings.
func NewWebDAVWithOptions(rawURL, user, password string, opts WebDAVOptions) (*WebDAV, error) {
	if _, err := url.Parse(rawURL); err != nil {
		return nil, err
	}
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = defaultDialTimeout
	}
	if opts.ResponseHeaderTimeout <= 0 {
		opts.ResponseHeaderTimeout = defaultResponseHeaderTimeout
	}
	if opts.ProbeTimeout <= 0 {
		opts.ProbeTimeout = defaultProbeTimeout
	}

	client := gowebdav.NewClient(rawURL, user, password)
	client.SetTransport(newWebDAVTransport(opts.DialTimeout, opts.ResponseHeaderTimeout))

	probeClient := gowebdav.NewClient(rawURL, user, password)
	probeClient.SetTransport(newWebDAVTransport(opts.ProbeTimeout, opts.ProbeTimeout))
	probeClient.SetTimeout(opts.ProbeTimeout)

	return &WebDAV{client: client, probeClient: probeClient}, nil
}

func newWebDAVTransport(dialTimeout, responseHeaderTimeout time.Duration) *http.Transport {
	var tr *http.Transport
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = base.Clone()
	} else {
		tr = &http.Transport{}
	}
	tr.DialContext = (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	tr.ResponseHeaderTimeout = responseHeaderTimeout
	return tr
}

// Put writes body to name, creating parent collections.
func (w *WebDAV) Put(_ context.Context, name string, body io.Reader) error {
	if err := w.client.MkdirAll(path.Dir(name), 0o755); err != nil {
		return err
	}
	return w.client.WriteStream(name, body, 0o644)
}

// Open streams name.
func (w *WebDAV) Open(_ context.Context, name string) (io.ReadCloser, error) {
	r, err := w.client.ReadStream(name)
	return r, mapErr(err)
}

// Exists reports whether name exists.
func (w *WebDAV) Exists(_ context.Context, name string) (bool, error) {
	_, err := w.client.Stat(name)
	if err = mapErr(err); errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Delete removes name; a missing object is not an error.
func (w *WebDAV) Delete(_ context.Context, name string) error {
	if err := mapErr(w.client.Remove(name)); !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

// List returns the full object paths directly inside dir, excluding subdirectories.
func (w *WebDAV) List(_ context.Context, dir string) ([]string, error) {
	entries, err := w.client.ReadDir(dir)
	if err != nil {
		return nil, mapErr(err)
	}
	paths := make([]string, 0, len(entries))
	for _, info := range entries {
		if info.IsDir() {
			continue
		}
		paths = append(paths, path.Join("/", dir, info.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

// Probe performs a bounded Stat("/") against the WebDAV root and classifies failures.
func (w *WebDAV) Probe(_ context.Context) error {
	_, err := w.probeClient.Stat("/")
	if err == nil {
		return nil
	}
	if gowebdav.IsErrCode(err, http.StatusUnauthorized) || gowebdav.IsErrCode(err, http.StatusForbidden) {
		return fmt.Errorf("%w: %w", ErrAuth, err)
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return fmt.Errorf("%w: %w", ErrTimeout, err)
	}
	return fmt.Errorf("%w: %w", ErrUnreachable, err)
}

func mapErr(err error) error {
	if err != nil && (gowebdav.IsErrNotFound(err) || errors.Is(err, fs.ErrNotExist)) {
		return ErrNotFound
	}
	return err
}
