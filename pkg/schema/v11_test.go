package schema

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestPRD66Example(t *testing.T) {
	// PRD 6.6 excerpt (plus required <meta> fields).
	input := `<document version="1.1">` + metaBlock + `
  <body>
    <section title="第二轮：方法与证据">
      <claim level="claim" source="§1 Abstract">推理延迟较扩散式方法降低一个数量级。</claim>
      <claim level="evidence" source="Table 2">在 nuScenes 上 30 FPS，对比 Vista-v2 为 8 FPS（同为单卡 H100，batch=1）。</claim>
      <claim level="unverified">训练预算与随机种子数：论文未报告。</claim>

      <equation id="loss" number="5" caption="流匹配目标"><![CDATA[\mathcal{L}=\mathbb{E}_{t,x}\lVert v_\theta(x_t,t)-u_t(x_t)\rVert^2]]></equation>

      <mermaid caption="整体流程"><![CDATA[flowchart LR
  A[多视角视频] --> B[编码器] --> C[世界模型] --> D[规划头]]]></mermaid>

      <compare-table caption="与相关工作对比">
        <columns><col>方案</col><col>帧率</col><col>代码</col></columns>
        <row><cell>DriveFlow</cell><cell tag="evidence">30 FPS</cell><cell>已开源</cell></row>
        <row><cell>Vista-v2</cell><cell tag="na">8 FPS（协议不同）</cell><cell>仅权重</cell></row>
      </compare-table>
    </section>
  </body>
</document>`
	_, issues := Parse([]byte(input), Options{})
	if len(issues) != 0 {
		t.Fatalf("PRD 6.6 example invalid: %+v", issues)
	}
}

