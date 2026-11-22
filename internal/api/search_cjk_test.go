package api

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// cjkEnv ingests the documents shared by the Chinese search tests (acceptance 82-86).
func cjkEnv(t *testing.T) (e *env, admin *client, search func(string) []string) {
	t.Helper()
	e = newEnv(t)
	admin = e.admin()
	dir := admin.createDir("", "CJK", "")
	for _, d := range []docOpts{
		{title: "评测记录", summary: "记录一", md: "大模型训练与评测", date: "2026-11-01"},
		{title: "标点文档", summary: "记录二", md: "模型，训练", date: "2026-11-02"},
		{title: "模型综述", summary: "记录三", md: "完全无关的内容", date: "2026-11-03"},
		{title: "GPT 实践", summary: "记录四", md: "GPT 模型的使用", date: "2026-11-04"},
		{title: "GPT 单独", summary: "记录五", md: "只有英文 GPT", date: "2026-11-05"},
		{title: "无关文档", summary: "记录六", md: "苹果和香蕉", date: "2026-11-06"},
	} {
		admin.ingestOK(ingestReq{xml: xmlDoc(d), dirs: []string{dir}})
	}
	search = func(query string) []string {
		return titles(t, admin.do("GET", q("/api/v1/dirs/"+dir+"/search", query), nil))
	}
	return e, admin, search
}

func has(got []string, title string) bool {
	for _, g := range got {
		if g == title {
			return true
		}
	}
	return false
}

// Acceptance 82. The SQL check builds the statement directly (no database needed) and asserts the
// CJK table is queried and LIKE is not used.
func TestSearchChineseUsesBigramTable(t *testing.T) {
	built, err := buildSearch(searchQuery{text: "模型", limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(built.sql, "items_fts_cjk MATCH ?") || strings.Contains(built.sql, "LIKE") {
		t.Errorf("sql = %s", built.sql)
	}
	if len(built.args) < 1 || built.args[0] != `"模型"` {
		t.Errorf("args = %v, want the phrase \"模型\" first", built.args)
	}
	longer, err := buildSearch(searchQuery{text: "大模型训练", limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if longer.args[0] != `"大模 模型 型训 训练"` {
		t.Errorf("args = %v, want the bigram phrase", longer.args)
	}

	_, _, search := cjkEnv(t)
	for _, query := range []string{"模型", "训练", "练与", "大模型", "型训", "与评", "大模型训练与评测"} {
		if got := search(query); !has(got, "评测记录") {
			t.Errorf("%s -> %v, want to include 评测记录", query, got)
		}
	}
	if got := search("模型"); has(got, "无关文档") || has(got, "GPT 单独") {
		t.Errorf("模型 matched documents without it: %v", got)
	}
	if got := search("苹果"); len(got) != 1 || got[0] != "无关文档" {
		t.Errorf("苹果 -> %v", got)
	}
}

// Acceptance 83.
func TestSearchChineseBigramsStopAtPunctuation(t *testing.T) {
	_, _, search := cjkEnv(t)
	if got := search("型训"); has(got, "标点文档") {
		t.Errorf("型训 crossed the comma: %v", got)
	}
	if got := search("型训"); !has(got, "评测记录") {
		t.Errorf("型训 must still match unbroken text: %v", got)
	}
	for _, query := range []string{"模型", "训练"} {
		if got := search(query); !has(got, "标点文档") {
			t.Errorf("%s -> %v, want to include 标点文档", query, got)
		}
	}
}

// Acceptance 84.
func TestSearchSingleCharacterAndMixedQueries(t *testing.T) {
	_, _, search := cjkEnv(t)

	built, err := buildSearch(searchQuery{text: "型", limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(built.sql, "LIKE") || strings.Contains(built.sql, "items_fts") {
		t.Errorf("single character sql = %s", built.sql)
	}
	got := search("型")
	for _, want := range []string{"评测记录", "标点文档", "模型综述", "GPT 实践"} {
		if !has(got, want) {
			t.Errorf("型 -> %v, want to include %s", got, want)
		}
	}
	if has(got, "无关文档") {
		t.Errorf("型 matched 无关文档: %v", got)
	}

	// GPT is answered by items_fts and 模型 by items_fts_cjk, in one statement.
	for _, query := range []string{"GPT 模型", "GPT模型"} {
		if got := search(query); len(got) != 1 || got[0] != "GPT 实践" {
			t.Errorf("%s -> %v, want only GPT 实践", query, got)
		}
	}
	// A single character is ANDed with the other segments as well.
	if got := search("GPT 型"); len(got) != 1 || got[0] != "GPT 实践" {
		t.Errorf("GPT 型 -> %v", got)
	}
}

// Acceptance 85.
func TestSearchChineseSnippetComesFromTheOriginalText(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "Snip", "")
	long := strings.Repeat("前文很长的一段话", 10) + "大模型训练与评测" + strings.Repeat("后文也很长的一段话", 10)
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "T1", summary: "S1", md: "大模型训练与评测"}), dirs: []string{dir}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "T2", summary: "S2", md: long}), dirs: []string{dir}})

	resp := admin.do("GET", q("/api/v1/dirs/"+dir+"/search", "模型"), nil)
	snippets := map[string]string{}
	for _, r := range resp.json()["results"].([]any) {
		m := r.(map[string]any)
		snippets[m["title"].(string)] = m["snippet"].(string)
	}
	if len(snippets) != 2 {
		t.Fatalf("results = %v", snippets)
	}
	for title, snippet := range snippets {
		if !strings.Contains(snippet, "大<mark>模型</mark>训练与评测") {
			t.Errorf("%s snippet lacks the original context and mark: %q", title, snippet)
		}
		for _, expanded := range []string{"大模 模型 型训", "模型 型训", "\x01", "\x02"} {
			if strings.Contains(snippet, expanded) {
				t.Errorf("%s snippet contains %q: %q", title, expanded, snippet)
			}
		}
		if !utf8.ValidString(snippet) {
			t.Errorf("%s snippet is not valid UTF-8: %q", title, snippet)
		}
	}
	// The long text is cut on rune boundaries and shows ellipses on both sides.
	if s := snippets["T2"]; !strings.HasPrefix(s, "…") || !strings.HasSuffix(s, "…") || utf8.RuneCountInString(s) > 80 {
		t.Errorf("long snippet = %q (%d runes)", s, utf8.RuneCountInString(s))
	}
}

