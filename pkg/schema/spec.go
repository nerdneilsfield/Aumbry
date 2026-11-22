package schema

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ProtocolSpec is the single version-aware source of truth for the Aumbry XML protocol
// used by `aumbry schema --format md|json` and validated against runtime rules in tests.
type ProtocolSpec struct {
	Versions       map[string]VersionRules `json:"versions"`
	Elements       []ElementSpec           `json:"elements"`
	Anchors        []AnchorSpec            `json:"anchors"`
	ErrorCodes     []ErrorCodeSpec         `json:"error_codes"`
	MinimalExample string                  `json:"minimal_example"`
}

// VersionRules summarizes container rules, depth caps, limits, and exceptions per protocol version.
type VersionRules struct {
	Version            string                   `json:"version"`
	BodyChildren       []string                 `json:"body_children"`
	SectionMaxDepth    int                      `json:"section_max_depth"`
	SectionDepthNote   string                   `json:"section_depth_note"`
	Containers         map[string]ContainerRule `json:"containers"`
	DocumentCaps       map[string]int           `json:"document_caps,omitempty"`
	PositionExceptions []string                 `json:"position_exceptions"`
}

// ContainerRule defines allowed parent/child relationships and child counts for a container tag.
type ContainerRule struct {
	AllowedParents  []string `json:"allowed_parents"`
	AllowedChildren []string `json:"allowed_children"`
	MinChildren     int      `json:"min_children,omitempty"`
	MaxChildren     int      `json:"max_children,omitempty"`
	DirectText      bool     `json:"direct_text"`
	Notes           string   `json:"notes,omitempty"`
}

// ElementSpec documents an XML element across v1.0 and v1.1, including version-gated attributes.
type ElementSpec struct {
	Name        string     `json:"name"`
	Since       string     `json:"since"`
	Parents     []string   `json:"parents"`
	Children    []string   `json:"children,omitempty"`
	MinChildren int        `json:"min_children,omitempty"`
	MaxChildren int        `json:"max_children,omitempty"`
	DocMax      int        `json:"doc_max,omitempty"`
	MaxBytes    int        `json:"max_bytes,omitempty"`
	MaxRunes    int        `json:"max_runes,omitempty"`
	TextMode    string     `json:"text_mode"`
	Attributes  []AttrSpec `json:"attributes,omitempty"`
	Exceptions  []string   `json:"exceptions,omitempty"`
	Description string     `json:"description"`
}

