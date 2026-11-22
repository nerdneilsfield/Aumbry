package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/webdav"

	"github.com/nerdneilsfield/aumbry/internal/auth"
	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/internal/storage"
)

func serveReq(handler http.Handler, method, target, remoteAddr string, headers map[string]string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestTrustedProxiesRateLimitAndCookies(t *testing.T) {
	t.Run("default config ignores XFF and X-Forwarded-Proto", func(t *testing.T) {
		e := newEnv(t)

		// Exhaust login budget from one peer while rotating spoofed XFF and X-Forwarded-Proto.
		for i := 1; i <= 10; i++ {
			rec := serveReq(e.router, http.MethodPost, "/api/v1/admin/login", "198.51.100.9:1234", map[string]string{
				"X-Forwarded-For":   fmt.Sprintf("203.0.113.%d", i),
				"X-Forwarded-Proto": "https",
			}, `{"username":"admin","password":"wrong"}`)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("attempt %d status = %d, want 401", i, rec.Code)
			}
		}
		// 11th attempt with a fresh spoofed XFF must still share the TCP peer bucket -> 429.
		rec := serveReq(e.router, http.MethodPost, "/api/v1/admin/login", "198.51.100.9:5678", map[string]string{
			"X-Forwarded-For": "203.0.113.200",
		}, `{"username":"admin","password":"wrong"}`)
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("11th attempt status = %d, want 429", rec.Code)
		}

		// Valid login from a different peer with X-Forwarded-Proto: https must NOT set Secure cookie.
		okRec := serveReq(e.router, http.MethodPost, "/api/v1/admin/login", "198.51.100.10:1234", map[string]string{
			"X-Forwarded-Proto": "https",
		}, fmt.Sprintf(`{"username":%q,"password":%q}`, adminUser, adminPassword))
		if okRec.Code != http.StatusOK {
			t.Fatalf("valid login = %d %s", okRec.Code, okRec.Body.String())
		}
		if setCookie := strings.Join(okRec.Header().Values("Set-Cookie"), "\n"); strings.Contains(setCookie, "Secure") {
			t.Errorf("Set-Cookie %q must not have Secure under default config", setCookie)
		}
	})

	t.Run("trusted proxy resolves XFF right-to-left and ignores untrusted peers", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
		e := newEnvWithOptions(t, RouterOptions{
			TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
		}, logger)

		// Trusted proxy 10.0.0.5 forwarding client 203.0.113.7 (with an extra trusted hop 10.0.0.9 on right).
		for i := 1; i <= 10; i++ {
			rec := serveReq(e.router, http.MethodPost, "/api/v1/admin/login", "10.0.0.5:4000", map[string]string{
				"X-Forwarded-For": "1.1.1.1, 203.0.113.7, 10.0.0.9",
			}, `{"username":"admin","password":"wrong"}`)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("client A attempt %d = %d, want 401", i, rec.Code)
			}
		}
		// Verify logged client.ip is 203.0.113.7 (not 10.0.0.5 or 1.1.1.1).
		if !strings.Contains(logBuf.String(), `"client.ip":"203.0.113.7"`) {
			t.Errorf("request log missing resolved client.ip 203.0.113.7:\n%s", logBuf.String())
		}

		// Second client behind the same trusted proxy still has its full 10-attempt budget.
		for i := 1; i <= 10; i++ {
			rec := serveReq(e.router, http.MethodPost, "/api/v1/admin/login", "10.0.0.5:4001", map[string]string{
				"X-Forwarded-For": "203.0.113.8",
			}, `{"username":"admin","password":"wrong"}`)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("client B attempt %d = %d, want 401 (must not share bucket with client A)", i, rec.Code)
			}
		}
		// 11th attempt for client A -> 429, and 11th for client B -> 429.
		if got := serveReq(e.router, http.MethodPost, "/api/v1/admin/login", "10.0.0.5:4002", map[string]string{
			"X-Forwarded-For": "203.0.113.7",
		}, `{"username":"admin","password":"wrong"}`).Code; got != http.StatusTooManyRequests {
			t.Errorf("client A 11th = %d, want 429", got)
		}
		if got := serveReq(e.router, http.MethodPost, "/api/v1/admin/login", "10.0.0.5:4003", map[string]string{
			"X-Forwarded-For": "203.0.113.8",
		}, `{"username":"admin","password":"wrong"}`).Code; got != http.StatusTooManyRequests {
			t.Errorf("client B 11th = %d, want 429", got)
		}

		// Missing or malformed XFF from trusted proxy falls back to RemoteAddr (10.0.0.99).
		logBuf.Reset()
		_ = serveReq(e.router, http.MethodGet, "/api/v1/nope", "10.0.0.99:5000", map[string]string{
			"X-Forwarded-For": "not-an-ip",
		}, "")
		if !strings.Contains(logBuf.String(), `"client.ip":"10.0.0.99"`) {
			t.Errorf("fallback log missing peer IP 10.0.0.99:\n%s", logBuf.String())
		}

		// Untrusted peer 198.51.100.9 trying to spoof client A's IP or X-Forwarded-Proto is ignored.
		logBuf.Reset()
		untrusted := serveReq(e.router, http.MethodPost, "/api/v1/admin/login", "198.51.100.9:5000", map[string]string{
			"X-Forwarded-For":   "203.0.113.7",
			"X-Forwarded-Proto": "https",
		}, fmt.Sprintf(`{"username":%q,"password":%q}`, adminUser, adminPassword))
		if untrusted.Code != http.StatusOK {
			t.Fatalf("untrusted peer login = %d %s (should not hit 203.0.113.7's exhausted bucket)", untrusted.Code, untrusted.Body.String())
		}
		if setCookie := strings.Join(untrusted.Header().Values("Set-Cookie"), "\n"); strings.Contains(setCookie, "Secure") {
			t.Errorf("untrusted peer got Secure cookie: %q", setCookie)
		}
		if !strings.Contains(logBuf.String(), `"client.ip":"198.51.100.9"`) {
			t.Errorf("untrusted peer log missing 198.51.100.9:\n%s", logBuf.String())
		}

		// Trusted peer with X-Forwarded-Proto: https gets Secure cookie.
		trustedHTTPS := serveReq(e.router, http.MethodPost, "/api/v1/admin/login", "10.0.0.5:5001", map[string]string{
			"X-Forwarded-For":   "203.0.113.50",
			"X-Forwarded-Proto": "https",
		}, fmt.Sprintf(`{"username":%q,"password":%q}`, adminUser, adminPassword))
		if trustedHTTPS.Code != http.StatusOK {
			t.Fatalf("trusted peer login = %d %s", trustedHTTPS.Code, trustedHTTPS.Body.String())
		}
		if setCookie := strings.Join(trustedHTTPS.Header().Values("Set-Cookie"), "\n"); !strings.Contains(setCookie, "Secure") {
			t.Errorf("trusted proxy HTTPS request missing Secure cookie: %q", setCookie)
		}
	})
}

