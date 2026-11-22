// Package client talks to the Aumbry server's ingest API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	ingestPath     = "/api/v1/ingest"
	agentDirsPath  = "/api/v1/agent/dirs"
	agentItemsPath = "/api/v1/agent/items"
	defaultTimeout = 2 * time.Minute
	maxResponse    = 1 << 20
)

// Asset is one local figure file and the normalized src it is referenced by in the XML.
type Asset struct {
	Src  string // normalized, slash-separated path as written in <figure src>
	Path string // location on the local filesystem
}

// PushRequest describes one ingest call.
type PushRequest struct {
	Server      string
	Token       string
	XML         []byte
	Assets      []Asset
	Dirs        []string
	ItemID      string
	ExternalKey string
	Draft       bool
	CreateOnly  bool
	// HTTPClient is optional; the default has a two-minute overall timeout.
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
		return nil, fmt.Errorf("push to %s: %w", endpoint, err)
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
		return nil, fmt.Errorf("%s %s: %w", method, endpoint, err)
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

func endpointURL(server string, elem ...string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(server))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("invalid server URL %q: expected http(s)://host", server)
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
	for _, asset := range req.Assets {
		if err := writeAsset(form, asset); err != nil {
			return err
		}
	}
	return form.Close()
}

func writeAsset(form *multipart.Writer, asset Asset) error {
	file, err := os.Open(asset.Path)
	if err != nil {
		return fmt.Errorf("open figure %s: %w", asset.Src, err)
	}
	defer func() { _ = file.Close() }()
	name := filepath.Base(asset.Path)
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
