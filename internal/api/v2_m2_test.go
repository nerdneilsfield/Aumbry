package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"golang.org/x/crypto/bcrypt"

	"github.com/nerdneilsfield/aumbry/internal/auth"
	"github.com/nerdneilsfield/aumbry/internal/db"
)

func buildMinimalTestPDF(t *testing.T, pages int) []byte {
	t.Helper()
	if pages < 1 {
		pages = 1
	}
	kids := make([]string, 0, pages)
	objs := make([]string, 0, pages)
	for i := range pages {
		objNum := 3 + i
		kids = append(kids, fmt.Sprintf("%d 0 R", objNum))
		objs = append(objs, fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n", objNum))
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, pages+2)
	addObj := func(body string) {
		offsets = append(offsets, buf.Len())
		buf.WriteString(body)
	}
	addObj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	addObj(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", strings.Join(kids, " "), pages))
	for _, o := range objs {
		addObj(o)
	}
	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xrefOffset)
	return buf.Bytes()
}

func encryptMinimalTestPDF(t *testing.T, raw []byte, userPW, ownerPW string) []byte {
	t.Helper()
	conf := model.NewAESConfiguration(userPW, ownerPW, 256)
	conf.Offline = true
	conf.ValidationMode = model.ValidationRelaxed
	var out bytes.Buffer
	if err := pdfcpuapi.Encrypt(context.Background(), bytes.NewReader(raw), &out, conf); err != nil {
		t.Fatalf("Encrypt test PDF: %v", err)
	}
	return out.Bytes()
}

func validTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func validTestWebP() []byte {
	return []byte{
		'R', 'I', 'F', 'F', 22, 0, 0, 0, 'W', 'E', 'B', 'P',
		'V', 'P', '8', ' ', 10, 0, 0, 0, 0x30, 0x01, 0x00, 0x9d, 0x01, 0x2a, 0x01, 0x00, 0x01, 0x00,
	}
}

func callMCPJSON(t *testing.T, sess *mcp.ClientSession, name string, args any) (map[string]any, string, bool) {
	t.Helper()
	res, err := sess.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) transport error: %v", name, err)
	}
	var raw string
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcp.TextContent); ok {
			raw = tc.Text
		}
	}
	var parsed map[string]any
	if !res.IsError && raw != "" {
		if uErr := json.Unmarshal([]byte(raw), &parsed); uErr != nil {
			t.Fatalf("CallTool(%s) non-JSON result %q: %v", name, raw, uErr)
		}
	}
	return parsed, raw, res.IsError
}