// AttrSpec describes a single attribute on an element.
type AttrSpec struct {
	Name        string   `json:"name"`
	Since       string   `json:"since"`
	Required    bool     `json:"required,omitempty"`
	Values      []string `json:"values,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Description string   `json:"description"`
}

// AnchorSpec describes how element IDs map to URL hash anchors and next-steps targets.
type AnchorSpec struct {
	Element              string `json:"element"`
	Prefix               string `json:"prefix"`
	Format               string `json:"format"`
	ValidNextStepsTarget bool   `json:"valid_next_steps_target"`
}

// ErrorCodeSpec documents a machine-readable validation code and when it is emitted.
type ErrorCodeSpec struct {
	Code    string `json:"code"`
	Since   string `json:"since"`
	Trigger string `json:"trigger"`
}

var (
	v10SectionBlocks = []string{
		"md", "figure", "paper-card", "repo-card", "compare-table", "callout", "collapse", "sources",
	}
	v11LeafBlocks = []string{
		"md", "figure", "callout", "claim", "equation", "mermaid", "chart", "code-block", "quote",
	}
	v11AllBlocks = []string{
		"section", "sources", "md", "figure", "paper-card", "repo-card", "compare-table", "callout", "collapse",
		"claim", "equation", "mermaid", "chart", "metrics", "steps", "tabs", "timeline", "glossary",
		"references", "figure-group", "code-block", "quote", "checklist", "search-log", "stages",
		"quiz", "next-steps", "tldr", "news-item",
	}
)

// Spec returns the version-aware Aumbry XML protocol specification.
func Spec() ProtocolSpec {
	v11Collapse := append(slices.Clone(v11LeafBlocks), "search-log")
	v11StageChildren := slices.DeleteFunc(slices.Clone(v11AllBlocks), func(s string) bool { return s == "stages" })

	return ProtocolSpec{
		Versions: map[string]VersionRules{
			"1.0": {
				Version:          "1.0",
				BodyChildren:     []string{"section", "sources"},
				SectionMaxDepth:  1,
				SectionDepthNote: "top-level <section> inside <body> only; no nested <section>",
				Containers: map[string]ContainerRule{
					"body":          {AllowedParents: []string{"document"}, AllowedChildren: []string{"section", "sources"}},
					"section":       {AllowedParents: []string{"body"}, AllowedChildren: slices.Clone(v10SectionBlocks)},
					"collapse":      {AllowedParents: []string{"section"}, AllowedChildren: []string{"md", "figure"}},
					"compare-table": {AllowedParents: []string{"section"}, AllowedChildren: []string{"columns", "row"}},
					"sources":       {AllowedParents: []string{"body", "section"}, AllowedChildren: []string{"source"}, MinChildren: 1},
				},
				PositionExceptions: []string{
					"all v1.1-only elements and new v1.1 attributes on v1.0 elements fail with TAG_REQUIRES_1_1 (no duplicate UNKNOWN_TAG)",
					"collapse only allowed directly inside section and only accepts md and figure",
				},
			},
			"1.1": {
				Version:          "1.1",
				BodyChildren:     slices.Clone(v11AllBlocks),
				SectionMaxDepth:  maxSectionDepth,
				SectionDepthNote: "at most 3 nested <section> levels counting only <section> ancestors (<stage> does not count)",
				Containers: map[string]ContainerRule{
					"body":          {AllowedParents: []string{"document"}, AllowedChildren: slices.Clone(v11AllBlocks)},
					"section":       {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: slices.Clone(v11AllBlocks), Notes: "section depth <= 3"},
					"stages":        {AllowedParents: []string{"body", "section"}, AllowedChildren: []string{"stage"}, MinChildren: 1, MaxChildren: maxStagesPerGroup, Notes: "cannot be nested inside any stage descendent"},
					"stage":         {AllowedParents: []string{"stages"}, AllowedChildren: v11StageChildren, Notes: "allows all body/section blocks and <section>, except nested <stages>"},
					"collapse":      {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: v11Collapse, Notes: "allows search-log in addition to leaf blocks"},
					"tab":           {AllowedParents: []string{"tabs"}, AllowedChildren: slices.Clone(v11LeafBlocks)},
					"tabs":          {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"tab"}, MinChildren: 2},
					"step":          {AllowedParents: []string{"steps"}, AllowedChildren: slices.Clone(v11LeafBlocks)},
					"steps":         {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"step"}, MinChildren: 1, Notes: `interactive="true" requires >= 2 <step>`},
					"metrics":       {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"metric"}, MinChildren: 1, MaxChildren: maxMetricsCount},
					"timeline":      {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"event"}, MinChildren: 1},
					"glossary":      {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"term"}, MinChildren: 1},
					"references":    {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"ref"}, MinChildren: 1},
					"figure-group":  {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"figure"}, MinChildren: minFigureGroupItems, MaxChildren: maxFigureGroupItems},
					"compare-table": {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"columns", "row"}},
					"columns":       {AllowedParents: []string{"compare-table"}, AllowedChildren: []string{"col"}, MinChildren: 1},
					"row":           {AllowedParents: []string{"compare-table"}, AllowedChildren: []string{"cell"}, Notes: "cell count must match columns/col count"},
					"checklist":     {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"check"}, MinChildren: 1},
					"search-log":    {AllowedParents: []string{"body", "section", "stage", "collapse"}, AllowedChildren: slices.Clone(searchLogAllowed), Notes: "requires >= 1 <query>; <= 1 <criteria>; <= 1 <stop>"},
					"criteria":      {AllowedParents: []string{"search-log"}, AllowedChildren: []string{"include", "exclude"}},
					"quiz":          {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: slices.Clone(quizAllowed), Notes: "1 <question>, 2..8 <option> (>= 1 correct=\"true\"), 0..1 <explain>"},
					"next-steps":    {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"option"}, MinChildren: 1, MaxChildren: maxNextStepsOptions},
					"tldr":          {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"point", "verdict"}, MinChildren: 1, Notes: "direct child of body/section/stage only; at most 1 per container; 1..7 <point>; 0..1 <verdict>"},
					"news-item":     {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"detail"}, MaxChildren: 1, DirectText: true, Notes: "direct text summary <= 600 runes; 0..1 <detail> <= 8 KB"},
					"sources":       {AllowedParents: []string{"body", "section", "stage"}, AllowedChildren: []string{"source"}, MinChildren: 1},
				},
				DocumentCaps: map[string]int{
					"mermaid": maxMermaidBlocks,
					"chart":   maxChartBlocks,
					"stages":  maxStagesGroups,
					"stage":   maxTotalStages,
				},
				PositionExceptions: []string{
					"<stages> may appear multiple times in <body> or <section>, but NEVER inside any <stage> descendent (INVALID_STAGES)",
					"<tldr> must be a direct child of <body>, <section>, or <stage>, at most 1 per container (INVALID_TLDR)",
					"<search-log> is allowed inside <collapse> as well as <body>/<section>/<stage>, but not <tab> or <step>",
					"<section> nesting depth <= 3 counts only <section> ancestors (<stage> does not increment section depth)",
					`<steps interactive="true"> requires >= 2 <step> elements`,
					"all IDs across section, stages, stage, quiz, figure, equation, and ref share ONE document-wide namespace (DUPLICATE_ID)",
					"positional #sec-<N> anchors cannot be used as <next-steps> targets (INVALID_TARGET)",
				},
			},
		},
		Elements: buildElementSpecs(),
		Anchors:  buildAnchorSpecs(),
		ErrorCodes: []ErrorCodeSpec{
			{Code: "INVALID_XML", Since: "1.0", Trigger: "malformed XML or disallowed DOCTYPE; includes CDATA hint when unescaped < or & triggers inside text blocks"},
			{Code: "INVALID_ROOT", Since: "1.0", Trigger: "root element is not <document>"},
			{Code: "UNSUPPORTED_VERSION", Since: "1.0", Trigger: `document @version is not "1.0" or "1.1"`},
			{Code: "UNEXPECTED_TEXT", Since: "1.0", Trigger: "non-whitespace text placed directly inside structural containers"},
			{Code: "MISSING_META", Since: "1.0", Trigger: "missing <meta> element"},
			{Code: "MISSING_BODY", Since: "1.0", Trigger: "missing <body> element"},
			{Code: "DUPLICATE_META", Since: "1.0", Trigger: "more than one <meta> element"},
			{Code: "DUPLICATE_BODY", Since: "1.0", Trigger: "more than one <body> element"},
			{Code: "REQUIRED_FIELD", Since: "1.0", Trigger: "missing required <meta> child (<title>, <type>, <date>, <summary>, <source-agent>)"},
			{Code: "INVALID_TYPE", Since: "1.0", Trigger: "<meta>/<type> is not report or post"},
			{Code: "INVALID_DATE", Since: "1.0", Trigger: "invalid date on <meta>/<date>, <event>@date, <news-item>@published, <search-log>@date, or <query>@date"},
			{Code: "INVALID_TAG", Since: "1.0", Trigger: "unexpected or empty element inside <meta>/<tags>"},
			{Code: "INVALID_ID", Since: "1.0", Trigger: "invalid 12-char item id (<meta>/<related-ids>/<id>, paper-card@related-id, news-item@detail-id) or invalid anchor id (^[a-z0-9-]{1,32}$)"},
			{Code: "UNKNOWN_TAG", Since: "1.0", Trigger: "unexpected element inside a container or inside a leaf block"},
			{Code: "MISSING_ATTR", Since: "1.0", Trigger: "required attribute missing or blank"},
			{Code: "INVALID_ATTR", Since: "1.0", Trigger: "invalid enum attribute value, empty <next-steps>/<option> text, or container count constraint (callout@type, claim@level, cell@tag, read-depth, figure-group, tabs count, steps count, glossary term uniqueness, next-steps option count)"},
			{Code: "INVALID_URL", Since: "1.0", Trigger: "URL is not an absolute http(s) URL (canonical-url, paper-card@url|code-url, repo-card@url, source@url, ref@url, quote@url, news-item@url)"},
			{Code: "INVALID_ASSET_PATH", Since: "1.0", Trigger: "<figure src> is not a clean relative path inside document directory"},
			{Code: "INVALID_ASSET_TYPE", Since: "1.0", Trigger: "<figure src> extension is not .png, .jpg, .jpeg, .webp, or .svg"},
			{Code: "MISSING_ASSET", Since: "1.0", Trigger: "referenced local <figure src> file does not exist"},
			{Code: "TABLE_NO_COLUMNS", Since: "1.0", Trigger: "<compare-table> missing <columns>/<col>"},
			{Code: "TABLE_NO_ROWS", Since: "1.0", Trigger: "<compare-table> has no <row>"},
			{Code: "TABLE_COL_MISMATCH", Since: "1.0", Trigger: "<row> <cell> count differs from <columns>/<col> count"},
			{Code: "MISSING_SOURCE", Since: "1.0", Trigger: "<sources> has no <source>"},
			{Code: "TAG_REQUIRES_1_1", Since: "1.1", Trigger: `v1.1 element or v1.1 attribute used inside a version="1.0" document (with upgrade hint)`},
			{Code: "SECTION_TOO_DEEP", Since: "1.1", Trigger: "<section> nested deeper than 3 <section> levels"},
			{Code: "TOO_MANY_BLOCKS", Since: "1.1", Trigger: "document exceeds caps (>10 mermaid, >10 chart, >8 stages groups, >40 total stage elements)"},
			{Code: "DUPLICATE_ID", Since: "1.1", Trigger: "duplicate anchor id across the document-wide shared id namespace"},
			{Code: "DUPLICATE_TAB", Since: "1.1", Trigger: "duplicate <tab title> within the same <tabs>"},
			{Code: "INVALID_TARGET", Since: "1.1", Trigger: `<next-steps>/<option target="#..."> fails to resolve or uses #sec-N; <code-block eq="..."> missing matching <equation id>`},
			{Code: "INVALID_CHART", Since: "1.1", Trigger: "bad chart@type, JSON syntax error, JSON Schema failure, length mismatch, non-finite NaN/Inf, >2000 total points, or invalid series[].err (reported with JSON path)"},
			{Code: "INVALID_EQUATION", Since: "1.1", Trigger: "<equation> empty or exceeds 4 KB"},
			{Code: "INVALID_MERMAID", Since: "1.1", Trigger: "<mermaid> empty or exceeds 20 KB"},
			{Code: "INVALID_CODE_BLOCK", Since: "1.1", Trigger: "<code-block> empty, exceeds 32 KB, invalid diff flag, or invalid highlight line spec"},
			{Code: "INVALID_METRIC", Since: "1.1", Trigger: "<metric> missing/empty label or value, bad trend, or <metrics> outside 1..12 items"},
			{Code: "INVALID_DOI", Since: "1.1", Trigger: `paper-card@doi or ref@doi fails ^10\.\d{4,9}/[-._;()/:A-Za-z0-9]+$`},
			{Code: "INVALID_STAGES", Since: "1.1", Trigger: "nested <stages> inside <stage>, missing stages/stage @id, missing stage @title, invalid @gated, or group outside 1..8 <stage>"},
			{Code: "INVALID_QUIZ", Since: "1.1", Trigger: "missing @id, question count != 1, option outside 2..8, zero correct=\"true\" options, >1 explain, or child XML tags inside fields"},
			{Code: "INVALID_TLDR", Since: "1.1", Trigger: "<tldr> outside body/section/stage, >1 in same container, bad placement, 0 or >7 <point>, >1 <verdict>, or >300 Unicode runes"},
			{Code: "INVALID_NEWS", Since: "1.1", Trigger: "<news-item> missing title, invalid importance, >1 <detail>, summary >600 runes, or <detail> >8 KB"},
			{Code: "INVALID_CHECK", Since: "1.1", Trigger: "<checklist> empty or <check status> not in done|todo|blocked|risk"},
			{Code: "INVALID_SEARCH_LOG", Since: "1.1", Trigger: "<search-log> missing <query>, >1 <criteria>/<stop>, or non-integer negative hits/kept"},
		},
		MinimalExample: `<?xml version="1.0" encoding="UTF-8"?>
<document version="1.1">
  <meta>
    <title>Minimal v1.1 Document</title>
    <type>report</type>
    <date>2026-11-20</date>
    <summary>One-line summary for cards and search results.</summary>
    <source-agent>research-agent</source-agent>
  </meta>
  <body>
    <md><![CDATA[Hello **Aumbry v1.1**.]]></md>
  </body>
</document>`,
	}
}

