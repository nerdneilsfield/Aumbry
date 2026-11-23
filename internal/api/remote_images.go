package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/lestrrat-go/helium"
	"golang.org/x/sync/errgroup"

	"github.com/nerdneilsfield/aumbry/pkg/schema"
)

const (
	maxRemoteImagesPerDoc  = 20
	remoteImageConcurrency = 4
	remoteImagePerTimeout  = 15 * time.Second
	remoteImageTotalBudget = 60 * time.Second
	maxRemoteRedirects     = 5
	remoteImageAcceptHdr   = "image/png,image/jpeg,image/webp,image/svg+xml"
)

var (
	errRemoteBlocked      = errors.New("remote asset blocked by SSRF policy")
	errRemoteTooManyRedir = fmt.Errorf("%w: exceeded redirect limit (%d)", errRemoteBlocked, maxRemoteRedirects)
)

type remoteFetchIssueError struct {
	line int
	code string
	msg  string
	hint string
}

func (e *remoteFetchIssueError) Error() string { return e.code + ": " + e.msg }

func checkRemoteSocketAddr(address string) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: invalid remote address %q: %v", errRemoteBlocked, address, err)
	}
	if !schema.IsPublicAddr(ap.Addr()) {
		return fmt.Errorf("%w: non-public remote IP %s", errRemoteBlocked, ap.Addr().Unmap())
	}
	return nil
}

func newRemoteImageTransport(dialHook func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Transport {
	dialContext := dialHook
	if dialContext == nil {
		baseDialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 15 * time.Second,
			// Connection-level SSRF defense: inspect resolved remote socket IP immediately before connect.
			Control: func(_, address string, _ syscall.RawConn) error {
				return checkRemoteSocketAddr(address)
			},
		}
		dialContext = baseDialer.DialContext
	}
	return &http.Transport{
		// Explicitly disable environment HTTP(S)_PROXY inheritance so Control always sees real target IPs.
		Proxy:                 nil,
		DialContext:           dialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
	}
}

func (s *Server) newRemoteImageHTTPClient() (*http.Client, func()) {
	if s.remoteImageClient != nil {
		c := *s.remoteImageClient
		return &c, func() {}
	}
	tr := newRemoteImageTransport(s.remoteImageDialContext)
	client := &http.Client{
		Transport: tr,
		Timeout:   remoteImagePerTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRemoteRedirects {
				return errRemoteTooManyRedir
			}
			_, msg, _, ok := schema.ValidateRemoteImageURL(req.URL.String())
			if !ok {
				return fmt.Errorf("%w: redirect target %s rejected: %s", errRemoteBlocked, req.URL.Redacted(), msg)
			}
			return nil
		},
	}
	return client, tr.CloseIdleConnections
}

