package api

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func titles(t *testing.T, resp response) []string {
	t.Helper()
	if resp.status != http.StatusOK {
		t.Fatalf("search status %d: %s", resp.status, resp.body)
	}
	var out []string
	for _, r := range resp.json()["results"].([]any) {
		out = append(out, r.(map[string]any)["title"].(string))
	}
	return out
}

func q(path, query string) string { return path + "?q=" + url.QueryEscape(query) }

func TestSearch(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "Search", "")
	for _, d := range []docOpts{
		{title: "世界模型推理加速", summary: "端到端自动驾驶", md: "流匹配替代扩散", date: "2026-11-18", tags: []string{"自动驾驶"}},
		{title: "DriveFlow benchmark", summary: "latency study", md: "uses 100% fp16 and snake_case names", date: "2026-11-19"},
		{title: "Script injection", summary: "escaping", md: "payload <script>alert(1)</script> in the text", date: "2026-11-20"},
	} {
		admin.ingestOK(ingestReq{xml: xmlDoc(d), dirs: []string{dir}})
	}
	search := func(query string) []string {
		return titles(t, admin.do("GET", q("/api/v1/dirs/"+dir+"/search", query), nil))
	}

	t.Run("two character Chinese query", func(t *testing.T) {
		if got := search("世界"); len(got) != 1 || got[0] != "世界模型推理加速" {
			t.Errorf("世界 -> %v", got)
		}
		if got := search("驾驶"); len(got) != 1 {
			t.Errorf("驾驶 (summary and tag) -> %v", got)
		}
		if got := search("无此词"); len(got) != 0 {
			t.Errorf("no match -> %v", got)
		}
	})
	t.Run("three or more characters use the trigram index", func(t *testing.T) {
		if got := search("世界模型"); len(got) != 1 {
			t.Errorf("世界模型 -> %v", got)
		}
		if got := search("driveflow"); len(got) != 1 || got[0] != "DriveFlow benchmark" {
			t.Errorf("driveflow (case-insensitive) -> %v", got)
		}
		if got := search("driveflow latency"); len(got) != 1 {
			t.Errorf("two tokens are ANDed -> %v", got)
		}
		if got := search("driveflow 世界模型"); len(got) != 0 {
			t.Errorf("tokens from different items -> %v", got)
		}
	})
	t.Run("mixed short and long tokens", func(t *testing.T) {
		if got := search("fp 100%"); len(got) != 1 || got[0] != "DriveFlow benchmark" {
			t.Errorf("fp 100%% -> %v", got)
		}
	})
	t.Run("FTS syntax in the query never errors", func(t *testing.T) {
		for _, query := range []string{`"`, `*`, `AND`, `foo AND`, `"driveflow`, `driveflow*`, `NEAR(a b)`, `title:driveflow`, `-driveflow`, `(`, `driveflow OR 世界模型`, `'; DROP TABLE items; --`, `"" ""`} {
			resp := admin.do("GET", q("/api/v1/dirs/"+dir+"/search", query), nil)
			if resp.status != http.StatusOK {
				t.Errorf("query %q: %d %s", query, resp.status, resp.body)
			}
		}
		// Operators are literal text, not syntax.
		if got := search("driveflow OR zebra"); len(got) != 0 {
			t.Errorf("OR must be literal: %v", got)
		}
		if got := search("*"); len(got) != 0 {
			t.Errorf("* must be literal: %v", got)
		}
	})
	t.Run("LIKE wildcards are literal", func(t *testing.T) {
		if got := search("%"); len(got) != 1 {
			t.Errorf("%% should match only the text containing a percent sign: %v", got)
		}
		if got := search("__"); len(got) != 0 {
			t.Errorf("__ must not match everything: %v", got)
		}
		if got := search("s_s"); len(got) != 0 {
			t.Errorf("s_s must not act as a wildcard: %v", got)
		}
	})
	t.Run("snippet is escaped and marked", func(t *testing.T) {
		resp := admin.do("GET", q("/api/v1/dirs/"+dir+"/search", "alert"), nil)
		results := resp.json()["results"].([]any)
		if len(results) != 1 {
			t.Fatalf("results = %v", results)
		}
		snippet := results[0].(map[string]any)["snippet"].(string)
		if strings.Contains(snippet, "<script>") || !strings.Contains(snippet, "&lt;script&gt;") || !strings.Contains(snippet, "<mark>alert</mark>") {
			t.Errorf("snippet = %q", snippet)
		}
		for _, bad := range []string{"\x01", "\x02"} {
			if strings.Contains(snippet, bad) {
				t.Errorf("snippet leaks marker: %q", snippet)
			}
		}
		// Without MATCH there is no snippet.
		short := admin.do("GET", q("/api/v1/dirs/"+dir+"/search", "世"), nil).json()["results"].([]any)[0].(map[string]any)
		if short["snippet"] != "" {
			t.Errorf("snippet for LIKE-only search = %v", short["snippet"])
		}
		item := short
		for _, key := range []string{"id", "type", "title", "date", "summary", "tags", "source_agent", "snippet"} {
			if _, ok := item[key]; !ok {
				t.Errorf("result lacks %q: %v", key, item)
			}
		}
	})
	t.Run("empty query lists newest first and paginates", func(t *testing.T) {
		got := search("")
		if len(got) != 3 || got[0] != "Script injection" || got[2] != "世界模型推理加速" {
			t.Errorf("order = %v", got)
		}
		body := admin.do("GET", "/api/v1/dirs/"+dir+"/search?limit=1&offset=1", nil).json()
		if r := body["results"].([]any); len(r) != 1 || r[0].(map[string]any)["title"] != "DriveFlow benchmark" || body["limit"] != float64(1) || body["offset"] != float64(1) {
			t.Errorf("page = %v", body)
		}
		if body := admin.do("GET", "/api/v1/dirs/"+dir+"/search?limit=1000", nil).json(); body["limit"] != float64(100) {
			t.Errorf("limit clamp = %v", body["limit"])
		}
		if body := admin.do("GET", "/api/v1/dirs/"+dir+"/search", nil).json(); body["limit"] != float64(20) || body["offset"] != float64(0) {
			t.Errorf("defaults = %v", body)
		}
		for _, bad := range []string{"limit=abc", "limit=0", "offset=-1"} {
			if got := admin.do("GET", "/api/v1/dirs/"+dir+"/search?"+bad, nil).status; got != http.StatusBadRequest {
				t.Errorf("%s = %d, want 400", bad, got)
			}
		}
	})
}

