// Package client talks to the Aumbry server's ingest API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	ingestPath      = "/api/v1/ingest"
	agentDirsPath   = "/api/v1/agent/dirs"
	agentItemsPath  = "/api/v1/agent/items"
	agentStatusPath = "/api/v1/agent/status"
	defaultTimeout  = 5 * time.Minute
	statusTimeout   = 15 * time.Second
	maxResponse     = 1 << 20
)

// Asset is one local media file and the normalized src it is referenced by in the XML.
type Asset struct {
	Src  string // normalized, slash-separated path as written in <figure src> or <audio src>
	Path string // location on the local filesystem
}

// PushRequest describes one ingest call.
type PushRequest struct {
	Server      string
	Token       string
	XML         []byte
	AssetDir    string // optional document root; when set, assets are opened via os.OpenRoot(AssetDir)
	Assets      []Asset
	Dirs        []string
	ItemID      string
	ExternalKey string
	Draft       bool
	CreateOnly  bool
	// HTTPClient is optional; the default has a five-minute overall timeout.
	HTTPClient *http.Client
}

// AgentDir mirrors one entry returned by GET /api/v1/agent/dirs.
type AgentDir struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Path        string  `json:"path"`
	ParentID    *string `json:"parent_id"`
	HasPassword bool    `json:"has_password"`
	Protected   bool    `json:"protected"`
	ItemCount   int     `json:"item_count"`
}

// BuildInfo mirrors the build metadata inside ServerStatus.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// RuntimeStatus mirrors runtime resource telemetry returned by GET /api/v1/agent/status.
type RuntimeStatus struct {
	GoVersion       string  `json:"go_version"`
	GOOS            string  `json:"goos"`
	GOARCH          string  `json:"goarch"`
	NumCPU          int     `json:"num_cpu"`
	Goroutines      int     `json:"goroutines"`
	UptimeSeconds   int64   `json:"uptime_seconds"`
	StartedAt       string  `json:"started_at"`
	CPUSecondsTotal float64 `json:"cpu_seconds_total"`
	CPUPercent      float64 `json:"cpu_percent"`
	HeapAllocBytes  uint64  `json:"heap_alloc_bytes"`
	HeapInuseBytes  uint64  `json:"heap_inuse_bytes"`
	SysBytes        uint64  `json:"sys_bytes"`
	GCCount         uint32  `json:"gc_count"`
	ProcessRSSBytes uint64  `json:"process_rss_bytes,omitempty"`
}

// StorageStatus mirrors storage telemetry returned by GET /api/v1/agent/status.
type StorageStatus struct {
	Backend     string `json:"backend"`
	SQLiteBytes int64  `json:"sqlite_bytes,omitempty"`
}

// ContentCounts mirrors content counts returned by GET /api/v1/agent/status.
type ContentCounts struct {
	Directories          int   `json:"directories"`
	ProtectedDirectories int   `json:"protected_directories"`
	Items                int   `json:"items"`
	PublishedItems       int   `json:"published_items"`
	DraftItems           int   `json:"draft_items"`
	Revisions            int   `json:"revisions"`
	Assets               int   `json:"assets"`
	AssetBytes           int64 `json:"asset_bytes"`
	DirectoryTokens      int   `json:"directory_tokens"`
}

// ActivityStatus mirrors active concurrency counters returned by GET /api/v1/agent/status.
type ActivityStatus struct {
	IngestInFlight       int `json:"ingest_in_flight"`
	IngestLimit          int `json:"ingest_limit"`
	MCPActiveSessions    int `json:"mcp_active_sessions"`
	MCPAnonSessions      int `json:"mcp_anon_sessions"`
	MCPAnonPostsInFlight int `json:"mcp_anon_posts_in_flight"`
}

// ConfigStatus mirrors non-secret server operational limits returned by GET /api/v1/agent/status.
type ConfigStatus struct {
	MaxIngestBodyBytes  int64 `json:"max_ingest_body_bytes"`
	MaxRevisionsPerItem int   `json:"max_revisions_per_item"`
	AllowRemoteImages   bool  `json:"allow_remote_images"`
	PublicURLConfigured bool  `json:"public_url_configured"`
	TrustedProxiesCount int   `json:"trusted_proxies_count"`
}

// ServerStatus mirrors GET /api/v1/agent/status.
type ServerStatus struct {
	Status   string            `json:"status"`
	Checks   map[string]string `json:"checks"`
	Build    BuildInfo         `json:"build"`
	Runtime  RuntimeStatus     `json:"runtime"`
	Storage  StorageStatus     `json:"storage"`
	Counts   *ContentCounts    `json:"counts,omitempty"`
	Activity ActivityStatus    `json:"activity"`
	Config   ConfigStatus      `json:"config"`
}

// ListItemsRequest filters GET /api/v1/agent/items.
type ListItemsRequest struct {
	Server     string
	Token      string
	Key        string
	Query      string
	Dir        string
	Limit      int
	Offset     int
	HTTPClient *http.Client
}