// localizeRemoteImages downloads, SSRF-checks, MIME-verifies, and SVG-parses every unique v1.2 remote
// <figure src> URL into temporary files. On any error all temp files are removed and no DB/store state changes.
func (s *Server) localizeRemoteImages(ctx context.Context, refs []schema.RemoteFigureRef) (assets []storedAsset, cleanup func(), issues []schema.ValidationError) {
	noop := func() {}
	if len(refs) == 0 {
		return nil, noop, nil
	}
	if !s.allowRemoteImages {
		return nil, noop, []schema.ValidationError{{
			Line:    refs[0].Line,
			Code:    "REMOTE_ASSET_BLOCKED",
			Field:   "figure.src",
			Message: fmt.Sprintf("remote figure localization is disabled by server configuration (%s)", refs[0].URL),
			Hint:    "enable ingest.allow_remote_images or upload local image files instead",
		}}
	}
	if len(refs) > maxRemoteImagesPerDoc {
		return nil, noop, []schema.ValidationError{{
			Line:    refs[maxRemoteImagesPerDoc].Line,
			Code:    "REMOTE_ASSET_FETCH_FAILED",
			Field:   "figure.src",
			Message: fmt.Sprintf("document references %d distinct remote figure URLs, exceeding the limit of %d", len(refs), maxRemoteImagesPerDoc),
			Hint:    "reduce the number of remote figures to at most 20 per document",
		}}
	}

	budgetCtx, cancelBudget := context.WithTimeout(ctx, remoteImageTotalBudget)
	defer cancelBudget()

	client, closeTransport := s.newRemoteImageHTTPClient()
	defer closeTransport()

	results := make([]storedAsset, len(refs))
	tempPaths := make([]string, len(refs))
	cleanupAll := func() {
		for _, p := range tempPaths {
			if p != "" {
				_ = os.Remove(p)
			}
		}
	}

	g, gCtx := errgroup.WithContext(budgetCtx)
	g.SetLimit(remoteImageConcurrency)

	for i, ref := range refs {
		g.Go(func() error {
			asset, tmpPath, err := fetchSingleRemoteImage(gCtx, client, ref)
			tempPaths[i] = tmpPath
			if err != nil {
				return err
			}
			results[i] = asset
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		cleanupAll()
		var fi *remoteFetchIssueError
		if errors.As(err, &fi) {
			return nil, noop, []schema.ValidationError{{
				Line:    fi.line,
				Code:    fi.code,
				Field:   "figure.src",
				Message: fi.msg,
				Hint:    fi.hint,
			}}
		}
		code := "REMOTE_ASSET_FETCH_FAILED"
		if errors.Is(err, errRemoteBlocked) {
			code = "REMOTE_ASSET_BLOCKED"
		}
		return nil, noop, []schema.ValidationError{{
			Line:    refs[0].Line,
			Code:    code,
			Field:   "figure.src",
			Message: err.Error(),
		}}
	}

	return results, cleanupAll, nil
}

func fetchSingleRemoteImage(ctx context.Context, client *http.Client, ref schema.RemoteFigureRef) (asset storedAsset, tmpPath string, err error) {
	if code, msg, hint, ok := schema.ValidateRemoteImageURL(ref.URL); !ok {
		return storedAsset{}, "", &remoteFetchIssueError{line: ref.Line, code: code, msg: msg, hint: hint}
	}

	perCtx, cancel := context.WithTimeout(ctx, remoteImagePerTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(perCtx, http.MethodGet, ref.URL, nil)
	if err != nil {
		return storedAsset{}, "", &remoteFetchIssueError{line: ref.Line, code: "REMOTE_ASSET_BLOCKED", msg: fmt.Sprintf("invalid remote url %q: %v", ref.URL, err)}
	}
	req.Header.Set("Accept", remoteImageAcceptHdr)

	resp, err := client.Do(req)
	if err != nil {
		code := "REMOTE_ASSET_FETCH_FAILED"
		if errors.Is(err, errRemoteBlocked) {
			code = "REMOTE_ASSET_BLOCKED"
		}
		return storedAsset{}, "", &remoteFetchIssueError{
			line: ref.Line,
			code: code,
			msg:  fmt.Sprintf("fetch remote figure %q: %v", ref.URL, err),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return storedAsset{}, "", &remoteFetchIssueError{
			line: ref.Line,
			code: "REMOTE_ASSET_FETCH_FAILED",
			msg:  fmt.Sprintf("remote figure %q returned HTTP %d", ref.URL, resp.StatusCode),
		}
	}
	if resp.ContentLength > maxImageSize {
		return storedAsset{}, "", &remoteFetchIssueError{
			line: ref.Line,
			code: "REMOTE_ASSET_FETCH_FAILED",
			msg:  fmt.Sprintf("remote figure %q exceeds the 20MB limit", ref.URL),
		}
	}

	tmpFile, err := os.CreateTemp("", "aumbry-remote-img-*")
	if err != nil {
		return storedAsset{}, "", fmt.Errorf("create temp file for remote image: %w", err)
	}
	tmpPath = tmpFile.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmpFile.Close()
		}
	}()

	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(hasher, tmpFile), io.LimitReader(resp.Body, maxImageSize+1))
	if copyErr != nil {
		code := "REMOTE_ASSET_FETCH_FAILED"
		if errors.Is(copyErr, errRemoteBlocked) {
			code = "REMOTE_ASSET_BLOCKED"
		}
		return storedAsset{}, tmpPath, &remoteFetchIssueError{
			line: ref.Line,
			code: code,
			msg:  fmt.Sprintf("read remote figure %q: %v", ref.URL, copyErr),
		}
	}
	if written == 0 {
		return storedAsset{}, tmpPath, &remoteFetchIssueError{
			line: ref.Line,
			code: "REMOTE_ASSET_FETCH_FAILED",
			msg:  fmt.Sprintf("remote figure %q returned an empty response", ref.URL),
		}
	}
	if written > maxImageSize {
		return storedAsset{}, tmpPath, &remoteFetchIssueError{
			line: ref.Line,
			code: "REMOTE_ASSET_FETCH_FAILED",
			msg:  fmt.Sprintf("remote figure %q exceeds the 20MB limit", ref.URL),
		}
	}
	if _, seekErr := tmpFile.Seek(0, io.SeekStart); seekErr != nil {
		return storedAsset{}, tmpPath, seekErr
	}

	detected, mimeErr := mimetype.DetectReader(tmpFile)
	if mimeErr != nil {
		return storedAsset{}, tmpPath, mimeErr
	}
	detectedStr := detected.String()
	ext, okImg := imageAssetTypes[detectedStr]
	if !okImg {
		return storedAsset{}, tmpPath, &remoteFetchIssueError{
			line: ref.Line,
			code: "REMOTE_ASSET_FETCH_FAILED",
			msg:  fmt.Sprintf("remote figure %q has unsupported MIME type %s; only png, jpeg, webp, and svg are accepted", ref.URL, detectedStr),
		}
	}
	if detectedStr == "image/svg+xml" {
		if _, seekErr := tmpFile.Seek(0, io.SeekStart); seekErr != nil {
			return storedAsset{}, tmpPath, seekErr
		}
		if svgErr := validateRemoteSVG(perCtx, tmpFile); svgErr != nil {
			return storedAsset{}, tmpPath, &remoteFetchIssueError{
				line: ref.Line,
				code: "REMOTE_ASSET_FETCH_FAILED",
				msg:  fmt.Sprintf("remote figure %q is not a valid safe SVG: %v", ref.URL, svgErr),
			}
		}
	}
	if closeErr := tmpFile.Close(); closeErr != nil {
		return storedAsset{}, tmpPath, closeErr
	}
	closed = true

	hashHex := hex.EncodeToString(hasher.Sum(nil))
	savedPath := tmpPath
	return storedAsset{
		src:  ref.URL,
		hash: hashHex,
		mime: detectedStr,
		path: "/assets/" + hashHex + ext,
		size: written,
		open: func() (io.ReadSeekCloser, error) {
			return os.Open(savedPath)
		},
	}, tmpPath, nil
}

func validateRemoteSVG(ctx context.Context, rs io.ReadSeeker) error {
	raw, err := io.ReadAll(io.LimitReader(rs, maxImageSize+1))
	if err != nil {
		return err
	}
	if len(raw) == 0 || len(raw) > maxImageSize {
		return errors.New("svg size out of bounds")
	}
	// Validate well-formed XML via helium (rejects malformed/truncated XML, DOCTYPE/entity expansion, and non-<svg> roots).
	tree, err := helium.NewParser().Parse(ctx, raw)
	if err != nil {
		return err
	}
	if tree.IntSubset() != nil {
		return errors.New("DOCTYPE declarations are not allowed in SVG")
	}
	root := tree.DocumentElement()
	if root == nil || (root.Name() != "svg" && !strings.HasSuffix(root.Name(), ":svg")) {
		return errors.New("root XML element must be <svg>")
	}
	return nil
}
