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

	// #69: Version diff 1.0 vs 1.1 is explicit (containers, parents/children, counts, exceptions, error codes).
	v10, ok10 := parsed.Versions["1.0"]
	v11, ok11 := parsed.Versions["1.1"]
	if !ok10 || !ok11 {
		t.Fatalf("Versions keys missing: %+v", parsed.Versions)
	}
	if v10.SectionMaxDepth != 1 || v11.SectionMaxDepth != maxSectionDepth {
		t.Errorf("section depth diff = 1.0:%d 1.1:%d", v10.SectionMaxDepth, v11.SectionMaxDepth)
	}
	if slices.Contains(v10.BodyChildren, "md") || !slices.Contains(v11.BodyChildren, "md") {
		t.Errorf("body_children diff broken: v10=%v", v10.BodyChildren)
	}
	for _, container := range []string{"stages", "stage", "tabs", "tab", "steps", "step", "metrics", "figure-group", "search-log", "quiz", "next-steps", "tldr", "news-item"} {
		if _, ok := v11.Containers[container]; !ok {
			t.Errorf("v1.1 containers missing %q", container)
		}
	}
	if len(v11.PositionExceptions) == 0 || len(v11.DocumentCaps) == 0 {
		t.Errorf("expected v1.1 position exceptions and document caps")
	}

	// Verify Spec().MinimalExample passes validator cleanly.
	if _, issues := Parse([]byte(parsed.MinimalExample), Options{}); len(issues) != 0 {
		t.Fatalf("MinimalExample invalid: %+v", issues)
	}

	// Verify validator v1.0 gating covers every v1.1-only tag in Spec().Elements.
	for _, el := range parsed.Elements {
		if el.Since != "1.1" {
			continue
		}
		xml := doc(`<section title="s"><` + el.Name + `/></section>`)
		_, issues := Parse([]byte(xml), Options{})
		if got := codes(issues); len(got) == 0 || !slices.Contains(got, "TAG_REQUIRES_1_1") || slices.Contains(got, "UNKNOWN_TAG") {
			t.Errorf("v1.1 element %q in v1.0: got codes %v, want [TAG_REQUIRES_1_1] without UNKNOWN_TAG\nissues=%+v", el.Name, got, issues)
		}
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
		"Version `1.0`", "Version `1.1`", "TAG_REQUIRES_1_1", "SECTION_TOO_DEEP",
		"INVALID_CHART", "<search-log>", "<stages>", "<tldr>", "<news-item>", `version="1.1"`,
	} {
		if !strings.Contains(md, needle) {
			t.Errorf("RenderSchemaMarkdown() missing %q", needle)
		}
	}
}