// ValidationRejected is returned when the server answers 422 with its validation result.
type ValidationRejected struct {
	Body []byte
}

func (*ValidationRejected) Error() string { return "server rejected the document as invalid" }

// APIError is a non-2xx response other than a validation rejection.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
}

// Push uploads the document and returns the server's JSON response body.
func Push(ctx context.Context, req PushRequest) ([]byte, error) {
	endpoint, err := ingestURL(req.Server)
	if err != nil {
		return nil, err
	}
	body, contentType := multipartBody(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		_ = body.Close()
		return nil, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("Authorization", "Bearer "+req.Token)
	httpReq.Header.Set("Accept", "application/json")

	httpClient := req.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, formatRequestError("push to", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, fmt.Errorf("read server response: %w", err)
	}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return data, nil
	case resp.StatusCode == http.StatusUnprocessableEntity:
		return nil, &ValidationRejected{Body: data}
	default:
		return nil, newAPIError(resp.StatusCode, data)
	}
}

// FullURL resolves relURL against the configured server base URL.
func FullURL(server, relURL string) string {
	base, err := url.Parse(strings.TrimSpace(server))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return relURL
	}
	ref, err := url.Parse(relURL)
	if err != nil {
		return relURL
	}
	base.Path = strings.TrimSuffix(base.Path, "/") + "/" + strings.TrimPrefix(ref.Path, "/")
	base.RawQuery = ref.RawQuery
	base.Fragment = ref.Fragment
	return base.String()
}

// GetStatus calls GET /api/v1/agent/status and returns the parsed payload plus raw JSON.
func GetStatus(ctx context.Context, server, token string, httpClient *http.Client) (ServerStatus, []byte, error) {
	endpoint, err := endpointURL(server, agentStatusPath)
	if err != nil {
		return ServerStatus{}, nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: statusTimeout}
	}
	raw, err := doJSON(ctx, http.MethodGet, endpoint, token, httpClient)
	if err != nil {
		return ServerStatus{}, nil, err
	}
	var st ServerStatus
	if err := json.Unmarshal(raw, &st); err != nil {
		return ServerStatus{}, nil, fmt.Errorf("decode status response: %w", err)
	}
	return st, raw, nil
}

// ListDirs calls GET /api/v1/agent/dirs and returns the parsed slice plus raw JSON.
func ListDirs(ctx context.Context, server, token string, httpClient *http.Client) ([]AgentDir, []byte, error) {
	endpoint, err := endpointURL(server, agentDirsPath)
	if err != nil {
		return nil, nil, err
	}
	raw, err := doJSON(ctx, http.MethodGet, endpoint, token, httpClient)
	if err != nil {
		return nil, nil, err
	}
	var dirs []AgentDir
	if err := json.Unmarshal(raw, &dirs); err != nil {
		return nil, nil, fmt.Errorf("decode dirs response: %w", err)
	}
	return dirs, raw, nil
}

// ListItems calls GET /api/v1/agent/items and returns the raw JSON body.
func ListItems(ctx context.Context, req ListItemsRequest) ([]byte, error) {
	endpoint, err := endpointURL(req.Server, agentItemsPath)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(endpoint)
	q := u.Query()
	if req.Key != "" {
		q.Set("key", req.Key)
	}
	if req.Query != "" {
		q.Set("q", req.Query)
	}
	if req.Dir != "" {
		q.Set("dir", req.Dir)
	}
	if req.Limit > 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}
	if req.Offset > 0 {
		q.Set("offset", strconv.Itoa(req.Offset))
	}
	u.RawQuery = q.Encode()
	return doJSON(ctx, http.MethodGet, u.String(), req.Token, req.HTTPClient)
}

// Unpublish calls DELETE /api/v1/agent/items/{id}/mounts.
func Unpublish(ctx context.Context, server, token, id string, httpClient *http.Client) error {
	endpoint, err := endpointURL(server, agentItemsPath, id, "mounts")
	if err != nil {
		return err
	}
	_, err = doJSON(ctx, http.MethodDelete, endpoint, token, httpClient)
	return err
}

// DeleteItem calls DELETE /api/v1/agent/items/{id}.
func DeleteItem(ctx context.Context, server, token, id string, httpClient *http.Client) error {
	endpoint, err := endpointURL(server, agentItemsPath, id)
	if err != nil {
		return err
	}
	_, err = doJSON(ctx, http.MethodDelete, endpoint, token, httpClient)
	return err
}

func doJSON(ctx context.Context, method, endpoint, token string, httpClient *http.Client) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Accept", "application/json")
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, formatRequestError(method, endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, fmt.Errorf("read server response: %w", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return data, nil
	}
	return nil, newAPIError(resp.StatusCode, data)
}

// Indent pretty-prints a JSON document, returning it unchanged if it is not valid JSON.
func Indent(data []byte) []byte {
	var out bytes.Buffer
	if err := json.Indent(&out, bytes.TrimSpace(data), "", "  "); err != nil {
		return data
	}
	out.WriteByte('\n')
	return out.Bytes()
}

