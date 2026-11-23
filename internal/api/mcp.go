package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/jmoiron/sqlx"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nerdneilsfield/aumbry/internal/auth"
	"github.com/nerdneilsfield/aumbry/internal/storage"
	"github.com/nerdneilsfield/aumbry/pkg/schema"
)

const (
	mcpMaxRequestBodyBytes  = 16 << 20 // 16 MiB transport limit so 10 MiB escaped XML succeeds
	mcpSessionTimeout       = 30 * time.Minute
	mcpMaxAnonSessionsPerIP = 16
	mcpAnonRateLimitPerMin  = 120
	mcpDefaultListLimit     = 20
	mcpMaxListLimit         = 100
	mcpHeaderExtraDirTokens = "X-Aumbry-Dir-Tokens"
	mcpSessionHeader        = "Mcp-Session-Id"
)

type mcpAuthKind int

const (
	mcpAuthAnon mcpAuthKind = iota
	mcpAuthDirToken
	mcpAuthAPIToken
)

type mcpPrincipal struct {
	kind        mcpAuthKind
	apiDigest   string
	visibleDirs map[string]bool
}

func (p mcpPrincipal) filterVisible(dirIDs []string) []string {
	out := make([]string, 0, len(dirIDs))
	for _, id := range dirIDs {
		if p.visibleDirs[id] {
			out = append(out, id)
		}
	}
	return out
}

func (p mcpPrincipal) visibleSlice() []string {
	out := make([]string, 0, len(p.visibleDirs))
	for id := range p.visibleDirs {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

type mcpSessionRecord struct {
	ownerKey string
	anonIP   string
	lastSeen time.Time
}

type mcpSessionTracker struct {
	mu       sync.Mutex
	sessions map[string]mcpSessionRecord
}

func newMCPSessionTracker() *mcpSessionTracker {
	return &mcpSessionTracker{sessions: make(map[string]mcpSessionRecord)}
}

func (tr *mcpSessionTracker) pruneLocked(now time.Time) {
	cutoff := now.Add(-mcpSessionTimeout)
	for sid, rec := range tr.sessions {
		if rec.lastSeen.Before(cutoff) {
			delete(tr.sessions, sid)
		}
	}
}

func (tr *mcpSessionTracker) checkExisting(sid, ownerKey string) (known bool, allowed bool) {
	now := time.Now()
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.pruneLocked(now)
	rec, ok := tr.sessions[sid]
	if !ok {
		return false, true
	}
	if rec.ownerKey != ownerKey {
		return true, false
	}
	rec.lastSeen = now
	tr.sessions[sid] = rec
	return true, true
}

func (tr *mcpSessionTracker) countAnonForIP(ip string) int {
	now := time.Now()
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.pruneLocked(now)
	n := 0
	for _, rec := range tr.sessions {
		if rec.anonIP == ip {
			n++
		}
	}
	return n
}

func (tr *mcpSessionTracker) bind(sid, ownerKey, anonIP string) {
	if sid == "" {
		return
	}
	now := time.Now()
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.pruneLocked(now)
	tr.sessions[sid] = mcpSessionRecord{ownerKey: ownerKey, anonIP: anonIP, lastSeen: now}
}

func (tr *mcpSessionTracker) remove(sid string) {
	if sid == "" {
		return
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	delete(tr.sessions, sid)
}

type mcpStatusCaptureWriter struct {
	http.ResponseWriter
	status int
}

func (w *mcpStatusCaptureWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *mcpStatusCaptureWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (w *mcpStatusCaptureWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) newMCPHTTPHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "aumbry",
		Version: "0.2.0",
	}, nil)

	s.registerMCPTools(server)

	sdkHandler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		JSONResponse:               true,
		SessionTimeout:             mcpSessionTimeout,
		DisableLocalhostProtection: true, // enforced explicitly below across loopback and reverse-proxy setups
		MaxRequestBodyBytes:        mcpMaxRequestBodyBytes,
		Logger:                     s.logger,
	})

	bearerMiddleware := mcpauth.RequireBearerToken(s.verifyMCPBearerToken, &mcpauth.RequireBearerTokenOptions{
		AllowMissingExpiration: true,
	})
	anonRate := httprate.NewRateLimiter(mcpAnonRateLimitPerMin, time.Minute,
		httprate.WithKeyFuncs(clientIPKey),
		httprate.WithLimitHandler(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "anonymous MCP rate limit exceeded", http.StatusTooManyRequests)
		}),
	)
	tracker := newMCPSessionTracker()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.allowMCPHostAndOrigin(r) {
			http.Error(w, "forbidden host or origin", http.StatusForbidden)
			return
		}

		clientIP := httprate.CanonicalizeIP(middleware.GetClientIP(r.Context()))
		cloned := r.Clone(r.Context())
		cloned.Header.Set("X-Aumbry-Internal-Client-IP", clientIP)

		sid := strings.TrimSpace(r.Header.Get(mcpSessionHeader))
		authHdr := strings.TrimSpace(r.Header.Get("Authorization"))

		if authHdr != "" {
			wrapped := bearerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, authedReq *http.Request) {
				ti := mcpauth.TokenInfoFromContext(authedReq.Context())
				ownerKey := ""
				if ti != nil {
					ownerKey = ti.UserID
				}
				if sid != "" {
					if _, ok := tracker.checkExisting(sid, ownerKey); !ok {
						http.Error(w, "session owner mismatch", http.StatusForbidden)
						return
					}
				}
				s.serveTrackedMCP(sdkHandler, tracker, ownerKey, "", sid, w, authedReq)
			}))
			wrapped.ServeHTTP(w, cloned)
			return
		}

		if anonRate.RespondOnLimit(w, cloned, clientIP) {
			return
		}
		ownerKey := "anon:" + clientIP
		if sid != "" {
			if _, ok := tracker.checkExisting(sid, ownerKey); !ok {
				http.Error(w, "session owner mismatch", http.StatusForbidden)
				return
			}
		} else if r.Method == http.MethodPost && tracker.countAnonForIP(clientIP) >= mcpMaxAnonSessionsPerIP {
			http.Error(w, "too many active anonymous MCP sessions", http.StatusTooManyRequests)
			return
		}
		s.serveTrackedMCP(sdkHandler, tracker, ownerKey, clientIP, sid, w, cloned)
	})
}

