// Package storage is the private object store; all reads go through the server.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

const (
	defaultDialTimeout           = 5 * time.Second
	defaultResponseHeaderTimeout = 30 * time.Second
	defaultProbeTimeout          = 2 * time.Second
	webdavCleanupTimeout         = 5 * time.Second
	maxWebDAVAuthAttempts        = 4
)

var (
	contentAddressedAssetRe = regexp.MustCompile(`^/assets/[0-9a-f]{64}\.[a-z0-9]+$`)
	contentAddressedXMLRe   = regexp.MustCompile(`^/xml/[A-Za-z0-9_-]{12}-[0-9a-f]{64}\.xml$`)
)

func isContentAddressedPath(name string) bool {
	return contentAddressedAssetRe.MatchString(name) || contentAddressedXMLRe.MatchString(name)
}

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
	// OpenRange reads an inclusive byte range [start, end] where 0 <= start <= end.
	OpenRange(ctx context.Context, name string, start, end int64) (io.ReadCloser, error)
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
	baseURL     *url.URL
	user        string
	password    string
	auth        gowebdav.Authorizer
	client      *gowebdav.Client
	probeClient *gowebdav.Client
	httpClient  *http.Client
}

// NewWebDAV connects to the WebDAV root at rawURL using default transport timeouts.
func NewWebDAV(rawURL, user, password string) (*WebDAV, error) {
	return NewWebDAVWithOptions(rawURL, user, password, WebDAVOptions{})
}

// NewWebDAVWithOptions connects to the WebDAV root at rawURL with explicit timeout settings.
func NewWebDAVWithOptions(rawURL, user, password string, opts WebDAVOptions) (*WebDAV, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
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

	auth := gowebdav.NewAutoAuth(user, password)
	client := gowebdav.NewAuthClient(rawURL, auth)
	client.SetTransport(newHTTPTransport(opts.DialTimeout, opts.ResponseHeaderTimeout))

	probeClient := gowebdav.NewAuthClient(rawURL, auth)
	probeClient.SetTransport(newHTTPTransport(opts.ProbeTimeout, opts.ProbeTimeout))
	probeClient.SetTimeout(opts.ProbeTimeout)

	httpClient := &http.Client{
		Transport: newHTTPTransport(opts.DialTimeout, opts.ResponseHeaderTimeout),
	}

	return &WebDAV{
		baseURL:     parsedURL,
		user:        user,
		password:    password,
		auth:        auth,
		client:      client,
		probeClient: probeClient,
		httpClient:  httpClient,
	}, nil
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

type limitedReadCloser struct {
	r io.Reader
	c io.Closer
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	return l.r.Read(p)
}

func (l *limitedReadCloser) Close() error {
	return l.c.Close()
}

func validateRange(start, end int64) error {
	if start < 0 || end < start {
		return fmt.Errorf("storage: invalid byte range [%d, %d]", start, end)
	}
	if end-start == math.MaxInt64 {
		return errors.New("storage: byte range length overflow")
	}
	return nil
}

func discardWithContext(ctx context.Context, r io.Reader, n int64) error {
	remaining := n
	buf := make([]byte, 32*1024)
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := len(buf)
		if remaining < int64(chunk) {
			chunk = int(remaining)
		}
		readN, err := r.Read(buf[:chunk])
		remaining -= int64(readN)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
	return nil
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

func (w *WebDAV) targetURL(objPath string) *url.URL {
	rel := strings.TrimPrefix(objPath, "/")
	target := *w.baseURL
	basePath := strings.TrimSuffix(target.Path, "/")
	if rel == "" {
		target.Path = basePath + "/"
	} else {
		target.Path = basePath + "/" + rel
	}
	target.RawPath = ""
	return &target
}

// doAuthRequest executes a context-bound WebDAV HTTP request using w.auth (supporting Basic and Digest
// challenges) while streaming body directly from io.ReadSeeker without copying into RAM.
func (w *WebDAV) doAuthRequest(
	ctx context.Context,
	method, objPath string,
	body io.ReadSeeker,
	headerFn func(*http.Request),
) (*http.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	u := w.targetURL(objPath)
	uri := u.String()
	authURI := u.EscapedPath()
	if authURI == "" {
		authURI = "/"
	}

	var bodySize int64
	if body != nil {
		endPos, err := body.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
		if _, err = body.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		bodySize = endPos
	}

	authenticator, _ := w.auth.NewAuthenticator(nil)
	defer func() { _ = authenticator.Close() }()

	lastStatus := 0
	for range maxWebDAVAuthAttempts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var reqBody io.Reader
		if body != nil {
			if _, seekErr := body.Seek(0, io.SeekStart); seekErr != nil {
				return nil, seekErr
			}
			reqBody = io.NopCloser(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, uri, reqBody)
		if err != nil {
			return nil, err
		}
		if body != nil {
			req.ContentLength = bodySize
			req.GetBody = func() (io.ReadCloser, error) {
				if _, seekErr := body.Seek(0, io.SeekStart); seekErr != nil {
					return nil, seekErr
				}
				return io.NopCloser(body), nil
			}
		}
		if headerFn != nil {
			headerFn(req)
		}
		if err = authenticator.Authorize(w.httpClient, req, authURI); err != nil {
			return nil, err
		}
		// Keep our seeking GetBody rather than authShim's buffered nil-body GetBody.
		if body != nil {
			req.GetBody = func() (io.ReadCloser, error) {
				if _, seekErr := body.Seek(0, io.SeekStart); seekErr != nil {
					return nil, seekErr
				}
				return io.NopCloser(body), nil
			}
		}

		resp, doErr := w.httpClient.Do(req)
		if doErr != nil {
			return nil, doErr
		}
		lastStatus = resp.StatusCode

		redo, verifyErr := authenticator.Verify(w.httpClient, resp, authURI)
		if redo {
			_ = resp.Body.Close()
			continue
		}
		if verifyErr != nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return nil, fmt.Errorf("%w: webdav status %d", ErrAuth, resp.StatusCode)
			}
			return nil, verifyErr
		}
		return resp, nil
	}

	if lastStatus == http.StatusUnauthorized || lastStatus == http.StatusForbidden {
		return nil, fmt.Errorf("%w: webdav status %d", ErrAuth, lastStatus)
	}
	return nil, fmt.Errorf("%w: webdav auth negotiation failed", ErrAuth)
}

