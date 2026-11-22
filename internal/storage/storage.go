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
	"os"
	"path"
	"sort"
	"strings"
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
	Put(ctx context.Context, name string, body io.ReadSeeker) error
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
	client.SetTransport(newHTTPTransport(opts.DialTimeout, opts.ResponseHeaderTimeout))

	probeClient := gowebdav.NewClient(rawURL, user, password)
	probeClient.SetTransport(newHTTPTransport(opts.ProbeTimeout, opts.ProbeTimeout))
	probeClient.SetTimeout(opts.ProbeTimeout)

	return &WebDAV{client: client, probeClient: probeClient}, nil
}

func newHTTPTransport(dialTimeout, responseHeaderTimeout time.Duration) *http.Transport {
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

// normalizeObjectPath validates an absolute, canonical object path beneath /xml/ or /assets/
// and returns the relative path without the leading slash.
func normalizeObjectPath(name string) (string, error) {
	if name == "" || !strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00") {
		return "", fmt.Errorf("invalid storage object path %q", name)
	}
	if name != path.Clean(name) {
		return "", fmt.Errorf("non-canonical storage object path %q", name)
	}
	if !strings.HasPrefix(name, "/xml/") && !strings.HasPrefix(name, "/assets/") {
		return "", fmt.Errorf("storage object path %q must be under /xml/ or /assets/", name)
	}
	return strings.TrimPrefix(name, "/"), nil
}

// normalizeDirPath validates an absolute, canonical directory path under /xml or /assets
// and returns the relative path without the leading slash.
func normalizeDirPath(dir string) (string, error) {
	if dir == "" || !strings.HasPrefix(dir, "/") || strings.ContainsAny(dir, "\\\x00") {
		return "", fmt.Errorf("invalid storage directory path %q", dir)
	}
	if dir != path.Clean(dir) {
		return "", fmt.Errorf("non-canonical storage directory path %q", dir)
	}
	if dir != "/xml" && dir != "/assets" && !strings.HasPrefix(dir, "/xml/") && !strings.HasPrefix(dir, "/assets/") {
		return "", fmt.Errorf("storage directory path %q must be /xml, /assets, or a child path", dir)
	}
	return strings.TrimPrefix(dir, "/"), nil
}

// Put writes body to name, creating parent collections.
func (w *WebDAV) Put(ctx context.Context, name string, body io.ReadSeeker) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := w.client.MkdirAll(path.Dir(name), 0o755); err != nil {
		return err
	}
	return w.client.WriteStream(name, body, 0o644)
}

// Open streams name.
func (w *WebDAV) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r, err := w.client.ReadStream(name)
	return r, mapWebDAVErr(err)
}

// Exists reports whether name exists.
func (w *WebDAV) Exists(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	info, err := w.client.Stat(name)
	if err = mapWebDAVErr(err); errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !validFileInfo(info) {
		return false, fmt.Errorf("%w: incomplete WebDAV Stat response for %s", ErrUnreachable, name)
	}
	return true, nil
}

// Delete removes name; a missing object is not an error.
func (w *WebDAV) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := mapWebDAVErr(w.client.Remove(name)); !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

// List returns the full object paths directly inside dir, excluding subdirectories.
func (w *WebDAV) List(ctx context.Context, dir string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := w.client.ReadDir(dir)
	if err != nil {
		return nil, mapWebDAVErr(err)
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
func (w *WebDAV) Probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %w", ErrTimeout, err)
		}
		return fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	info, err := w.probeClient.Stat("/")
	if err == nil {
		if !validFileInfo(info) {
			return fmt.Errorf("%w: incomplete WebDAV Stat response for /", ErrUnreachable)
		}
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

// gowebdav.Stat returns a typed-nil (*gowebdav.File)(nil) with err == nil when a 207 response
// omits a 200 propstat or fails XML decoding; reject that as an incomplete/unhealthy response.
func validFileInfo(info os.FileInfo) bool {
	if info == nil {
		return false
	}
	if f, ok := info.(*gowebdav.File); ok {
		return f != nil
	}
	return true
}

func mapWebDAVErr(err error) error {
	if err != nil && (gowebdav.IsErrNotFound(err) || errors.Is(err, fs.ErrNotExist)) {
		return ErrNotFound
	}
	return err
}