func TestVisitorSearchStaysInsideTheSubtree(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	one, two := admin.createDir("", "One", ""), admin.createDir("", "Two", "")
	oneChild := admin.createDir(one, "OneChild", "")
	locked := admin.createDir(one, "OneLocked", "pw")
	mk := func(title string, dirs ...string) {
		admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: title, md: "commonterm"}), dirs: dirs})
	}
	mk("in one", one)
	mk("in one child", oneChild)
	mk("in two", two)
	mk("in locked", locked)
	mk("unmounted")
	mk("in one and two", one, two)

	v := e.anon()
	for _, query := range []string{"commonterm", "in", "term", "", "*", "in one"} {
		got := titles(t, v.do("GET", q("/api/v1/dirs/"+one+"/search", query), nil))
		for _, title := range got {
			if title == "in two" || title == "unmounted" || title == "in locked" {
				t.Errorf("query %q leaked %q: %v", query, title, got)
			}
		}
	}
	got := titles(t, v.do("GET", q("/api/v1/dirs/"+one+"/search", "commonterm"), nil))
	if len(got) != 3 {
		t.Errorf("subtree search = %v, want the three items visible in One and its open child", got)
	}
	// The same item mounted in two directories is listed once.
	count := 0
	for _, title := range got {
		if title == "in one and two" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("duplicate result for an item mounted twice: %v", got)
	}
	// Visitors have no way into the admin search or the root.
	if got := v.do("GET", "/api/v1/admin/search?q=commonterm", nil).status; got != http.StatusUnauthorized {
		t.Errorf("visitor admin search = %d", got)
	}
}