func TestV2M2PDFAttachments(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	anon := e.anon()
	pubDir := admin.createDir("", "Public Papers", "")

	pdf3 := buildMinimalTestPDF(t, 3)
	ownerOnlyPDF := encryptMinimalTestPDF(t, pdf3, "", "owner-secret")
	userLockedPDF := encryptMinimalTestPDF(t, pdf3, "user-secret", "owner-secret")

	validXML := `<?xml version="1.0" encoding="UTF-8"?><document version="1.2">
<meta><title>PDF Paper Study</title><type>post</type><date>2026-11-23</date><summary>With PDF attachment</summary><source-agent>test-agent</source-agent></meta>
<body>
  <section id="sec1" title="Paper PDF">
    <md><![CDATA[Body text here.]]></md>
    <attachment id="orig" type="pdf" src="files/secretfilenamepdf.pdf" title="AppendixOriginalPDF"/>
    <attachment id="ownerpdf" type="pdf" src="files/owner-only.pdf" title="OwnerLockedAppendix"/>
    <next-steps><option target="#att-orig:p=2">Jump page 2</option></next-steps>
  </section>
</body></document>`

	resp := admin.ingest(ingestReq{
		xml: validXML,
		assets: []figure{
			{src: "files/secretfilenamepdf.pdf", name: "secretfilenamepdf.pdf", data: pdf3},
			{src: "files/owner-only.pdf", name: "owner-only.pdf", data: ownerOnlyPDF},
		},
		dirs: []string{pubDir},
	})
	if resp.status != http.StatusCreated {
		t.Fatalf("valid multi-page + owner-PW PDF ingest = %d %s", resp.status, resp.body)
	}
	itemID := resp.json()["id"].(string)

	// Verify page_count stored accurately for both plain and owner-PW PDFs.
	var pageCounts []int
	if err := e.db.Select(&pageCounts, `SELECT page_count FROM assets WHERE mime_type = 'application/pdf' ORDER BY page_count`); err != nil {
		t.Fatal(err)
	}
	if len(pageCounts) != 2 || pageCounts[0] != 3 || pageCounts[1] != 3 {
		t.Fatalf("PDF page_counts = %v, want [3 3]", pageCounts)
	}

	// Verify Range & HEAD work for stored PDF asset.
	var pdfHash string
	if err := e.db.Get(&pdfHash, `SELECT asset_hash FROM item_assets WHERE item_id = ? LIMIT 1`, itemID); err != nil {
		t.Fatal(err)
	}
	headReq, _ := http.NewRequest(http.MethodHead, e.srv.URL+"/api/v1/assets/"+pdfHash, nil)
	if hRes := anon.send(headReq); hRes.status != http.StatusOK || hRes.header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("HEAD PDF = %d headers=%v", hRes.status, hRes.header)
	}
	rangeReq, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/api/v1/assets/"+pdfHash, nil)
	rangeReq.Header.Set("Range", "bytes=0-31")
	if rRes := anon.send(rangeReq); rRes.status != http.StatusPartialContent || !bytes.HasPrefix(rRes.body, []byte("%PDF-1.")) {
		t.Fatalf("Range PDF = %d len=%d", rRes.status, len(rRes.body))
	}

	// Verify attachment title is searchable while local filename is not.
	if hit := anon.do("GET", "/api/v1/dirs/"+pubDir+"/search?q=AppendixOriginalPDF", nil); hit.status != http.StatusOK || len(hit.json()["results"].([]any)) != 1 {
		t.Fatalf("search attachment title = %d %s", hit.status, hit.body)
	}
	if miss := anon.do("GET", "/api/v1/dirs/"+pubDir+"/search?q=secretfilenamepdf", nil); miss.status != http.StatusOK || len(miss.json()["results"].([]any)) != 0 {
		t.Fatalf("PDF filename leaked into FTS search: %d %s", miss.status, miss.body)
	}

	// Reject user-password PDF and corrupt PDF with 422 INVALID_ATTACHMENT.
	badDoc := `<?xml version="1.0" encoding="UTF-8"?><document version="1.2">
<meta><title>Bad PDF</title><type>post</type><date>2026-11-23</date><summary>Bad</summary><source-agent>test-agent</source-agent></meta>
<body><attachment id="bad" type="pdf" src="bad.pdf" title="BadPDF"/></body></document>`

	for name, payload := range map[string][]byte{
		"user-password": userLockedPDF,
		"corrupt":       []byte("%PDF-1.4\ncorrupt"),
	} {
		r := admin.ingest(ingestReq{
			xml:    badDoc,
			assets: []figure{{src: "bad.pdf", name: "bad.pdf", data: payload}},
			dirs:   []string{pubDir},
		})
		if r.status != http.StatusUnprocessableEntity || !strings.Contains(string(r.body), "INVALID_ATTACHMENT") {
			t.Fatalf("%s PDF = %d %s, want 422 INVALID_ATTACHMENT", name, r.status, r.body)
		}
	}

	// Reject p=0 and p > page_count with 422 INVALID_TARGET.
	for _, target := range []string{"#att-p:p=0", "#att-p:p=99"} {
		oobXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><document version="1.2">
<meta><title>OOB</title><type>post</type><date>2026-11-23</date><summary>OOB</summary><source-agent>test-agent</source-agent></meta>
<body><attachment id="p" type="pdf" src="p.pdf" title="Paper"/><next-steps><option target="%s">Page</option></next-steps></body></document>`, target)
		r := admin.ingest(ingestReq{
			xml:    oobXML,
			assets: []figure{{src: "p.pdf", name: "p.pdf", data: pdf3}},
			dirs:   []string{pubDir},
		})
		if r.status != http.StatusUnprocessableEntity || !strings.Contains(string(r.body), "INVALID_TARGET") {
			t.Fatalf("page %s = %d %s, want 422 INVALID_TARGET", target, r.status, r.body)
		}
	}
}

func TestV2M2RemoteImagesAndSSRF(t *testing.T) {
	var proxyHits atomic.Int32
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyHits.Add(1)
		http.Error(w, "proxy must never be contacted", http.StatusTeapot)
	}))
	defer proxySrv.Close()

	// Ensure Proxy: nil ignores HTTP(S)_PROXY env vars even when set!
	t.Setenv("HTTP_PROXY", proxySrv.URL)
	t.Setenv("HTTPS_PROXY", proxySrv.URL)

	pngBytes := validTestPNG(t)
	webpBytes := validTestWebP()
	svgBytes := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8"/></svg>`)

	var cdnPort string
	cdnSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		case "/alias.jpg":
			// Common CDN case: .jpg extension serving actual WebP bytes.
			w.Header().Set("Content-Type", "image/webp")
			_, _ = w.Write(webpBytes)
		case "/ok.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write(svgBytes)
		case "/redir-loopback":
			http.Redirect(w, r, "http://127.0.0.1:"+cdnPort+"/ok.png", http.StatusFound)
		case "/redir-rebind":
			http.Redirect(w, r, "http://rebind.example.org/ok.png", http.StatusFound)
		case "/hop/0", "/hop/1", "/hop/2", "/hop/3", "/hop/4", "/hop/5":
			n := int(r.URL.Path[len("/hop/")] - '0')
			http.Redirect(w, r, fmt.Sprintf("http://cdn.example.org/hop/%d", n+1), http.StatusFound)
		case "/hop/6":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		case "/fake-html.png":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!doctype html><html><body>not an image</body></html>"))
		case "/bad-svg.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><unclosed>`))
		case "/dtd-svg.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(`<!DOCTYPE svg [<!ENTITY x "y">]><svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`))
		case "/not-svg-root.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(`<!-- <svg --><html><body>hi</body></html>`))
		case "/signed-redir":
			http.Redirect(w, r, "http://cdn.example.org/check-referer", http.StatusFound)
		case "/check-referer":
			if r.Header.Get("Referer") != "" {
				http.Error(w, "Referer leaked signed source URL: "+r.Header.Get("Referer"), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		default:
			if strings.HasPrefix(r.URL.Path, "/many/") {
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(pngBytes)
				return
			}
			http.NotFound(w, r)
		}
	}))
	defer cdnSrv.Close()
	_, cdnPort, _ = net.SplitHostPort(strings.TrimPrefix(cdnSrv.URL, "http://"))

	t.Run("real transport SSRF blocks non-public IPs and ignores proxy", func(t *testing.T) {
		e := newEnv(t)
		admin := e.admin()
		dir := admin.createDir("", "Public", "")
		for _, target := range []string{
			"http://127.0.0.1:" + cdnPort + "/ok.png",
			"http://[::ffff:127.0.0.1]:" + cdnPort + "/ok.png",
			"http://169.254.169.254/latest/meta-data/",
		} {
			xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><document version="1.2">
<meta><title>SSRF</title><type>post</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta>
<body><figure src="%s" caption="x"/></body></document>`, target)
			res := admin.ingest(ingestReq{xml: xml, dirs: []string{dir}})
			if res.status != http.StatusUnprocessableEntity || !strings.Contains(string(res.body), "REMOTE_ASSET_BLOCKED") {
				t.Fatalf("target %s = %d %s, want 422 REMOTE_ASSET_BLOCKED", target, res.status, res.body)
			}
		}
		if proxyHits.Load() != 0 {
			t.Fatalf("proxyHits = %d, want 0", proxyHits.Load())
		}
	})

	dialHook := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, _ := net.SplitHostPort(addr)
		if strings.EqualFold(host, "cdn.example.org") {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", cdnPort))
		}
		if strings.EqualFold(host, "rebind.example.org") {
			// Simulate DNS rebinding to loopback with real Socket Control active.
			d := &net.Dialer{
				Timeout: 5 * time.Second,
				Control: func(_, a string, _ syscall.RawConn) error {
					return checkRemoteSocketAddr(a)
				},
			}
			return d.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", cdnPort))
		}
		return (&net.Dialer{
			Timeout: 5 * time.Second,
			Control: func(_, a string, _ syscall.RawConn) error { return checkRemoteSocketAddr(a) },
		}).DialContext(ctx, network, addr)
	}

	e := newEnvWithOptions(t, RouterOptions{remoteImageDialContext: dialHook}, nil)
	admin := e.admin()
	dir := admin.createDir("", "Public CDN", "")

	t.Run("localizes PNG, WebP-on-jpg, valid SVG, preserves canonical-url, zero proxy hits", func(t *testing.T) {
		canonicalURL := "http://cdn.example.org/ok.png"
		xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><document version="1.2">
<meta><title>Remote OK</title><type>post</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent><canonical-url>%s</canonical-url></meta>
<body>
  <figure id="f1" src="%s" caption="png"/>
  <figure id="f2" src="http://cdn.example.org/alias.jpg" caption="webp"/>
  <figure id="f3" src="http://cdn.example.org/ok.svg" caption="svg"/>
  <figure id="f4" src="http://cdn.example.org/signed-redir?sig=topsecret" caption="redir"/>
</body></document>`, canonicalURL, canonicalURL)

		resp := admin.ingest(ingestReq{xml: xml, dirs: []string{dir}})
		if resp.status != http.StatusCreated {
			t.Fatalf("remote ok ingest = %d %s", resp.status, resp.body)
		}
		itemID := resp.json()["id"].(string)

		// Stored XML must rewrite <figure src> while preserving <canonical-url>.
		storedResp := e.anon().do("GET", "/api/v1/items/"+itemID+"/content", nil)
		if storedResp.status != http.StatusOK {
			t.Fatalf("get stored xml = %d", storedResp.status)
		}
		storedXML := string(storedResp.body)
		if !strings.Contains(storedXML, "<canonical-url>"+canonicalURL+"</canonical-url>") {
			t.Fatalf("<canonical-url> was modified or dropped:\n%s", storedXML)
		}
		if strings.Contains(storedXML, `src="http://cdn.example.org/`) {
			t.Fatalf("remote <figure src> was not localized:\n%s", storedXML)
		}

		// Verify .jpg returning WebP was stored with .webp extension & MIME image/webp.
		var webpPath string
		if err := e.db.Get(&webpPath, `SELECT storage_path FROM assets WHERE mime_type = 'image/webp'`); err != nil {
			t.Fatalf("webp asset lookup: %v", err)
		}
		if !strings.HasSuffix(webpPath, ".webp") {
			t.Fatalf("webp storage_path = %q, want .webp", webpPath)
		}
		if proxyHits.Load() != 0 {
			t.Fatalf("proxyHits = %d, want 0 (Proxy: nil failed to bypass HTTP_PROXY)", proxyHits.Load())
		}
	})

	t.Run("atomic rejections on SSRF redirects, 6-hop cap, fake HTML, malformed SVG, and >20 URLs", func(t *testing.T) {
		beforeItems := countItems(t, admin)
		cases := []struct {
			name     string
			urls     []string
			wantCode string
		}{
			{"redirect to loopback", []string{"http://cdn.example.org/redir-loopback"}, "REMOTE_ASSET_BLOCKED"},
			{"redirect DNS rebinding to loopback", []string{"http://cdn.example.org/redir-rebind"}, "REMOTE_ASSET_BLOCKED"},
			{"exceed 5 redirects", []string{"http://cdn.example.org/hop/0"}, "REMOTE_ASSET_BLOCKED"},
			{"text/html disguised image", []string{"http://cdn.example.org/fake-html.png"}, "REMOTE_ASSET_FETCH_FAILED"},
			{"malformed SVG XML", []string{"http://cdn.example.org/bad-svg.svg"}, "REMOTE_ASSET_FETCH_FAILED"},
			{"SVG with DOCTYPE", []string{"http://cdn.example.org/dtd-svg.svg"}, "REMOTE_ASSET_FETCH_FAILED"},
			{"non-svg root XML", []string{"http://cdn.example.org/not-svg-root.svg"}, "REMOTE_ASSET_FETCH_FAILED"},
		}
		for _, tc := range cases {
			var figs strings.Builder
			for i, u := range tc.urls {
				fmt.Fprintf(&figs, `<figure id="x%d" src="%s" caption="x"/>`, i, u)
			}
			xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><document version="1.2">
<meta><title>Reject</title><type>post</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta>
<body>%s</body></document>`, figs.String())
			r := admin.ingest(ingestReq{xml: xml, dirs: []string{dir}})
			if r.status != http.StatusUnprocessableEntity || !strings.Contains(string(r.body), tc.wantCode) {
				t.Fatalf("%s = %d %s, want 422 %s", tc.name, r.status, r.body, tc.wantCode)
			}
		}

		// >20 distinct remote URLs rejected cleanly.
		var twentyOne strings.Builder
		for i := range 21 {
			fmt.Fprintf(&twentyOne, `<figure id="m%d" src="http://cdn.example.org/many/%d.png" caption="x"/>`, i, i)
		}
		xml21 := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><document version="1.2">
<meta><title>TooMany</title><type>post</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta>
<body>%s</body></document>`, twentyOne.String())
		if r := admin.ingest(ingestReq{xml: xml21, dirs: []string{dir}}); r.status != http.StatusUnprocessableEntity {
			t.Fatalf(">20 URLs = %d %s, want 422", r.status, r.body)
		}

		if after := countItems(t, admin); after != beforeItems {
			t.Fatalf("failed ingests mutated DB: items %d -> %d", beforeItems, after)
		}
	})

	t.Run("AllowRemoteImages=false blocks remote figures", func(t *testing.T) {
		dis := false
		eNo := newEnvWithOptions(t, RouterOptions{AllowRemoteImages: &dis, remoteImageDialContext: dialHook}, nil)
		adNo := eNo.admin()
		dNo := adNo.createDir("", "D", "")
		xml := `<?xml version="1.0" encoding="UTF-8"?><document version="1.2">
<meta><title>Off</title><type>post</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta>
<body><figure src="http://cdn.example.org/ok.png" caption="png"/></body></document>`
		r := adNo.ingest(ingestReq{xml: xml, dirs: []string{dNo}})
		if r.status != http.StatusUnprocessableEntity || !strings.Contains(string(r.body), "REMOTE_ASSET_BLOCKED") {
			t.Fatalf("disabled remote = %d %s, want 422 REMOTE_ASSET_BLOCKED", r.status, r.body)
		}
	})
}