func TestSecurityHeadersOnAllResponses(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "Public", "")
	id := admin.ingestOK(ingestReq{
		xml:    xmlDoc(docOpts{title: "HeaderCheck", figures: []string{"./a.svg"}}),
		assets: []figure{{src: "./a.svg", name: "a.svg", data: []byte(testSVG)}},
		dirs:   []string{dir},
	})
	hash := hashOf(t, admin, id)

	// Exhaust login rate limit on one IP so we can check a 429 response too.
	for range 10 {
		_ = serveReq(e.router, http.MethodPost, "/api/v1/admin/login", "192.0.2.77:1111", nil, `{"username":"admin","password":"wrong"}`)
	}

	assertHeaders := func(t *testing.T, name string, h http.Header) {
		t.Helper()
		for k, want := range map[string]string{
			"Referrer-Policy":        "no-referrer",
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
		} {
			if got := h.Get(k); got != want {
				t.Errorf("%s: header %s = %q, want %q", name, k, got, want)
			}
		}
	}

	// 200 JSON (/healthz)
	rHealth := serveReq(e.router, http.MethodGet, "/healthz", "192.0.2.1:1000", nil, "")
	assertHeaders(t, "healthz", rHealth.Header())

	// 404 JSON
	r404 := serveReq(e.router, http.MethodGet, "/api/v1/nope", "192.0.2.1:1000", nil, "")
	if r404.Code != http.StatusNotFound {
		t.Fatalf("404 status = %d", r404.Code)
	}
	assertHeaders(t, "api 404", r404.Header())

	// 429 rate limit
	r429 := serveReq(e.router, http.MethodPost, "/api/v1/admin/login", "192.0.2.77:1111", nil, `{"username":"admin","password":"wrong"}`)
	if r429.Code != http.StatusTooManyRequests {
		t.Fatalf("429 status = %d", r429.Code)
	}
	assertHeaders(t, "429", r429.Header())

	// SPA fallback (returns 503 when dist/ only has .gitkeep, or 200 when built — either way headers are set)
	rSPA := serveReq(e.router, http.MethodGet, "/d/example", "192.0.2.1:1000", nil, "")
	assertHeaders(t, "spa fallback", rSPA.Header())

	// Asset and XML content preserve sandbox CSP while gaining global security headers.
	for _, target := range []string{"/api/v1/assets/" + hash, "/api/v1/items/" + id + "/content"} {
		resp := e.anon().do(http.MethodGet, target, nil)
		if resp.status != http.StatusOK {
			t.Fatalf("%s status = %d", target, resp.status)
		}
		assertHeaders(t, target, resp.header)
		if csp := resp.header.Get("Content-Security-Policy"); csp != "default-src 'none'; sandbox" {
			t.Errorf("%s CSP = %q, want sandbox CSP preserved", target, csp)
		}
	}
}