func (s *Server) serveTrackedMCP(sdkHandler http.Handler, tracker *mcpSessionTracker, ownerKey, anonIP, reqSID string, w http.ResponseWriter, r *http.Request) {
	cw := &mcpStatusCaptureWriter{ResponseWriter: w}
	sdkHandler.ServeHTTP(cw, r)
	status := cw.status
	if status == 0 {
		status = http.StatusOK
	}
	if r.Method == http.MethodDelete && reqSID != "" && status < http.StatusBadRequest {
		tracker.remove(reqSID)
		return
	}
	if newSID := strings.TrimSpace(w.Header().Get(mcpSessionHeader)); newSID != "" && status < http.StatusBadRequest {
		tracker.bind(newSID, ownerKey, anonIP)
	}
}

func (s *Server) verifyMCPBearerToken(ctx context.Context, token string, _ *http.Request) (*mcpauth.TokenInfo, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return nil, mcpauth.ErrInvalidToken
	}
	if strings.HasPrefix(trimmed, dirTokenPrefix) {
		tokHash := auth.TokenDigest(trimmed)
		var dirID string
		err := s.db.GetContext(ctx, &dirID, `SELECT directory_id FROM directory_tokens WHERE token_hash = ?`, tokHash)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, mcpauth.ErrInvalidToken
		}
		if err != nil {
			return nil, err
		}
		return &mcpauth.TokenInfo{
			UserID: "dir:" + tokHash,
		}, nil
	}
	digest, ok := s.auth.AuthenticateToken("Bearer " + trimmed)
	if !ok {
		return nil, mcpauth.ErrInvalidToken
	}
	return &mcpauth.TokenInfo{
		UserID: "api:" + digest,
	}, nil
}

func (s *Server) allowMCPHostAndOrigin(r *http.Request) bool {
	if r.Host == "" {
		return false
	}
	if s.publicURL != "" {
		pubURL, err := url.Parse(s.publicURL)
		if err != nil || pubURL.Host == "" {
			return false
		}
		if !sameHostPort(r.Host, pubURL.Host, pubURL.Scheme) {
			return false
		}
	} else {
		// Enforce DNS rebinding check whenever the request terminates on a loopback interface or came directly from loopback.
		if localAddr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && localAddr != nil {
			if isLoopbackAddrStr(localAddr.String()) && !isLoopbackHost(r.Host) {
				return false
			}
		} else if isLoopbackAddrStr(r.RemoteAddr) && !isLoopbackHost(r.Host) {
			return false
		}
	}

	rawOrigin := strings.TrimSpace(r.Header.Get("Origin"))
	if rawOrigin == "" {
		return true
	}
	if strings.EqualFold(rawOrigin, "null") {
		return false
	}
	origURL, err := url.Parse(rawOrigin)
	if err != nil || origURL.Host == "" {
		return false
	}
	origScheme := strings.ToLower(origURL.Scheme)
	if origScheme != "http" && origScheme != "https" {
		return false
	}

	expectedBase := s.resolvePublicURL(r)
	expURL, err := url.Parse(expectedBase)
	if err != nil || expURL.Host == "" {
		return false
	}
	return strings.EqualFold(origScheme, expURL.Scheme) && sameHostPort(origURL.Host, expURL.Host, expURL.Scheme)
}

func sameHostPort(a, b, scheme string) bool {
	return strings.EqualFold(normalizeHostPort(a, scheme), normalizeHostPort(b, scheme))
}

func normalizeHostPort(hostPort, scheme string) string {
	h, p, err := net.SplitHostPort(hostPort)
	if err != nil {
		h = strings.Trim(hostPort, "[]")
		p = ""
	}
	if p == "" {
		if strings.EqualFold(scheme, "https") {
			p = "443"
		} else {
			p = "80"
		}
	}
	return strings.ToLower(h) + ":" + p
}

