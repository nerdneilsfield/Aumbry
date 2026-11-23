package schema

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const metaBlock = `<meta><title>T</title><type>report</type><date>2026-11-20</date><summary>S</summary><source-agent>a</source-agent></meta>`

func doc(body string) string {
	return `<document version="1.1">` + metaBlock + `<body>` + body + `</body></document>`
}

func codes(issues []ValidationError) []string {
	var out []string
	for _, i := range issues {
		out = append(out, i.Code)
	}
	return out
}

func TestParseRules(t *testing.T) {
	always := Options{HasAsset: func(string) bool { return true }}
	tests := []struct {
		name string
		xml  string
		want []string // expected codes, in order
	}{
		{"minimal valid", doc(``), nil},
		{"malformed XML", `<document>`, []string{"INVALID_XML"}},
		{"html entity is not XML", doc(`<section title="a"><md>&nbsp;</md></section>`), []string{"INVALID_XML"}},
		{"doctype", `<!DOCTYPE document [<!ENTITY x "y">]>` + doc(`<section title="a"><md>&x;</md></section>`), []string{"INVALID_XML"}},
		{"wrong root", `<report/>`, []string{"INVALID_ROOT"}},
		{"version 1.0 is unsupported", strings.Replace(doc(``), `"1.1"`, `"1.0"`, 1), []string{"UNSUPPORTED_VERSION"}},
		{"unknown version", strings.Replace(doc(``), `"1.1"`, `"2.0"`, 1), []string{"UNSUPPORTED_VERSION"}},
		{"missing version", strings.Replace(doc(``), ` version="1.1"`, ``, 1), []string{"UNSUPPORTED_VERSION"}},
		{
			"bad type and date",
			`<document version="1.1"><meta><title>T</title><type>note</type><date>20-11-2026</date><summary>S</summary><source-agent>a</source-agent></meta><body/></document>`,
			[]string{"INVALID_TYPE", "INVALID_DATE"},
		},
		{
			"missing meta fields", `<document version="1.1"><meta><title>T</title></meta><body/></document>`,
			[]string{"REQUIRED_FIELD", "REQUIRED_FIELD", "REQUIRED_FIELD", "REQUIRED_FIELD"},
		},
		{"second body is never rendered", strings.Replace(doc(``), `</document>`, `<body/></document>`, 1), []string{"DUPLICATE_BODY"}},
		{"second meta", strings.Replace(doc(``), `<body>`, metaBlock+`<body>`, 1), []string{"DUPLICATE_META"}},
		{"section needs title", doc(`<section/>`), []string{"MISSING_ATTR"}},
		{"unknown tag", doc(`<section title="a"><video/></section>`), []string{"UNKNOWN_TAG"}},
		{"bare text in section", doc(`<section title="a">hello</section>`), []string{"UNEXPECTED_TEXT"}},
		{"javascript url", doc(`<section title="a"><paper-card title="p" url="javascript:alert(1)"/></section>`), []string{"INVALID_URL"}},
		{"bad related id", doc(`<section title="a"><paper-card title="p" url="https://x.org" related-id="short"/></section>`), []string{"INVALID_ID"}},
		{"callout type", doc(`<section title="a"><callout type="danger">x</callout></section>`), []string{"INVALID_ATTR"}},
		{"collapse rejects non-leaf blocks", doc(`<section title="a"><collapse title="c"><paper-card title="p" url="https://x.org"/></collapse></section>`), []string{"UNKNOWN_TAG"}},
		{"sources need an entry", doc(`<sources/>`), []string{"MISSING_SOURCE"}},
		{"source url", doc(`<sources><source title="t" url="ftp://x"/></sources>`), []string{"INVALID_URL"}},
		{
			"table cell mismatch",
			doc(`<section title="a"><compare-table><columns><col>a</col><col>b</col></columns><row><cell>1</cell><cell>2</cell></row><row><cell>1</cell></row></compare-table></section>`),
			[]string{"TABLE_COL_MISMATCH"},
		},
		{"table needs rows", doc(`<section title="a"><compare-table><columns><col>a</col></columns></compare-table></section>`), []string{"TABLE_NO_ROWS"}},
		{"figure absolute path", doc(`<section title="a"><figure src="/etc/x.png"/></section>`), []string{"INVALID_ASSET_PATH"}},
		{"figure remote", doc(`<section title="a"><figure src="https://x.org/a.png"/></section>`), []string{"INVALID_ASSET_PATH"}},
		{"figure escapes dir", doc(`<section title="a"><figure src="../a.png"/></section>`), []string{"INVALID_ASSET_PATH"}},
		{"figure type", doc(`<section title="a"><figure src="a.gif"/></section>`), []string{"INVALID_ASSET_TYPE"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, issues := Parse([]byte(test.xml), always)
			got := codes(issues)
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("codes = %v, want %v\n%+v", got, test.want, issues)
			}
		})
	}
}