func TestV2M2MCPServerSecurityAndTools(t *testing.T) {
	e := newEnvWithOptions(t, RouterOptions{PublicURL: "https://hub.example.org"}, nil)
	admin := e.admin()
	ag := e.anon()

	pubP := admin.createDir("", "Public P", "")
	pubQ := admin.createDir("", "Public Q", "")
	lockedUnderPResp := admin.do("POST", "/api/v1/admin/dirs", map[string]any{
		"name": "Locked Child Under P", "description": "TOP SECRET DESC", "parent_id": pubP, "password": "pw-p-child",
	})
	lockedUnderP := lockedUnderPResp.json()["id"].(string)

	protD := admin.createDir("", "Protected D", "pw-d")
	protDSub := admin.createDir(protD, "Open Child Of D", "")
	protC := admin.createDir(protD, "Locked Grandchild C", "pw-c")

	tokDResp := admin.do("POST", "/api/v1/admin/dirs/"+protD+"/tokens", map[string]string{"label": "mcp-d"})
	tokDID := tokDResp.json()["id"].(string)
	tokD := tokDResp.json()["token"].(string)

	// Items for visibility, multi-mount, B1 history, and ownership assertions.
	pubID := ag.ingestOK(ingestReq{
		xml:  xmlDoc(docOpts{title: "Public ItemAlpha", md: "public uniqueomega body"}),
		dirs: []string{pubP},
	})
	dID := ag.ingestOK(ingestReq{
		token: "other",
		xml:   xmlDoc(docOpts{title: "Protected D ItemBeta", md: "d uniqueomega secret"}),
		dirs:  []string{protDSub},
	})
	cID := ag.ingestOK(ingestReq{
		token: "other",
		xml:   xmlDoc(docOpts{title: "Locked C ItemGamma", md: "c uniqueomega topsecret"}),
		dirs:  []string{protC},
	})
	multiSecretID := ag.ingestOK(ingestReq{
		xml:  xmlDoc(docOpts{title: "Multi Mounted Item", md: "multi uniqueomega"}),
		dirs: []string{pubP, protC},
	})
	// Cross-mount item in pubQ + lockedUnderP: MUST NOT match dir_id=pubP for anon!
	crossMountID := ag.ingestOK(ingestReq{
		xml:  xmlDoc(docOpts{title: "Cross Locked Mount", md: "crosslock uniqueomega"}),
		dirs: []string{pubQ, lockedUnderP},
	})
	// Draft created by primary apiToken vs draft/protected item created by "other" token.
	ownDraftID := ag.ingestOK(ingestReq{
		xml:   xmlDoc(docOpts{title: "My API Draft", md: "owndraft uniqueomega"}),
		draft: true,
	})
	otherDraftID := ag.ingestOK(ingestReq{
		token: "other",
		xml:   xmlDoc(docOpts{title: "Other API Draft", md: "otherdraft uniqueomega"}),
		draft: true,
	})
	otherProtID := ag.ingestOK(ingestReq{
		token: "other",
		xml:   xmlDoc(docOpts{title: "Other Token Protected", md: "otherprot uniqueomega"}),
		dirs:  []string{protC},
	})

	// B1 historical scenario owned by "other": v1 published in protC -> unpublish -> v2 sanitized published in pubQ.
	b1Key := "mcp:b1:history"
	b1ID := ag.ingestOK(ingestReq{
		token: "other",
		key:   b1Key,
		xml:   xmlDoc(docOpts{title: "B1 Secret V1", md: "classified v1 line"}),
		dirs:  []string{protC},
	})
	nonOwnerUnpub, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/api/v1/agent/items/"+b1ID+"/mounts", nil)
	nonOwnerUnpub.Header.Set("Authorization", "Bearer "+apiToken)
	if st := ag.send(nonOwnerUnpub); st.status != http.StatusForbidden {
		t.Fatalf("expected non-owner unpublish 403, got %d", st.status)
	}
	unpubReq, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/api/v1/agent/items/"+b1ID+"/mounts", nil)
	unpubReq.Header.Set("Authorization", "Bearer other")
	if r := ag.send(unpubReq); r.status != http.StatusNoContent {
		t.Fatalf("other unpublish = %d %s", r.status, r.body)
	}
	_ = ag.ingestOK(ingestReq{
		token: "other",
		key:   b1Key,
		xml:   xmlDoc(docOpts{title: "B1 Public V2", md: "sanitized public v2"}),
		dirs:  []string{pubQ},
	})
	// Even clearing protC's password must NOT expose rev=1 to anon/apiToken!
	_ = admin.do("PATCH", "/api/v1/admin/dirs/"+protC, map[string]any{"password": ""})
	// Restore protC password for remainder of tree checks (note B1 rev=1 stays permanently restricted via was_protected=1).
	_ = admin.do("PATCH", "/api/v1/admin/dirs/"+protC, map[string]any{"password": "pw-c"})

	validHdr := http.Header{
		"Origin": []string{"https://hub.example.org"},
	}
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

	t.Run("HTTP Host, Origin, Bearer 401, 16MiB 413, and Session Hijack 403", func(t *testing.T) {
		sendMCP := func(host, origin, authHdr, sid, body string) *http.Response {
			req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/mcp", strings.NewReader(body))
			if host != "" {
				req.Host = host
			}
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			if authHdr != "" {
				req.Header.Set("Authorization", authHdr)
			}
			if sid != "" {
				req.Header.Set(mcpSessionHeader, sid)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			return resp
		}

		// Bad Host vs PublicURL -> 403.
		r := sendMCP("evil.example", "https://hub.example.org", "", "", initBody)
		_ = r.Body.Close()
		if r.StatusCode != http.StatusForbidden {
			t.Fatalf("bad Host = %d, want 403", r.StatusCode)
		}

		// Bad Origin / Origin: null -> 403.
		for _, badOrigin := range []string{"https://evil.example", "null"} {
			r = sendMCP("hub.example.org", badOrigin, "", "", initBody)
			_ = r.Body.Close()
			if r.StatusCode != http.StatusForbidden {
				t.Fatalf("Origin %q = %d, want 403", badOrigin, r.StatusCode)
			}
		}

		// Invalid Bearer token -> 401.
		r = sendMCP("hub.example.org", "https://hub.example.org", "Bearer bogus_tok", "", initBody)
		_ = r.Body.Close()
		if r.StatusCode != http.StatusUnauthorized {
			t.Fatalf("bad bearer = %d, want 401", r.StatusCode)
		}

		// > 16 MiB body -> 413.
		oversized := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"validate_xml","arguments":{"xml":"` +
			strings.Repeat("a", (16<<20)+1024) + `"}}}`
		r = sendMCP("hub.example.org", "https://hub.example.org", "Bearer "+apiToken, "", oversized)
		_ = r.Body.Close()
		if r.StatusCode != http.StatusRequestEntityTooLarge {
			t.Fatalf("oversized >16MiB = %d, want 413", r.StatusCode)
		}

		// Cross-principal Mcp-Session-Id hijack -> 403.
		r = sendMCP("hub.example.org", "https://hub.example.org", "Bearer "+apiToken, "", initBody)
		sid := r.Header.Get(mcpSessionHeader)
		_ = r.Body.Close()
		if r.StatusCode != http.StatusOK || sid == "" {
			t.Fatalf("valid init = %d sid=%q", r.StatusCode, sid)
		}
		hijack := sendMCP("hub.example.org", "https://hub.example.org", "Bearer other", sid, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
		_ = hijack.Body.Close()
		if hijack.StatusCode != http.StatusForbidden {
			t.Fatalf("cross-token session reuse = %d, want 403", hijack.StatusCode)
		}
		anonHijack := sendMCP("hub.example.org", "https://hub.example.org", "", sid, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
		_ = anonHijack.Body.Close()
		if anonHijack.StatusCode != http.StatusForbidden {
			t.Fatalf("anon session reuse = %d, want 403", anonHijack.StatusCode)
		}
	})

	// Configure valid Host via roundtripper for go-sdk Client calls.
	baseTransport := http.DefaultTransport
	mcpWithHeaders := func(extra map[string]string) *mcp.ClientSession {
		hdr := validHdr.Clone()
		for k, v := range extra {
			hdr.Set(k, v)
		}
		// Ensure Host header matches PublicURL.
		c := mcp.NewClient(&mcp.Implementation{Name: "sdk-client", Version: "1.0"}, nil)
		rt := &hostInjectRT{base: baseTransport, host: "hub.example.org", headers: hdr}
		tr := &mcp.StreamableClientTransport{
			Endpoint:             e.srv.URL + "/mcp",
			HTTPClient:           &http.Client{Transport: rt},
			DisableStandaloneSSE: true,
		}
		sess, err := c.Connect(t.Context(), tr, nil)
		if err != nil {
			t.Fatalf("mcp Connect: %v", err)
		}
		t.Cleanup(func() { _ = sess.Close() })
		return sess
	}

	t.Run("anonymous identity: public only, locked child hides desc, B1 enforced, push_item denied", func(t *testing.T) {
		sess := mcpWithHeaders(nil)

		// list_dirs hides locked child description.
		dirsRes, rawDirs, isErr := callMCPJSON(t, sess, "list_dirs", map[string]any{})
		if isErr {
			t.Fatalf("anon list_dirs failed: %s", rawDirs)
		}
		if strings.Contains(rawDirs, "TOP SECRET DESC") {
			t.Fatalf("locked child description leaked in list_dirs: %s", rawDirs)
		}
		var sawLockedChild bool
		for _, rawD := range dirsRes["directories"].([]any) {
			d := rawD.(map[string]any)
			if d["id"] == lockedUnderP {
				sawLockedChild = true
				if d["locked"] != true || d["description"] != "" {
					t.Fatalf("locked child metadata = %v", d)
				}
			}
			if d["id"] == protD || d["id"] == protC {
				t.Fatalf("anon list_dirs exposed protected root subtree %v", d)
			}
		}
		if !sawLockedChild {
			t.Fatalf("expected locked direct child %s under pubP in %s", lockedUnderP, rawDirs)
		}

		// Unauthorized parent_id -> IsError forbidden.
		if _, msg, toolErr := callMCPJSON(t, sess, "list_dirs", map[string]any{"parent_id": protD}); !toolErr || !strings.Contains(msg, "forbidden") {
			t.Fatalf("anon list_dirs(protD) = err=%v msg=%q, want forbidden", toolErr, msg)
		}

		// list_items + search_items: zero xml/plain_text, filtered dirs, and NO cross-mount leak under dir_id=pubP!
		listRes, rawList, isErr := callMCPJSON(t, sess, "list_items", map[string]any{"dir_id": pubP})
		if isErr || strings.Contains(rawList, `"xml"`) || strings.Contains(rawList, `"plain_text"`) {
			t.Fatalf("anon list_items unexpected isErr=%v body=%s", isErr, rawList)
		}
		for _, rawIt := range listRes["items"].([]any) {
			it := rawIt.(map[string]any)
			if it["id"] == crossMountID {
				t.Fatalf("cross-mounted item %s in lockedUnderP leaked under dir_id=pubP: %s", crossMountID, rawList)
			}
			if it["id"] == multiSecretID {
				dirs := fmt.Sprint(it["dirs"])
				if strings.Contains(dirs, protC) {
					t.Fatalf("multi-mounted item exposed protected dir %s: %s", protC, dirs)
				}
			}
		}

		searchRes, rawSearch, isErr := callMCPJSON(t, sess, "search_items", map[string]any{"query": "uniqueomega", "dir_id": pubP})
		if isErr || strings.Contains(rawSearch, `"xml"`) || strings.Contains(rawSearch, `"plain_text"`) {
			t.Fatalf("anon search_items unexpected isErr=%v body=%s", isErr, rawSearch)
		}
		for _, rawR := range searchRes["results"].([]any) {
			if rawR.(map[string]any)["id"] == crossMountID {
				t.Fatalf("search_items(dir_id=pubP) leaked cross-mounted item %s", crossMountID)
			}
		}

		// get_item text + xml + B1 historical rev=1 denied even though protC was cleared briefly.
		if gotXML, _, getErr := callMCPJSON(t, sess, "get_item", map[string]any{"id": pubID, "format": "xml"}); getErr || !strings.Contains(gotXML["content"].(string), "<document") {
			t.Fatalf("anon get_item xml = err=%v %v", getErr, gotXML)
		}
		if gotTxt, _, getErr := callMCPJSON(t, sess, "get_item", map[string]any{"id": b1ID, "format": "text"}); getErr || !strings.Contains(gotTxt["content"].(string), "sanitized public v2") {
			t.Fatalf("anon get_item v2 text = err=%v %v", getErr, gotTxt)
		}
		if _, revMsg, revErr := callMCPJSON(t, sess, "get_item", map[string]any{"id": b1ID, "rev": 1}); !revErr || !strings.Contains(revMsg, "forbidden") {
			t.Fatalf("B1 violated via MCP get_item rev=1: err=%v msg=%q", revErr, revMsg)
		}

		// Anon push_item -> IsError unauthorized.
		if _, pushMsg, pushErr := callMCPJSON(t, sess, "push_item", map[string]any{"xml": xmlDoc(docOpts{title: "No"})}); !pushErr || !strings.Contains(pushMsg, "unauthorized") {
			t.Fatalf("anon push_item = err=%v msg=%q", pushErr, pushMsg)
		}
	})

	t.Run("API token: own draft isolation, dir_id intersection, X-Aumbry-Dir-Tokens, validate_xml, >4MiB push_item", func(t *testing.T) {
		sess := mcpWithHeaders(map[string]string{
			"Authorization":       "Bearer " + apiToken,
			"X-Aumbry-Dir-Tokens": tokD,
		})

		// list_items without dir_id includes ownDraftID + protDSub (via X-Aumbry-Dir-Tokens), excludes protC & otherProtID!
		allRes, rawAll, isErr := callMCPJSON(t, sess, "list_items", map[string]any{"limit": 100})
		if isErr {
			t.Fatalf("api list_items = %s", rawAll)
		}
		seen := map[string]bool{}
		for _, rawIt := range allRes["items"].([]any) {
			seen[rawIt.(map[string]any)["id"].(string)] = true
		}
		if !seen[ownDraftID] || !seen[dID] || !seen[pubID] {
			t.Fatalf("api token missing expected visible items in %v", seen)
		}
		if seen[cID] || seen[otherProtID] || seen[otherDraftID] {
			t.Fatalf("api token saw unauthorized protected/other items in %v", seen)
		}

		// Passing dir_id=pubP intersects visibility -> ownDraftID MUST NOT bypass dir_id!
		subRes, rawSub, isErr := callMCPJSON(t, sess, "list_items", map[string]any{"dir_id": pubP})
		if isErr {
			t.Fatalf("api list_items(dir_id=pubP) = %s", rawSub)
		}
		for _, rawIt := range subRes["items"].([]any) {
			id := rawIt.(map[string]any)["id"].(string)
			if id == ownDraftID || id == dID || id == crossMountID {
				t.Fatalf("api list_items(dir_id=pubP) leaked %s: %s", id, rawSub)
			}
		}

		// search_items(dir_id=pubP) also must not bypass with ownDraftID!
		sRes, rawSearch, isErr := callMCPJSON(t, sess, "search_items", map[string]any{"query": "uniqueomega", "dir_id": pubP})
		if isErr {
			t.Fatalf("api search_items(dir_id=pubP) = %s", rawSearch)
		}
		for _, rawR := range sRes["results"].([]any) {
			id := rawR.(map[string]any)["id"].(string)
			if id == ownDraftID || id == dID || id == crossMountID {
				t.Fatalf("api search_items(dir_id=pubP) leaked %s", id)
			}
		}

		// validate_xml: valid vs invalid.
		vOK, rawVOK, isErr := callMCPJSON(t, sess, "validate_xml", map[string]any{"xml": xmlDoc(docOpts{title: "OK"})})
		if isErr || vOK["valid"] != true {
			t.Fatalf("validate_xml valid = err=%v %s", isErr, rawVOK)
		}
		vBad, rawVBad, isErr := callMCPJSON(t, sess, "validate_xml", map[string]any{"xml": "<document version=\"1.2\"><meta/></document>"})
		if isErr || vBad["valid"] != false {
			t.Fatalf("validate_xml invalid = err=%v %s", isErr, rawVBad)
		}

		// push_item rejects local relative assets (<figure src>, <attachment src>).
		if _, pMsg, pErr := callMCPJSON(t, sess, "push_item", map[string]any{
			"xml":  xmlDoc(docOpts{title: "Local", figures: []string{"./imgs/a.png"}}),
			"dirs": []string{pubP},
		}); !pErr || !strings.Contains(pMsg, "invalid_request") {
			t.Fatalf("push_item local asset = err=%v msg=%q, want invalid_request", pErr, pMsg)
		}

		// push_item succeeds on > 4 MiB (~5 MiB <= 10 MiB) XML!
		largeMD := strings.Repeat("A", 5<<20)
		largeXML := xmlDoc(docOpts{title: "FiveMiBDoc", md: largeMD})
		pushed, rawPush, pErr := callMCPJSON(t, sess, "push_item", map[string]any{
			"xml":  largeXML,
			"dirs": []string{pubP},
		})
		if pErr || pushed["published"] != true || pushed["id"] == "" {
			t.Fatalf("push_item 5 MiB XML = err=%v raw=%s", pErr, rawPush)
		}

		// Another token "other" cannot read ownDraftID, and CAN read its own B1 rev=1.
		otherSess := mcpWithHeaders(map[string]string{"Authorization": "Bearer other"})
		if _, _, getErr := callMCPJSON(t, otherSess, "get_item", map[string]any{"id": ownDraftID}); !getErr {
			t.Fatal("other API token must not read first token's draft")
		}
		if b1Rev1, _, getErr := callMCPJSON(t, otherSess, "get_item", map[string]any{"id": b1ID, "rev": 1, "format": "text"}); getErr || !strings.Contains(b1Rev1["content"].(string), "classified v1 line") {
			t.Fatalf("owner other failed to read own B1 rev=1: err=%v %v", getErr, b1Rev1)
		}
	})

	t.Run("ad_ token enforces A1 subtree, denies push_item, and revokes live mid-session", func(t *testing.T) {
		sess := mcpWithHeaders(map[string]string{"Authorization": "Bearer " + tokD})

		// Can see D and Open Child, NOT Locked C.
		res, rawList, isErr := callMCPJSON(t, sess, "list_items", map[string]any{})
		if isErr {
			t.Fatalf("ad_D list_items = %s", rawList)
		}
		seen := map[string]bool{}
		for _, rawIt := range res["items"].([]any) {
			seen[rawIt.(map[string]any)["id"].(string)] = true
		}
		if !seen[dID] || seen[cID] {
			t.Fatalf("ad_D A1 violation: seen=%v", seen)
		}

		// push_item returns forbidden tool error.
		if _, pushMsg, pushErr := callMCPJSON(t, sess, "push_item", map[string]any{"xml": xmlDoc(docOpts{title: "No"})}); !pushErr || !strings.Contains(pushMsg, "forbidden") {
			t.Fatalf("ad_D push_item = err=%v msg=%q, want forbidden", pushErr, pushMsg)
		}

		// Delete ad_D token via admin API while MCP session stays open -> very next CallTool fails immediately!
		if st := admin.do("DELETE", "/api/v1/admin/dirs/"+protD+"/tokens/"+tokDID, nil).status; st != http.StatusNoContent {
			t.Fatalf("revoke tokD = %d", st)
		}
		if _, nextErr := sess.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_dirs", Arguments: map[string]any{}}); nextErr == nil {
			t.Fatal("revoked ad_D token still worked inside active MCP session")
		}
	})
}

type hostInjectRT struct {
	base    http.RoundTripper
	host    string
	headers http.Header
}

func (rt *hostInjectRT) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	if rt.host != "" {
		clone.Host = rt.host
	}
	for k, vals := range rt.headers {
		clone.Header.Del(k)
		for _, v := range vals {
			clone.Header.Add(k, v)
		}
	}
	return rt.base.RoundTrip(clone)
}

func TestV2M2ReviewRegressions(t *testing.T) {
	const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

	t.Run("H1 per-revision creator B1 boundary on MCP get_item and revision_count", func(t *testing.T) {
		e := newEnv(t)
		admin := e.admin()
		ag := e.anon()
		pubQ := admin.createDir("", "Public Q", "")
		protC := admin.createDir("", "Protected C", "pw-c")

		// Token A creates item (rev=1 in pubQ); admin moves mount to protC so Token B's rev=2 is recorded strictly under protC.
		itemID := ag.ingestOK(ingestReq{
			key:  "h1:shared:item",
			xml:  xmlDoc(docOpts{title: "H1 Rev1", md: "token A initial public"}),
			dirs: []string{pubQ},
		})
		if st := admin.do("PUT", "/api/v1/admin/items/"+itemID+"/mounts", map[string]any{"dirs": []string{protC}}).status; st != http.StatusNoContent {
			t.Fatalf("admin move mount to protC = %d", st)
		}
		_ = ag.ingestOK(ingestReq{
			token: "other",
			key:   "h1:shared:item",
			xml:   xmlDoc(docOpts{title: "H1 Rev2 Secret", md: "token B private draft in protC"}),
			dirs:  []string{protC},
		})
		// Clear mounts (or via creator unpublish), then Token B publishes sanitized rev=3 to pubQ.
		if st := admin.do("PUT", "/api/v1/admin/items/"+itemID+"/mounts", map[string]any{"dirs": []string{}}).status; st != http.StatusNoContent {
			t.Fatalf("admin clear mounts = %d", st)
		}
		_ = ag.ingestOK(ingestReq{
			token: "other",
			key:   "h1:shared:item",
			xml:   xmlDoc(docOpts{title: "H1 Rev3 Public", md: "token B sanitized public"}),
			dirs:  []string{pubQ},
		})

		connectToken := func(tok string) *mcp.ClientSession {
			c := mcp.NewClient(&mcp.Implementation{Name: "h1", Version: "1.0"}, nil)
			tr := &mcp.StreamableClientTransport{
				Endpoint: e.srv.URL + "/mcp",
				HTTPClient: &http.Client{
					Transport: &hostInjectRT{
						base:    http.DefaultTransport,
						headers: http.Header{"Authorization": []string{"Bearer " + tok}},
					},
				},
				DisableStandaloneSSE: true,
			}
			sess, err := c.Connect(t.Context(), tr, nil)
			if err != nil {
				t.Fatalf("connect(%s): %v", tok, err)
			}
			t.Cleanup(func() { _ = sess.Close() })
			return sess
		}

		sessA := connectToken(apiToken)
		sessB := connectToken("other")

		// Token A is item creator, NOT creator of rev=2 -> cannot see rev=2 in count or get_item!
		listA, rawA, isErr := callMCPJSON(t, sessA, "list_items", map[string]any{"key": "h1:shared:item"})
		if isErr {
			t.Fatalf("sessA list_items = %s", rawA)
		}
		if gotCnt := int(listA["items"].([]any)[0].(map[string]any)["revision_count"].(float64)); gotCnt != 2 {
			t.Fatalf("Token A revision_count = %d, want 2 (excluding B's private rev=2)", gotCnt)
		}
		if _, rev2Msg, rev2Err := callMCPJSON(t, sessA, "get_item", map[string]any{"id": itemID, "rev": 2}); !rev2Err || !strings.Contains(rev2Msg, "forbidden") {
			t.Fatalf("Token A read B's private rev=2 = err=%v msg=%q, want forbidden", rev2Err, rev2Msg)
		}
		if rev1Out, _, rev1Err := callMCPJSON(t, sessA, "get_item", map[string]any{"id": itemID, "rev": 1, "format": "text"}); rev1Err || !strings.Contains(rev1Out["content"].(string), "token A initial public") {
			t.Fatalf("Token A read own rev=1 failed: err=%v %v", rev1Err, rev1Out)
		}

		// Token B is creator of rev=2 -> sees all 3 revisions and can read rev=2.
		listB, rawB, isErr := callMCPJSON(t, sessB, "list_items", map[string]any{"key": "h1:shared:item"})
		if isErr {
			t.Fatalf("sessB list_items = %s", rawB)
		}
		if gotCnt := int(listB["items"].([]any)[0].(map[string]any)["revision_count"].(float64)); gotCnt != 3 {
			t.Fatalf("Token B revision_count = %d, want 3", gotCnt)
		}
		if rev2Out, _, rev2Err := callMCPJSON(t, sessB, "get_item", map[string]any{"id": itemID, "rev": 2, "format": "text"}); rev2Err || !strings.Contains(rev2Out["content"].(string), "token B private draft") {
			t.Fatalf("Token B read own rev=2 failed: err=%v %v", rev2Err, rev2Out)
		}
	})

	t.Run("H2 + M1 unknown sid 404, atomic anon session cap 16, concurrent anon POST cap 4, and IPv6 /64 isolation", func(t *testing.T) {
		e := newEnv(t)

		// Unknown Mcp-Session-Id -> 404 instead of forwarding to SDK.
		badSIDReq, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
		badSIDReq.Header.Set("Content-Type", "application/json")
		badSIDReq.Header.Set("Accept", "application/json, text/event-stream")
		badSIDReq.Header.Set(mcpSessionHeader, "unknown-session-id")
		badSIDResp, err := http.DefaultClient.Do(badSIDReq)
		if err != nil {
			t.Fatal(err)
		}
		_ = badSIDResp.Body.Close()
		if badSIDResp.StatusCode != http.StatusNotFound {
			t.Fatalf("unknown sid = %d, want 404", badSIDResp.StatusCode)
		}

		// Concurrent anonymous init burst (24 parallel requests) never exceeds 16 sessions.
		var okCount, rejectCount atomic.Int32
		var firstSID atomic.Value
		var wg sync.WaitGroup
		for range 24 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/mcp", strings.NewReader(initBody))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json, text/event-stream")
				resp, rErr := http.DefaultClient.Do(req)
				if rErr != nil {
					t.Errorf("concurrent init: %v", rErr)
					return
				}
				_ = resp.Body.Close()
				switch resp.StatusCode {
				case http.StatusOK:
					okCount.Add(1)
					if sid := resp.Header.Get(mcpSessionHeader); sid != "" {
						firstSID.CompareAndSwap(nil, sid)
					}
				case http.StatusTooManyRequests:
					rejectCount.Add(1)
				default:
					t.Errorf("unexpected concurrent init status: %d", resp.StatusCode)
				}
			}()
		}
		wg.Wait()
		if okCount.Load() > 16 || okCount.Load()+rejectCount.Load() != 24 || rejectCount.Load() == 0 {
			t.Fatalf("concurrent init cap failed: ok=%d reject=%d", okCount.Load(), rejectCount.Load())
		}

		// Concurrent anonymous POST body reads on an existing session cap at 4 in flight per IP.
		sid, _ := firstSID.Load().(string)
		if sid == "" {
			t.Fatal("missing initial anonymous session id")
		}
		pipes := make([]*io.PipeWriter, 4)
		var postWG sync.WaitGroup
		entered := make(chan struct{}, 4)
		for i := range 4 {
			pr, pw := io.Pipe()
			pipes[i] = pw
			postWG.Add(1)
			go func(r io.Reader) {
				defer postWG.Done()
				req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/mcp", r)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json, text/event-stream")
				req.Header.Set(mcpSessionHeader, sid)
				entered <- struct{}{}
				resp, pErr := http.DefaultClient.Do(req)
				if pErr == nil {
					_ = resp.Body.Close()
				}
			}(pr)
			_, _ = pw.Write([]byte(`{"jsonrpc":"2.0",`))
		}
		for range 4 {
			<-entered
		}
		// Give handlers a moment to acquire their in-flight slot cleanly.
		time.Sleep(50 * time.Millisecond)

		req5, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"tools/list"}`))
		req5.Header.Set("Content-Type", "application/json")
		req5.Header.Set("Accept", "application/json, text/event-stream")
		req5.Header.Set(mcpSessionHeader, sid)
		resp5, err5 := http.DefaultClient.Do(req5)
		if err5 != nil {
			t.Fatal(err5)
		}
		_ = resp5.Body.Close()
		for _, pw := range pipes {
			_ = pw.Close()
		}
		postWG.Wait()
		if resp5.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("5th concurrent anon POST = %d, want 429", resp5.StatusCode)
		}

		// Two distinct IPv6 addresses sharing a /64 must NOT share anonymous session ownership.
		eV6 := newEnv(t)
		initV6 := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", strings.NewReader(initBody))
		initV6.Host = "localhost"
		initV6.RemoteAddr = "[2001:db8::1]:12345"
		initV6.Header.Set("Content-Type", "application/json")
		initV6.Header.Set("Accept", "application/json, text/event-stream")
		recV6 := httptest.NewRecorder()
		eV6.router.ServeHTTP(recV6, initV6)
		v6SID := recV6.Header().Get(mcpSessionHeader)
		if recV6.Code != http.StatusOK || v6SID == "" {
			t.Fatalf("IPv6 init = %d sid=%q", recV6.Code, v6SID)
		}

		peerV6 := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
		peerV6.Host = "localhost"
		peerV6.RemoteAddr = "[2001:db8::2]:12345"
		peerV6.Header.Set("Content-Type", "application/json")
		peerV6.Header.Set("Accept", "application/json, text/event-stream")
		peerV6.Header.Set(mcpSessionHeader, v6SID)
		recPeer := httptest.NewRecorder()
		eV6.router.ServeHTTP(recPeer, peerV6)
		if recPeer.Code != http.StatusForbidden {
			t.Fatalf("same-/64 sibling IPv6 session reuse = %d, want 403", recPeer.Code)
		}
	})

	t.Run("H3 + M2 fail-closed Host when public_url empty and port preservation", func(t *testing.T) {
		eNoPub := newEnv(t)
		serveLocalAddr := func(host, origin string) int {
			req := httptest.NewRequest(http.MethodPost, "http://"+host+"/mcp", strings.NewReader(initBody))
			req.Host = host
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			ctx := context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 8080})
			rec := httptest.NewRecorder()
			eNoPub.router.ServeHTTP(rec, req.WithContext(ctx))
			return rec.Code
		}
		if st := serveLocalAddr("evil.example:8080", "http://evil.example:8080"); st != http.StatusForbidden {
			t.Fatalf("non-loopback DNS rebind without public_url = %d, want 403", st)
		}
		if st := serveLocalAddr("10.0.0.5:8080", "http://10.0.0.5:8080"); st != http.StatusForbidden {
			t.Fatalf("LAN IP host without public_url = %d, want 403", st)
		}
		if st := serveLocalAddr("localhost:8080", "http://localhost:8080"); st != http.StatusOK {
			t.Fatalf("loopback localhost with empty public_url = %d, want 200", st)
		}

		ePort := newEnvWithOptions(t, RouterOptions{PublicURL: "https://hub.example.org:8443"}, nil)
		checkPortHost := func(host, origin string) int {
			req := httptest.NewRequest(http.MethodPost, "https://"+host+"/mcp", strings.NewReader(initBody))
			req.Host = host
			req.Header.Set("Origin", origin)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			rec := httptest.NewRecorder()
			ePort.router.ServeHTTP(rec, req)
			return rec.Code
		}
		if st := checkPortHost("hub.example.org:8443", "https://hub.example.org:8443"); st != http.StatusOK {
			t.Fatalf("matching non-standard port = %d, want 200", st)
		}
		if st := checkPortHost("hub.example.org", "https://hub.example.org:8443"); st != http.StatusForbidden {
			t.Fatalf("port-stripped Host = %d, want 403", st)
		}
	})

	t.Run("L2 configured API token starting with ad_ works on /mcp", func(t *testing.T) {
		const adAPIToken = "ad_configured_api_token_123456"
		database, err := db.Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = database.Close() })
		pwHash, _ := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.MinCost)
		sessions, err := auth.New(auth.Settings{
			AdminUser:         adminUser,
			AdminPasswordHash: string(pwHash),
			APITokens:         []string{adAPIToken},
			CookieSecret:      cookieSecret,
		})
		if err != nil {
			t.Fatal(err)
		}
		store := &memStore{files: map[string][]byte{}}
		router, err := NewRouter(database, store, sessions, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(router)
		t.Cleanup(srv.Close)

		c := mcp.NewClient(&mcp.Implementation{Name: "ad-api", Version: "1.0"}, nil)
		tr := &mcp.StreamableClientTransport{
			Endpoint: srv.URL + "/mcp",
			HTTPClient: &http.Client{
				Transport: &hostInjectRT{
					base:    http.DefaultTransport,
					headers: http.Header{"Authorization": []string{"Bearer " + adAPIToken}},
				},
			},
			DisableStandaloneSSE: true,
		}
		sess, err := c.Connect(t.Context(), tr, nil)
		if err != nil {
			t.Fatalf("ad_-prefixed API token Connect failed: %v", err)
		}
		t.Cleanup(func() { _ = sess.Close() })

		out, raw, isErr := callMCPJSON(t, sess, "push_item", map[string]any{
			"xml":   xmlDoc(docOpts{title: "AD Prefixed API Token Push"}),
			"draft": true,
		})
		if isErr || out["id"] == "" {
			t.Fatalf("push_item with ad_-prefixed API token = err=%v raw=%s", isErr, raw)
		}
	})

	t.Run("R2-H1 admin mount draft attributes revision to updated_by and fails closed on NULL", func(t *testing.T) {
		e := newEnv(t)
		admin := e.admin()
		ag := e.anon()
		pubQ := admin.createDir("", "Public Q", "")
		protC := admin.createDir("", "Protected C", "pw-c")

		// Token A creates item and unpublishes it.
		itemID := ag.ingestOK(ingestReq{
			key:  "r2:h1:draftmount",
			xml:  xmlDoc(docOpts{title: "DraftMount V1", md: "token A initial"}),
			dirs: []string{pubQ},
		})
		unpubReq, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/api/v1/agent/items/"+itemID+"/mounts", nil)
		unpubReq.Header.Set("Authorization", "Bearer "+apiToken)
		if r := ag.send(unpubReq); r.status != http.StatusNoContent {
			t.Fatalf("unpublish = %d %s", r.status, r.body)
		}

		// Token B ("other") overwrites with --draft, then admin mounts that draft into protC.
		_ = ag.ingestOK(ingestReq{
			token: "other",
			key:   "r2:h1:draftmount",
			xml:   xmlDoc(docOpts{title: "DraftMount V2 Secret", md: "token B confidential draft"}),
			draft: true,
		})
		if st := admin.do("PUT", "/api/v1/admin/items/"+itemID+"/mounts", map[string]any{"dirs": []string{protC}}).status; st != http.StatusNoContent {
			t.Fatalf("admin mount draft into protC = %d", st)
		}
		// Admin clears mounts and Token B publishes sanitized v3 to pubQ.
		if st := admin.do("PUT", "/api/v1/admin/items/"+itemID+"/mounts", map[string]any{"dirs": []string{}}).status; st != http.StatusNoContent {
			t.Fatalf("admin clear mounts = %d", st)
		}
		_ = ag.ingestOK(ingestReq{
			token: "other",
			key:   "r2:h1:draftmount",
			xml:   xmlDoc(docOpts{title: "DraftMount V3 Public", md: "token B sanitized public"}),
			dirs:  []string{pubQ},
		})

		connectToken := func(tok string) *mcp.ClientSession {
			c := mcp.NewClient(&mcp.Implementation{Name: "r2-h1", Version: "1.0"}, nil)
			tr := &mcp.StreamableClientTransport{
				Endpoint: e.srv.URL + "/mcp",
				HTTPClient: &http.Client{
					Transport: &hostInjectRT{
						base:    http.DefaultTransport,
						headers: http.Header{"Authorization": []string{"Bearer " + tok}},
					},
				},
				DisableStandaloneSSE: true,
			}
			sess, err := c.Connect(t.Context(), tr, nil)
			if err != nil {
				t.Fatalf("connect(%s): %v", tok, err)
			}
			t.Cleanup(func() { _ = sess.Close() })
			return sess
		}

		sessA := connectToken(apiToken)
		sessB := connectToken("other")

		listA, rawA, isErr := callMCPJSON(t, sessA, "list_items", map[string]any{"key": "r2:h1:draftmount"})
		if isErr {
			t.Fatalf("sessA list_items = %s", rawA)
		}
		if gotCnt := int(listA["items"].([]any)[0].(map[string]any)["revision_count"].(float64)); gotCnt != 2 {
			t.Fatalf("Token A revision_count = %d, want 2 (excluding B's draft-mounted rev=2)", gotCnt)
		}
		if _, rev2Msg, rev2Err := callMCPJSON(t, sessA, "get_item", map[string]any{"id": itemID, "rev": 2}); !rev2Err || !strings.Contains(rev2Msg, "forbidden") {
			t.Fatalf("Token A read B's draft-mounted rev=2 = err=%v msg=%q, want forbidden", rev2Err, rev2Msg)
		}
		listB, rawB, isErr := callMCPJSON(t, sessB, "list_items", map[string]any{"key": "r2:h1:draftmount"})
		if isErr {
			t.Fatalf("sessB list_items = %s", rawB)
		}
		if gotCnt := int(listB["items"].([]any)[0].(map[string]any)["revision_count"].(float64)); gotCnt != 3 {
			t.Fatalf("Token B revision_count = %d, want 3", gotCnt)
		}
		if rev2Out, _, rev2Err := callMCPJSON(t, sessB, "get_item", map[string]any{"id": itemID, "rev": 2, "format": "text"}); rev2Err || !strings.Contains(rev2Out["content"].(string), "token B confidential draft") {
			t.Fatalf("Token B read rev=2 = err=%v %v", rev2Err, rev2Out)
		}

		// Legacy pre-v0.2.0 draft with NULL updated_by fails closed to historical mount B1 auth even for Token A.
		legacyID := ag.ingestOK(ingestReq{
			key:   "r2:h1:null-updated-by",
			xml:   xmlDoc(docOpts{title: "Legacy Draft V1", md: "legacy secret draft"}),
			draft: true,
		})
		if _, err := e.db.Exec(`UPDATE items SET updated_by = NULL WHERE id = ?`, legacyID); err != nil {
			t.Fatal(err)
		}
		if st := admin.do("PUT", "/api/v1/admin/items/"+legacyID+"/mounts", map[string]any{"dirs": []string{protC}}).status; st != http.StatusNoContent {
			t.Fatalf("admin mount legacy draft = %d", st)
		}
		if st := admin.do("PUT", "/api/v1/admin/items/"+legacyID+"/mounts", map[string]any{"dirs": []string{}}).status; st != http.StatusNoContent {
			t.Fatalf("admin clear legacy mounts = %d", st)
		}
		_ = ag.ingestOK(ingestReq{
			key:  "r2:h1:null-updated-by",
			xml:  xmlDoc(docOpts{title: "Legacy Draft V2 Public", md: "legacy public v2"}),
			dirs: []string{pubQ},
		})
		if _, rev1Msg, rev1Err := callMCPJSON(t, sessA, "get_item", map[string]any{"id": legacyID, "rev": 1}); !rev1Err || !strings.Contains(rev1Msg, "forbidden") {
			t.Fatalf("Token A read NULL-updated_by rev=1 = err=%v msg=%q, want forbidden", rev1Err, rev1Msg)
		}
	})

	t.Run("R2-M1 idle GET SSE does not extend tracker TTL and 404 removes session", func(t *testing.T) {
		now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
		tr := newMCPSessionTracker()
		tr.now = func() time.Time { return now }

		for i := range mcpMaxAnonSessionsPerIP {
			if !tr.tryReserveAnonInit("1.2.3.4") {
				t.Fatalf("reserve %d failed unexpectedly", i)
			}
			tr.bind(fmt.Sprintf("s%d", i), "anon:1.2.3.4", "1.2.3.4", true)
		}
		if tr.tryReserveAnonInit("1.2.3.4") {
			t.Fatal("17th anon session reservation succeeded")
		}

		// Simulate GET SSE requests opening at +29m and staying idle past the 30m timeout.
		now = now.Add(29 * time.Minute)
		for i := range mcpMaxAnonSessionsPerIP {
			found, allowed := tr.acquireExisting(fmt.Sprintf("s%d", i), "anon:1.2.3.4", false)
			if !found || !allowed {
				t.Fatalf("GET acquire s%d = (%v, %v)", i, found, allowed)
			}
			tr.releaseExisting(fmt.Sprintf("s%d", i), false)
		}

		// At +30m, SDK closes the idle sessions (GET activity did NOT bump lastSeen).
		now = now.Add(time.Minute)
		if !tr.tryReserveAnonInit("1.2.3.4") {
			t.Fatal("expired sessions blocked new anon reservation after idle GET SSE")
		}
		tr.releaseAnonInitReservation("1.2.3.4")

		// Active POST prevents pruning while in flight and extends lastSeen on completion.
		tr.bind("active-post", "anon:1.2.3.4", "1.2.3.4", false)
		if found, allowed := tr.acquireExisting("active-post", "anon:1.2.3.4", true); !found || !allowed {
			t.Fatalf("POST acquire = (%v, %v)", found, allowed)
		}
		now = now.Add(40 * time.Minute)
		tr.mu.Lock()
		tr.pruneLocked(now)
		_, stillInFlight := tr.sessions["active-post"]
		tr.mu.Unlock()
		if !stillInFlight {
			t.Fatal("in-flight POST session was pruned early")
		}
		tr.releaseExisting("active-post", true)
		now = now.Add(29 * time.Minute)
		if found, _ := tr.acquireExisting("active-post", "anon:1.2.3.4", false); !found {
			t.Fatal("POST completion did not refresh lastSeen")
		}
		now = now.Add(time.Minute)
		if found, _ := tr.acquireExisting("active-post", "anon:1.2.3.4", false); found {
			t.Fatal("idle session after POST timeout was not pruned")
		}

		// SDK 404 from serveTrackedMCP immediately evicts the session from tracker.
		tr.bind("sid-404", "anon:1.2.3.4", "1.2.3.4", false)
		req404 := httptest.NewRequest(http.MethodGet, "http://localhost/mcp", nil)
		rec404 := httptest.NewRecorder()
		(&Server{}).serveTrackedMCP(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "session not found", http.StatusNotFound)
			}),
			tr, "anon:1.2.3.4", "1.2.3.4", nil, "sid-404", rec404, req404,
		)
		if found, _ := tr.acquireExisting("sid-404", "anon:1.2.3.4", false); found {
			t.Fatal("SDK 404 did not remove sid-404 from tracker")
		}
	})

	t.Run("R2-M3 rewritten XML >11MiB fails 413 without uploading assets", func(t *testing.T) {
		e := newEnv(t)
		admin := e.admin()
		ag := e.anon()
		pubQ := admin.createDir("", "Public Q", "")
		pngData := validTestPNG(t)

		// 25k repeated short <figure src="a.png"/> tags (~575 KiB) expand by 25k*74 = ~1.85 MiB upon hash rewrite,
		// pushing a ~10.07 MiB (< 10 MiB limit) input XML over the 11 MiB stored XML ceiling.
		var sb strings.Builder
		sb.Grow(10 << 20)
		sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?><document version="1.2"><meta><title>Expand</title><type>report</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta><body>`)
		sb.WriteString(strings.Repeat(`<figure src="a.png"/>`, 25_000))
		sb.WriteString(`<md>`)
		sb.WriteString(strings.Repeat("x", 9_500_000))
		sb.WriteString(`</md></body></document>`)

		resp := ag.ingest(ingestReq{
			xml:    sb.String(),
			assets: []figure{{name: "a.png", src: "a.png", data: pngData}},
			dirs:   []string{pubQ},
		})
		if resp.status != http.StatusRequestEntityTooLarge {
			t.Fatalf("oversized rewritten XML status = %d %s, want 413", resp.status, resp.body)
		}
		e.store.mu.Lock()
		storedCount := len(e.store.files)
		e.store.mu.Unlock()
		if storedCount != 0 {
			t.Fatalf("stored objects after 413 rewritten XML = %d, want 0 (no orphan asset upload)", storedCount)
		}
	})

	t.Run("R2-L1 loopback Host works on /mcp even when public_url is configured", func(t *testing.T) {
		ePort := newEnvWithOptions(t, RouterOptions{PublicURL: "https://hub.example.org:8443"}, nil)
		check := func(host, origin string) int {
			req := httptest.NewRequest(http.MethodPost, "http://"+host+"/mcp", strings.NewReader(initBody))
			req.Host = host
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			rec := httptest.NewRecorder()
			ePort.router.ServeHTTP(rec, req)
			return rec.Code
		}
		if st := check("127.0.0.1:8080", ""); st != http.StatusOK {
			t.Fatalf("loopback without Origin = %d, want 200", st)
		}
		if st := check("127.0.0.1:8080", "http://127.0.0.1:8080"); st != http.StatusOK {
			t.Fatalf("loopback with matching Origin = %d, want 200", st)
		}
		if st := check("[::1]:8080", "http://[::1]:8080"); st != http.StatusOK {
			t.Fatalf("IPv6 loopback with matching Origin = %d, want 200", st)
		}
		if st := check("127.0.0.1:8080", "https://evil.example"); st != http.StatusForbidden {
			t.Fatalf("loopback with evil Origin = %d, want 403", st)
		}
		if st := check("evil.example", ""); st != http.StatusForbidden {
			t.Fatalf("non-loopback mismatched Host = %d, want 403", st)
		}
	})
}