func isLoopbackAddrStr(raw string) bool {
	host, _, err := net.SplitHostPort(raw)
	if err != nil {
		host = raw
	}
	host = strings.Trim(host, "[]")
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback()
}

func isLoopbackHost(hostHeader string) bool {
	host := hostHeader
	if h, _, err := net.SplitHostPort(hostHeader); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(strings.TrimSuffix(host, ".")), "[]")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback()
}

func mcpToolError(code, msg string) error {
	return fmt.Errorf("%s: %s", code, msg)
}

func resolveMCPPrincipalTx(ctx context.Context, q sqlx.QueryerContext, s *Server, t *dirTree, hdr http.Header) (mcpPrincipal, error) {
	visible := make(map[string]bool)
	for id := range t.dirs {
		if t.isPublic(id) {
			visible[id] = true
		}
	}

	authHdr := strings.TrimSpace(hdr.Get("Authorization"))
	if authHdr == "" {
		return mcpPrincipal{kind: mcpAuthAnon, visibleDirs: visible}, nil
	}
	scheme, rawTok, ok := strings.Cut(authHdr, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return mcpPrincipal{}, mcpToolError("unauthorized", "invalid Authorization header")
	}
	rawTok = strings.TrimSpace(rawTok)
	if rawTok == "" {
		return mcpPrincipal{}, mcpToolError("unauthorized", "empty bearer token")
	}

	if strings.HasPrefix(rawTok, dirTokenPrefix) {
		var dirID string
		err := sqlx.GetContext(ctx, q, &dirID, `SELECT directory_id FROM directory_tokens WHERE token_hash = ?`, auth.TokenDigest(rawTok))
		if errors.Is(err, sql.ErrNoRows) {
			return mcpPrincipal{}, mcpToolError("unauthorized", "directory read token is invalid or revoked")
		}
		if err != nil {
			return mcpPrincipal{}, err
		}
		dirVisible := make(map[string]bool)
		for _, id := range t.subtreeVisibleByDirToken(dirID) {
			dirVisible[id] = true
		}
		return mcpPrincipal{kind: mcpAuthDirToken, visibleDirs: dirVisible}, nil
	}

	digest, ok := s.auth.AuthenticateToken("Bearer " + rawTok)
	if !ok {
		return mcpPrincipal{}, mcpToolError("unauthorized", "invalid API token")
	}
	if extra := strings.TrimSpace(hdr.Get(mcpHeaderExtraDirTokens)); extra != "" {
		for _, part := range strings.Split(extra, ",") {
			tok := strings.TrimSpace(part)
			if !strings.HasPrefix(tok, dirTokenPrefix) || len(tok) <= len(dirTokenPrefix) {
				continue
			}
			var dirID string
			err := sqlx.GetContext(ctx, q, &dirID, `SELECT directory_id FROM directory_tokens WHERE token_hash = ?`, auth.TokenDigest(tok))
			if err != nil {
				continue
			}
			for _, id := range t.subtreeVisibleByDirToken(dirID) {
				visible[id] = true
			}
		}
	}
	return mcpPrincipal{kind: mcpAuthAPIToken, apiDigest: digest, visibleDirs: visible}, nil
}

type mcpListDirsInput struct {
	ParentID string `json:"parent_id,omitempty" jsonschema:"Optional parent directory ID to list direct children"`
}

type mcpDirEntry struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Protected   bool    `json:"protected"`
	Locked      bool    `json:"locked"`
	ParentID    *string `json:"parent_id,omitempty"`
}

type mcpListDirsOutput struct {
	Directories []mcpDirEntry `json:"directories"`
}

type mcpListItemsInput struct {
	DirID  string `json:"dir_id,omitempty" jsonschema:"Filter by directory ID (intersects caller visibility)"`
	Key    string `json:"key,omitempty" jsonschema:"Filter by exact external_key"`
	Type   string `json:"type,omitempty" jsonschema:"Filter by type: report or post"`
	Tag    string `json:"tag,omitempty" jsonschema:"Filter by tag"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size (1..100, default 20)"`
	Offset int    `json:"offset,omitempty" jsonschema:"Pagination offset (>= 0)"`
}

type mcpItemEntry struct {
	ID            string   `json:"id"`
	Type          string   `json:"type"`
	Title         string   `json:"title"`
	Date          string   `json:"date"`
	Summary       string   `json:"summary"`
	Tags          []string `json:"tags"`
	SourceAgent   string   `json:"source_agent"`
	Dirs          []string `json:"dirs"`
	URL           string   `json:"url"`
	RevisionCount int      `json:"revision_count"`
}

type mcpListItemsOutput struct {
	Items  []mcpItemEntry `json:"items"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

type mcpSearchItemsInput struct {
	Query  string `json:"query" jsonschema:"Search query text (required)"`
	DirID  string `json:"dir_id,omitempty" jsonschema:"Optional directory ID filter"`
	Type   string `json:"type,omitempty" jsonschema:"Optional type filter: report or post"`
	Tag    string `json:"tag,omitempty" jsonschema:"Optional tag filter"`
	From   string `json:"from,omitempty" jsonschema:"Optional start date YYYY-MM-DD"`
	To     string `json:"to,omitempty" jsonschema:"Optional end date YYYY-MM-DD"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size (1..100, default 20)"`
	Offset int    `json:"offset,omitempty" jsonschema:"Pagination offset (>= 0)"`
}