func TestMissingAssetReportsLine(t *testing.T) {
	input := "<document version=\"1.1\">" + metaBlock + "\n<body>\n<section title=\"a\">\n<figure src=\"./imgs/a.png\"/>\n</section></body></document>"
	_, issues := Parse([]byte(input), Options{HasAsset: func(string) bool { return false }})
	if len(issues) != 1 || issues[0].Code != "MISSING_ASSET" || issues[0].Line != 4 {
		t.Fatalf("issues = %+v, want one MISSING_ASSET on line 4", issues)
	}
}

// The PRD 6.1 example (with an svg figure) must validate and its figure must exist on disk.
func TestPRDExample(t *testing.T) {
	result, err := ValidateFile("../../examples/full-report.xml")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("PRD example invalid: %+v", result.Errors)
	}
}

func TestRewriteAssetsKeepsEverythingElse(t *testing.T) {
	input := doc(`<section title="a"><md><![CDATA[<figure src="./x.png"/> & text]]></md>` +
		`<figure src="./imgs/a.png" caption="A &amp; B"/><figure src="imgs/a.png"></figure></section>`)
	d, issues := Parse([]byte(input), Options{})
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	if len(d.Assets) != 1 || d.Assets[0] != "imgs/a.png" {
		t.Fatalf("assets = %v", d.Assets)
	}
	out, err := d.RewriteAssets(map[string]string{"imgs/a.png": "/api/v1/assets/abc"})
	if err != nil {
		t.Fatal(err)
	}
	want := doc(`<section title="a"><md><![CDATA[<figure src="./x.png"/> & text]]></md>` +
		`<figure src="/api/v1/assets/abc" caption="A &amp; B"/><figure src="/api/v1/assets/abc"/></section>`)
	if strings.TrimSuffix(string(out), "\n") != want { // the serializer ends the document with a newline
		t.Fatalf("rewrite =\n%s\nwant\n%s", out, want)
	}
}