func TestV11ValidationRules(t *testing.T) {
	always := Options{HasAsset: func(string) bool { return true }}

	tests := []struct {
		name      string
		xml       string
		wantCodes []string
		wantInMsg string
	}{
		// Freedom of structure (#76)
		{
			name: "body with single md passes",
			xml:  doc11(`<md>only markdown</md>`),
		},
		{
			name: "body direct blocks pass",
			xml: doc11(
				`<tldr><point>p1</point></tldr>` +
					`<claim level="claim">c</claim>` +
					`<chart type="bar"><![CDATA[{"categories":["a"],"series":[{"name":"s","data":[1]}]}]]></chart>` +
					`<metrics><metric label="FPS" value="30"/></metrics>` +
					`<quiz id="q1"><question>Q?</question><option correct="true">A</option><option>B</option></quiz>` +
					`<next-steps><option target="#quiz-q1">Jump</option></next-steps>` +
					`<news-item title="N">Summary</news-item>`,
			),
		},
		{
			name: "section nested 3 levels passes",
			xml:  doc11(`<section title="L1"><section title="L2"><section title="L3"><md>ok</md></section></section></section>`),
		},
		{
			name:      "section nested 4 levels fails SECTION_TOO_DEEP",
			xml:       doc11(`<section title="L1"><section title="L2"><section title="L3"><section title="L4"><md>deep</md></section></section></section></section>`),
			wantCodes: []string{"SECTION_TOO_DEEP"},
		},
		{
			name: "stage does not count as section depth",
			xml:  doc11(`<stages id="st"><stage id="r1" title="R1"><section title="L1"><section title="L2"><section title="L3"><md>ok</md></section></section></section></stage></stages>`),
		},
		{
			name:      "stage with 4 nested sections fails SECTION_TOO_DEEP",
			xml:       doc11(`<stages id="st"><stage id="r1" title="R1"><section title="L1"><section title="L2"><section title="L3"><section title="L4"><md>deep</md></section></section></section></section></stage></stages>`),
			wantCodes: []string{"SECTION_TOO_DEEP"},
		},

		// Shared ID namespace (#17, #63)
		{
			name:      "cross-type duplicate id fails DUPLICATE_ID",
			xml:       doc11(`<stages id="st"><stage id="x" title="S"><figure id="x" src="a.png"/></stage></stages>`),
			wantCodes: []string{"DUPLICATE_ID"},
		},
		{
			name:      "bad id format fails INVALID_ID",
			xml:       doc11(`<section id="Bad_ID" title="S"/>`),
			wantCodes: []string{"INVALID_ID"},
		},
		{
			name:      "two stages inside stage nested fails INVALID_STAGES",
			xml:       doc11(`<stages id="g1"><stage id="s1" title="S1"><stages id="g2"><stage id="s2" title="S2"><md>x</md></stage></stages></stage></stages>`),
			wantCodes: []string{"INVALID_STAGES"},
		},
		{
			name:      "stages missing id fails INVALID_STAGES",
			xml:       doc11(`<stages><stage id="s1" title="S1"><md>x</md></stage></stages>`),
			wantCodes: []string{"INVALID_STAGES"},
		},
		{
			name:      "stages with 0 stage fails INVALID_STAGES",
			xml:       doc11(`<stages id="g1"/>`),
			wantCodes: []string{"INVALID_STAGES"},
		},
		{
			name: "stages with 9 stage fails INVALID_STAGES",
			xml: doc11(`<stages id="g1">` +
				`<stage id="s1" title="1"/><stage id="s2" title="2"/><stage id="s3" title="3"/>` +
				`<stage id="s4" title="4"/><stage id="s5" title="5"/><stage id="s6" title="6"/>` +
				`<stage id="s7" title="7"/><stage id="s8" title="8"/><stage id="s9" title="9"/>` +
				`</stages>`),
			wantCodes: []string{"INVALID_STAGES"},
		},
		{
			name: "two independent stages groups pass (#63a)",
			xml: doc11(
				`<stages id="g1"><stage id="s1" title="1"><md>a</md></stage></stages>` +
					`<section title="Sec"><stages id="g2"><stage id="s2" title="2"><md>b</md></stage></stages></section>`,
			),
		},

		// Equation, Mermaid, Code-block
		{
			name:      "empty equation fails INVALID_EQUATION",
			xml:       doc11(`<equation>   </equation>`),
			wantCodes: []string{"INVALID_EQUATION"},
		},
		{
			name:      "oversize equation >4KB fails INVALID_EQUATION",
			xml:       doc11(`<equation>` + strings.Repeat("x", 4097) + `</equation>`),
			wantCodes: []string{"INVALID_EQUATION"},
		},
		{
			name:      "empty mermaid fails INVALID_MERMAID",
			xml:       doc11(`<mermaid/>`),
			wantCodes: []string{"INVALID_MERMAID"},
		},
		{
			name:      "oversize mermaid >20KB fails INVALID_MERMAID",
			xml:       doc11(`<mermaid><![CDATA[` + strings.Repeat("a", 20*1024+1) + `]]></mermaid>`),
			wantCodes: []string{"INVALID_MERMAID"},
		},
		{
			name:      "11 mermaid fails TOO_MANY_BLOCKS (#34)",
			xml:       doc11(strings.Repeat(`<mermaid><![CDATA[graph TD;A-->B]]></mermaid>`, 11)),
			wantCodes: []string{"TOO_MANY_BLOCKS"},
		},
		{
			name:      "11 chart fails TOO_MANY_BLOCKS",
			xml:       doc11(strings.Repeat(`<chart type="bar"><![CDATA[{"categories":["a"],"series":[{"name":"s","data":[1]}]}]]></chart>`, 11)),
			wantCodes: []string{"TOO_MANY_BLOCKS"},
		},
		{
			name:      "oversize code-block >32KB fails INVALID_CODE_BLOCK",
			xml:       doc11(`<code-block><![CDATA[` + strings.Repeat("x", 32*1024+1) + `]]></code-block>`),
			wantCodes: []string{"INVALID_CODE_BLOCK"},
		},
		{
			name:      "bad code-block highlight fails INVALID_CODE_BLOCK",
			xml:       doc11(`<code-block highlight="9-3">x</code-block>`),
			wantCodes: []string{"INVALID_CODE_BLOCK"},
		},
		{
			name: "code-block eq forward reference passes (#67)",
			xml:  doc11(`<code-block diff="true" highlight="1,3-4" eq="loss"><![CDATA[+line]]></code-block><equation id="loss">y=x</equation>`),
		},
		{
			name:      "code-block eq missing fails INVALID_TARGET (#67)",
			xml:       doc11(`<code-block eq="missing">x</code-block>`),
			wantCodes: []string{"INVALID_TARGET"},
		},

		// Cell tag (#65) & DOI (#33)
		{
			name: "cell unreported and approx pass (#65)",
			xml:  doc11(`<compare-table><columns><col>A</col></columns><row><cell tag="unreported">N/A</cell></row><row><cell tag="approx">~10</cell></row></compare-table>`),
		},
		{
			name:      "cell bad tag fails INVALID_ATTR (#65)",
			xml:       doc11(`<compare-table><columns><col>A</col></columns><row><cell tag="guess">1</cell></row></compare-table>`),
			wantCodes: []string{"INVALID_ATTR"},
		},
		{
			name: "ref doi valid passes (#33)",
			xml:  doc11(`<references><ref id="smith24" title="T" doi="10.48550/arXiv.2611.01234" read-depth="abstract"/></references>`),
		},
		{
			name:      "ref doi as URL fails INVALID_DOI (#33)",
			xml:       doc11(`<references><ref id="r1" title="T" doi="https://doi.org/10.1/2"/></references>`),
			wantCodes: []string{"INVALID_DOI"},
		},
		{
			name:      "paper-card bad doi fails INVALID_DOI",
			xml:       doc11(`<paper-card title="P" url="https://x.org" doi="not-a-doi"/>`),
			wantCodes: []string{"INVALID_DOI"},
		},

		// Metrics, Tabs, Steps, Checklist, Search-log
		{
			name:      "metrics empty label fails INVALID_METRIC",
			xml:       doc11(`<metrics><metric label="" value="1"/></metrics>`),
			wantCodes: []string{"INVALID_METRIC"},
		},
		{
			name:      "metrics 13 items fails INVALID_METRIC",
			xml:       doc11(`<metrics>` + strings.Repeat(`<metric label="k" value="1"/>`, 13) + `</metrics>`),
			wantCodes: []string{"INVALID_METRIC"},
		},
		{
			name:      "metrics invalid trend fails INVALID_METRIC",
			xml:       doc11(`<metrics><metric label="k" value="1" trend="sideways"/></metrics>`),
			wantCodes: []string{"INVALID_METRIC"},
		},
		{
			name:      "metrics with 0 metric fails INVALID_METRIC",
			xml:       doc11(`<metrics/>`),
			wantCodes: []string{"INVALID_METRIC"},
		},
		{
			name:      "glossary duplicate term name fails INVALID_ATTR",
			xml:       doc11(`<glossary><term name="LLM">a</term><term name="LLM">b</term></glossary>`),
			wantCodes: []string{"INVALID_ATTR"},
		},
		{
			name:      "ref missing id fails INVALID_ID",
			xml:       doc11(`<references><ref title="T"/></references>`),
			wantCodes: []string{"INVALID_ID"},
		},
		{
			name:      "ref duplicate id fails DUPLICATE_ID",
			xml:       doc11(`<references><ref id="r1" title="A"/><ref id="r1" title="B"/></references>`),
			wantCodes: []string{"DUPLICATE_ID"},
		},
		{
			name:      "ref javascript url fails INVALID_URL",
			xml:       doc11(`<references><ref id="r1" title="T" url="javascript:alert(1)"/></references>`),
			wantCodes: []string{"INVALID_URL"},
		},
		{
			name:      "stage id duplicated across stages groups fails DUPLICATE_ID",
			xml:       doc11(`<stages id="g1"><stage id="x" title="A"/></stages><stages id="g2"><stage id="x" title="B"/></stages>`),
			wantCodes: []string{"DUPLICATE_ID"},
		},
		{
			name:      "search-log with 2 stop fails INVALID_SEARCH_LOG",
			xml:       doc11(`<search-log><query source="arXiv">q</query><stop>first</stop><stop>second</stop></search-log>`),
			wantCodes: []string{"INVALID_SEARCH_LOG"},
		},
		{
			name:      "tabs with 1 tab fails (#17)",
			xml:       doc11(`<tabs><tab title="Only"><md>x</md></tab></tabs>`),
			wantCodes: []string{"INVALID_ATTR"},
		},
		{
			name:      "tabs duplicate title fails DUPLICATE_TAB",
			xml:       doc11(`<tabs><tab title="Same"><md>a</md></tab><tab title="Same"><md>b</md></tab></tabs>`),
			wantCodes: []string{"DUPLICATE_TAB"},
		},
		{
			name: "paper-card and repo-card v1.1 attributes pass",
			xml: doc11(
				`<paper-card title="P" url="https://x.org" venue="NeurIPS" doi="10.48550/arXiv.2611.01234" code-url="https://github.com/a/b" read-depth="full">s</paper-card>` +
					`<repo-card name="r" url="https://github.com/a/b" license="MIT" commit="deadbeef">s</repo-card>`,
			),
		},
		{
			name:      "paper-card bad read-depth fails INVALID_ATTR",
			xml:       doc11(`<paper-card title="P" url="https://x.org" read-depth="skim"/>`),
			wantCodes: []string{"INVALID_ATTR"},
		},
		{
			name: "interactive steps with 2 steps passes",
			xml:  doc11(`<steps interactive="true"><step title="1"><md>a</md></step><step title="2"><md>b</md></step></steps>`),
		},
		{
			name:      "interactive steps with 1 step fails (#64)",
			xml:       doc11(`<steps interactive="true"><step title="Only"><md>x</md></step></steps>`),
			wantCodes: []string{"INVALID_ATTR"},
		},
		{
			name:      "figure-group bad cols and too few figures fails",
			xml:       doc11(`<figure-group cols="4"><figure src="a.png"/></figure-group>`),
			wantCodes: []string{"INVALID_ATTR", "INVALID_ATTR"},
		},
		{
			name: "tldr allowed once each in body, section, and stage simultaneously",
			xml: doc11(
				`<tldr><point>body</point></tldr>` +
					`<section title="S"><tldr><point>sec</point></tldr><stages id="g1"><stage id="s1" title="St"><tldr><point>stage</point></tldr></stage></stages></section>`,
			),
		},
		{
			name:      "check bad status fails INVALID_CHECK (#36)",
			xml:       doc11(`<checklist><check status="maybe">x</check></checklist>`),
			wantCodes: []string{"INVALID_CHECK"},
		},
		{
			name:      "search-log missing query fails INVALID_SEARCH_LOG (#68)",
			xml:       doc11(`<search-log date="2026-11-20"><criteria><include>all</include></criteria></search-log>`),
			wantCodes: []string{"INVALID_SEARCH_LOG"},
		},
		{
			name: "search-log inside collapse passes",
			xml:  doc11(`<collapse title="Audit"><search-log date="2026-11-20"><query source="arXiv" date="2026-11-20" hits="12" kept="3">world model</query><excluded title="X" reason="off topic"/></search-log></collapse>`),
		},
		// PRD 6.2: collapse/tab/step hold leaf blocks only (no section/collapse nesting).
		{
			name:      "collapse inside collapse fails UNKNOWN_TAG",
			xml:       doc11(`<collapse title="a"><collapse title="b"><md>x</md></collapse></collapse>`),
			wantCodes: []string{"UNKNOWN_TAG"},
		},
		{
			name:      "section inside collapse fails UNKNOWN_TAG",
			xml:       doc11(`<collapse title="a"><section title="b"><md>x</md></section></collapse>`),
			wantCodes: []string{"UNKNOWN_TAG"},
		},
		{
			name:      "section inside tab fails UNKNOWN_TAG",
			xml:       doc11(`<tabs><tab title="a"><section title="b"><md>x</md></section></tab><tab title="c"><md>y</md></tab></tabs>`),
			wantCodes: []string{"UNKNOWN_TAG"},
		},
		{
			name:      "section inside step fails UNKNOWN_TAG",
			xml:       doc11(`<steps><step title="a"><section title="b"><md>x</md></section></step></steps>`),
			wantCodes: []string{"UNKNOWN_TAG"},
		},
		{
			name:      "collapse inside tab fails UNKNOWN_TAG",
			xml:       doc11(`<tabs><tab title="a"><collapse title="b"><md>x</md></collapse></tab><tab title="c"><md>y</md></tab></tabs>`),
			wantCodes: []string{"UNKNOWN_TAG"},
		},
		{
			name:      "collapse inside step fails UNKNOWN_TAG",
			xml:       doc11(`<steps><step title="a"><collapse title="b"><md>x</md></collapse></step></steps>`),
			wantCodes: []string{"UNKNOWN_TAG"},
		},
		{
			name:      "stages hidden under collapse section fails UNKNOWN_TAG",
			xml:       doc11(`<collapse title="a"><section title="b"><stages id="g"><stage id="g1" title="t"><md>x</md></stage></stages></section></collapse>`),
			wantCodes: []string{"UNKNOWN_TAG"},
		},
		{
			name:      "timeline bad date fails INVALID_DATE",
			xml:       doc11(`<timeline><event date="2026-13-01" title="E">d</event></timeline>`),
			wantCodes: []string{"INVALID_DATE"},
		},
		{
			name: "timeline YYYY and YYYY-MM pass",
			xml:  doc11(`<timeline><event date="2024" title="E1">a</event><event date="2024-05" title="E2">b</event></timeline>`),
		},

		// Quiz (#61)
		{
			name:      "quiz missing correct option fails INVALID_QUIZ",
			xml:       doc11(`<quiz id="q1"><question>Q</question><option>A</option><option>B</option></quiz>`),
			wantCodes: []string{"INVALID_QUIZ"},
		},
		{
			name:      "quiz with 1 option fails INVALID_QUIZ",
			xml:       doc11(`<quiz id="q1"><question>Q</question><option correct="true">A</option></quiz>`),
			wantCodes: []string{"INVALID_QUIZ"},
		},
		{
			name:      "quiz missing id fails INVALID_QUIZ",
			xml:       doc11(`<quiz><question>Q</question><option correct="true">A</option><option>B</option></quiz>`),
			wantCodes: []string{"INVALID_QUIZ"},
		},
		{
			name:      "quiz question with child element fails INVALID_QUIZ",
			xml:       doc11(`<quiz id="q1"><question><img src="x"/></question><option correct="true">A</option><option>B</option></quiz>`),
			wantCodes: []string{"INVALID_QUIZ"},
		},
		{
			name: "quiz escaped html inside question/CDATA passes",
			xml:  doc11(`<quiz id="q1"><question><![CDATA[<img onerror=alert(1)>]]></question><option correct="true">A</option><option>B</option></quiz>`),
		},

		// Next-steps & Anchor targets (#62, #80)
		{
			name: "next-steps valid targets resolve forward and backward",
			xml: doc11(
				`<next-steps>` +
					`<option target="#s-method">Method</option>` +
					`<option target="#stages-g1">Stages</option>` +
					`<option target="#stage-r1">Stage 1</option>` +
					`<option target="#quiz-q1">Quiz</option>` +
					`<option target="#fig-arch">Fig</option>` +
					`</next-steps>` +
					`<section id="method" title="Method">` +
					`<figure id="arch" src="a.png"/>` +
					`<stages id="g1"><stage id="r1" title="R1"><quiz id="q1"><question>Q</question><option correct="true">A</option><option>B</option></quiz></stage></stages>` +
					`</section>`,
			),
		},
		{
			name:      "next-steps positional #sec-3 fails INVALID_TARGET (#62, #80)",
			xml:       doc11(`<next-steps><option target="#sec-3">S3</option></next-steps>`),
			wantCodes: []string{"INVALID_TARGET"},
		},
		{
			name:      "next-steps external URL / javascript fails INVALID_TARGET",
			xml:       doc11(`<next-steps><option target="https://evil.com">E</option><option target="javascript:alert(1)">J</option><option target="/admin">A</option></next-steps>`),
			wantCodes: []string{"INVALID_TARGET", "INVALID_TARGET", "INVALID_TARGET"},
		},
		{
			name:      "next-steps wrong anchor prefix fails INVALID_TARGET",
			xml:       doc11(`<section id="m" title="M"/><next-steps><option target="#fig-m">Wrong</option></next-steps>`),
			wantCodes: []string{"INVALID_TARGET"},
		},

		// TLDR (#70) & Rune count
		{
			name: "tldr 300 Chinese runes passes",
			xml:  doc11(`<tldr placement="top"><point>` + strings.Repeat("中", 300) + `</point><verdict>` + strings.Repeat("结", 300) + `</verdict></tldr>`),
		},
		{
			name:      "tldr 301 Chinese runes fails INVALID_TLDR (#70)",
			xml:       doc11(`<tldr><point>` + strings.Repeat("中", 301) + `</point></tldr>`),
			wantCodes: []string{"INVALID_TLDR"},
		},
		{
			name:      "tldr multi-line point fails INVALID_TLDR",
			xml:       doc11("<tldr><point>line one\nline two</point></tldr>"),
			wantCodes: []string{"INVALID_TLDR"},
		},
		{
			name:      "tldr multi-line point in CDATA fails INVALID_TLDR",
			xml:       doc11("<tldr><point><![CDATA[line one\r\nline two]]></point></tldr>"),
			wantCodes: []string{"INVALID_TLDR"},
		},
		{
			name: "tldr point with surrounding whitespace newlines passes",
			xml:  doc11("<tldr><point>\n  one line\n</point></tldr>"),
		},
		{
			name:      "two body-level tldr fails INVALID_TLDR (#70)",
			xml:       doc11(`<tldr><point>a</point></tldr><section title="s"><md>x</md></section><tldr><point>b</point></tldr>`),
			wantCodes: []string{"INVALID_TLDR"},
		},
		{
			name:      "two section-level tldr fails INVALID_TLDR (#70)",
			xml:       doc11(`<section title="s"><tldr><point>a</point></tldr><tldr><point>b</point></tldr></section>`),
			wantCodes: []string{"INVALID_TLDR"},
		},
		{
			name:      "tldr 0 points or 8 points fails INVALID_TLDR",
			xml:       doc11(`<tldr><verdict>v</verdict></tldr><section title="s"><tldr>` + strings.Repeat(`<point>p</point>`, 8) + `</tldr></section>`),
			wantCodes: []string{"INVALID_TLDR", "INVALID_TLDR"},
		},
		{
			name:      "tldr inside collapse fails INVALID_TLDR",
			xml:       doc11(`<collapse title="c"><tldr><point>a</point></tldr></collapse>`),
			wantCodes: []string{"INVALID_TLDR"},
		},

		// News-item (#73)
		{
			name: "news-item with detail and RFC3339 passes; detail not counted against 600-rune summary",
			xml:  doc11(`<news-item title="Release" importance="high" published="2026-11-20T12:00:00Z" detail-id="abcdefghij12">` + strings.Repeat("简", 600) + `<detail>` + strings.Repeat("详", 1000) + `</detail></news-item>`),
		},
		{
			name:      "news-item missing title fails INVALID_NEWS (#73)",
			xml:       doc11(`<news-item>Summary</news-item>`),
			wantCodes: []string{"INVALID_NEWS"},
		},
		{
			name:      "news-item bad detail-id fails INVALID_ID (#73)",
			xml:       doc11(`<news-item title="N" detail-id="short">Summary</news-item>`),
			wantCodes: []string{"INVALID_ID"},
		},
		{
			name:      "news-item summary >600 runes fails INVALID_NEWS",
			xml:       doc11(`<news-item title="N">` + strings.Repeat("字", 601) + `</news-item>`),
			wantCodes: []string{"INVALID_NEWS"},
		},
		{
			name:      "news-item detail >8KB fails INVALID_NEWS",
			xml:       doc11(`<news-item title="N"><detail>` + strings.Repeat("a", 8193) + `</detail></news-item>`),
			wantCodes: []string{"INVALID_NEWS"},
		},

		// G7: v1.1 <source>, <cell>, <col> disallow child elements.
		{
			name:      "v1.1 source child element rejected UNKNOWN_TAG",
			xml:       doc11(`<sources><source title="t" url="https://x.org"><quiz/></source></sources>`),
			wantCodes: []string{"UNKNOWN_TAG"},
		},
		{
			name:      "v1.1 cell child element rejected UNKNOWN_TAG",
			xml:       doc11(`<compare-table><columns><col>A</col></columns><row><cell><quiz/></cell></row></compare-table>`),
			wantCodes: []string{"UNKNOWN_TAG"},
		},
		{
			name:      "v1.1 col child element rejected UNKNOWN_TAG",
			xml:       doc11(`<compare-table><columns><col><quiz/></col></columns><row><cell>1</cell></row></compare-table>`),
			wantCodes: []string{"UNKNOWN_TAG"},
		},

		// G9: <next-steps><option> requires non-empty label text.
		{
			name:      "next-steps empty option fails INVALID_ATTR",
			xml:       doc11(`<section id="x" title="X"/><next-steps><option target="#s-x"/></next-steps>`),
			wantCodes: []string{"INVALID_ATTR"},
		},
		{
			name:      "next-steps whitespace-only option fails INVALID_ATTR",
			xml:       doc11(`<section id="x" title="X"/><next-steps><option target="#s-x">   </option></next-steps>`),
			wantCodes: []string{"INVALID_ATTR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, issues := Parse([]byte(tt.xml), always)
			got := codes(issues)
			if !slices.Equal(got, tt.wantCodes) {
				t.Fatalf("codes = %v, want %v\nissues = %+v", got, tt.wantCodes, issues)
			}
			if tt.wantInMsg != "" && (len(issues) == 0 || !strings.Contains(issues[0].Message, tt.wantInMsg)) {
				t.Fatalf("expected %q in message, got %+v", tt.wantInMsg, issues)
			}
		})
	}
}