func buildAnchorSpecs() []AnchorSpec {
	order := []string{"section", "stages", "stage", "quiz", "figure", "equation", "ref"}
	out := make([]AnchorSpec, 0, len(order)+1)
	for _, elem := range order {
		prefix := anchorPrefixes[elem]
		out = append(out, AnchorSpec{
			Element:              elem,
			Prefix:               "#" + prefix,
			Format:               "#" + prefix + "<id>",
			ValidNextStepsTarget: true,
		})
	}
	out = append(out, AnchorSpec{
		Element:              "section (without id)",
		Prefix:               "#sec-",
		Format:               "#sec-<N>",
		ValidNextStepsTarget: false,
	})
	return out
}

func buildElementSpecs() []ElementSpec {
	return []ElementSpec{
		{
			Name: "document", Since: "1.0", Parents: []string{"(root)"}, Children: []string{"meta", "body"},
			TextMode: "none", Description: "Root element of every Aumbry XML document.",
			Attributes: []AttrSpec{{Name: "version", Since: "1.0", Required: true, Values: []string{"1.0", "1.1"}, Description: "Protocol version"}},
		},
		{
			Name: "meta", Since: "1.0", Parents: []string{"document"}, Children: []string{"title", "type", "date", "summary", "source-agent", "canonical-url", "tags", "related-ids"},
			MinChildren: 5, TextMode: "none", Description: "Required document metadata.",
		},
		{Name: "title", Since: "1.0", Parents: []string{"meta"}, TextMode: "text", Description: "Document title (required)."},
		{Name: "type", Since: "1.0", Parents: []string{"meta"}, TextMode: "text", Description: `Document type: "report" or "post" (required).`},
		{Name: "date", Since: "1.0", Parents: []string{"meta"}, TextMode: "text", Description: "ISO YYYY-MM-DD calendar date (required)."},
		{Name: "summary", Since: "1.0", Parents: []string{"meta"}, TextMode: "text", Description: "One-line card/search summary (required)."},
		{Name: "source-agent", Since: "1.0", Parents: []string{"meta"}, TextMode: "text", Description: "Origin agent identifier (required)."},
		{Name: "canonical-url", Since: "1.0", Parents: []string{"meta"}, TextMode: "text", Description: "Optional http(s) canonical URL."},
		{Name: "tags", Since: "1.0", Parents: []string{"meta"}, Children: []string{"tag"}, TextMode: "none", Description: "Optional list of topic tags."},
		{Name: "tag", Since: "1.0", Parents: []string{"tags"}, TextMode: "text", Description: "Non-empty tag string."},
		{Name: "related-ids", Since: "1.0", Parents: []string{"meta"}, Children: []string{"id"}, TextMode: "none", Description: "Optional list of related 12-char item IDs."},
		{Name: "id", Since: "1.0", Parents: []string{"related-ids"}, TextMode: "text", Description: "12-char item ID matching ^[A-Za-z0-9_-]{12}$."},
		{
			Name: "body", Since: "1.0", Parents: []string{"document"}, Children: slices.Clone(v11AllBlocks),
			TextMode: "none", Description: "Document body (1.0 allows section/sources; 1.1 allows all block tags directly).",
			Exceptions: []string{"version=1.0 restricts children to section and sources"},
		},
		{
			Name: "section", Since: "1.0", Parents: []string{"body", "section", "stage"}, Children: slices.Clone(v11AllBlocks),
			TextMode: "none", Description: "Titled section (nestable up to 3 section levels in 1.1).",
			Exceptions: []string{"version=1.0 allows section only under body (no nesting) and restricts children to v1.0 section blocks"},
			Attributes: []AttrSpec{
				{Name: "title", Since: "1.0", Required: true, Description: "Section heading"},
				{Name: "id", Since: "1.1", Pattern: anchorIDRe.String(), Description: "Unique anchor id mapped to #s-<id>"},
			},
		},
		{Name: "md", Since: "1.0", Parents: []string{"body", "section", "stage", "collapse", "tab", "step"}, TextMode: "markdown-cdata", Description: "Rich Markdown content block (wrap in CDATA)."},
		{
			Name: "figure", Since: "1.0", Parents: []string{"body", "section", "stage", "collapse", "tab", "step", "figure-group"},
			TextMode: "none", Description: "Local image asset reference (.png/.jpg/.jpeg/.webp/.svg).",
			Attributes: []AttrSpec{
				{Name: "src", Since: "1.0", Required: true, Description: "Relative local asset path"},
				{Name: "caption", Since: "1.0", Description: "Figure caption (indexed)"},
				{Name: "id", Since: "1.1", Pattern: anchorIDRe.String(), Description: "Unique anchor id mapped to #fig-<id>"},
				{Name: "alt", Since: "1.1", Description: "Accessibility alt text (falls back to caption)"},
			},
		},
		{
			Name: "paper-card", Since: "1.0", Parents: []string{"body", "section", "stage"}, TextMode: "markdown-cdata",
			Description: "Academic paper summary card.",
			Attributes: []AttrSpec{
				{Name: "title", Since: "1.0", Required: true, Description: "Paper title"},
				{Name: "url", Since: "1.0", Required: true, Description: "Paper http(s) URL"},
				{Name: "authors", Since: "1.0", Description: "Authors list (indexed)"},
				{Name: "date", Since: "1.0", Description: "Publication date"},
				{Name: "related-id", Since: "1.0", Pattern: itemIDRe.String(), Description: "Linked 12-char item ID"},
				{Name: "venue", Since: "1.1", Description: "Conference or journal venue"},
				{Name: "doi", Since: "1.1", Pattern: doiRe.String(), Description: "Bare DOI (rendered as https://doi.org/<doi>)"},
				{Name: "code-url", Since: "1.1", Description: "Implementation repository http(s) URL"},
				{Name: "read-depth", Since: "1.1", Values: slices.Clone(readDepths), Description: "Reading depth badge"},
			},
		},
		{
			Name: "repo-card", Since: "1.0", Parents: []string{"body", "section", "stage"}, TextMode: "markdown-cdata",
			Description: "Open-source repository summary card.",
			Attributes: []AttrSpec{
				{Name: "name", Since: "1.0", Required: true, Description: "Repository owner/name"},
				{Name: "url", Since: "1.0", Required: true, Description: "Repository http(s) URL"},
				{Name: "stars", Since: "1.0", Description: "Star count display string"},
				{Name: "lang", Since: "1.0", Description: "Primary language"},
				{Name: "license", Since: "1.1", Description: "SPDX or license label"},
				{Name: "commit", Since: "1.1", Description: "Pinned commit hash or tag"},
			},
		},
		{
			Name: "compare-table", Since: "1.0", Parents: []string{"body", "section", "stage"}, Children: []string{"columns", "row"},
			TextMode: "none", Description: "Structured comparison table.",
			Attributes: []AttrSpec{{Name: "caption", Since: "1.0", Description: "Optional table caption"}},
		},
		{Name: "columns", Since: "1.0", Parents: []string{"compare-table"}, Children: []string{"col"}, MinChildren: 1, TextMode: "none", Description: "Column header container."},
		{Name: "col", Since: "1.0", Parents: []string{"columns"}, TextMode: "text", Description: "Table column header (leaf element in 1.1; disallows child elements)."},
		{Name: "row", Since: "1.0", Parents: []string{"compare-table"}, Children: []string{"cell"}, TextMode: "none", Description: "Table row matching column count."},
		{
			Name: "cell", Since: "1.0", Parents: []string{"row"}, TextMode: "text", Description: "Table cell (leaf element in 1.1; disallows child elements).",
			Attributes: []AttrSpec{{Name: "tag", Since: "1.1", Values: slices.Clone(cellTags), Description: "Semantic comparison badge"}},
		},
		{
			Name: "callout", Since: "1.0", Parents: []string{"body", "section", "stage", "collapse", "tab", "step"}, TextMode: "markdown-cdata",
			Description: "Highlighted callout box.",
			Attributes: []AttrSpec{
				{Name: "type", Since: "1.0", Required: true, Values: []string{"info", "warn", "insight"}, Description: "Callout style"},
				{Name: "title", Since: "1.0", Description: "Optional callout title"},
			},
		},
		{
			Name: "collapse", Since: "1.0", Parents: []string{"body", "section", "stage"}, Children: append(slices.Clone(v11LeafBlocks), "search-log"),
			TextMode: "none", Description: "Collapsible block (v1.0: md/figure; v1.1: leaf blocks + search-log).",
			Exceptions: []string{"version=1.0 restricts children to md and figure"},
			Attributes: []AttrSpec{{Name: "title", Since: "1.0", Required: true, Description: "Toggle header"}},
		},
		{Name: "sources", Since: "1.0", Parents: []string{"body", "section", "stage"}, Children: []string{"source"}, MinChildren: 1, TextMode: "none", Description: "External link list."},
		{
			Name: "source", Since: "1.0", Parents: []string{"sources"}, TextMode: "none", Description: "External source link (leaf element in 1.1; disallows child elements).",
			Attributes: []AttrSpec{
				{Name: "title", Since: "1.0", Required: true, Description: "Source label"},
				{Name: "url", Since: "1.0", Required: true, Description: "http(s) URL"},
			},
		},
		{
			Name: "claim", Since: "1.1", Parents: []string{"body", "section", "stage", "collapse", "tab", "step"}, TextMode: "markdown-cdata",
			Description: "Annotated assertion/evidence/inference/unverified block.",
			Attributes: []AttrSpec{
				{Name: "level", Since: "1.1", Required: true, Values: slices.Clone(claimLevels), Description: "Epistemic level"},
				{Name: "source", Since: "1.1", Description: "Inline paper location reference (e.g. Fig. 4)"},
			},
		},
		{
			Name: "equation", Since: "1.1", Parents: []string{"body", "section", "stage", "collapse", "tab", "step"},
			MaxBytes: maxEquationBytes, TextMode: "latex", Description: "Display LaTeX equation (non-empty, <= 4 KB; body excluded from search).",
			Attributes: []AttrSpec{
				{Name: "id", Since: "1.1", Pattern: anchorIDRe.String(), Description: "Unique anchor id mapped to #eq-<id> and referenced by code-block@eq"},
				{Name: "number", Since: "1.1", Description: "Display equation number"},
				{Name: "caption", Since: "1.1", Description: "Searchable caption"},
			},
		},
		{
			Name: "mermaid", Since: "1.1", Parents: []string{"body", "section", "stage", "collapse", "tab", "step"},
			DocMax: maxMermaidBlocks, MaxBytes: maxMermaidBytes, TextMode: "mermaid-cdata",
			Description: "Mermaid diagram source (non-empty, <= 20 KB, max 10 per document; source excluded from search).",
			Attributes:  []AttrSpec{{Name: "caption", Since: "1.1", Description: "Searchable diagram caption"}},
		},
		{
			Name: "chart", Since: "1.1", Parents: []string{"body", "section", "stage", "collapse", "tab", "step"},
			DocMax: maxChartBlocks, TextMode: "chart-json-cdata",
			Description: "Minimal chart JSON validated against chart_schema.json + cross-field checks (<= 20 series, <= 2000 total points, max 10 charts per document; JSON excluded from search).",
			Attributes: []AttrSpec{
				{Name: "type", Since: "1.1", Required: true, Values: slices.Clone(chartTypes), Description: "Chart kind (scatter disallows categories and err; pie requires 1 series and disallows err)"},
				{Name: "caption", Since: "1.1", Description: "Searchable chart caption"},
			},
		},
		{
			Name: "metrics", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: []string{"metric"},
			MinChildren: 1, MaxChildren: maxMetricsCount, TextMode: "none", Description: "KPI metric card grid (1-12 items).",
		},
		{
			Name: "metric", Since: "1.1", Parents: []string{"metrics"}, TextMode: "none", Description: "Single KPI metric.",
			Attributes: []AttrSpec{
				{Name: "label", Since: "1.1", Required: true, Description: "Metric label (indexed)"},
				{Name: "value", Since: "1.1", Required: true, Description: "Metric value (indexed)"},
				{Name: "unit", Since: "1.1", Description: "Optional unit"},
				{Name: "delta", Since: "1.1", Description: "Optional change delta"},
				{Name: "trend", Since: "1.1", Values: slices.Clone(metricTrends), Description: "Trend direction"},
			},
		},
		{
			Name: "steps", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: []string{"step"},
			MinChildren: 1, TextMode: "none", Description: "Numbered workflow or reproduction steps.",
			Exceptions: []string{`interactive="true" requires >= 2 <step> children`},
			Attributes: []AttrSpec{{Name: "interactive", Since: "1.1", Values: slices.Clone(boolStrings), Description: "Step-by-step interactive viewer"}},
		},
		{
			Name: "step", Since: "1.1", Parents: []string{"steps"}, Children: slices.Clone(v11LeafBlocks), TextMode: "none", Description: "Single step container.",
			Attributes: []AttrSpec{{Name: "title", Since: "1.1", Required: true, Description: "Step heading"}},
		},
		{
			Name: "tabs", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: []string{"tab"},
			MinChildren: 2, TextMode: "none", Description: "Tabbed container requiring at least 2 uniquely titled tabs.",
		},
		{
			Name: "tab", Since: "1.1", Parents: []string{"tabs"}, Children: slices.Clone(v11LeafBlocks), TextMode: "none", Description: "Single tab panel.",
			Attributes: []AttrSpec{{Name: "title", Since: "1.1", Required: true, Description: "Unique tab title within parent <tabs>"}},
		},
		{Name: "timeline", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: []string{"event"}, MinChildren: 1, TextMode: "none", Description: "Chronological event timeline."},
		{
			Name: "event", Since: "1.1", Parents: []string{"timeline"}, TextMode: "markdown-cdata", Description: "Timeline entry.",
			Attributes: []AttrSpec{
				{Name: "date", Since: "1.1", Required: true, Pattern: `^\d{4}(-\d{2}(-\d{2})?)?$`, Description: "YYYY, YYYY-MM, or YYYY-MM-DD"},
				{Name: "title", Since: "1.1", Required: true, Description: "Event heading"},
			},
		},
		{Name: "glossary", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: []string{"term"}, MinChildren: 1, TextMode: "none", Description: "Definition list."},
		{
			Name: "term", Since: "1.1", Parents: []string{"glossary"}, TextMode: "inline-markdown", Description: "Glossary entry definition.",
			Attributes: []AttrSpec{{Name: "name", Since: "1.1", Required: true, Description: "Unique term name within parent <glossary>"}},
		},
		{Name: "references", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: []string{"ref"}, MinChildren: 1, TextMode: "none", Description: "Numbered bibliography referenced via #ref-<id>."},
		{
			Name: "ref", Since: "1.1", Parents: []string{"references"}, TextMode: "none", Description: "Academic bibliographic entry.",
			Attributes: []AttrSpec{
				{Name: "id", Since: "1.1", Required: true, Pattern: anchorIDRe.String(), Description: "Unique anchor id mapped to #ref-<id>"},
				{Name: "title", Since: "1.1", Required: true, Description: "Reference title"},
				{Name: "authors", Since: "1.1", Description: "Authors (indexed)"},
				{Name: "year", Since: "1.1", Description: "Publication year"},
				{Name: "venue", Since: "1.1", Description: "Publication venue"},
				{Name: "url", Since: "1.1", Description: "http(s) URL"},
				{Name: "doi", Since: "1.1", Pattern: doiRe.String(), Description: "Bare DOI"},
				{Name: "read-depth", Since: "1.1", Values: slices.Clone(readDepths), Description: "Reading depth"},
			},
		},
		{
			Name: "figure-group", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: []string{"figure"},
			MinChildren: minFigureGroupItems, MaxChildren: maxFigureGroupItems, TextMode: "none", Description: "Multi-column figure grid (2-6 figures).",
			Attributes: []AttrSpec{
				{Name: "cols", Since: "1.1", Required: true, Values: slices.Clone(figureGroupCols), Description: "Column count"},
				{Name: "caption", Since: "1.1", Description: "Group caption"},
			},
		},
		{
			Name: "code-block", Since: "1.1", Parents: []string{"body", "section", "stage", "collapse", "tab", "step"},
			MaxBytes: maxCodeBlockBytes, TextMode: "code-cdata", Description: "Syntax-highlighted code or unified diff (<= 32 KB; only file/caption indexed).",
			Attributes: []AttrSpec{
				{Name: "lang", Since: "1.1", Description: "Language name"},
				{Name: "file", Since: "1.1", Description: "Source filename (indexed)"},
				{Name: "lines", Since: "1.1", Description: "Line range display hint (e.g. 66-78)"},
				{Name: "commit", Since: "1.1", Description: "Commit reference"},
				{Name: "caption", Since: "1.1", Description: "Block caption (indexed)"},
				{Name: "eq", Since: "1.1", Pattern: anchorIDRe.String(), Description: "Raw <equation id> mapped equation link"},
				{Name: "highlight", Since: "1.1", Description: `Highlighted line numbers/ranges (e.g. "3,5-7")`},
				{Name: "diff", Since: "1.1", Values: slices.Clone(boolStrings), Description: "Render unified +/- diff highlighting"},
			},
		},
		{
			Name: "quote", Since: "1.1", Parents: []string{"body", "section", "stage", "collapse", "tab", "step"}, TextMode: "markdown-cdata",
			Description: "Verbatim excerpt distinct from analytical prose.",
			Attributes: []AttrSpec{
				{Name: "source", Since: "1.1", Description: "Location citation (indexed)"},
				{Name: "author", Since: "1.1", Description: "Speaker/author (indexed)"},
				{Name: "url", Since: "1.1", Description: "Source http(s) URL"},
			},
		},
		{Name: "checklist", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: []string{"check"}, MinChildren: 1, TextMode: "none", Description: "Read-only verification checklist."},
		{
			Name: "check", Since: "1.1", Parents: []string{"checklist"}, TextMode: "text", Description: "Single checklist item.",
			Attributes: []AttrSpec{
				{Name: "status", Since: "1.1", Required: true, Values: slices.Clone(checkStatuses), Description: "Status badge"},
				{Name: "detail", Since: "1.1", Description: "Optional detail annotation (indexed)"},
			},
		},
		{
			Name: "search-log", Since: "1.1", Parents: []string{"body", "section", "stage", "collapse"}, Children: slices.Clone(searchLogAllowed),
			TextMode: "none", Description: "Audit log for literature search workflows.",
			Attributes: []AttrSpec{{Name: "date", Since: "1.1", Pattern: `^\d{4}-\d{2}-\d{2}$`, Description: "Search date YYYY-MM-DD"}},
		},
		{Name: "criteria", Since: "1.1", Parents: []string{"search-log"}, Children: []string{"include", "exclude"}, TextMode: "none", Description: "At most 1 inclusion/exclusion criteria group."},
		{Name: "include", Since: "1.1", Parents: []string{"criteria"}, TextMode: "text", Description: "Inclusion criterion (indexed)."},
		{Name: "exclude", Since: "1.1", Parents: []string{"criteria"}, TextMode: "text", Description: "Exclusion criterion (indexed)."},
		{
			Name: "query", Since: "1.1", Parents: []string{"search-log"}, TextMode: "text", Description: "Executed search query (>= 1 required inside <search-log>).",
			Attributes: []AttrSpec{
				{Name: "source", Since: "1.1", Description: "Search source/database"},
				{Name: "date", Since: "1.1", Pattern: `^\d{4}-\d{2}-\d{2}$`, Description: "YYYY-MM-DD"},
				{Name: "hits", Since: "1.1", Description: "Non-negative hit count"},
				{Name: "kept", Since: "1.1", Description: "Non-negative retained count"},
			},
		},
		{
			Name: "excluded", Since: "1.1", Parents: []string{"search-log"}, TextMode: "none", Description: "Excluded candidate item (title and reason are indexed).",
			Attributes: []AttrSpec{
				{Name: "title", Since: "1.1", Description: "Item title"},
				{Name: "reason", Since: "1.1", Description: "Exclusion reason"},
			},
		},
		{Name: "gap", Since: "1.1", Parents: []string{"search-log"}, TextMode: "text", Description: "Identified literature gap (indexed)."},
		{Name: "stop", Since: "1.1", Parents: []string{"search-log"}, TextMode: "text", Description: "Stopping rule (at most 1 inside <search-log>)."},
		{
			Name: "stages", Since: "1.1", Parents: []string{"body", "section"}, Children: []string{"stage"},
			MinChildren: 1, MaxChildren: maxStagesPerGroup, DocMax: maxStagesGroups, TextMode: "none",
			Description: "Independent progressive-reading group (1-8 stages; max 8 groups per document; never nested inside stage).",
			Exceptions:  []string{"forbidden inside any <stage> descendent"},
			Attributes: []AttrSpec{
				{Name: "id", Since: "1.1", Required: true, Pattern: anchorIDRe.String(), Description: "Unique anchor id mapped to #stages-<id>"},
				{Name: "gated", Since: "1.1", Values: slices.Clone(boolStrings), Description: `Progressive reveal gate (default "true")`},
			},
		},
		{
			Name: "stage", Since: "1.1", Parents: []string{"stages"}, Children: slices.DeleteFunc(slices.Clone(v11AllBlocks), func(s string) bool { return s == "stages" }),
			DocMax: maxTotalStages, TextMode: "none", Description: "Single progressive stage (max 40 total across document).",
			Attributes: []AttrSpec{
				{Name: "id", Since: "1.1", Required: true, Pattern: anchorIDRe.String(), Description: "Unique anchor id mapped to #stage-<id>"},
				{Name: "title", Since: "1.1", Required: true, Description: "Stage heading"},
				{Name: "level", Since: "1.1", Description: "Optional difficulty/round label"},
			},
		},
		{
			Name: "quiz", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: slices.Clone(quizAllowed),
			TextMode: "none", Description: "Inline interactive self-check quiz (options and correctness excluded from search).",
			Attributes: []AttrSpec{{Name: "id", Since: "1.1", Required: true, Pattern: anchorIDRe.String(), Description: "Unique anchor id mapped to #quiz-<id>"}},
		},
		{Name: "question", Since: "1.1", Parents: []string{"quiz"}, TextMode: "text", Description: "Exactly 1 non-empty plain-text question inside <quiz> (indexed)."},
		{
			Name: "option", Since: "1.1", Parents: []string{"quiz", "next-steps"}, TextMode: "text",
			Description: "Quiz answer choice (2-8 per quiz; >=1 correct=\"true\"; excluded from search) OR non-empty navigation button inside <next-steps> (1-5 per block; excluded from search).",
			Attributes: []AttrSpec{
				{Name: "correct", Since: "1.1", Values: slices.Clone(boolStrings), Description: "Quiz correctness flag (valid inside <quiz>)"},
				{Name: "target", Since: "1.1", Pattern: targetRe.String(), Description: "Prefixed in-page anchor target (required inside <next-steps>)"},
			},
		},
		{Name: "explain", Since: "1.1", Parents: []string{"quiz"}, TextMode: "text", Description: "Optional explanation shown after quiz submission (0-1 per quiz; indexed)."},
		{Name: "next-steps", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: []string{"option"}, MinChildren: 1, MaxChildren: maxNextStepsOptions, TextMode: "none", Description: "In-document action buttons targeting anchored sections/stages/figures/equations/quizzes/refs."},
		{
			Name: "tldr", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: []string{"point", "verdict"},
			MinChildren: 1, TextMode: "none", Description: "Key takeaways card (direct child of body/section/stage only; at most 1 per container).",
			Exceptions: []string{"only direct child of body, section, or stage; at most 1 per parent container"},
			Attributes: []AttrSpec{
				{Name: "title", Since: "1.1", Description: "Card heading"},
				{Name: "placement", Since: "1.1", Values: slices.Clone(tldrPlacements), Description: "Display placement (top or bottom)"},
			},
		},
		{Name: "point", Since: "1.1", Parents: []string{"tldr"}, MaxRunes: maxTLDRRunes, TextMode: "inline-markdown", Description: "Single takeaway point (1-7 per tldr, non-empty, <= 300 Unicode runes)."},
		{Name: "verdict", Since: "1.1", Parents: []string{"tldr"}, MaxRunes: maxTLDRRunes, TextMode: "inline-markdown", Description: "Optional summary verdict (0-1 per tldr, non-empty, <= 300 Unicode runes)."},
		{
			Name: "news-item", Since: "1.1", Parents: []string{"body", "section", "stage"}, Children: []string{"detail"}, MaxChildren: 1, MaxRunes: maxNewsSummaryRunes,
			TextMode: "inline-markdown", Description: "News digest entry with inline summary (<= 600 Unicode runes) and optional expandable <detail>.",
			Attributes: []AttrSpec{
				{Name: "title", Since: "1.1", Required: true, Description: "News headline"},
				{Name: "source", Since: "1.1", Description: "Publisher/source name (indexed)"},
				{Name: "url", Since: "1.1", Description: "Source http(s) URL"},
				{Name: "published", Since: "1.1", Description: "YYYY-MM-DD or RFC 3339 timestamp"},
				{Name: "importance", Since: "1.1", Values: slices.Clone(newsImportances), Description: "Priority badge (high or normal)"},
				{Name: "detail-id", Since: "1.1", Pattern: itemIDRe.String(), Description: "Linked 12-char detail item ID"},
			},
		},
		{Name: "detail", Since: "1.1", Parents: []string{"news-item"}, MaxBytes: maxNewsDetailBytes, TextMode: "markdown-cdata", Description: "Optional inline expandable detail Markdown (<= 8 KB; indexed)."},
	}
}