func TestReadyzProbeCachingAndRecovery(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	e := newEnvWithOptions(t, RouterOptions{
		ReadyCacheTTL: 60 * time.Millisecond,
		ReadyTimeout:  200 * time.Millisecond,
	}, logger)

	// Healthy initially.
	rec := serveReq(e.router, http.MethodGet, "/readyz", "127.0.0.1:1000", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("initial /readyz = %d %s", rec.Code, rec.Body.String())
	}

	// Subsequent request within cache TTL hits cache rather than re-probing WebDAV.
	if calls := e.store.probeCalls.Load(); calls != 1 {
		t.Fatalf("probeCalls = %d, want 1", calls)
	}
	rec2 := serveReq(e.router, http.MethodGet, "/readyz", "127.0.0.1:1000", nil, "")
	if rec2.Code != http.StatusOK || e.store.probeCalls.Load() != 1 {
		t.Fatalf("cached /readyz status=%d probeCalls=%d, want 200 and 1", rec2.Code, e.store.probeCalls.Load())
	}

	// Wait for cache expiry, simulate unreachable WebDAV with embedded credentials in error string.
	time.Sleep(80 * time.Millisecond)
	e.store.setProbeErr(fmt.Errorf("%w: Get \"https://davuser:topsecret@dav.example.internal/\": dial failed", storage.ErrUnreachable))

	recDown := serveReq(e.router, http.MethodGet, "/readyz", "127.0.0.1:1000", nil, "")
	if recDown.Code != http.StatusServiceUnavailable {
		t.Fatalf("down /readyz = %d %s, want 503", recDown.Code, recDown.Body.String())
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(recDown.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "unavailable" || body.Checks["sqlite"] != "ok" || body.Checks["storage"] != "unreachable" {
		t.Fatalf("body = %+v", body)
	}
	if _, hasOldKey := body.Checks["webdav"]; hasOldKey {
		t.Fatalf("/readyz still exposes checks.webdav: %+v", body.Checks)
	}
	if strings.Contains(recDown.Body.String(), "topsecret") || strings.Contains(recDown.Body.String(), "dav.example.internal") {
		t.Errorf("/readyz response leaked internal details: %s", recDown.Body.String())
	}
	if strings.Contains(logBuf.String(), "topsecret") {
		t.Errorf("readiness failure log leaked credentials: %s", logBuf.String())
	}

	// /healthz remains 200 while WebDAV is down.
	if h := serveReq(e.router, http.MethodGet, "/healthz", "127.0.0.1:1000", nil, "").Code; h != http.StatusOK {
		t.Fatalf("/healthz while WebDAV down = %d, want 200", h)
	}

	// Auth failure and timeout categories.
	time.Sleep(80 * time.Millisecond)
	e.store.setProbeErr(storage.ErrAuth)
	if recAuth := serveReq(e.router, http.MethodGet, "/readyz", "127.0.0.1:1000", nil, ""); !strings.Contains(recAuth.Body.String(), `"auth"`) {
		t.Errorf("auth failure body = %s, want category auth", recAuth.Body.String())
	}

	time.Sleep(80 * time.Millisecond)
	e.store.setProbeErr(storage.ErrTimeout)
	if recTO := serveReq(e.router, http.MethodGet, "/readyz", "127.0.0.1:1000", nil, ""); !strings.Contains(recTO.Body.String(), `"timeout"`) {
		t.Errorf("timeout failure body = %s, want category timeout", recTO.Body.String())
	}

	// Recovery after TTL returns 200.
	time.Sleep(80 * time.Millisecond)
	e.store.setProbeErr(nil)
	if recUp := serveReq(e.router, http.MethodGet, "/readyz", "127.0.0.1:1000", nil, ""); recUp.Code != http.StatusOK {
		t.Fatalf("recovered /readyz = %d %s, want 200", recUp.Code, recUp.Body.String())
	}
}

func TestReadyzCoalescesConcurrentProbes(t *testing.T) {
	e := newEnvWithOptions(t, RouterOptions{
		ReadyCacheTTL: 5 * time.Second,
		ReadyTimeout:  time.Second,
	}, nil)

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	e.store.onProbe = func() {
		once.Do(func() { close(entered) })
		<-release
	}

	const n = 5
	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			codes[idx] = serveReq(e.router, http.MethodGet, "/readyz", "127.0.0.1:1000", nil, "").Code
		}(i)
	}
	<-entered
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, c := range codes {
		if c != http.StatusOK {
			t.Errorf("caller %d status = %d, want 200", i, c)
		}
	}
	if calls := e.store.probeCalls.Load(); calls != 1 {
		t.Errorf("probeCalls = %d, want 1 coalesced probe", calls)
	}
}