func TestAdminSearchFilters(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "D", "")
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "report old", date: "2026-01-05"}), dirs: []string{dir}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "post mid", typ: "post", date: "2026-06-15"}), dirs: []string{dir}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "report loose", date: "2026-11-01"})})

	search := func(query string) []string { return titles(t, admin.do("GET", "/api/v1/admin/search?"+query, nil)) }
	check := func(query string, want ...string) {
		t.Helper()
		got := search(query)
		if len(got) != len(want) {
			t.Errorf("%s -> %v, want %v", query, got, want)
			return
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s -> %v, want %v", query, got, want)
				return
			}
		}
	}
	check("", "report loose", "post mid", "report old")
	check("type=post", "post mid")
	check("type=report", "report loose", "report old")
	check("from=2026-06-01", "report loose", "post mid")
	check("to=2026-06-30", "post mid", "report old")
	check("from=2026-02-01&to=2026-07-01", "post mid")
	check("dir="+dir, "post mid", "report old")
	check("unmounted=1", "report loose")
	check("q=report&type=report&dir="+dir, "report old")
	for _, bad := range []string{"type=memo", "from=yesterday", "to=2026-13-01"} {
		if got := admin.do("GET", "/api/v1/admin/search?"+bad, nil).status; got != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, got)
		}
	}
}

func TestSearchV11Projection(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "V11Search", "")

	v11XML := `<document version="1.1">
  <meta>
    <title>V11 Search Projection</title>
    <type>report</type>
    <date>2026-11-20</date>
    <summary>Summary</summary>
    <source-agent>agent</source-agent>
  </meta>
  <body>
    <tldr><point>TldrBulletToken</point></tldr>
    <claim level="evidence">ClaimSearchHit 推理加速</claim>
    <equation id="eq1" caption="LossCaption"><![CDATA[\mathcal{EquationLatexOnly} \text{公式仅在公式内}]]></equation>
    <mermaid caption="MermaidCap"><![CDATA[graph LR; HiddenMermaidNode-->B]]></mermaid>
    <chart type="bar" caption="ChartCap"><![CDATA[{"categories":["HiddenChartCat"],"series":[{"name":"HiddenSeries","data":[1]}]}]]></chart>
    <code-block file="model_v11.go" caption="CodeCap"><![CDATA[func HiddenCodeFunc() {}]]></code-block>
    <quote author="AdaQuoteAuthor" source="Note G">Verba text</quote>
    <checklist><check status="done" detail="CheckDetailHit">Verified</check></checklist>
    <search-log><query source="arXiv">ti:world</query><excluded title="Old" reason="ExcludedReasonHit"/></search-log>
    <stages id="g1"><stage id="s1" title="G1S1"><md>Group1</md></stage></stages>
    <stages id="g2">
      <stage id="s2a" title="A"><md>A</md></stage>
      <stage id="s2b" title="B"><md>B</md></stage>
      <stage id="s2c" title="C"><md>StageThreeSecondGroupHit</md></stage>
    </stages>
    <quiz id="q1">
      <question>QuizQuestionHit</question>
      <option correct="true">HiddenQuizChoiceToken</option>
      <option>AnotherHiddenChoice</option>
      <explain>QuizExplainHit</explain>
    </quiz>
    <next-steps><option target="#stage-s2c">HiddenNextBtnToken</option></next-steps>
    <news-item title="NewsHeadlineHit" source="DailyWire">Headline<detail>NewsDetailHit 简报</detail></news-item>
  </body>
</document>`

	admin.ingestOK(ingestReq{xml: v11XML, dirs: []string{dir}})
	search := func(query string) []string {
		return titles(t, admin.do("GET", q("/api/v1/dirs/"+dir+"/search", query), nil))
	}

	// #18 + #81 FTS (>=3 chars) hits
	for _, hit := range []string{
		"ClaimSearchHit",
		"TldrBulletToken",
		"StageThreeSecondGroupHit",
		"ExcludedReasonHit",
		"AdaQuoteAuthor",
		"CheckDetailHit",
		"QuizQuestionHit",
		"QuizExplainHit",
		"NewsDetailHit",
		"model_v11.go",
	} {
		if got := search(hit); len(got) != 1 || got[0] != "V11 Search Projection" {
			t.Errorf("search(%q) = %v, want 1 match", hit, got)
		}
	}

	// #81 2-char Chinese search (items_fts_cjk): hit in claim & detail, excluded in equation
	if got := search("推理"); len(got) != 1 {
		t.Errorf("2-char search(推理) = %v, want 1 match", got)
	}
	if got := search("简报"); len(got) != 1 {
		t.Errorf("2-char search(简报) = %v, want 1 match", got)
	}
	if got := search("公式"); len(got) != 0 {
		t.Errorf("2-char inside <equation> should miss: search(公式) = %v", got)
	}

	// #18 + #81 excluded fragments must return 0 matches
	for _, miss := range []string{
		"EquationLatexOnly",
		"HiddenMermaidNode",
		"HiddenChartCat",
		"HiddenCodeFunc",
		"HiddenQuizChoiceToken",
		"HiddenNextBtnToken",
	} {
		if got := search(miss); len(got) != 0 {
			t.Errorf("search(%q) = %v, want 0 matches", miss, got)
		}
	}
}

