package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"

	"github.com/nerdneilsfield/aumbry/internal/auth"
	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/internal/storage"
)

const (
	adminUser     = "admin"
	adminPassword = "admin-pw"
	apiToken      = "amb_test_token"
	cookieSecret  = "0123456789abcdef0123456789abcdef"
)

// memStore is an in-memory storage.Store.
type memStore struct {
	mu         sync.Mutex
	files      map[string][]byte
	probeErr   error
	existsErr  error
	putErr     error
	probeCalls atomic.Int32
	onProbe    func()
	onPut      func()
	onPutPath  func(name string)
}

func (m *memStore) Put(_ context.Context, name string, body io.ReadSeeker) error {
	if m.onPut != nil {
		m.onPut()
	}
	if m.onPutPath != nil {
		m.onPutPath(name)
	}
	data, err := io.ReadAll(body)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil && m.putErr != nil {
		return m.putErr
	}
	m.files[name] = data
	return err
}

func (m *memStore) Open(_ context.Context, name string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[name]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *memStore) Exists(_ context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.existsErr != nil {
		return false, m.existsErr
	}
	_, ok := m.files[name]
	return ok, nil
}

func (m *memStore) Delete(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, name)
	return nil
}

func (m *memStore) List(_ context.Context, dir string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prefix := strings.TrimSuffix(path.Clean("/"+dir), "/") + "/"
	var out []string
	for name := range m.files {
		if strings.HasPrefix(name, prefix) && !strings.Contains(strings.TrimPrefix(name, prefix), "/") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m *memStore) Probe(_ context.Context) error {
	m.probeCalls.Add(1)
	if m.onProbe != nil {
		m.onProbe()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.probeErr
}

func (m *memStore) setProbeErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probeErr = err
}

func (m *memStore) count(prefix string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for name := range m.files {
		if strings.HasPrefix(name, prefix) {
			n++
		}
	}
	return n
}

// has reports whether an object whose name starts with prefix holds exactly content.
func (m *memStore) has(prefix, content string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, data := range m.files {
		if strings.HasPrefix(name, prefix) && string(data) == content {
			return true
		}
	}
	return false
}

type env struct {
	t      *testing.T
	srv    *httptest.Server
	router http.Handler
	db     *sqlx.DB
	store  *memStore
	secret string
	dir    string // scratch slot for tests
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return newEnvWithOptions(t, RouterOptions{}, nil)
}

func newEnvWithOptions(t *testing.T, opts RouterOptions, logger *slog.Logger) *env {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.New(auth.Settings{
		AdminUser: adminUser, AdminPasswordHash: string(hash), APITokens: []string{"other", apiToken}, CookieSecret: cookieSecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &memStore{files: map[string][]byte{}}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	router, err := NewRouter(database, store, sessions, logger, opts)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, router: router, db: database, store: store, secret: cookieSecret}
}

// client is an HTTP client with its own cookie jar, i.e. one browser.
type client struct {
	e      *env
	http   *http.Client
	cookie string // sent verbatim when set
}

func (e *env) anon() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, http: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (e *env) admin() *client {
	c := e.anon()
	if resp := c.do("POST", "/api/v1/admin/login", map[string]string{"username": adminUser, "password": adminPassword}); resp.status != http.StatusOK {
		e.t.Fatalf("admin login: %d %s", resp.status, resp.body)
	}
	return c
}

type response struct {
	t      *testing.T
	status int
	header http.Header
	body   []byte
}

func (r response) json() map[string]any {
	r.t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		r.t.Fatalf("response is not a JSON object: %v: %s", err, r.body)
	}
	return m
}

func (c *client) send(req *http.Request) response {
	c.e.t.Helper()
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return response{t: c.e.t, status: resp.StatusCode, header: resp.Header, body: body}
}

// do sends body (a value marshalled as JSON, or nil) to path.
func (c *client) do(method, path string, body any) response {
	c.e.t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, c.e.srv.URL+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.send(req)
}

// rawCookie returns the value of the named cookie held by this client's jar.
func (c *client) rawCookie(name string) string {
	for _, ck := range c.http.Jar.Cookies(mustURL(c.e.srv.URL)) {
		if ck.Name == name {
			return ck.Value
		}
	}
	return ""
}

func (c *client) createDir(parent, name, password string) string {
	c.e.t.Helper()
	body := map[string]any{"name": name}
	if parent != "" {
		body["parent_id"] = parent
	}
	if password != "" {
		body["password"] = password
	}
	resp := c.do("POST", "/api/v1/admin/dirs", body)
	if resp.status != http.StatusCreated {
		c.e.t.Fatalf("create dir %q: %d %s", name, resp.status, resp.body)
	}
	return resp.json()["id"].(string)
}

type figure struct {
	src  string
	name string
	data []byte
}

type ingestReq struct {
	token      string // empty means the valid token; "-" sends none
	xml        string
	assets     []figure
	dirs       []string
	itemID     string
	key        string
	rawKey     *string
	draft      bool
	createOnly bool
	rawSrcs    []string // overrides asset_srcs when non-nil
	commaDir   bool
}

func (c *client) ingest(in ingestReq) response {
	c.e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("xml", "doc.xml")
	_, _ = part.Write([]byte(in.xml))
	for _, f := range in.assets {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="assets[]"; filename="%s"`, f.name))
		p, _ := mw.CreatePart(h)
		_, _ = p.Write(f.data)
	}
	srcs := in.rawSrcs
	if srcs == nil {
		for _, f := range in.assets {
			srcs = append(srcs, f.src)
		}
	}
	for _, s := range srcs {
		_ = mw.WriteField("asset_srcs", s)
	}
	if in.commaDir {
		_ = mw.WriteField("dirs", strings.Join(in.dirs, ","))
	} else {
		for _, d := range in.dirs {
			_ = mw.WriteField("dirs", d)
		}
	}
	if in.itemID != "" {
		_ = mw.WriteField("item_id", in.itemID)
	}
	if in.rawKey != nil {
		_ = mw.WriteField("external_key", *in.rawKey)
	} else if in.key != "" {
		_ = mw.WriteField("external_key", in.key)
	}
	if in.draft {
		_ = mw.WriteField("draft", "1")
	}
	if in.createOnly {
		_ = mw.WriteField("create_only", "1")
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", c.e.srv.URL+"/api/v1/ingest", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	switch in.token {
	case "":
		req.Header.Set("Authorization", "Bearer "+apiToken)
	case "-":
	default:
		req.Header.Set("Authorization", "Bearer "+in.token)
	}
	return c.send(req)
}

// ingestOK ingests and returns the new item's id.
func (c *client) ingestOK(in ingestReq) string {
	c.e.t.Helper()
	resp := c.ingest(in)
	if resp.status != http.StatusCreated && resp.status != http.StatusOK {
		c.e.t.Fatalf("ingest: %d %s", resp.status, resp.body)
	}
	return resp.json()["id"].(string)
}

type docOpts struct {
	title, typ, date, summary string
	tags                      []string
	related                   []string
	md                        string
	figures                   []string
}

// xmlDoc builds a valid Aumbry document.
func xmlDoc(o docOpts) string {
	if o.typ == "" {
		o.typ = "report"
	}
	if o.date == "" {
		o.date = "2026-11-20"
	}
	if o.summary == "" {
		o.summary = "summary of " + o.title
	}
	if o.md == "" {
		o.md = "body text"
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><document version="1.1"><meta>`)
	fmt.Fprintf(&b, "<title>%s</title><type>%s</type><date>%s</date><summary>%s</summary><source-agent>test-agent</source-agent>", o.title, o.typ, o.date, o.summary)
	if len(o.tags) > 0 {
		b.WriteString("<tags>")
		for _, tag := range o.tags {
			fmt.Fprintf(&b, "<tag>%s</tag>", tag)
		}
		b.WriteString("</tags>")
	}
	if len(o.related) > 0 {
		b.WriteString("<related-ids>")
		for _, id := range o.related {
			fmt.Fprintf(&b, "<id>%s</id>", id)
		}
		b.WriteString("</related-ids>")
	}
	b.WriteString(`</meta><body><section title="Main">`)
	fmt.Fprintf(&b, "<md><![CDATA[%s]]></md>", o.md)
	for _, src := range o.figures {
		fmt.Fprintf(&b, `<figure src="%s" caption="fig"/>`, src)
	}
	b.WriteString(`</section></body></document>`)
	return b.String()
}

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}
