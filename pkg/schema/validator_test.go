package schema

import (
	"strings"
	"testing"
)

const metaBlock = `<meta><title>T</title><type>report</type><date>2026-11-20</date><summary>S</summary><source-agent>a</source-agent></meta>`

func doc(body string) string {
	return `<document version="1.0">` + metaBlock + `<body>` + body + `</body></document>`
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
		{"wrong root", `<report/>`, []string{"INVALID_ROOT"}},
		{"bad version", strings.Replace(doc(``), `"1.0"`, `"2.0"`, 1), []string{"UNSUPPORTED_VERSION"}},
		{"bad type and date",
			`<document version="1.0"><meta><title>T</title><type>note</type><date>20-11-2026</date><summary>S</summary><source-agent>a</source-agent></meta><body/></document>`,
			[]string{"INVALID_TYPE", "INVALID_DATE"}},
		{"missing meta fields", `<document version="1.0"><meta><title>T</title></meta><body/></document>`,
			[]string{"REQUIRED_FIELD", "REQUIRED_FIELD", "REQUIRED_FIELD", "REQUIRED_FIELD"}},
		{"section needs title", doc(`<section/>`), []string{"MISSING_ATTR"}},
		{"unknown tag", doc(`<section title="a"><video/></section>`), []string{"UNKNOWN_TAG"}},
		{"bare text in section", doc(`<section title="a">hello</section>`), []string{"UNEXPECTED_TEXT"}},
		{"javascript url", doc(`<section title="a"><paper-card title="p" url="javascript:alert(1)"/></section>`), []string{"INVALID_URL"}},
		{"bad related id", doc(`<section title="a"><paper-card title="p" url="https://x.org" related-id="short"/></section>`), []string{"INVALID_ID"}},
		{"callout type", doc(`<section title="a"><callout type="danger">x</callout></section>`), []string{"INVALID_ATTR"}},
		{"collapse allows only md and figure", doc(`<section title="a"><collapse title="c"><callout type="info">x</callout></collapse></section>`), []string{"UNKNOWN_TAG"}},
		{"sources need an entry", doc(`<sources/>`), []string{"MISSING_SOURCE"}},
		{"source url", doc(`<sources><source title="t" url="ftp://x"/></sources>`), []string{"INVALID_URL"}},
		{"table cell mismatch",
			doc(`<section title="a"><compare-table><columns><col>a</col><col>b</col></columns><row><cell>1</cell><cell>2</cell></row><row><cell>1</cell></row></compare-table></section>`),
			[]string{"TABLE_COL_MISMATCH"}},
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
	input := "<document version=\"1.0\">" + metaBlock + "\n<body>\n<section title=\"a\">\n<figure src=\"./imgs/a.png\"/>\n</section></body></document>"
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
		`<figure src="/api/v1/assets/abc" caption="A &amp; B"/><figure src="/api/v1/assets/abc"></figure></section>`)
	if string(out) != want {
		t.Fatalf("rewrite =\n%s\nwant\n%s", out, want)
	}
}

func TestPlainText(t *testing.T) {
	d, _ := Parse([]byte(doc(`<section title="Sec"><md><![CDATA[hello world]]></md></section>`)), Options{})
	if got := d.PlainText(); !strings.Contains(got, "hello world") || !strings.Contains(got, "Sec") {
		t.Fatalf("PlainText() = %q", got)
	}
}