type mcpSearchItemEntry struct {
	mcpItemEntry
	Snippet string `json:"snippet"`
}

type mcpSearchItemsOutput struct {
	Results []mcpSearchItemEntry `json:"results"`
	Limit   int                  `json:"limit"`
	Offset  int                  `json:"offset"`
}

type mcpGetItemInput struct {
	ID     string `json:"id" jsonschema:"12-character item ID (required)"`
	Rev    *int   `json:"rev,omitempty" jsonschema:"Optional positive historical revision number"`
	Format string `json:"format,omitempty" jsonschema:"Output format: xml (default) or text"`
}

type mcpGetItemOutput struct {
	ID      string   `json:"id"`
	Rev     *int     `json:"rev,omitempty"`
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Date    string   `json:"date"`
	Format  string   `json:"format"`
	Content string   `json:"content"`
	Dirs    []string `json:"dirs"`
}

type mcpValidateXMLInput struct {
	XML string `json:"xml" jsonschema:"Aumbry XML document source to validate (<= 10 MiB)"`
}

type mcpPushItemInput struct {
	XML         string   `json:"xml" jsonschema:"Aumbry XML document source (<= 10 MiB; local asset paths are not accepted over MCP)"`
	Dirs        []string `json:"dirs,omitempty" jsonschema:"Directory IDs to mount the published item into"`
	ItemID      string   `json:"item_id,omitempty" jsonschema:"Existing 12-character item ID to update"`
	ExternalKey string   `json:"external_key,omitempty" jsonschema:"Idempotency key (upsert)"`
	Draft       bool     `json:"draft,omitempty" jsonschema:"Save as unmounted draft (cannot combine with dirs)"`
	CreateOnly  bool     `json:"create_only,omitempty" jsonschema:"Fail with conflict if external_key already exists"`
}

func (s *Server) registerMCPTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_dirs",
		Description: "List visible directories (with optional parent_id). Protected child directories omit descriptions until authorized.",
	}, s.mcpListDirs)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_items",
		Description: "List accessible items with metadata only (respecting caller permissions and optional dir_id intersection).",
	}, s.mcpListItems)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_items",
		Description: "Full-text search accessible items with metadata and snippets (never returns full XML or plain text).",
	}, s.mcpSearchItems)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_item",
		Description: "Fetch an accessible item (or historical revision) as XML or indexed plain text.",
	}, s.mcpGetItem)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "validate_xml",
		Description: "Validate Aumbry XML against the v1.1/v1.2 schema offline without side effects.",
	}, s.mcpValidateXML)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "push_item",
		Description: "Validate and ingest a text/remote-figure XML document (requires API Bearer token; rejects local relative binary assets).",
	}, s.mcpPushItem)
}

func (s *Server) mcpListDirs(ctx context.Context, req *mcp.CallToolRequest, in mcpListDirsInput) (*mcp.CallToolResult, mcpListDirsOutput, error) {
	var zero mcpListDirsOutput
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, zero, err
	}
	defer func() { _ = tx.Rollback() }()

	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		return nil, zero, err
	}
	principal, err := resolveMCPPrincipalTx(ctx, tx, s, t, requestHeaders(req))
	if err != nil {
		return nil, zero, err
	}

	out := make([]mcpDirEntry, 0)
	appendEntry := func(d *dirRec, visible bool) {
		var parentPtr *string
		if d.ParentID != "" && principal.visibleDirs[d.ParentID] {
			p := d.ParentID
			parentPtr = &p
		}
		desc := d.Description
		if !visible {
			desc = ""
		}
		out = append(out, mcpDirEntry{
			ID:          d.ID,
			Name:        d.Name,
			Description: desc,
			Protected:   !t.isPublic(d.ID),
			Locked:      !visible,
			ParentID:    parentPtr,
		})
	}

	parentID := strings.TrimSpace(in.ParentID)
	if parentID != "" {
		d := t.dirs[parentID]
		if d == nil {
			return nil, zero, mcpToolError("not_found", "directory not found")
		}
		if !principal.visibleDirs[d.ID] {
			return nil, zero, mcpToolError("forbidden", "directory is not accessible")
		}
		for _, kidID := range t.kids[parentID] {
			if kid := t.dirs[kidID]; kid != nil {
				appendEntry(kid, principal.visibleDirs[kid.ID])
			}
		}
		return nil, mcpListDirsOutput{Directories: out}, nil
	}

	// Root listing: include every visible directory plus direct locked children under visible parents (with cleared descriptions).
	seen := make(map[string]bool)
	var walk func(parent string)
	walk = func(parent string) {
		for _, id := range t.kids[parent] {
			d := t.dirs[id]
			if d == nil || seen[id] {
				continue
			}
			vis := principal.visibleDirs[id]
			if vis {
				seen[id] = true
				appendEntry(d, true)
			} else if parent != "" && principal.visibleDirs[parent] {
				seen[id] = true
				appendEntry(d, false)
			}
			walk(id)
		}
	}
	walk("")
	return nil, mcpListDirsOutput{Directories: out}, nil
}