func TestTooManyBlocksStagesCaps(t *testing.T) {
	always := Options{HasAsset: func(string) bool { return true }}

	// 9 groups x 2 stages -> TOO_MANY_BLOCKS (#63)
	var b9 strings.Builder
	for i := range 9 {
		fmt.Fprintf(&b9, `<stages id="g%d"><stage id="s%da" title="A"/><stage id="s%db" title="B"/></stages>`, i, i, i)
	}
	_, issues := Parse([]byte(doc11(b9.String())), always)
	if got := codes(issues); !slices.Equal(got, []string{"TOO_MANY_BLOCKS"}) {
		t.Fatalf("9 groups codes = %v, want [TOO_MANY_BLOCKS]\n%+v", got, issues)
	}

	// 8 groups totaling 41 stages -> TOO_MANY_BLOCKS (#63)
	counts := []int{6, 6, 6, 6, 6, 6, 3, 2} // sum = 41
	var b41 strings.Builder
	stageIdx := 0
	for g, c := range counts {
		fmt.Fprintf(&b41, `<stages id="g%d">`, g)
		for range c {
			stageIdx++
			fmt.Fprintf(&b41, `<stage id="s%d" title="S"/>`, stageIdx)
		}
		b41.WriteString(`</stages>`)
	}
	_, issues = Parse([]byte(doc11(b41.String())), always)
	if got := codes(issues); !slices.Equal(got, []string{"TOO_MANY_BLOCKS"}) {
		t.Fatalf("41 stages codes = %v, want [TOO_MANY_BLOCKS]\n%+v", got, issues)
	}
}

