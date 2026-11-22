package schema

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestSpecConsistencyAndCompleteness(t *testing.T) {
	raw, err := RenderSchemaJSON()
	if err != nil {
		t.Fatalf("RenderSchemaJSON: %v", err)
	}
	var parsed ProtocolSpec
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal schema JSON: %v", err)
	}

	// #25: Every 6.2 tag must appear in Spec().Elements.
	elemMap := make(map[string]ElementSpec, len(parsed.Elements))
	for _, el := range parsed.Elements {
		elemMap[el.Name] = el
	}
	requiredTags := []string{
		"document", "meta", "title", "type", "date", "summary", "source-agent", "canonical-url", "tags", "tag", "related-ids", "id", "body",
		"section", "md", "figure", "paper-card", "repo-card", "compare-table", "columns", "col", "row", "cell", "callout", "collapse", "sources", "source",
		"claim", "equation", "mermaid", "chart", "metrics", "metric", "steps", "step", "tabs", "tab", "timeline", "event",
		"glossary", "term", "references", "ref", "figure-group", "code-block", "quote", "checklist", "check",
		"search-log", "criteria", "include", "exclude", "query", "excluded", "gap", "stop",
		"stages", "stage", "quiz", "question", "option", "explain", "next-steps", "tldr", "point", "verdict", "news-item", "detail",
	}
	for _, tag := range requiredTags {
		if _, ok := elemMap[tag]; !ok {
			t.Errorf("schema JSON missing element %q", tag)
		}
	}

	// #25: Every new attribute listed in PRD 6.2/6.9 must appear on its element.
	wantAttrs := map[string][]string{
		"section":      {"id"},
		"figure":       {"id", "alt"},
		"cell":         {"tag"},
		"paper-card":   {"venue", "doi", "code-url", "read-depth"},
		"repo-card":    {"license", "commit"},
		"claim":        {"level", "source"},
		"equation":     {"id", "number", "caption"},
		"mermaid":      {"caption"},
		"chart":        {"type", "caption"},
		"metric":       {"label", "value", "unit", "delta", "trend"},
		"steps":        {"interactive"},
		"step":         {"title"},
		"tab":          {"title"},
		"event":        {"date", "title"},
		"term":         {"name"},
		"ref":          {"id", "title", "authors", "year", "venue", "url", "doi", "read-depth"},
		"figure-group": {"cols", "caption"},
		"code-block":   {"lang", "file", "lines", "commit", "caption", "eq", "highlight", "diff"},
		"quote":        {"source", "author", "url"},
		"check":        {"status", "detail"},
		"search-log":   {"date"},
		"query":        {"source", "date", "hits", "kept"},
		"excluded":     {"title", "reason"},
		"stages":       {"id", "gated"},
		"stage":        {"id", "title", "level"},
		"quiz":         {"id"},
		"option":       {"correct", "target"},
		"tldr":         {"title", "placement"},
		"news-item":    {"title", "source", "url", "published", "importance", "detail-id"},
	}
	for elemName, expected := range wantAttrs {
		el, ok := elemMap[elemName]
		if !ok {
			t.Fatalf("missing %s", elemName)
		}
		have := make(map[string]bool, len(el.Attributes))
		for _, a := range el.Attributes {
			have[a.Name] = true
		}
		for _, attr := range expected {
			if !have[attr] {
				t.Errorf("element %q missing attribute %q in schema spec", elemName, attr)
			}
		}
	}

	// #69: The structure rules cover every container, with explicit exceptions and document caps.
	if parsed.Version != "1.1" {
		t.Errorf("version = %q, want 1.1", parsed.Version)
	}
	rules := parsed.Structure
	if rules.SectionMaxDepth != maxSectionDepth {
		t.Errorf("section depth = %d, want %d", rules.SectionMaxDepth, maxSectionDepth)
	}
	if !slices.Contains(rules.BodyChildren, "md") {
		t.Errorf("body_children = %v, want md among them", rules.BodyChildren)
	}
	for _, container := range []string{"stages", "stage", "tabs", "tab", "steps", "step", "metrics", "figure-group", "search-log", "quiz", "next-steps", "tldr", "news-item"} {
		if _, ok := rules.Containers[container]; !ok {
			t.Errorf("containers missing %q", container)
		}
	}
	if len(rules.PositionExceptions) == 0 || len(rules.DocumentCaps) == 0 {
		t.Errorf("expected position exceptions and document caps")
	}

	// Verify Spec().MinimalExample passes validator cleanly.
	if _, issues := Parse([]byte(parsed.MinimalExample), Options{}); len(issues) != 0 {
		t.Fatalf("MinimalExample invalid: %+v", issues)
	}

	// Verify all anchor prefixes & #sec-N exclusion.
	anchorSet := make(map[string]AnchorSpec, len(parsed.Anchors))
	for _, a := range parsed.Anchors {
		anchorSet[a.Prefix] = a
	}
	for elem, pref := range anchorPrefixes {
		a, ok := anchorSet["#"+pref]
		if !ok || !a.ValidNextStepsTarget {
			t.Errorf("anchor #%s (%s) missing or invalid target flag", pref, elem)
		}
	}
	if secN, ok := anchorSet["#sec-"]; !ok || secN.ValidNextStepsTarget {
		t.Errorf("positional #sec- anchor must exist with ValidNextStepsTarget=false, got %+v", secN)
	}

	// Verify Markdown output shares the same rule table cleanly.
	md := RenderSchemaMarkdown()
	for _, needle := range []string{
		"UNSUPPORTED_VERSION", "SECTION_TOO_DEEP",
		"INVALID_CHART", "<search-log>", "<stages>", "<tldr>", "<news-item>", `version="1.1"`,
	} {
		if !strings.Contains(md, needle) {
			t.Errorf("RenderSchemaMarkdown() missing %q", needle)
		}
	}
}