// RenderSchemaJSON encodes Spec() as indented JSON.
func RenderSchemaJSON() ([]byte, error) {
	return json.MarshalIndent(Spec(), "", "  ")
}

// RenderSchemaMarkdown renders the protocol specification from Spec() as Markdown.
func RenderSchemaMarkdown() string {
	s := Spec()
	var b strings.Builder
	b.WriteString("# Aumbry XML Protocol Specification\n\n")

	b.WriteString("## Version Comparison (1.0 vs 1.1)\n\n")
	for _, ver := range []string{"1.0", "1.1"} {
		vr := s.Versions[ver]
		fmt.Fprintf(&b, "### Version `%s`\n\n", ver)
		fmt.Fprintf(&b, "- Section depth: %d (%s)\n", vr.SectionMaxDepth, vr.SectionDepthNote)
		fmt.Fprintf(&b, "- `<body>` direct children: %s\n", strings.Join(vr.BodyChildren, ", "))
		if len(vr.DocumentCaps) > 0 {
			caps := make([]string, 0, len(vr.DocumentCaps))
			for k := range vr.DocumentCaps {
				caps = append(caps, k)
			}
			slices.Sort(caps)
			var parts []string
			for _, k := range caps {
				parts = append(parts, fmt.Sprintf("`%s` <= %d", k, vr.DocumentCaps[k]))
			}
			fmt.Fprintf(&b, "- Document-wide caps: %s\n", strings.Join(parts, ", "))
		}
		if len(vr.PositionExceptions) > 0 {
			b.WriteString("- Position & compatibility rules:\n")
			for _, ex := range vr.PositionExceptions {
				fmt.Fprintf(&b, "  - %s\n", ex)
			}
		}
		b.WriteString("\n| Container | Allowed Parents | Allowed Children | Bounds | Notes |\n")
		b.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
		names := make([]string, 0, len(vr.Containers))
		for k := range vr.Containers {
			names = append(names, k)
		}
		slices.Sort(names)
		for _, name := range names {
			cr := vr.Containers[name]
			bounds := "-"
			if cr.MinChildren > 0 || cr.MaxChildren > 0 {
				maxStr := "n"
				if cr.MaxChildren > 0 {
					maxStr = fmt.Sprintf("%d", cr.MaxChildren)
				}
				bounds = fmt.Sprintf("%d..%s", cr.MinChildren, maxStr)
			}
			fmt.Fprintf(&b, "| `<%s>` | %s | %s | %s | %s |\n",
				name, strings.Join(cr.AllowedParents, ", "), strings.Join(cr.AllowedChildren, ", "), bounds, cr.Notes)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Elements & Attributes\n\n")
	b.WriteString("| Element | Since | Parents | Attributes | Limits & Notes |\n")
	b.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
	for _, el := range s.Elements {
		var attrs []string
		for _, a := range el.Attributes {
			tag := a.Name
			if a.Since == "1.1" {
				tag += " (1.1)"
			}
			if a.Required {
				tag += "*"
			}
			if len(a.Values) > 0 {
				tag += "=[" + strings.Join(a.Values, "|") + "]"
			}
			attrs = append(attrs, "`"+tag+"`")
		}
		attrStr := "-"
		if len(attrs) > 0 {
			attrStr = strings.Join(attrs, ", ")
		}
		var limits []string
		if el.MinChildren > 0 || el.MaxChildren > 0 {
			maxStr := "n"
			if el.MaxChildren > 0 {
				maxStr = fmt.Sprintf("%d", el.MaxChildren)
			}
			limits = append(limits, fmt.Sprintf("children: %d..%s", el.MinChildren, maxStr))
		}
		if el.DocMax > 0 {
			limits = append(limits, fmt.Sprintf("doc max: %d", el.DocMax))
		}
		if el.MaxBytes > 0 {
			limits = append(limits, fmt.Sprintf("<= %d B", el.MaxBytes))
		}
		if el.MaxRunes > 0 {
			limits = append(limits, fmt.Sprintf("<= %d runes", el.MaxRunes))
		}
		limitStr := el.Description
		if len(limits) > 0 {
			limitStr = strings.Join(limits, "; ") + " — " + el.Description
		}
		fmt.Fprintf(&b, "| `<%s>` | %s | %s | %s | %s |\n",
			el.Name, el.Since, strings.Join(el.Parents, ", "), attrStr, limitStr)
	}

	b.WriteString("\n## Chart JSON Schema (`<chart>`)\n\n")
	b.WriteString("`<chart>` takes XML attributes `type` (required: `bar`, `line`, `pie`, `scatter`) and `caption` (optional chart title). ")
	b.WriteString("Both the top-level JSON object and each `series[]` object set `additionalProperties: false` — unknown keys such as `title`, `xAxis`, or `yAxis` fail validation with `INVALID_CHART` (put the chart title in `<chart caption=\"...\">` and axis labels in `x_label`/`y_label`). Wrap the JSON body in `<![CDATA[ ... ]]>`.\n\n")
	b.WriteString("### Top-Level JSON Fields\n\n")
	b.WriteString("| Field | Type | Applies To | Description |\n")
	b.WriteString("| :--- | :--- | :--- | :--- |\n")
	b.WriteString("| `categories` | `string[]` (`minItems: 1`) | Required for `bar`, `line`, `pie`; **disallowed** for `scatter` | X-axis or slice labels; non-scatter `series[].data` length must equal `categories` length |\n")
	fmt.Fprintf(&b, "| `series` | `object[]` (`minItems: 1`) | Required (`bar`/`line`/`scatter`: `1..%d`; `pie`: exact `1`) | Data series list |\n", maxChartSeries)
	b.WriteString("| `x_label` | `string` | Optional (`bar`, `line`, `pie`, `scatter`) | Optional X-axis label |\n")
	b.WriteString("| `y_label` | `string` | Optional (`bar`, `line`, `pie`, `scatter`) | Optional Y-axis label |\n\n")
	b.WriteString("### `series[]` Item Fields\n\n")
	b.WriteString("| Field | Type | Applies To | Description |\n")
	b.WriteString("| :--- | :--- | :--- | :--- |\n")
	b.WriteString("| `name` | `string` (`minLength: 1`) | Required (`bar`, `line`, `pie`, `scatter`) | Series legend label (trimmed non-empty) |\n")
	b.WriteString("| `data` | `number[]` (`bar`/`line`/`pie`) or `[number, number][]` (`scatter`), `minItems: 1` | Required (all types) | Finite JSON numbers only; `bar`/`line`/`pie` length must match `categories`; `scatter` takes `[x, y]` numeric pairs |\n")
	b.WriteString("| `err` | `number[]` (`>= 0`) | Optional for `bar`, `line`; **disallowed** for `pie`, `scatter` | Non-negative finite error-bar magnitudes; length must equal `data` |\n\n")
	b.WriteString("### Cross-Field Rules & Limits\n\n")
	fmt.Fprintf(&b, "- Series count: `1..%d` for `bar`, `line`, and `scatter` (multi-series supported); exact `1` for `pie`.\n", maxChartSeries)
	fmt.Fprintf(&b, "- Point & block caps: total data points across all series in one `<chart>` <= `%d`; max `%d` `<chart>` blocks per document (`TOO_MANY_BLOCKS`).\n", maxChartPoints, maxChartBlocks)
	b.WriteString("- Finite numbers only: `NaN`, `Infinity`, `-Infinity`, nulls, or strings in `data`/`err` fail with `INVALID_CHART` and a JSON Pointer path (e.g. `chart/series/0/data/1`).\n\n")
	b.WriteString("### Minimal Examples by Type\n\n")
	b.WriteString("`type=\"line\"` or `type=\"bar\"` (multi-series + optional `err`):\n\n```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"categories\": [\"2-cam\", \"4-cam\", \"6-cam\"],\n")
	b.WriteString("  \"series\": [\n")
	b.WriteString("    {\"name\": \"Ours\", \"data\": [48.0, 36.2, 29.4], \"err\": [0.9, 0.7, 0.6]},\n")
	b.WriteString("    {\"name\": \"Baseline\", \"data\": [15.0, 11.2, 8.0]}\n")
	b.WriteString("  ],\n")
	b.WriteString("  \"x_label\": \"Cameras\",\n")
	b.WriteString("  \"y_label\": \"FPS\"\n")
	b.WriteString("}\n```\n\n")
	b.WriteString("`type=\"pie\"` (exact 1 series, no `err`):\n\n```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"categories\": [\"Encoder\", \"Flow\", \"Planner\"],\n")
	b.WriteString("  \"series\": [{\"name\": \"Latency Share (%)\", \"data\": [28, 52, 20]}]\n")
	b.WriteString("}\n```\n\n")
	b.WriteString("`type=\"scatter\"` (no `categories`, no `err`; `[x, y]` pairs):\n\n```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"series\": [{\"name\": \"Seeds\", \"data\": [[1.0, 29.1], [2.0, 29.8]]}],\n")
	b.WriteString("  \"x_label\": \"Steps\",\n")
	b.WriteString("  \"y_label\": \"FPS\"\n")
	b.WriteString("}\n")
	b.WriteString("```\n")

	b.WriteString("\n## Anchor Prefixes\n\n")
	b.WriteString("| Element | Format | Valid `next-steps` Target |\n")
	b.WriteString("| :--- | :--- | :--- |\n")
	for _, a := range s.Anchors {
		fmt.Fprintf(&b, "| `%s` | `%s` | %t |\n", a.Element, a.Format, a.ValidNextStepsTarget)
	}

	b.WriteString("\n## Error Codes\n\n")
	b.WriteString("| Code | Since | Trigger |\n")
	b.WriteString("| :--- | :--- | :--- |\n")
	for _, ec := range s.ErrorCodes {
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", ec.Code, ec.Since, ec.Trigger)
	}

	b.WriteString("\n## Minimal Example (`version=\"1.1\"`)\n\n```xml\n")
	b.WriteString(s.MinimalExample)
	b.WriteString("\n```\n")
	return b.String()
}