func (s *Server) mcpListItems(ctx context.Context, req *mcp.CallToolRequest, in mcpListItemsInput) (*mcp.CallToolResult, mcpListItemsOutput, error) {
	var zero mcpListItemsOutput
	typ := strings.TrimSpace(in.Type)
	if typ != "" && typ != "report" && typ != "post" {
		return nil, zero, mcpToolError("invalid_request", "type must be report or post")
	}
	if in.Offset < 0 {
		return nil, zero, mcpToolError("invalid_request", "offset must be >= 0")
	}
	limit := in.Limit
	if limit == 0 {
		limit = mcpDefaultListLimit
	} else if limit < 0 {
		return nil, zero, mcpToolError("invalid_request", "limit must be >= 1")
	}
	limit = min(limit, mcpMaxListLimit)

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, zero, err
	}
	defer func() { _ = tx.Rollback() }()

	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		return nil, zero, err
	}
	principal, err := resolveMCPPrincipalTx(ctx, tx, s, t, requestHeaders(req))
	if err != nil {
		return nil, zero, err
	}

	dirID := strings.TrimSpace(in.DirID)
	var subtreeIDs []string
	if dirID != "" {
		if t.dirs[dirID] == nil {
			return nil, zero, mcpToolError("not_found", "directory not found")
		}
		if !principal.visibleDirs[dirID] {
			return nil, zero, mcpToolError("forbidden", "directory is not accessible")
		}
		subtreeIDs = principal.filterVisible(t.subtree(dirID))
	}

	visSlice := principal.visibleSlice()
	if len(visSlice) == 0 && principal.kind != mcpAuthAPIToken {
		return nil, mcpListItemsOutput{Items: []mcpItemEntry{}, Limit: limit, Offset: in.Offset}, nil
	}

	var where []string
	var args []any
	switch {
	case principal.kind == mcpAuthAPIToken && len(visSlice) > 0:
		where = append(where, `(i.id IN (SELECT item_id FROM directory_items WHERE directory_id IN (?)) OR i.created_by = ?)`)
		args = append(args, visSlice, principal.apiDigest)
	case principal.kind == mcpAuthAPIToken:
		where = append(where, `i.created_by = ?`)
		args = append(args, principal.apiDigest)
	default:
		where = append(where, `i.id IN (SELECT item_id FROM directory_items WHERE directory_id IN (?))`)
		args = append(args, visSlice)
	}
	if len(subtreeIDs) > 0 {
		where = append(where, `i.id IN (SELECT item_id FROM directory_items WHERE directory_id IN (?))`)
		args = append(args, subtreeIDs)
	}
	if key := strings.TrimSpace(in.Key); key != "" {
		where = append(where, `i.external_key = ?`)
		args = append(args, key)
	}
	if typ != "" {
		where = append(where, `i.type = ?`)
		args = append(args, typ)
	}
	if tag := strings.TrimSpace(in.Tag); tag != "" {
		where = append(where, `EXISTS (SELECT 1 FROM json_each(i.tags_json) WHERE json_each.value = ?)`)
		args = append(args, tag)
	}

	querySQL := `SELECT ` + itemColumns + `, COALESCE(i.created_by, '') AS created_by FROM items i WHERE ` +
		strings.Join(where, ` AND `) +
		` ORDER BY i.report_date DESC, i.created_at DESC, i.id LIMIT ? OFFSET ?`
	args = append(args, limit, in.Offset)
	flatSQL, flatArgs, err := sqlx.In(querySQL, args...)
	if err != nil {
		return nil, zero, err
	}

	var rows []struct {
		itemRow
		CreatedBy string `db:"created_by"`
	}
	if err = sqlx.SelectContext(ctx, tx, &rows, flatSQL, flatArgs...); err != nil {
		return nil, zero, err
	}

	itemIDs := make([]string, len(rows))
	for i, r := range rows {
		itemIDs[i] = r.ID
	}
	mountsByItem, err := mountsForItemsTx(ctx, tx, itemIDs)
	if err != nil {
		return nil, zero, err
	}

	entries := make([]mcpItemEntry, len(rows))
	for i, row := range rows {
		revCount, revErr := mcpRevisionCountTx(ctx, tx, t, principal, row.ID, row.CreatedBy)
		if revErr != nil {
			return nil, zero, revErr
		}
		entries[i] = toMCPItemEntry(row.summary(), principal.filterVisible(mountsByItem[row.ID]), revCount)
	}
	return nil, mcpListItemsOutput{Items: entries, Limit: limit, Offset: in.Offset}, nil
}