// TestSpecContainerRulesMatchValidator checks every declared parent/child pair of the block
// containers against the real validator, so Spec() and protocol.sch cannot drift apart again.
func TestSpecContainerRulesMatchValidator(t *testing.T) {
	always := Options{HasAsset: func(string) bool { return true }}
	containers := Spec().Structure.Containers

	snippets := map[string]string{
		"section":       `<section title="x"/>`,
		"sources":       `<sources><source title="t" url="https://example.com"/></sources>`,
		"md":            `<md>x</md>`,
		"figure":        `<figure src="a.png"/>`,
		"paper-card":    `<paper-card title="t" url="https://example.com"/>`,
		"repo-card":     `<repo-card name="n" url="https://example.com"/>`,
		"compare-table": `<compare-table><columns><col>A</col></columns><row><cell>1</cell></row></compare-table>`,
		"callout":       `<callout type="info">x</callout>`,
		"collapse":      `<collapse title="c"><md>x</md></collapse>`,
		"claim":         `<claim level="claim">x</claim>`,
		"equation":      `<equation>x</equation>`,
		"mermaid":       `<mermaid>graph TD;A-->B</mermaid>`,
		"chart":         `<chart type="bar"><![CDATA[{"categories":["a"],"series":[{"name":"s","data":[1]}]}]]></chart>`,
		"metrics":       `<metrics><metric label="l" value="1"/></metrics>`,
		"steps":         `<steps><step title="s"><md>x</md></step></steps>`,
		"tabs":          `<tabs><tab title="a"><md>x</md></tab><tab title="b"><md>y</md></tab></tabs>`,
		"timeline":      `<timeline><event date="2024" title="e">d</event></timeline>`,
		"glossary":      `<glossary><term name="t">d</term></glossary>`,
		"references":    `<references><ref id="r1" title="t"/></references>`,
		"figure-group":  `<figure-group cols="2"><figure src="a.png"/><figure src="b.png"/></figure-group>`,
		"code-block":    `<code-block>x</code-block>`,
		"quote":         `<quote>x</quote>`,
		"checklist":     `<checklist><check status="done">x</check></checklist>`,
		"search-log":    `<search-log><query source="s">q</query></search-log>`,
		"stages":        `<stages id="g"><stage id="g1" title="s"/></stages>`,
		"quiz":          `<quiz id="q"><question>Q</question><option correct="true">A</option><option>B</option></quiz>`,
		"next-steps":    `<next-steps><option target="#s-anchor">go</option></next-steps>`,
		"tldr":          `<tldr><point>p</point></tldr>`,
		"news-item":     `<news-item title="n">s</news-item>`,
	}
	wrappers := map[string]func(string) string{
		"body":     func(s string) string { return s },
		"section":  func(s string) string { return `<section title="w">` + s + `</section>` },
		"stage":    func(s string) string { return `<stages id="w"><stage id="w1" title="w">` + s + `</stage></stages>` },
		"collapse": func(s string) string { return `<collapse title="w">` + s + `</collapse>` },
		"tab": func(s string) string {
			return `<tabs><tab title="a">` + s + `</tab><tab title="b"><md>y</md></tab></tabs>`
		},
		"step": func(s string) string { return `<steps><step title="a">` + s + `</step></steps>` },
	}

	for _, block := range allBlocks {
		if _, ok := snippets[block]; !ok {
			t.Fatalf("test has no snippet for block tag %q; add one", block)
		}
	}
	for container, wrap := range wrappers {
		rule, ok := containers[container]
		if !ok {
			t.Fatalf("Spec() has no container %q", container)
		}
		for _, block := range allBlocks {
			want := slices.Contains(rule.AllowedChildren, block)
			input := doc(`<section id="anchor" title="anchor"/>` + wrap(snippets[block]))
			_, issues := Parse([]byte(input), always)
			if want && len(issues) != 0 {
				t.Errorf("Spec() allows <%s> inside <%s> but validator rejects it: %+v", block, container, issues)
			}
			if !want && len(issues) == 0 {
				t.Errorf("Spec() forbids <%s> inside <%s> but validator accepts it", block, container)
			}
			// The parent side of the declaration must agree with the child side.
			if blockRule, ok := containers[block]; ok {
				if got := slices.Contains(blockRule.AllowedParents, container); got != want {
					t.Errorf("Spec() disagrees with itself: <%s> children allow <%s>=%v but <%s> parents allow <%s>=%v", container, block, want, block, container, got)
				}
			}
		}
	}
}