func TestChartValidation(t *testing.T) {
	tests := []struct {
		name     string
		typ      string
		payload  string
		wantOK   bool
		wantPath string
	}{
		{
			name:    "valid bar chart with err (#66)",
			typ:     "bar",
			payload: `{"categories":["A","B"],"series":[{"name":"FPS","data":[30,8],"err":[1.2,0.5]}],"x_label":"Model","y_label":"FPS"}`,
			wantOK:  true,
		},
		{
			name:    "valid pie chart",
			typ:     "pie",
			payload: `{"categories":["X","Y"],"series":[{"name":"Share","data":[60,40]}]}`,
			wantOK:  true,
		},
		{
			name:    "valid scatter chart",
			typ:     "scatter",
			payload: `{"series":[{"name":"Pts","data":[[1,2],[3.5,4.5]]}]}`,
			wantOK:  true,
		},
		{
			name:     "mismatched data length (#17, #32)",
			typ:      "bar",
			payload:  `{"categories":["A","B"],"series":[{"name":"FPS","data":[30]}]}`,
			wantPath: "/series/0/data",
		},
		{
			name:     "empty categories (#32)",
			typ:      "line",
			payload:  `{"categories":[],"series":[{"name":"FPS","data":[1]}]}`,
			wantPath: "/categories",
		},
		{
			name:     "pie multiple series (#32)",
			typ:      "pie",
			payload:  `{"categories":["A"],"series":[{"name":"S1","data":[1]},{"name":"S2","data":[2]}]}`,
			wantPath: "/series",
		},
		{
			name:     "unquoted NaN reports JSON path (#17, #32)",
			typ:      "bar",
			payload:  `{"categories":["A","B"],"series":[{"name":"FPS","data":[30,NaN]}]}`,
			wantPath: "/series/0/data/1",
		},
		{
			name:     "quoted NaN reports JSON path",
			typ:      "bar",
			payload:  `{"categories":["A"],"series":[{"name":"FPS","data":["NaN"]}]}`,
			wantPath: "/series/0/data/0",
		},
		{
			name:     "missing series[].name reports path",
			typ:      "bar",
			payload:  `{"categories":["A"],"series":[{"data":[1]}]}`,
			wantPath: "/series/0",
		},
		{
			name:     "scatter with categories fails with path",
			typ:      "scatter",
			payload:  `{"categories":["A"],"series":[{"name":"S","data":[[1,2]]}]}`,
			wantPath: "/categories",
		},
		{
			name:     "series[].err length mismatch (#66)",
			typ:      "line",
			payload:  `{"categories":["A","B"],"series":[{"name":"FPS","data":[10,20],"err":[1]}]}`,
			wantPath: "/series/0/err",
		},
		{
			name:     "series[].err negative value (#66)",
			typ:      "bar",
			payload:  `{"categories":["A"],"series":[{"name":"FPS","data":[10],"err":[-0.5]}]}`,
			wantPath: "/series/0/err/0",
		},
		{
			name:     "series[].err unquoted NaN (#66)",
			typ:      "bar",
			payload:  `{"categories":["A"],"series":[{"name":"FPS","data":[10],"err":[NaN]}]}`,
			wantPath: "/series/0/err/0",
		},
		{
			name:     "series[].err rejected on pie (#66)",
			typ:      "pie",
			payload:  `{"categories":["A"],"series":[{"name":"Share","data":[10],"err":[1]}]}`,
			wantPath: "/series/0/err",
		},
		{
			name:     "series[].err rejected on scatter (#66)",
			typ:      "scatter",
			payload:  `{"series":[{"name":"Pts","data":[[1,2]],"err":[0.5]}]}`,
			wantPath: "/series/0/err",
		},
		{
			name:     "total points > 2000 rejected",
			typ:      "scatter",
			payload:  `{"series":[{"name":"Big","data":[` + strings.Repeat("[1,2],", 2000) + `[3,4]]}]}`,
			wantPath: "/series",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xml := doc11(`<chart type="` + tt.typ + `"><![CDATA[` + tt.payload + `]]></chart>`)
			_, issues := Parse([]byte(xml), Options{})
			if tt.wantOK {
				if len(issues) != 0 {
					t.Fatalf("unexpected issues: %+v", issues)
				}
				return
			}
			if len(issues) == 0 {
				t.Fatalf("expected INVALID_CHART at %s, got valid", tt.wantPath)
			}
			if issues[0].Code != "INVALID_CHART" {
				t.Fatalf("code = %q, want INVALID_CHART\n%+v", issues[0].Code, issues)
			}
			if !strings.Contains(issues[0].Message, tt.wantPath) && !strings.Contains(issues[0].Field, tt.wantPath) {
				t.Fatalf("expected JSON path %q in Field/Message, got %+v", tt.wantPath, issues[0])
			}
		})
	}
}