func (s *Server) mcpSearchItems(ctx context.Context, req *mcp.CallToolRequest, in mcpSearchItemsInput) (*mcp.CallToolResult, mcpSearchItemsOutput, error) {
	var zero mcpSearchItemsOutput
	if strings.TrimSpace(in.Query) == "" {
		return nil, zero, mcpToolError("invalid_request", "query is required")
	}
	typ := strings.TrimSpace(in.Type)
	if typ != "" && typ != "report" && typ != "post" {
		return nil, zero, mcpToolError("invalid_request", "type must be report or post")
	}
	for _, date := range []string{in.From, in.To} {
		if date != "" {
			if _, err := time.Parse(time.DateOnly, date); err != nil {
				return nil, zero, mcpToolError("invalid_request", "from and to must be YYYY-MM-DD")
			}
		}
	}
	if in.Offset < 0 {
		return nil, zero, mcpToolError("invalid_request", "offset must be >= 0")
	}
	limit := in.Limit
	if limit == 0 {
		limit = mcpDefaultListLimit
	} else if limit < 0 {
		return nil, zero, mcpToolError("invalid_request", "limit must be >= 1")
	}
	limit = min(limit, mcpMaxListLimit)

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, zero, err
	}
	defer func() { _ = tx.Rollback() }()

	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		return nil, zero, err
	}
	principal, err := resolveMCPPrincipalTx(ctx, tx, s, t, requestHeaders(req))
	if err != nil {
		return nil, zero, err
	}

	dirID := strings.TrimSpace(in.DirID)
	var subtreeIDs []string
	if dirID != "" {
		if t.dirs[dirID] == nil {
			return nil, zero, mcpToolError("not_found", "directory not found")
		}
		if !principal.visibleDirs[dirID] {
			return nil, zero, mcpToolError("forbidden", "directory is not accessible")
		}
		subtreeIDs = principal.filterVisible(t.subtree(dirID))
	}

	visSlice := principal.visibleSlice()
	if len(visSlice) == 0 && principal.kind != mcpAuthAPIToken {
		return nil, mcpSearchItemsOutput{Results: []mcpSearchItemEntry{}, Limit: limit, Offset: in.Offset}, nil
	}

	q := searchQuery{
		text:          in.Query,
		dirIDs:        visSlice,
		subtreeDirIDs: subtreeIDs,
		typ:           typ,
		tag:           strings.TrimSpace(in.Tag),
		from:          strings.TrimSpace(in.From),
		to:            strings.TrimSpace(in.To),
		limit:         limit,
		offset:        in.Offset,
	}
	if principal.kind == mcpAuthAPIToken {
		q.ownerDigest = principal.apiDigest
	}

	results, err := s.searchFrom(ctx, tx, q)
	if err != nil {
		return nil, zero, err
	}
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	mountsByItem, err := mountsForItemsTx(ctx, tx, ids)
	if err != nil {
		return nil, zero, err
	}
	ownersByItem, err := ownersForItemsTx(ctx, tx, ids)
	if err != nil {
		return nil, zero, err
	}

	out := make([]mcpSearchItemEntry, len(results))
	for i, res := range results {
		revCount, revErr := mcpRevisionCountTx(ctx, tx, t, principal, res.ID, ownersByItem[res.ID])
		if revErr != nil {
			return nil, zero, revErr
		}
		out[i] = mcpSearchItemEntry{
			mcpItemEntry: toMCPItemEntry(res.itemSummary, principal.filterVisible(mountsByItem[res.ID]), revCount),
			Snippet:      res.Snippet,
		}
	}
	return nil, mcpSearchItemsOutput{Results: out, Limit: limit, Offset: in.Offset}, nil
}