func (w *WebDAV) deleteTemp(tmpName string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), webdavCleanupTimeout)
	defer cancel()
	resp, err := w.doAuthRequest(cleanupCtx, http.MethodDelete, tmpName, nil, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	_ = err
}

func (w *WebDAV) ensureDir(ctx context.Context, dir string) error {
	clean := path.Clean(dir)
	if clean == "/" || clean == "." || clean == "" {
		return nil
	}
	resp, err := w.doAuthRequest(ctx, "MKCOL", clean, nil, nil)
	if err != nil {
		return err
	}
	status := resp.StatusCode
	_ = resp.Body.Close()

	switch status {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent, http.StatusMethodNotAllowed:
		return nil
	case http.StatusConflict:
		parent := path.Dir(clean)
		if parent == clean || parent == "/" || parent == "." {
			return fmt.Errorf("webdav: MKCOL %s failed with status %d", clean, status)
		}
		if err := w.ensureDir(ctx, parent); err != nil {
			return err
		}
		resp2, err2 := w.doAuthRequest(ctx, "MKCOL", clean, nil, nil)
		if err2 != nil {
			return err2
		}
		status2 := resp2.StatusCode
		_ = resp2.Body.Close()
		if status2 == http.StatusOK || status2 == http.StatusCreated || status2 == http.StatusNoContent || status2 == http.StatusMethodNotAllowed {
			return nil
		}
		return fmt.Errorf("webdav: MKCOL %s returned status %d", clean, status2)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: webdav MKCOL status %d", ErrAuth, status)
	default:
		return fmt.Errorf("webdav: MKCOL %s returned status %d", clean, status)
	}
}

func (w *WebDAV) moveObject(ctx context.Context, src, dst string, overwrite bool) (int, error) {
	destURL := w.targetURL(dst).String()
	overwriteHdr := "F"
	if overwrite {
		overwriteHdr = "T"
	}
	resp, err := w.doAuthRequest(ctx, "MOVE", src, nil, func(req *http.Request) {
		req.Header.Set("Destination", destURL)
		req.Header.Set("Overwrite", overwriteHdr)
	})
	if err != nil {
		return 0, err
	}
	status := resp.StatusCode
	_ = resp.Body.Close()
	return status, nil
}