func TestIngestRateLimitAndThrottle(t *testing.T) {
	t.Run("120 per minute limit cannot be bypassed with spoofed XFF", func(t *testing.T) {
		e := newEnv(t)
		for i := 1; i <= writeRateLimitPerMin; i++ {
			rec := serveReq(e.router, http.MethodPost, "/api/v1/ingest", "198.51.100.42:1000", map[string]string{
				"X-Forwarded-For": fmt.Sprintf("203.0.113.%d", i%250+1),
			}, "")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("request %d = %d, want 401 (before rate limit)", i, rec.Code)
			}
		}
		rec := serveReq(e.router, http.MethodPost, "/api/v1/ingest", "198.51.100.42:1000", map[string]string{
			"X-Forwarded-For": "203.0.113.250",
		}, "")
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request 121 = %d %s, want 429", rec.Code, rec.Body.String())
		}
	})

	t.Run("throttle caps concurrent ingest requests at 4", func(t *testing.T) {
		e := newEnv(t)
		entered := make(chan struct{}, ingestConcurrencyLimit)
		release := make(chan struct{})
		e.store.onPut = func() {
			entered <- struct{}{}
			<-release
		}

		c := e.anon()
		var wg sync.WaitGroup
		for i := range ingestConcurrencyLimit {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				resp := c.ingest(ingestReq{
					xml: xmlDoc(docOpts{title: fmt.Sprintf("Concurrent-%d", idx)}),
				})
				if resp.status != http.StatusCreated {
					t.Errorf("concurrent ingest %d = %d %s", idx, resp.status, resp.body)
				}
			}(i)
		}
		for range ingestConcurrencyLimit {
			<-entered
		}

		// 5th concurrent request while 4 are in-flight must be rejected with 429 immediately.
		fifth := serveReq(e.router, http.MethodPost, "/api/v1/ingest", "127.0.0.1:9999", map[string]string{
			"Authorization": "Bearer " + apiToken,
		}, "")
		if fifth.Code != http.StatusTooManyRequests {
			t.Errorf("5th concurrent ingest = %d %s, want 429", fifth.Code, fifth.Body.String())
		}

		close(release)
		wg.Wait()
	})
}