// PRD 7.4: INVALID_CHART carries a hint, including the semantic (non JSON syntax) failures.
func TestChartSemanticErrorsCarryHint(t *testing.T) {
	manySeries := strings.TrimSuffix(strings.Repeat(`{"name":"s","data":[1]},`, maxChartSeries+1), ",")
	tests := []struct {
		name    string
		typ     string
		payload string
	}{
		{"data length differs from categories", "bar", `{"categories":["A","B"],"series":[{"name":"s","data":[1]}]}`},
		{"empty series name", "bar", `{"categories":["A"],"series":[{"name":" ","data":[1]}]}`},
		{"pie with two series", "pie", `{"categories":["A"],"series":[{"name":"a","data":[1]},{"name":"b","data":[2]}]}`},
		{"too many series", "line", `{"categories":["A"],"series":[` + manySeries + `]}`},
		{"err length differs from data", "line", `{"categories":["A","B"],"series":[{"name":"s","data":[1,2],"err":[1]}]}`},
		{"negative err", "bar", `{"categories":["A"],"series":[{"name":"s","data":[1],"err":[-1]}]}`},
		{"err on pie", "pie", `{"categories":["A"],"series":[{"name":"s","data":[1],"err":[1]}]}`},
		{"err on scatter", "scatter", `{"series":[{"name":"s","data":[[1,2]],"err":[1]}]}`},
		{"too many points", "scatter", `{"series":[{"name":"s","data":[` + strings.Repeat("[1,2],", maxChartPoints) + `[3,4]]}]}`},
		{"non-finite number", "bar", `{"categories":["A"],"series":[{"name":"s","data":[1e999]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, issues := Parse([]byte(doc11(`<chart type="`+tt.typ+`"><![CDATA[`+tt.payload+`]]></chart>`)), Options{})
			if len(issues) == 0 {
				t.Fatal("expected INVALID_CHART, got valid")
			}
			for _, is := range issues {
				if is.Code != "INVALID_CHART" {
					t.Fatalf("code = %q, want INVALID_CHART\n%+v", is.Code, issues)
				}
				if strings.TrimSpace(is.Hint) == "" {
					t.Errorf("INVALID_CHART issue without hint: %+v", is)
				}
			}
		})
	}
}

func TestPlainTextV11Projection(t *testing.T) {
	v11XML := doc11(`
		<tldr><point>KeyPointSummary</point><verdict>FinalVerdict</verdict></tldr>
		<claim level="evidence">ClaimBodyToken</claim>
		<equation id="eq1" caption="EquationCaptionTok"><![CDATA[\mathcal{L}_{ExcludedLatexTok}]]></equation>
		<mermaid caption="MermaidCaptionTok"><![CDATA[flowchart LR
			A[ExcludedMermaidTok] --> B]]></mermaid>
		<chart type="bar" caption="ChartCaptionTok"><![CDATA[{"categories":["ExcludedChartCat"],"series":[{"name":"ExcludedSeriesName","data":[1]}]}]]></chart>
		<code-block file="ExcludedCodeFile.go" caption="CodeCaptionTok"><![CDATA[func ExcludedCodeSourceTok() {}]]></code-block>
		<metrics><metric label="MetricLabelTok" value="99.9MetricVal"/></metrics>
		<steps><step title="StepTitleTok"><md>StepMdTok</md></step></steps>
		<tabs><tab title="TabTitleOne"><md>TabBodyOne</md></tab><tab title="TabTitleTwo"><md>TabBodyTwo</md></tab></tabs>
		<timeline><event date="2026" title="TimelineTitleTok">TimelineBodyTok</event></timeline>
		<glossary><term name="TermNameTok">TermDefTok</term></glossary>
		<references><ref id="r1" title="RefTitleTok" authors="RefAuthorTok"/></references>
		<quote author="QuoteAuthorTok" source="QuoteSourceTok">QuoteBodyTok</quote>
		<checklist><check status="done" detail="CheckDetailTok">CheckBodyTok</check></checklist>
		<search-log><criteria><include>CriteriaIncludeTok</include><exclude>CriteriaExcludeTok</exclude></criteria><query source="arXiv">SearchQueryTok</query><excluded title="ExclTitleTok" reason="ExcludedReasonTok"/><gap>SearchGapTok</gap></search-log>
		<stages id="g1"><stage id="s1" title="StageOneTitle"/><stage id="s2" title="StageTwoTitle"><md>StageTwoBodyTok</md></stage></stages>
		<quiz id="q1"><question>QuizQuestionTok</question><option correct="true">ExcludedQuizOptionTok</option><option>AnotherExcludedOpt</option><explain>QuizExplainTok</explain></quiz>
		<next-steps><option target="#stage-s2">ExcludedNextStepBtn</option></next-steps>
		<news-item title="NewsTitleTok" source="NewsSourceTok">NewsSummaryTok<detail>NewsDetailTok</detail></news-item>
	`)
	d, issues := Parse([]byte(v11XML), Options{})
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	got := d.PlainText()

	mustInclude := []string{
		"KeyPointSummary", "FinalVerdict",
		"ClaimBodyToken",
		"EquationCaptionTok", "MermaidCaptionTok", "ChartCaptionTok", "ExcludedCodeFile.go", "CodeCaptionTok",
		"MetricLabelTok", "99.9MetricVal",
		"StepTitleTok", "StepMdTok",
		"TabTitleOne", "TabBodyTwo",
		"TimelineTitleTok", "TimelineBodyTok",
		"TermNameTok", "TermDefTok",
		"RefTitleTok", "RefAuthorTok",
		"QuoteAuthorTok", "QuoteSourceTok", "QuoteBodyTok",
		"CheckDetailTok", "CheckBodyTok",
		"CriteriaIncludeTok", "CriteriaExcludeTok", "SearchQueryTok", "ExclTitleTok", "ExcludedReasonTok", "SearchGapTok",
		"StageOneTitle", "StageTwoBodyTok",
		"QuizQuestionTok", "QuizExplainTok",
		"NewsTitleTok", "NewsSourceTok", "NewsSummaryTok", "NewsDetailTok",
	}
	for _, want := range mustInclude {
		if !strings.Contains(got, want) {
			t.Errorf("PlainText() missing %q\ngot=%s", want, got)
		}
	}

	mustExclude := []string{
		"ExcludedLatexTok",
		"ExcludedMermaidTok",
		"ExcludedChartCat",
		"ExcludedSeriesName",
		"ExcludedCodeSourceTok",
		"ExcludedQuizOptionTok",
		"AnotherExcludedOpt",
		"ExcludedNextStepBtn",
	}
	for _, banned := range mustExclude {
		if strings.Contains(got, banned) {
			t.Errorf("PlainText() should exclude %q", banned)
		}
	}

	// Verify v1.0 PlainText() keeps legacy //text() projection.
	v10XML := doc(`<section title="Sec"><md>legacytext</md></section>`)
	d10, issues := Parse([]byte(v10XML), Options{})
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	if !strings.Contains(d10.PlainText(), "legacytext") {
		t.Fatalf("v1.0 PlainText() = %q", d10.PlainText())
	}
}

func TestMissingAssetInV11ContainersAndIndirectNestedStages(t *testing.T) {
	missing := Options{HasAsset: func(string) bool { return false }}

	// #31: MISSING_ASSET inside <step>, <tab>, <figure-group>
	for name, body := range map[string]string{
		"step":         `<steps><step title="S1"><figure src="./missing.png"/></step></steps>`,
		"tab":          `<tabs><tab title="T1"><figure src="./missing.png"/></tab><tab title="T2"><md>x</md></tab></tabs>`,
		"figure-group": `<figure-group cols="2"><figure src="./missing.png"/><figure src="./missing2.png"/></figure-group>`,
	} {
		_, issues := Parse([]byte(doc11(body)), missing)
		if got := codes(issues); !slices.Contains(got, "MISSING_ASSET") {
			t.Errorf("%s: codes = %v, want MISSING_ASSET\n%+v", name, got, issues)
		}
	}

	// Nested <stages> indirectly via <stage><section><stages>
	indirect := doc11(`<stages id="g1"><stage id="s1" title="S1"><section title="Inner"><stages id="g2"><stage id="s2" title="S2"><md>x</md></stage></stages></section></stage></stages>`)
	_, issues := Parse([]byte(indirect), Options{})
	if got := codes(issues); !slices.Equal(got, []string{"INVALID_STAGES"}) {
		t.Fatalf("indirect nested stages codes = %v, want [INVALID_STAGES]\n%+v", got, issues)
	}
}

func TestInvalidXMLCDATAHint(t *testing.T) {
	cases := []struct {
		name     string
		xml      string
		wantHint bool
	}{
		{
			name:     "single-line bare ampersand inside md (#44)",
			xml:      doc11(`<md>A & B</md>`),
			wantHint: true,
		},
		{
			name:     "single-line generic angle brackets inside md (#44)",
			xml:      doc11(`<md>Use Vec<T> here</md>`),
			wantHint: true,
		},
		{
			name:     "multi-line generic angle brackets inside md (#44)",
			xml:      "<document version=\"1.1\">" + metaBlock + "\n<body>\n<md>\nreturn Vec<Option<T>>\n</md>\n</body></document>",
			wantHint: true,
		},
		{
			name:     "multi-line bare ampersand inside mermaid",
			xml:      "<document version=\"1.1\">" + metaBlock + "\n<body>\n<mermaid>\nA --> B & C\n</mermaid>\n</body></document>",
			wantHint: true,
		},
		{
			name:     "unrelated unclosed root does not attach CDATA hint",
			xml:      `<document>`,
			wantHint: false,
		},
		{
			name:     "valid CDATA followed by unrelated broken tag does not attach CDATA hint",
			xml:      doc11(`<md><![CDATA[Vec<T> & ok]]></md><section>`),
			wantHint: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, issues := Parse([]byte(tc.xml), Options{})
			if len(issues) != 1 || issues[0].Code != "INVALID_XML" {
				t.Fatalf("issues = %+v, want single INVALID_XML", issues)
			}
			hasCDATA := strings.Contains(issues[0].Hint, "CDATA")
			if hasCDATA != tc.wantHint {
				t.Fatalf("Hint = %q, wantHint=%v (issue=%+v)", issues[0].Hint, tc.wantHint, issues[0])
			}
		})
	}
}