func ingestURL(server string) (string, error) {
	return endpointURL(server, ingestPath)
}

var (
	errURLUserInfoRe = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://|//)[^/@\s]+@`)
	errURLQueryRe    = regexp.MustCompile(`\?[^\s"'<>]+`)
)

// SanitizeURL strips credentials, query parameters, and fragments from a URL before display or logging.
func SanitizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Host != "" {
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return strings.TrimRight(parsed.String(), "/")
	}
	if before, _, ok := strings.Cut(trimmed, "#"); ok {
		trimmed = before
	}
	if before, _, ok := strings.Cut(trimmed, "?"); ok {
		trimmed = before
	}
	trimmed = errURLUserInfoRe.ReplaceAllString(trimmed, "${1}")
	return strings.TrimRight(trimmed, "/")
}

func sanitizeErrorMessage(msg string) string {
	redacted := errURLUserInfoRe.ReplaceAllString(msg, "${1}REDACTED@")
	return errURLQueryRe.ReplaceAllString(redacted, "?REDACTED")
}

func formatRequestError(prefix, endpoint string, err error) error {
	inner := err
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		inner = urlErr.Err
	}
	if errors.Is(inner, context.DeadlineExceeded) || errors.Is(inner, context.Canceled) {
		return fmt.Errorf("%s %s: %w", prefix, SanitizeURL(endpoint), inner)
	}
	return fmt.Errorf("%s %s: %s", prefix, SanitizeURL(endpoint), sanitizeErrorMessage(inner.Error()))
}

func endpointURL(server string, elem ...string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(server))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("invalid server URL: expected http(s)://host")
	}
	return parsed.JoinPath(elem...).String(), nil
}

func newAPIError(status int, data []byte) *APIError {
	apiErr := &APIError{Status: status}
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &payload) == nil {
		apiErr.Code, apiErr.Message = payload.Error, payload.Message
	}
	if apiErr.Message == "" {
		apiErr.Message = apiErr.Code
	}
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(status)
	}
	return apiErr
}

// multipartBody streams the request body through a pipe so figure files are never fully buffered.
// Text fields come first so the server can read them before the file parts.
func multipartBody(req PushRequest) (io.ReadCloser, string) {
	reader, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	go func() { writer.CloseWithError(writeForm(form, req)) }()
	return reader, form.FormDataContentType()
}

func writeForm(form *multipart.Writer, req PushRequest) error {
	for _, dir := range req.Dirs {
		if err := form.WriteField("dirs", dir); err != nil {
			return err
		}
	}
	for _, field := range []struct{ name, value string }{{"item_id", req.ItemID}, {"external_key", req.ExternalKey}} {
		if field.value == "" {
			continue
		}
		if err := form.WriteField(field.name, field.value); err != nil {
			return err
		}
	}
	if req.Draft {
		if err := form.WriteField("draft", "1"); err != nil {
			return err
		}
	}
	if req.CreateOnly {
		if err := form.WriteField("create_only", "1"); err != nil {
			return err
		}
	}
	for _, asset := range req.Assets {
		if err := form.WriteField("asset_srcs", asset.Src); err != nil {
			return err
		}
	}
	if err := writeFile(form, "xml", "document.xml", "application/xml", bytes.NewReader(req.XML)); err != nil {
		return err
	}
	var assetRoot *os.Root
	if req.AssetDir != "" && len(req.Assets) > 0 {
		var err error
		assetRoot, err = os.OpenRoot(req.AssetDir)
		if err != nil {
			return fmt.Errorf("open asset directory %s: %w", req.AssetDir, err)
		}
		defer func() { _ = assetRoot.Close() }()
	}
	for _, asset := range req.Assets {
		if err := writeAsset(form, assetRoot, asset); err != nil {
			return err
		}
	}
	return form.Close()
}

func writeAsset(form *multipart.Writer, root *os.Root, asset Asset) error {
	var file *os.File
	var err error
	if root != nil {
		file, err = root.Open(filepath.FromSlash(asset.Src))
		if err != nil {
			return fmt.Errorf("open asset %s: %w", asset.Src, err)
		}
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return fmt.Errorf("stat asset %s: %w", asset.Src, statErr)
		}
		if !info.Mode().IsRegular() {
			_ = file.Close()
			return fmt.Errorf("open asset %s: not a regular file", asset.Src)
		}
	} else {
		file, err = os.Open(asset.Path)
		if err != nil {
			return fmt.Errorf("open asset %s: %w", asset.Src, err)
		}
	}
	defer func() { _ = file.Close() }()
	name := path.Base(asset.Src)
	if name == "" || name == "." || name == "/" {
		name = filepath.Base(asset.Path)
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return writeFile(form, "assets[]", name, contentType, file)
}

func writeFile(form *multipart.Writer, field, filename, contentType string, src io.Reader) error {
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": filename}))
	header.Set("Content-Type", contentType)
	part, err := form.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, src)
	return err
}