func TestIngestReturns5xxOnWebDAVHeaderTimeout(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	sessions, err := auth.New(auth.Settings{
		AdminUser: adminUser, AdminPasswordHash: testHash(t), APITokens: []string{apiToken}, CookieSecret: cookieSecret,
	})
	if err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	slowDav := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer slowDav.Close()
	defer close(release)

	store, err := storage.NewWebDAVWithOptions(slowDav.URL, "u", "p", storage.WebDAVOptions{
		DialTimeout:           100 * time.Millisecond,
		ResponseHeaderTimeout: 60 * time.Millisecond,
		ProbeTimeout:          50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	router, err := NewRouter(database, store, sessions, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	e := &env{t: t, srv: srv, router: router}
	resp := e.anon().ingest(ingestReq{xml: xmlDoc(docOpts{title: "SlowDAV"})})
	if resp.status < 500 || resp.status >= 600 {
		t.Fatalf("ingest against hanging WebDAV status = %d %s, want 5xx", resp.status, resp.body)
	}
}

func TestS3ReadyzAndErrorSanitization(t *testing.T) {
	const (
		secretBucket = "aumbry-private-bucket"
		secretAK     = "AKIA_SECRET_KEY_ID"
		secretSK     = "super_secret_access_key_value"
	)

	var mode atomic.Int32 // 0 = 200 ok, 1 = 403 auth, 2 = hang timeout
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode.Load() {
		case 1:
			w.WriteHeader(http.StatusForbidden)
		case 2:
			<-r.Context().Done()
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	store, err := storage.NewS3(storage.S3Options{
		Endpoint:              srv.URL,
		Region:                "us-east-1",
		Bucket:                secretBucket,
		AccessKeyID:           secretAK,
		SecretAccessKey:       secretSK,
		PathStyle:             true,
		DialTimeout:           100 * time.Millisecond,
		ResponseHeaderTimeout: 100 * time.Millisecond,
		ProbeTimeout:          50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	sessions, err := auth.New(auth.Settings{
		AdminUser: adminUser, AdminPasswordHash: testHash(t), APITokens: []string{apiToken}, CookieSecret: cookieSecret,
	})
	if err != nil {
		t.Fatal(err)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	router, err := NewRouter(database, store, sessions, logger, RouterOptions{
		ReadyCacheTTL: 40 * time.Millisecond,
		ReadyTimeout:  120 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	checkNoLeaks := func(body string) {
		t.Helper()
		for _, secret := range []string{srv.URL, secretBucket, secretAK, secretSK, "Signature", "webdav"} {
			if strings.Contains(body, secret) {
				t.Errorf("/readyz body leaked %q: %s", secret, body)
			}
		}
		for _, logSecret := range []string{secretAK, secretSK, "X-Amz-Signature"} {
			if strings.Contains(logBuf.String(), logSecret) {
				t.Errorf("readiness log leaked %q: %s", logSecret, logBuf.String())
			}
		}
	}

	rec := serveReq(router, http.MethodGet, "/readyz", "127.0.0.1:1000", nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"storage":"ok"`) {
		t.Fatalf("healthy S3 /readyz = %d %s", rec.Code, rec.Body.String())
	}
	checkNoLeaks(rec.Body.String())

	time.Sleep(60 * time.Millisecond)
	mode.Store(1)
	recAuth := serveReq(router, http.MethodGet, "/readyz", "127.0.0.1:1000", nil, "")
	if recAuth.Code != http.StatusServiceUnavailable || !strings.Contains(recAuth.Body.String(), `"storage":"auth"`) {
		t.Fatalf("403 S3 /readyz = %d %s", recAuth.Code, recAuth.Body.String())
	}
	checkNoLeaks(recAuth.Body.String())

	time.Sleep(60 * time.Millisecond)
	mode.Store(2)
	recTO := serveReq(router, http.MethodGet, "/readyz", "127.0.0.1:1000", nil, "")
	if recTO.Code != http.StatusServiceUnavailable || !strings.Contains(recTO.Body.String(), `"storage":"timeout"`) {
		t.Fatalf("timeout S3 /readyz = %d %s", recTO.Code, recTO.Body.String())
	}
	checkNoLeaks(recTO.Body.String())

	srv.Close()
	time.Sleep(60 * time.Millisecond)
	recDown := serveReq(router, http.MethodGet, "/readyz", "127.0.0.1:1000", nil, "")
	if recDown.Code != http.StatusServiceUnavailable || !strings.Contains(recDown.Body.String(), `"storage":"unreachable"`) {
		t.Fatalf("closed S3 /readyz = %d %s", recDown.Code, recDown.Body.String())
	}
	checkNoLeaks(recDown.Body.String())
}

func TestIngestReturns5xxOnS3HeaderTimeout(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	sessions, err := auth.New(auth.Settings{
		AdminUser: adminUser, AdminPasswordHash: testHash(t), APITokens: []string{apiToken}, CookieSecret: cookieSecret,
	})
	if err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	slowS3 := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer slowS3.Close()
	defer close(release)

	store, err := storage.NewS3(storage.S3Options{
		Endpoint:              slowS3.URL,
		Region:                "us-east-1",
		Bucket:                "aumbry",
		AccessKeyID:           "ak",
		SecretAccessKey:       "sk",
		PathStyle:             true,
		DialTimeout:           100 * time.Millisecond,
		ResponseHeaderTimeout: 60 * time.Millisecond,
		ProbeTimeout:          50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	router, err := NewRouter(database, store, sessions, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	e := &env{t: t, srv: srv, router: router}
	resp := e.anon().ingest(ingestReq{xml: xmlDoc(docOpts{title: "SlowS3"})})
	if resp.status < 500 || resp.status >= 600 {
		t.Fatalf("ingest against hanging S3 status = %d %s, want 5xx", resp.status, resp.body)
	}
}

// Keep unused imports check clean if webdav/net/atomic are exercised across helpers.
var (
	_ = net.IPv4zero
	_ = atomic.Int32{}
	_ = webdav.Handler{}
	_ = errors.New
)

func testHash(t *testing.T) string {
	t.Helper()
	// Matches helpers_test.go bcrypt cost.
	s, err := auth.New(auth.Settings{
		AdminUser: "a", AdminPasswordHash: "$2a$04$7/3lTQv5N3F2i9a8Z5E5mOq1rY3U5n9v1k4F8n9y0x2w4z6A8C0E2",
		APITokens: []string{"t"}, CookieSecret: cookieSecret,
	})
	_ = s
	_ = err
	return "$2a$04$7/3lTQv5N3F2i9a8Z5E5mOq1rY3U5n9v1k4F8n9y0x2w4z6A8C0E2"
}

func TestSanitizeErrorMasksSecrets(t *testing.T) {
	err := errors.New(`Get "https://user:pw@host/b?X-Amz-Signature=abc123&X-Amz-Credential=AKIAXYZ%2F2026": denied Authorization: AWS4-HMAC-SHA256 Credential=AKIAXYZ/2026/auto/s3/aws4_request, Signature=deadbeef`)
	got := sanitizeError(err)
	for _, secret := range []string{"pw@", "abc123", "AKIAXYZ", "deadbeef"} {
		if strings.Contains(got, secret) {
			t.Errorf("sanitizeError left %q in %q", secret, got)
		}
	}
}