// Empty elements come back self-closing: the XML tree does not record whether the input used <a></a> or <a/>.
func TestRewriteAssetsIsRepeatableAndKeepsDeclaration(t *testing.T) {
	input := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" + doc(`<section title="a"><figure src="a.png"/></section>`)
	d, issues := Parse([]byte(input), Options{})
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	for range 2 {
		out, err := d.RewriteAssets(map[string]string{"a.png": "/x"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(out), "<?xml version=\"1.0\"") || !strings.Contains(string(out), `<figure src="/x"/>`) {
			t.Fatalf("rewrite = %s", out)
		}
	}
	if _, err := d.RewriteAssets(map[string]string{}); err == nil {
		t.Fatal("missing mapping must fail")
	}
}

func TestIssuesCarryCodeFieldAndLine(t *testing.T) {
	input := "<document version=\"1.1\">" + metaBlock + "\n<body>\n<section title=\"a\">\n" +
		"<compare-table><columns><col>a</col><col>b</col><col>c</col><col>d</col></columns>\n" +
		"<row><cell>1</cell><cell>2</cell><cell>3</cell></row></compare-table>\n" +
		"<paper-card title=\"p\" url=\"HTTP://x.org\"/><paper-card title=\"p\" url=\"http://\"/>\n</section></body></document>"
	_, issues := Parse([]byte(input), Options{})
	want := []ValidationError{
		{Line: 5, Code: "TABLE_COL_MISMATCH", Field: "compare-table.row[1]", Message: "expected 4 cells to match columns, got 3"},
		{Line: 6, Code: "INVALID_URL", Field: "paper-card.url", Message: "must be an http or https URL"},
	}
	if !slices.Equal(issues, want) {
		t.Fatalf("issues = %+v\nwant %+v", issues, want)
	}
}

func TestMetaAndAssetsOrder(t *testing.T) {
	input := `<document version="1.1"><meta><title> T </title><type>post</type><date>2026-02-28</date><summary>S</summary>` +
		`<source-agent>a</source-agent><canonical-url>https://x.org/p</canonical-url><tags><tag>go</tag><tag>xml</tag></tags>` +
		`<related-ids><id>abcdefghij_-</id></related-ids></meta><body><section title="a">` +
		`<figure src="b.png"/><collapse title="c"><figure src="./a/../a.SVG"/></collapse><figure src="b.png"/></section></body></document>`
	d, issues := Parse([]byte(input), Options{})
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	want := Metadata{
		Title: "T", Type: "post", Date: "2026-02-28", Summary: "S", SourceAgent: "a", CanonicalURL: "https://x.org/p",
		Tags: []string{"go", "xml"}, RelatedIDs: []string{"abcdefghij_-"},
	}
	if d.Meta.Title != want.Title || d.Meta.CanonicalURL != want.CanonicalURL ||
		!slices.Equal(d.Meta.Tags, want.Tags) || !slices.Equal(d.Meta.RelatedIDs, want.RelatedIDs) {
		t.Fatalf("meta = %+v", d.Meta)
	}
	if !slices.Equal(d.Assets, []string{"b.png", "a.SVG"}) {
		t.Fatalf("assets = %v", d.Assets)
	}
	_, issues = Parse([]byte(strings.Replace(input, "2026-02-28", "2026-02-30", 1)), Options{})
	if got := codes(issues); !slices.Equal(got, []string{"INVALID_DATE"}) {
		t.Fatalf("impossible date: %v", got)
	}
}

func TestNormalizeAssetPath(t *testing.T) {
	for src, want := range map[string]string{"a.png": "a.png", "./imgs//a.png": "imgs/a.png", "x/../a.png": "a.png"} {
		if got, ok := NormalizeAssetPath(src); !ok || got != want {
			t.Errorf("NormalizeAssetPath(%q) = %q, %v; want %q", src, got, ok, want)
		}
	}
	for _, src := range []string{"", ".", "..", "../a.png", "a/../../b.png", "/a.png", "//h/a.png", "https://x.org/a.png", "data:image/png;base64,AA"} {
		if got, ok := NormalizeAssetPath(src); ok {
			t.Errorf("NormalizeAssetPath(%q) = %q, want rejection", src, got)
		}
	}
}

func TestPlainText(t *testing.T) {
	d, _ := Parse([]byte(doc(`<section title="Sec"><md><![CDATA[hello world]]></md></section>`)), Options{})
	if got := d.PlainText(); !strings.Contains(got, "hello world") || !strings.Contains(got, "Sec") {
		t.Fatalf("PlainText() = %q", got)
	}
}

func TestGeneralizedFigureAndURLXPaths(t *testing.T) {
	always := Options{HasAsset: func(string) bool { return true }}

	t.Run("figures and URLs are found in every container", func(t *testing.T) {
		input := doc(
			`<md>hi</md>` +
				`<steps><step title="s1"><figure src="./s.png"/></step></steps>` +
				`<tabs><tab title="t1"><figure src="./t.png"/></tab><tab title="t2"><md>x</md></tab></tabs>` +
				`<figure-group cols="2"><figure src="./g1.png"/><figure src="./g2.png"/></figure-group>`,
		)
		d, issues := Parse([]byte(input), always)
		if len(issues) != 0 {
			t.Fatalf("unexpected issues: %+v", issues)
		}
		wantAssets := []string{"s.png", "t.png", "g1.png", "g2.png"}
		if !slices.Equal(d.Assets, wantAssets) {
			t.Fatalf("Assets = %v, want %v", d.Assets, wantAssets)
		}
		rewritten, err := d.RewriteAssets(map[string]string{
			"s.png":  "/api/v1/assets/1",
			"t.png":  "/api/v1/assets/2",
			"g1.png": "/api/v1/assets/3",
			"g2.png": "/api/v1/assets/4",
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, needle := range []string{"/api/v1/assets/1", "/api/v1/assets/2", "/api/v1/assets/3", "/api/v1/assets/4"} {
			if !strings.Contains(string(rewritten), needle) {
				t.Fatalf("missing rewritten asset %s in %s", needle, rewritten)
			}
		}

		badURLs := doc(
			`<paper-card title="p" url="https://x.org" code-url="javascript:1"/>` +
				`<references><ref id="r1" title="r" url="ftp://x.org"/></references>` +
				`<quote url="javascript:2">text</quote>` +
				`<news-item title="n" url="file:///etc/passwd">sum</news-item>`,
		)
		_, issues = Parse([]byte(badURLs), always)
		if got := codes(issues); !slices.Equal(got, []string{"INVALID_URL", "INVALID_URL", "INVALID_URL", "INVALID_URL"}) {
			t.Fatalf("bad URLs codes = %v\n%+v", got, issues)
		}
	})
}

func TestValidateFileRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside.png")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	docDir := filepath.Join(base, "doc")
	imgDir := filepath.Join(docDir, "imgs")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realImg := filepath.Join(docDir, "real.png")
	if err := os.WriteFile(realImg, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(imgDir, "leak.png")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "real.png"), filepath.Join(imgDir, "ok.png")); err != nil {
		t.Fatal(err)
	}

	escapeDoc := filepath.Join(docDir, "escape.xml")
	if err := os.WriteFile(escapeDoc, []byte(doc(`<figure src="./imgs/leak.png"/>`)), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ValidateFile(escapeDoc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatalf("ValidateFile accepted symlink escaping document directory: %+v", res)
	}

	inDirDoc := filepath.Join(docDir, "ok.xml")
	if err = os.WriteFile(inDirDoc, []byte(doc(`<figure src="./imgs/ok.png"/>`)), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = ValidateFile(inDirDoc)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid {
		t.Fatalf("ValidateFile rejected in-dir symlink: %+v", res.Errors)
	}
}

func TestParseStoredAssetsSkipsFigureChecks(t *testing.T) {
	input := doc(`<section title="a"><md><![CDATA[needle text]]></md><figure src="/api/v1/assets/` + strings.Repeat("a", 64) + `"/></section>`)
	if _, issues := Parse([]byte(input), Options{}); len(issues) != 1 || issues[0].Code != "INVALID_ASSET_PATH" {
		t.Fatalf("default options: issues = %+v, want one INVALID_ASSET_PATH", issues)
	}
	parsed, issues := Parse([]byte(input), Options{StoredAssets: true})
	if len(issues) != 0 {
		t.Fatalf("StoredAssets: issues = %+v, want none", issues)
	}
	if !strings.Contains(parsed.PlainText(), "needle text") {
		t.Errorf("PlainText = %q, want it to contain the md text", parsed.PlainText())
	}

	audioStored := `<document version="1.2">` + metaBlock + `<body><audio src="/api/v1/assets/` + strings.Repeat("b", 64) + `" title="StoredEp"/></body></document>`
	if _, aIssues := Parse([]byte(audioStored), Options{StoredAssets: true}); len(aIssues) != 0 {
		t.Fatalf("StoredAssets audio: issues = %+v, want none", aIssues)
	}
}

func doc12(body string) string {
	return `<document version="1.2">` + metaBlock + `<body>` + body + `</body></document>`
}

func TestV12AudioAssetsOutgoingLinksAndPrimaryAudio(t *testing.T) {
	always := Options{HasAsset: func(string) bool { return true }}

	xml := doc12(
		`<section id="intro" title="Overview">` +
			`<figure src="./imgs/chart.png" caption="Chart"/>` +
			`<audio id="ep1" src="./audio/ep1.mp3" title="EpisodeTitleTok">` +
			`<chapter start="00:00:00" title="ChapIntroTok" target="#s-intro"/>` +
			`<chapter start="00:01:30" title="ChapSameStartA"/>` +
			`<chapter start="00:01:30" title="ChapSameStartB" target="#audio-ep1"/>` +
			`<chapter start="100:05:59" title="LongRun"/>` +
			`<transcript><![CDATA[Spoken transcript needle & details.]]></transcript>` +
			`</audio>` +
			`<audio src="./audio/ep2.wav" title="EpisodeTwo"/>` +
			`<news-item title="N1" detail-id="Item00000001">S1</news-item>` +
			`<news-item title="N2" detail-id="Item00000001">S2</news-item>` +
			`<paper-card title="P1" url="https://example.org/p1" related-id="Item00000002">P</paper-card>` +
			`</section>`,
	)

	d, issues := Parse([]byte(xml), always)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	if d.Version != "1.2" {
		t.Fatalf("Version = %q, want 1.2", d.Version)
	}
	wantAssets := []string{"imgs/chart.png", "audio/ep1.mp3", "audio/ep2.wav"}
	if !slices.Equal(d.Assets, wantAssets) {
		t.Fatalf("Assets = %v, want %v", d.Assets, wantAssets)
	}
	wantRefs := []AssetRef{
		{Path: "imgs/chart.png", Kind: AssetKindImage},
		{Path: "audio/ep1.mp3", Kind: AssetKindAudio},
		{Path: "audio/ep2.wav", Kind: AssetKindAudio},
	}
	if !slices.Equal(d.AssetRefs, wantRefs) {
		t.Fatalf("AssetRefs = %+v, want %+v", d.AssetRefs, wantRefs)
	}

	// PlainText includes audio title, chapter title, and transcript; excludes src filename, start, target.
	pt := d.PlainText()
	for _, want := range []string{"EpisodeTitleTok", "ChapIntroTok", "Spoken transcript needle"} {
		if !strings.Contains(pt, want) {
			t.Errorf("PlainText missing %q in %q", want, pt)
		}
	}
	for _, banned := range []string{"ep1.mp3", "ep2.wav", "00:01:30", "#s-intro"} {
		if strings.Contains(pt, banned) {
			t.Errorf("PlainText should exclude %q in %q", banned, pt)
		}
	}

	// OutgoingLinks deduplicates same (target, kind).
	wantLinks := []OutgoingLink{
		{TargetItemID: "Item00000001", Kind: "news_detail"},
		{TargetItemID: "Item00000002", Kind: "paper_related"},
	}
	if gotLinks := d.OutgoingLinks(); !slices.Equal(gotLinks, wantLinks) {
		t.Fatalf("OutgoingLinks() = %+v, want %+v", gotLinks, wantLinks)
	}

	// PrimaryAudio returns first audio only.
	pa := d.PrimaryAudio()
	if pa == nil || pa.Src != "audio/ep1.mp3" || pa.Hash != "" || pa.Title != "EpisodeTitleTok" || len(pa.Chapters) != 4 {
		t.Fatalf("PrimaryAudio() before rewrite = %+v", pa)
	}
	if pa.Chapters[0].Target != "#s-intro" || pa.Chapters[3].Start != "100:05:59" {
		t.Fatalf("PrimaryAudio chapters = %+v", pa.Chapters)
	}

	h1 := strings.Repeat("a", 64)
	h2 := strings.Repeat("b", 64)
	h3 := strings.Repeat("c", 64)
	rewritten, err := d.RewriteAssets(map[string]string{
		"imgs/chart.png": "/api/v1/assets/" + h1,
		"audio/ep1.mp3":  "/api/v1/assets/" + h2,
		"audio/ep2.wav":  "/api/v1/assets/" + h3,
	})
	if err != nil {
		t.Fatalf("RewriteAssets: %v", err)
	}
	reParsed, reIssues := Parse(rewritten, Options{StoredAssets: true})
	if len(reIssues) != 0 {
		t.Fatalf("reParsed StoredAssets issues: %+v", reIssues)
	}
	pa2 := reParsed.PrimaryAudio()
	if pa2 == nil || pa2.Hash != h2 || pa2.Src != "/api/v1/assets/"+h2 {
		t.Fatalf("PrimaryAudio() after rewrite = %+v", pa2)
	}
}