func (s *Server) mcpGetItem(ctx context.Context, req *mcp.CallToolRequest, in mcpGetItemInput) (*mcp.CallToolResult, mcpGetItemOutput, error) {
	var zero mcpGetItemOutput
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return nil, zero, mcpToolError("invalid_request", "id is required")
	}
	format := strings.TrimSpace(in.Format)
	if format == "" {
		format = "xml"
	}
	if format != "xml" && format != "text" {
		return nil, zero, mcpToolError("invalid_request", "format must be xml or text")
	}
	if in.Rev != nil && *in.Rev < 1 {
		return nil, zero, mcpToolError("invalid_request", "rev must be a positive integer")
	}

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, zero, err
	}
	defer func() { _ = tx.Rollback() }()

	var itemRow struct {
		Title     string `db:"title"`
		Summary   string `db:"summary"`
		Date      string `db:"report_date"`
		XMLPath   string `db:"xml_storage_path"`
		PlainText string `db:"plain_text"`
		CreatedBy string `db:"created_by"`
	}
	err = sqlx.GetContext(ctx, tx, &itemRow, `SELECT title, summary, report_date, xml_storage_path, plain_text,
		COALESCE(created_by, '') AS created_by FROM items WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, zero, mcpToolError("not_found", "item not found")
	}
	if err != nil {
		return nil, zero, err
	}

	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		return nil, zero, err
	}
	principal, err := resolveMCPPrincipalTx(ctx, tx, s, t, requestHeaders(req))
	if err != nil {
		return nil, zero, err
	}

	mounts, err := itemMounts(ctx, tx, id)
	if err != nil {
		return nil, zero, err
	}
	visMounts := principal.filterVisible(mounts)
	isOwner := principal.kind == mcpAuthAPIToken && itemRow.CreatedBy != "" && itemRow.CreatedBy == principal.apiDigest
	if !isOwner && len(visMounts) == 0 {
		return nil, zero, mcpToolError("forbidden", "item is not accessible")
	}

	targetTitle, targetSummary, targetDate, targetXMLPath := itemRow.Title, itemRow.Summary, itemRow.Date, itemRow.XMLPath
	if in.Rev != nil {
		var revRow struct {
			ID      int64  `db:"id"`
			Title   string `db:"title"`
			Summary string `db:"summary"`
			Date    string `db:"report_date"`
			XMLPath string `db:"xml_storage_path"`
		}
		err = sqlx.GetContext(ctx, tx, &revRow,
			`SELECT id, title, summary, report_date, xml_storage_path FROM item_revisions WHERE item_id = ? AND rev = ?`,
			id, *in.Rev)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, zero, mcpToolError("not_found", "revision not found")
		}
		if err != nil {
			return nil, zero, err
		}
		if !isOwner {
			revMounts, mountErr := revisionMounts(ctx, tx, revRow.ID)
			if mountErr != nil {
				return nil, zero, mountErr
			}
			if !mcpCanViewRevisionMounts(t, principal, revMounts) {
				return nil, zero, mcpToolError("forbidden", "this revision is not accessible")
			}
		}
		targetTitle, targetSummary, targetDate, targetXMLPath = revRow.Title, revRow.Summary, revRow.Date, revRow.XMLPath
	}
	_ = tx.Rollback()

	var content string
	if format == "text" && in.Rev == nil {
		content = itemRow.PlainText
	} else {
		rc, openErr := s.store.Open(ctx, targetXMLPath)
		if errors.Is(openErr, storage.ErrNotFound) {
			return nil, zero, mcpToolError("not_found", "item content missing from storage")
		}
		if openErr != nil {
			return nil, zero, openErr
		}
		raw, readErr := io.ReadAll(io.LimitReader(rc, maxXMLSize+1))
		_ = rc.Close()
		if readErr != nil {
			return nil, zero, readErr
		}
		if format == "text" {
			parsed, _ := schema.Parse(raw, schema.Options{StoredAssets: true})
			if parsed == nil {
				return nil, zero, mcpToolError("not_found", "stored revision XML is unreadable")
			}
			content = parsed.PlainText()
		} else {
			content = string(raw)
		}
	}

	return nil, mcpGetItemOutput{
		ID:      id,
		Rev:     in.Rev,
		Title:   targetTitle,
		Summary: targetSummary,
		Date:    targetDate,
		Format:  format,
		Content: content,
		Dirs:    visMounts,
	}, nil
}

func (s *Server) mcpValidateXML(_ context.Context, _ *mcp.CallToolRequest, in mcpValidateXMLInput) (*mcp.CallToolResult, schema.ValidationResult, error) {
	if len(in.XML) > maxXMLSize {
		return nil, schema.ValidationResult{}, mcpToolError("invalid_request", "xml exceeds the 10MB limit")
	}
	_, issues := schema.Parse([]byte(in.XML), schema.Options{})
	return nil, schema.ValidationResult{
		Valid:  len(issues) == 0,
		File:   "mcp.xml",
		Errors: issues,
	}, nil
}

func (s *Server) mcpPushItem(ctx context.Context, req *mcp.CallToolRequest, in mcpPushItemInput) (*mcp.CallToolResult, ingestOutcome, error) {
	var zero ingestOutcome
	hdr := requestHeaders(req)

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, zero, err
	}
	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		_ = tx.Rollback()
		return nil, zero, err
	}
	principal, err := resolveMCPPrincipalTx(ctx, tx, s, t, hdr)
	_ = tx.Rollback()
	if err != nil {
		return nil, zero, err
	}
	if principal.kind == mcpAuthAnon {
		return nil, zero, mcpToolError("unauthorized", "an API token is required to call push_item")
	}
	if principal.kind != mcpAuthAPIToken {
		return nil, zero, mcpToolError("forbidden", "an API token is required to call push_item")
	}

	if len(in.XML) > maxXMLSize {
		return nil, zero, mcpToolError("invalid_request", "xml exceeds the 10MB limit")
	}
	var externalKey string
	if strings.TrimSpace(in.ExternalKey) != "" {
		externalKey, err = normalizeExternalKey(in.ExternalKey)
		if err != nil {
			return nil, zero, mcpToolError("invalid_request", err.Error())
		}
	}

	// Share the global write rate limit and 4-slot ingest concurrency pool with POST /api/v1/ingest.
	clientIP := strings.TrimSpace(hdr.Get("X-Aumbry-Internal-Client-IP"))
	if clientIP == "" {
		clientIP = "unknown"
	}
	fakeReq := &http.Request{Header: make(http.Header)}
	if s.writeRate != nil && s.writeRate.RespondOnLimit(httptest.NewRecorder(), fakeReq, clientIP) {
		return nil, zero, mcpToolError("too_many_requests", "write rate limit exceeded, try again later")
	}
	release, slotOK := s.acquireIngestSlot()
	if !slotOK {
		return nil, zero, mcpToolError("too_many_requests", "too many concurrent ingest requests, try again later")
	}
	defer release()

	// Reject local relative asset references over MCP before calling ingest (CLI handles binary uploads).
	doc, issues := schema.Parse([]byte(in.XML), schema.Options{})
	if doc != nil && len(doc.AssetRefs) > 0 {
		return nil, zero, mcpToolError("invalid_request",
			fmt.Sprintf("push_item does not accept local relative asset %q; upload via CLI `aumbry push` instead", doc.AssetRefs[0].Path))
	}
	if doc == nil || len(issues) > 0 {
		res := invalidResult("mcp.xml", issues...)
		raw, _ := json.Marshal(res)
		return nil, zero, mcpToolError("validation_failed", string(raw))
	}

	dirs := dedupe(in.Dirs)
	outcome, status, code, msg, valRes, storageErr, internalErr := s.executeIngest(ctx, ingestParams{
		tokenDigest: principal.apiDigest,
		draft:       in.Draft,
		createOnly:  in.CreateOnly,
		externalKey: externalKey,
		itemID:      strings.TrimSpace(in.ItemID),
		dirs:        dirs,
		fileLabel:   "mcp.xml",
		xmlBytes:    []byte(in.XML),
	})
	if valRes != nil {
		raw, _ := json.Marshal(valRes)
		return nil, zero, mcpToolError("validation_failed", string(raw))
	}
	if status != 0 {
		return nil, zero, mcpToolError(code, msg)
	}
	if storageErr != nil {
		return nil, zero, storageErr
	}
	if internalErr != nil {
		return nil, zero, internalErr
	}
	return nil, *outcome, nil
}

func requestHeaders(req *mcp.CallToolRequest) http.Header {
	if req != nil && req.Extra != nil && req.Extra.Header != nil {
		return req.Extra.Header
	}
	return make(http.Header)
}

func mcpCanViewRevisionMounts(t *dirTree, p mcpPrincipal, mounts []revMount) bool {
	for _, m := range mounts {
		if m.WasProtected && t.isPublic(m.DirID) {
			continue
		}
		if p.visibleDirs[m.DirID] {
			return true
		}
	}
	return false
}

func mcpRevisionCountTx(ctx context.Context, q sqlx.QueryerContext, t *dirTree, p mcpPrincipal, itemID, createdBy string) (int, error) {
	isOwner := p.kind == mcpAuthAPIToken && createdBy != "" && createdBy == p.apiDigest
	if isOwner {
		var count int
		err := sqlx.GetContext(ctx, q, &count, `SELECT count(*) FROM item_revisions WHERE item_id = ?`, itemID)
		return count, err
	}
	var revs []struct {
		ID  int64 `db:"id"`
		Rev int   `db:"rev"`
	}
	if err := sqlx.SelectContext(ctx, q, &revs, `SELECT id, rev FROM item_revisions WHERE item_id = ? ORDER BY rev DESC`, itemID); err != nil {
		return 0, err
	}
	if len(revs) == 0 {
		return 0, nil
	}
	ids := make([]int64, len(revs))
	for i, rv := range revs {
		ids[i] = rv.ID
	}
	mountsByRev, err := revisionMountsMap(ctx, q, ids)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, rv := range revs {
		if mcpCanViewRevisionMounts(t, p, mountsByRev[rv.ID]) {
			count++
		}
	}
	return count, nil
}

func mountsForItemsTx(ctx context.Context, q sqlx.QueryerContext, itemIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(itemIDs))
	if len(itemIDs) == 0 {
		return out, nil
	}
	query, args, err := sqlx.In(`SELECT item_id, directory_id FROM directory_items WHERE item_id IN (?) ORDER BY directory_id`, itemIDs)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ItemID string `db:"item_id"`
		DirID  string `db:"directory_id"`
	}
	if err = sqlx.SelectContext(ctx, q, &rows, query, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ItemID] = append(out[r.ItemID], r.DirID)
	}
	return out, nil
}

func ownersForItemsTx(ctx context.Context, q sqlx.QueryerContext, itemIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(itemIDs))
	if len(itemIDs) == 0 {
		return out, nil
	}
	query, args, err := sqlx.In(`SELECT id, COALESCE(created_by, '') AS created_by FROM items WHERE id IN (?)`, itemIDs)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID        string `db:"id"`
		CreatedBy string `db:"created_by"`
	}
	if err = sqlx.SelectContext(ctx, q, &rows, query, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = r.CreatedBy
	}
	return out, nil
}

func toMCPItemEntry(sum itemSummary, visDirs []string, revCount int) mcpItemEntry {
	prefix := "/r/"
	if sum.Type == "post" {
		prefix = "/p/"
	}
	if visDirs == nil {
		visDirs = []string{}
	}
	return mcpItemEntry{
		ID:            sum.ID,
		Type:          sum.Type,
		Title:         sum.Title,
		Date:          sum.Date,
		Summary:       sum.Summary,
		Tags:          sum.Tags,
		SourceAgent:   sum.SourceAgent,
		Dirs:          visDirs,
		URL:           prefix + sum.ID,
		RevisionCount: revCount,
	}
}