func TestOriginalSnippet(t *testing.T) {
	tests := []struct {
		name, text string
		terms      []string
		want       string
	}{
		{"no hit", "苹果", []string{"模型"}, ""},
		{"case is ignored", "Uses GPT-4 here", []string{"gpt"}, "Uses \x01GPT\x02-4 here"},
		{"several terms are all marked", "模型和训练", []string{"训练", "模型"}, "\x01模型\x02和\x01训练\x02"},
		{"overlapping terms are marked as one span", "大模型训练", []string{"大模型", "模型训练"}, "\x01大模型训练\x02"},
		{"every occurrence is marked", "模型A模型", []string{"模型"}, "\x01模型\x02A\x01模型\x02"},
		{"newlines are collapsed", "模型\n训练", []string{"训练"}, "模型 \x01训练\x02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := originalSnippet([]string{tt.text}, tt.terms); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	if got := originalSnippet([]string{"没有", "标题模型"}, []string{"模型"}); got != "标题\x01模型\x02" {
		t.Errorf("falls back to the next text: %q", got)
	}
}

// Acceptance 86: both FTS tables are weighted, so a title hit outranks a body hit, whichever table
// answers the query.
func TestSearchChineseRanking(t *testing.T) {
	_, admin, search := cjkEnv(t)
	got := search("模型")
	if len(got) < 3 || got[0] != "模型综述" {
		t.Errorf("模型 -> %v, want 模型综述 (title hit) first", got)
	}
	// The same through the trigram table, plus a bigram term.
	dir := admin.createDir("", "Rank", "")
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "body only", summary: "s", md: "rankword 模型 once", date: "2026-11-10"}), dirs: []string{dir}})
	admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "rankword 模型", summary: "s", md: "other", date: "2026-11-01"}), dirs: []string{dir}})
	got = titles(t, admin.do("GET", q("/api/v1/dirs/"+dir+"/search", "rankword 模型"), nil))
	if len(got) != 2 || got[0] != "rankword 模型" {
		t.Errorf("rankword 模型 -> %v, want the title hit first", got)
	}
}

// Acceptance 87 (the table follows add, update and delete; reindex is tested in cmd/server).
func TestItemsFTSCJKFollowsItem(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	dir := admin.createDir("", "Sync", "")
	rows := func(id string) (n int, body string) {
		t.Helper()
		if err := e.db.Get(&n, `SELECT count(*) FROM items_fts_cjk WHERE item_id = ?`, id); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			if err := e.db.Get(&body, `SELECT plain_text FROM items_fts_cjk WHERE item_id = ?`, id); err != nil {
				t.Fatal(err)
			}
		}
		return n, body
	}

	id := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "初稿", md: "旧内容"}), dirs: []string{dir}, key: "sync:1"})
	n, body := rows(id)
	if n != 1 || !strings.Contains(body, "旧内 内容") {
		t.Fatalf("after ingest: %d rows, plain_text = %q", n, body)
	}

	if got := admin.ingestOK(ingestReq{xml: xmlDoc(docOpts{title: "终稿", md: "新内容"}), key: "sync:1"}); got != id {
		t.Fatalf("update changed the id: %s -> %s", id, got)
	}
	n, body = rows(id)
	if n != 1 || !strings.Contains(body, "新内 内容") || strings.Contains(body, "旧内") {
		t.Fatalf("after update: %d rows, plain_text = %q", n, body)
	}

	if resp := admin.do("DELETE", "/api/v1/admin/items/"+id, nil); resp.status != 204 {
		t.Fatalf("delete: %d %s", resp.status, resp.body)
	}
	if n, _ = rows(id); n != 0 {
		t.Errorf("after delete: %d rows left", n)
	}
}