// Put writes body atomically via a context-bound sibling .tmp-* PUT and MOVE, using
// Overwrite: F for content-addressed paths so concurrent uploads deduplicate safely.
func (w *WebDAV) Put(ctx context.Context, name string, body io.ReadSeeker) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if body == nil {
		return errors.New("storage: nil body")
	}
	if _, err := normalizeObjectPath(name); err != nil {
		return err
	}
	parent := path.Dir(name)
	if err := w.ensureDir(ctx, parent); err != nil {
		return err
	}
	tmpName, err := randomTempName(parent, ".tmp-")
	if err != nil {
		return err
	}

	putResp, err := w.doAuthRequest(ctx, http.MethodPut, tmpName, body, nil)
	if err != nil {
		w.deleteTemp(tmpName)
		return err
	}
	putStatus := putResp.StatusCode
	_ = putResp.Body.Close()
	if putStatus < 200 || putStatus >= 300 {
		w.deleteTemp(tmpName)
		if putStatus == http.StatusUnauthorized || putStatus == http.StatusForbidden {
			return fmt.Errorf("%w: webdav PUT status %d", ErrAuth, putStatus)
		}
		return fmt.Errorf("webdav: PUT %s returned status %d", tmpName, putStatus)
	}
	if err = ctx.Err(); err != nil {
		w.deleteTemp(tmpName)
		return err
	}

	overwrite := !isContentAddressedPath(name)
	mvStatus, mvErr := w.moveObject(ctx, tmpName, name, overwrite)
	if mvErr != nil {
		w.deleteTemp(tmpName)
		return mvErr
	}
	if mvStatus == http.StatusConflict {
		if dirErr := w.ensureDir(ctx, parent); dirErr == nil {
			mvStatus, mvErr = w.moveObject(ctx, tmpName, name, overwrite)
			if mvErr != nil {
				w.deleteTemp(tmpName)
				return mvErr
			}
		}
	}

	switch mvStatus {
	case http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusPreconditionFailed:
		w.deleteTemp(tmpName)
		if !overwrite {
			exists, exErr := w.Exists(ctx, name)
			if exErr == nil && exists {
				return nil
			}
			if exErr != nil {
				return exErr
			}
		}
		return fmt.Errorf("webdav: MOVE %s -> %s returned status 412", tmpName, name)
	case http.StatusUnauthorized, http.StatusForbidden:
		w.deleteTemp(tmpName)
		return fmt.Errorf("%w: webdav MOVE status %d", ErrAuth, mvStatus)
	default:
		w.deleteTemp(tmpName)
		return fmt.Errorf("webdav: MOVE %s -> %s returned status %d", tmpName, name, mvStatus)
	}
}

// Open streams name.
func (w *WebDAV) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r, err := w.client.ReadStream(name)
	return r, mapWebDAVErr(err)
}

// OpenRange reads the inclusive byte range [start, end] from name using a context-bound,
// auto-authenticated HTTP request (supporting both Basic and Digest WebDAV servers).
func (w *WebDAV) OpenRange(ctx context.Context, name string, start, end int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateRange(start, end); err != nil {
		return nil, err
	}
	if _, err := normalizeObjectPath(name); err != nil {
		return nil, err
	}

	resp, err := w.doAuthRequest(ctx, http.MethodGet, name, nil, func(req *http.Request) {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
		req.Header.Set("Accept-Encoding", "identity")
	})
	if err != nil {
		return nil, err
	}

	limit := end - start + 1
	switch resp.StatusCode {
	case http.StatusPartialContent:
		return &limitedReadCloser{
			r: io.LimitReader(resp.Body, limit),
			c: resp.Body,
		}, nil
	case http.StatusOK:
		if start > 0 {
			if skipErr := discardWithContext(ctx, resp.Body, start); skipErr != nil {
				_ = resp.Body.Close()
				return nil, skipErr
			}
		}
		return &limitedReadCloser{
			r: io.LimitReader(resp.Body, limit),
			c: resp.Body,
		}, nil
	case http.StatusNotFound:
		_ = resp.Body.Close()
		return nil, ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: webdav status %d", ErrAuth, resp.StatusCode)
	default:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: webdav status %d", ErrUnreachable, resp.StatusCode)
	}
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

// List returns the full object paths directly inside dir, excluding subdirectories and .tmp-* staging files.
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
		if info.IsDir() || strings.HasPrefix(info.Name(), ".tmp-") {
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