func TestDirSearchTypeAndTagFilter(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dirA := admin.createDir("", "DirA", "")
	dirB := admin.createDir(dirA, "DirB", "pwB")

	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Visible WM Post", typ: "post", date: "2026-11-20", tags: []string{"World Model"}}), dirs: []string{dirA}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Locked WM Post", typ: "post", date: "2026-11-21", tags: []string{"World Model"}}), dirs: []string{dirB}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Substring Archive Report", typ: "report", date: "2026-11-19", tags: []string{"World Model Archive"}}), dirs: []string{dirA}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "Visible Vision Report", typ: "report", date: "2026-11-18", tags: []string{"Vision"}}), dirs: []string{dirA}})

	v := e.anon()
	// #45: locked subtree B stays excluded; exact tag match does not match "World Model Archive".
	gotTag := titles(t, v.do("GET", "/api/v1/dirs/"+dirA+"/search?tag="+url.QueryEscape("World Model"), nil))
	if len(gotTag) != 1 || gotTag[0] != "Visible WM Post" {
		t.Fatalf("visitor tag filter = %v, want [Visible WM Post]", gotTag)
	}
	// Empty q + type=post returns only visible posts.
	gotType := titles(t, v.do("GET", "/api/v1/dirs/"+dirA+"/search?type=post", nil))
	if len(gotType) != 1 || gotType[0] != "Visible WM Post" {
		t.Fatalf("visitor type=post = %v, want [Visible WM Post]", gotType)
	}
	// Combined tag + type.
	gotBoth := titles(t, v.do("GET", "/api/v1/dirs/"+dirA+"/search?type=report&tag=Vision", nil))
	if len(gotBoth) != 1 || gotBoth[0] != "Visible Vision Report" {
		t.Fatalf("visitor type+tag = %v, want [Visible Vision Report]", gotBoth)
	}
	// Invalid type returns 400.
	if resp := v.do("GET", "/api/v1/dirs/"+dirA+"/search?type=memo", nil); resp.status != http.StatusBadRequest {
		t.Errorf("invalid type = %d, want 400", resp.status)
	}
	// Unlocking B makes Locked WM Post visible under A's subtree tag filter.
	if resp := v.do("POST", "/api/v1/dirs/"+dirB+"/unlock", map[string]string{"password": "pwB"}); resp.status != http.StatusOK {
		t.Fatalf("unlock B = %d %s", resp.status, resp.body)
	}
	gotUnlocked := titles(t, v.do("GET", "/api/v1/dirs/"+dirA+"/search?tag="+url.QueryEscape("World Model"), nil))
	if len(gotUnlocked) != 2 {
		t.Fatalf("unlocked tag filter = %v, want 2 items", gotUnlocked)
	}
}
