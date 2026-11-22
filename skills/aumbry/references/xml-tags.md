# Aumbry XML Protocol Specification

## Version Comparison (1.0 vs 1.1)

### Version `1.0`

- Section depth: 1 (top-level <section> inside <body> only; no nested <section>)
- `<body>` direct children: section, sources
- Position & compatibility rules:
  - all v1.1-only elements and new v1.1 attributes on v1.0 elements fail with TAG_REQUIRES_1_1 (no duplicate UNKNOWN_TAG)
  - collapse only allowed directly inside section and only accepts md and figure

| Container | Allowed Parents | Allowed Children | Bounds | Notes |
| :--- | :--- | :--- | :--- | :--- |
| `<body>` | document | section, sources | - |  |
| `<collapse>` | section | md, figure | - |  |
| `<compare-table>` | section | columns, row | - |  |
| `<section>` | body | md, figure, paper-card, repo-card, compare-table, callout, collapse, sources | - |  |
| `<sources>` | body, section | source | 1..n |  |

### Version `1.1`

- Section depth: 3 (at most 3 nested <section> levels counting only <section> ancestors (<stage> does not count))
- `<body>` direct children: section, sources, md, figure, paper-card, repo-card, compare-table, callout, collapse, claim, equation, mermaid, chart, metrics, steps, tabs, timeline, glossary, references, figure-group, code-block, quote, checklist, search-log, stages, quiz, next-steps, tldr, news-item
- Document-wide caps: `chart` <= 10, `mermaid` <= 10, `stage` <= 40, `stages` <= 8
- Position & compatibility rules:
  - <stages> may appear multiple times in <body> or <section>, but NEVER inside any <stage> descendent (INVALID_STAGES)
  - <tldr> must be a direct child of <body>, <section>, or <stage>, at most 1 per container (INVALID_TLDR)
  - <search-log> is allowed inside <collapse> as well as <body>/<section>/<stage>, but not <tab> or <step>
  - <section> nesting depth <= 3 counts only <section> ancestors (<stage> does not increment section depth)
  - <steps interactive="true"> requires >= 2 <step> elements
  - all IDs across section, stages, stage, quiz, figure, equation, and ref share ONE document-wide namespace (DUPLICATE_ID)
  - positional #sec-<N> anchors cannot be used as <next-steps> targets (INVALID_TARGET)

| Container | Allowed Parents | Allowed Children | Bounds | Notes |
| :--- | :--- | :--- | :--- | :--- |
| `<body>` | document | section, sources, md, figure, paper-card, repo-card, compare-table, callout, collapse, claim, equation, mermaid, chart, metrics, steps, tabs, timeline, glossary, references, figure-group, code-block, quote, checklist, search-log, stages, quiz, next-steps, tldr, news-item | - |  |
| `<checklist>` | body, section, stage | check | 1..n |  |
| `<collapse>` | body, section, stage | md, figure, callout, claim, equation, mermaid, chart, code-block, quote, search-log | - | allows search-log in addition to leaf blocks |
| `<columns>` | compare-table | col | 1..n |  |
| `<compare-table>` | body, section, stage | columns, row | - |  |
| `<criteria>` | search-log | include, exclude | - |  |
| `<figure-group>` | body, section, stage | figure | 2..6 |  |
| `<glossary>` | body, section, stage | term | 1..n |  |
| `<metrics>` | body, section, stage | metric | 1..12 |  |
| `<news-item>` | body, section, stage | detail | 0..1 | direct text summary <= 600 runes; 0..1 <detail> <= 8 KB |
| `<next-steps>` | body, section, stage | option | 1..5 |  |
| `<quiz>` | body, section, stage | question, option, explain | - | 1 <question>, 2..8 <option> (>= 1 correct="true"), 0..1 <explain> |
| `<references>` | body, section, stage | ref | 1..n |  |
| `<row>` | compare-table | cell | - | cell count must match columns/col count |
| `<search-log>` | body, section, stage, collapse | criteria, query, excluded, gap, stop | - | requires >= 1 <query>; <= 1 <criteria>; <= 1 <stop> |
| `<section>` | body, section, stage | section, sources, md, figure, paper-card, repo-card, compare-table, callout, collapse, claim, equation, mermaid, chart, metrics, steps, tabs, timeline, glossary, references, figure-group, code-block, quote, checklist, search-log, stages, quiz, next-steps, tldr, news-item | - | section depth <= 3 |
| `<sources>` | body, section, stage | source | 1..n |  |
| `<stage>` | stages | section, sources, md, figure, paper-card, repo-card, compare-table, callout, collapse, claim, equation, mermaid, chart, metrics, steps, tabs, timeline, glossary, references, figure-group, code-block, quote, checklist, search-log, quiz, next-steps, tldr, news-item | - | allows all body/section blocks and <section>, except nested <stages> |
| `<stages>` | body, section | stage | 1..8 | cannot be nested inside any stage descendent |
| `<step>` | steps | md, figure, callout, claim, equation, mermaid, chart, code-block, quote | - |  |
| `<steps>` | body, section, stage | step | 1..n | interactive="true" requires >= 2 <step> |
| `<tab>` | tabs | md, figure, callout, claim, equation, mermaid, chart, code-block, quote | - |  |
| `<tabs>` | body, section, stage | tab | 2..n |  |
| `<timeline>` | body, section, stage | event | 1..n |  |
| `<tldr>` | body, section, stage | point, verdict | 1..n | direct child of body/section/stage only; at most 1 per container; 1..7 <point>; 0..1 <verdict> |

## Elements & Attributes

| Element | Since | Parents | Attributes | Limits & Notes |
| :--- | :--- | :--- | :--- | :--- |
| `<document>` | 1.0 | (root) | `version*=[1.0|1.1]` | Root element of every Aumbry XML document. |
| `<meta>` | 1.0 | document | - | children: 5..n — Required document metadata. |
| `<title>` | 1.0 | meta | - | Document title (required). |
| `<type>` | 1.0 | meta | - | Document type: "report" or "post" (required). |
| `<date>` | 1.0 | meta | - | ISO YYYY-MM-DD calendar date (required). |
| `<summary>` | 1.0 | meta | - | One-line card/search summary (required). |
| `<source-agent>` | 1.0 | meta | - | Origin agent identifier (required). |
| `<canonical-url>` | 1.0 | meta | - | Optional http(s) canonical URL. |
| `<tags>` | 1.0 | meta | - | Optional list of topic tags. |
| `<tag>` | 1.0 | tags | - | Non-empty tag string. |
| `<related-ids>` | 1.0 | meta | - | Optional list of related 12-char item IDs. |
| `<id>` | 1.0 | related-ids | - | 12-char item ID matching ^[A-Za-z0-9_-]{12}$. |
| `<body>` | 1.0 | document | - | Document body (1.0 allows section/sources; 1.1 allows all block tags directly). |
| `<section>` | 1.0 | body, section, stage | `title*`, `id (1.1)` | Titled section (nestable up to 3 section levels in 1.1). |
| `<md>` | 1.0 | body, section, stage, collapse, tab, step | - | Rich Markdown content block (wrap in CDATA). |
| `<figure>` | 1.0 | body, section, stage, collapse, tab, step, figure-group | `src*`, `caption`, `id (1.1)`, `alt (1.1)` | Local image asset reference (.png/.jpg/.jpeg/.webp/.svg). |
| `<paper-card>` | 1.0 | body, section, stage | `title*`, `url*`, `authors`, `date`, `related-id`, `venue (1.1)`, `doi (1.1)`, `code-url (1.1)`, `read-depth (1.1)=[full|abstract|snippet]` | Academic paper summary card. |
| `<repo-card>` | 1.0 | body, section, stage | `name*`, `url*`, `stars`, `lang`, `license (1.1)`, `commit (1.1)` | Open-source repository summary card. |
| `<compare-table>` | 1.0 | body, section, stage | `caption` | Structured comparison table. |
| `<columns>` | 1.0 | compare-table | - | children: 1..n — Column header container. |
| `<col>` | 1.0 | columns | - | Table column header (leaf element in 1.1; disallows child elements). |
| `<row>` | 1.0 | compare-table | - | Table row matching column count. |
| `<cell>` | 1.0 | row | `tag (1.1)=[claim|evidence|inference|na|unreported|approx]` | Table cell (leaf element in 1.1; disallows child elements). |
| `<callout>` | 1.0 | body, section, stage, collapse, tab, step | `type*=[info|warn|insight]`, `title` | Highlighted callout box. |
| `<collapse>` | 1.0 | body, section, stage | `title*` | Collapsible block (v1.0: md/figure; v1.1: leaf blocks + search-log). |
| `<sources>` | 1.0 | body, section, stage | - | children: 1..n — External link list. |
| `<source>` | 1.0 | sources | `title*`, `url*` | External source link (leaf element in 1.1; disallows child elements). |
| `<claim>` | 1.1 | body, section, stage, collapse, tab, step | `level (1.1)*=[claim|evidence|inference|unverified]`, `source (1.1)` | Annotated assertion/evidence/inference/unverified block. |
| `<equation>` | 1.1 | body, section, stage, collapse, tab, step | `id (1.1)`, `number (1.1)`, `caption (1.1)` | <= 4096 B — Display LaTeX equation (non-empty, <= 4 KB; body excluded from search). |
| `<mermaid>` | 1.1 | body, section, stage, collapse, tab, step | `caption (1.1)` | doc max: 10; <= 20480 B — Mermaid diagram source (non-empty, <= 20 KB, max 10 per document; source excluded from search). |
| `<chart>` | 1.1 | body, section, stage, collapse, tab, step | `type (1.1)*=[bar|line|pie|scatter]`, `caption (1.1)` | doc max: 10 — Minimal chart JSON validated against chart_schema.json + cross-field checks (<= 20 series, <= 2000 total points, max 10 charts per document; JSON excluded from search). |
| `<metrics>` | 1.1 | body, section, stage | - | children: 1..12 — KPI metric card grid (1-12 items). |
| `<metric>` | 1.1 | metrics | `label (1.1)*`, `value (1.1)*`, `unit (1.1)`, `delta (1.1)`, `trend (1.1)=[up|down|flat]` | Single KPI metric. |
| `<steps>` | 1.1 | body, section, stage | `interactive (1.1)=[true|false]` | children: 1..n — Numbered workflow or reproduction steps. |
| `<step>` | 1.1 | steps | `title (1.1)*` | Single step container. |
| `<tabs>` | 1.1 | body, section, stage | - | children: 2..n — Tabbed container requiring at least 2 uniquely titled tabs. |
| `<tab>` | 1.1 | tabs | `title (1.1)*` | Single tab panel. |
| `<timeline>` | 1.1 | body, section, stage | - | children: 1..n — Chronological event timeline. |
| `<event>` | 1.1 | timeline | `date (1.1)*`, `title (1.1)*` | Timeline entry. |
| `<glossary>` | 1.1 | body, section, stage | - | children: 1..n — Definition list. |
| `<term>` | 1.1 | glossary | `name (1.1)*` | Glossary entry definition. |
| `<references>` | 1.1 | body, section, stage | - | children: 1..n — Numbered bibliography referenced via #ref-<id>. |
| `<ref>` | 1.1 | references | `id (1.1)*`, `title (1.1)*`, `authors (1.1)`, `year (1.1)`, `venue (1.1)`, `url (1.1)`, `doi (1.1)`, `read-depth (1.1)=[full|abstract|snippet]` | Academic bibliographic entry. |
| `<figure-group>` | 1.1 | body, section, stage | `cols (1.1)*=[2|3]`, `caption (1.1)` | children: 2..6 — Multi-column figure grid (2-6 figures). |
| `<code-block>` | 1.1 | body, section, stage, collapse, tab, step | `lang (1.1)`, `file (1.1)`, `lines (1.1)`, `commit (1.1)`, `caption (1.1)`, `eq (1.1)`, `highlight (1.1)`, `diff (1.1)=[true|false]` | <= 32768 B — Syntax-highlighted code or unified diff (<= 32 KB; only file/caption indexed). |
| `<quote>` | 1.1 | body, section, stage, collapse, tab, step | `source (1.1)`, `author (1.1)`, `url (1.1)` | Verbatim excerpt distinct from analytical prose. |
| `<checklist>` | 1.1 | body, section, stage | - | children: 1..n — Read-only verification checklist. |
| `<check>` | 1.1 | checklist | `status (1.1)*=[done|todo|blocked|risk]`, `detail (1.1)` | Single checklist item. |
| `<search-log>` | 1.1 | body, section, stage, collapse | `date (1.1)` | Audit log for literature search workflows. |
| `<criteria>` | 1.1 | search-log | - | At most 1 inclusion/exclusion criteria group. |
| `<include>` | 1.1 | criteria | - | Inclusion criterion (indexed). |
| `<exclude>` | 1.1 | criteria | - | Exclusion criterion (indexed). |
| `<query>` | 1.1 | search-log | `source (1.1)`, `date (1.1)`, `hits (1.1)`, `kept (1.1)` | Executed search query (>= 1 required inside <search-log>). |
| `<excluded>` | 1.1 | search-log | `title (1.1)`, `reason (1.1)` | Excluded candidate item (title and reason are indexed). |
| `<gap>` | 1.1 | search-log | - | Identified literature gap (indexed). |
| `<stop>` | 1.1 | search-log | - | Stopping rule (at most 1 inside <search-log>). |
| `<stages>` | 1.1 | body, section | `id (1.1)*`, `gated (1.1)=[true|false]` | children: 1..8; doc max: 8 — Independent progressive-reading group (1-8 stages; max 8 groups per document; never nested inside stage). |
| `<stage>` | 1.1 | stages | `id (1.1)*`, `title (1.1)*`, `level (1.1)` | doc max: 40 — Single progressive stage (max 40 total across document). |
| `<quiz>` | 1.1 | body, section, stage | `id (1.1)*` | Inline interactive self-check quiz (options and correctness excluded from search). |
| `<question>` | 1.1 | quiz | - | Exactly 1 non-empty plain-text question inside <quiz> (indexed). |
| `<option>` | 1.1 | quiz, next-steps | `correct (1.1)=[true|false]`, `target (1.1)` | Quiz answer choice (2-8 per quiz; >=1 correct="true"; excluded from search) OR non-empty navigation button inside <next-steps> (1-5 per block; excluded from search). |
| `<explain>` | 1.1 | quiz | - | Optional explanation shown after quiz submission (0-1 per quiz; indexed). |
| `<next-steps>` | 1.1 | body, section, stage | - | children: 1..5 — In-document action buttons targeting anchored sections/stages/figures/equations/quizzes/refs. |
| `<tldr>` | 1.1 | body, section, stage | `title (1.1)`, `placement (1.1)=[top|bottom]` | children: 1..n — Key takeaways card (direct child of body/section/stage only; at most 1 per container). |
| `<point>` | 1.1 | tldr | - | <= 300 runes — Single takeaway point (1-7 per tldr, non-empty, <= 300 Unicode runes). |
| `<verdict>` | 1.1 | tldr | - | <= 300 runes — Optional summary verdict (0-1 per tldr, non-empty, <= 300 Unicode runes). |
| `<news-item>` | 1.1 | body, section, stage | `title (1.1)*`, `source (1.1)`, `url (1.1)`, `published (1.1)`, `importance (1.1)=[high|normal]`, `detail-id (1.1)` | children: 0..1; <= 600 runes — News digest entry with inline summary (<= 600 Unicode runes) and optional expandable <detail>. |
| `<detail>` | 1.1 | news-item | - | <= 8192 B — Optional inline expandable detail Markdown (<= 8 KB; indexed). |

## Chart JSON Schema (`<chart>`)

`<chart>` takes XML attributes `type` (required: `bar`, `line`, `pie`, `scatter`) and `caption` (optional chart title). Both the top-level JSON object and each `series[]` object set `additionalProperties: false` — unknown keys such as `title`, `xAxis`, or `yAxis` fail validation with `INVALID_CHART` (put the chart title in `<chart caption="...">` and axis labels in `x_label`/`y_label`). Wrap the JSON body in `<![CDATA[ ... ]]>`.

### Top-Level JSON Fields

| Field | Type | Applies To | Description |
| :--- | :--- | :--- | :--- |
| `categories` | `string[]` (`minItems: 1`) | Required for `bar`, `line`, `pie`; **disallowed** for `scatter` | X-axis or slice labels; non-scatter `series[].data` length must equal `categories` length |
| `series` | `object[]` (`minItems: 1`) | Required (`bar`/`line`/`scatter`: `1..20`; `pie`: exact `1`) | Data series list |
| `x_label` | `string` | Optional (`bar`, `line`, `pie`, `scatter`) | Optional X-axis label |
| `y_label` | `string` | Optional (`bar`, `line`, `pie`, `scatter`) | Optional Y-axis label |

### `series[]` Item Fields

| Field | Type | Applies To | Description |
| :--- | :--- | :--- | :--- |
| `name` | `string` (`minLength: 1`) | Required (`bar`, `line`, `pie`, `scatter`) | Series legend label (trimmed non-empty) |
| `data` | `number[]` (`bar`/`line`/`pie`) or `[number, number][]` (`scatter`), `minItems: 1` | Required (all types) | Finite JSON numbers only; `bar`/`line`/`pie` length must match `categories`; `scatter` takes `[x, y]` numeric pairs |
| `err` | `number[]` (`>= 0`) | Optional for `bar`, `line`; **disallowed** for `pie`, `scatter` | Non-negative finite error-bar magnitudes; length must equal `data` |

### Cross-Field Rules & Limits

- Series count: `1..20` for `bar`, `line`, and `scatter` (multi-series supported); exact `1` for `pie`.
- Point & block caps: total data points across all series in one `<chart>` <= `2000`; max `10` `<chart>` blocks per document (`TOO_MANY_BLOCKS`).
- Finite numbers only: `NaN`, `Infinity`, `-Infinity`, nulls, or strings in `data`/`err` fail with `INVALID_CHART` and a JSON Pointer path (e.g. `chart/series/0/data/1`).

### Minimal Examples by Type

`type="line"` or `type="bar"` (multi-series + optional `err`):

```json
{
  "categories": ["2-cam", "4-cam", "6-cam"],
  "series": [
    {"name": "Ours", "data": [48.0, 36.2, 29.4], "err": [0.9, 0.7, 0.6]},
    {"name": "Baseline", "data": [15.0, 11.2, 8.0]}
  ],
  "x_label": "Cameras",
  "y_label": "FPS"
}
```

`type="pie"` (exact 1 series, no `err`):

```json
{
  "categories": ["Encoder", "Flow", "Planner"],
  "series": [{"name": "Latency Share (%)", "data": [28, 52, 20]}]
}
```

`type="scatter"` (no `categories`, no `err`; `[x, y]` pairs):

```json
{
  "series": [{"name": "Seeds", "data": [[1.0, 29.1], [2.0, 29.8]]}],
  "x_label": "Steps",
  "y_label": "FPS"
}
```

## Anchor Prefixes

| Element | Format | Valid `next-steps` Target |
| :--- | :--- | :--- |
| `section` | `#s-<id>` | true |
| `stages` | `#stages-<id>` | true |
| `stage` | `#stage-<id>` | true |
| `quiz` | `#quiz-<id>` | true |
| `figure` | `#fig-<id>` | true |
| `equation` | `#eq-<id>` | true |
| `ref` | `#ref-<id>` | true |
| `section (without id)` | `#sec-<N>` | false |

## Error Codes

| Code | Since | Trigger |
| :--- | :--- | :--- |
| `INVALID_XML` | 1.0 | malformed XML or disallowed DOCTYPE; includes CDATA hint when unescaped < or & triggers inside text blocks |
| `INVALID_ROOT` | 1.0 | root element is not <document> |
| `UNSUPPORTED_VERSION` | 1.0 | document @version is not "1.0" or "1.1" |
| `UNEXPECTED_TEXT` | 1.0 | non-whitespace text placed directly inside structural containers |
| `MISSING_META` | 1.0 | missing <meta> element |
| `MISSING_BODY` | 1.0 | missing <body> element |
| `DUPLICATE_META` | 1.0 | more than one <meta> element |
| `DUPLICATE_BODY` | 1.0 | more than one <body> element |
| `REQUIRED_FIELD` | 1.0 | missing required <meta> child (<title>, <type>, <date>, <summary>, <source-agent>) |
| `INVALID_TYPE` | 1.0 | <meta>/<type> is not report or post |
| `INVALID_DATE` | 1.0 | invalid date on <meta>/<date>, <event>@date, <news-item>@published, <search-log>@date, or <query>@date |
| `INVALID_TAG` | 1.0 | unexpected or empty element inside <meta>/<tags> |
| `INVALID_ID` | 1.0 | invalid 12-char item id (<meta>/<related-ids>/<id>, paper-card@related-id, news-item@detail-id) or invalid anchor id (^[a-z0-9-]{1,32}$) |
| `UNKNOWN_TAG` | 1.0 | unexpected element inside a container or inside a leaf block |
| `MISSING_ATTR` | 1.0 | required attribute missing or blank |
| `INVALID_ATTR` | 1.0 | invalid enum attribute value, empty <next-steps>/<option> text, or container count constraint (callout@type, claim@level, cell@tag, read-depth, figure-group, tabs count, steps count, glossary term uniqueness, next-steps option count) |
| `INVALID_URL` | 1.0 | URL is not an absolute http(s) URL (canonical-url, paper-card@url|code-url, repo-card@url, source@url, ref@url, quote@url, news-item@url) |
| `INVALID_ASSET_PATH` | 1.0 | <figure src> is not a clean relative path inside document directory |
| `INVALID_ASSET_TYPE` | 1.0 | <figure src> extension is not .png, .jpg, .jpeg, .webp, or .svg |
| `MISSING_ASSET` | 1.0 | referenced local <figure src> file does not exist |
| `TABLE_NO_COLUMNS` | 1.0 | <compare-table> missing <columns>/<col> |
| `TABLE_NO_ROWS` | 1.0 | <compare-table> has no <row> |
| `TABLE_COL_MISMATCH` | 1.0 | <row> <cell> count differs from <columns>/<col> count |
| `MISSING_SOURCE` | 1.0 | <sources> has no <source> |
| `TAG_REQUIRES_1_1` | 1.1 | v1.1 element or v1.1 attribute used inside a version="1.0" document (with upgrade hint) |
| `SECTION_TOO_DEEP` | 1.1 | <section> nested deeper than 3 <section> levels |
| `TOO_MANY_BLOCKS` | 1.1 | document exceeds caps (>10 mermaid, >10 chart, >8 stages groups, >40 total stage elements) |
| `DUPLICATE_ID` | 1.1 | duplicate anchor id across the document-wide shared id namespace |
| `DUPLICATE_TAB` | 1.1 | duplicate <tab title> within the same <tabs> |
| `INVALID_TARGET` | 1.1 | <next-steps>/<option target="#..."> fails to resolve or uses #sec-N; <code-block eq="..."> missing matching <equation id> |
| `INVALID_CHART` | 1.1 | bad chart@type, JSON syntax error, JSON Schema failure, length mismatch, non-finite NaN/Inf, >2000 total points, or invalid series[].err (reported with JSON path) |
| `INVALID_EQUATION` | 1.1 | <equation> empty or exceeds 4 KB |
| `INVALID_MERMAID` | 1.1 | <mermaid> empty or exceeds 20 KB |
| `INVALID_CODE_BLOCK` | 1.1 | <code-block> empty, exceeds 32 KB, invalid diff flag, or invalid highlight line spec |
| `INVALID_METRIC` | 1.1 | <metric> missing/empty label or value, bad trend, or <metrics> outside 1..12 items |
| `INVALID_DOI` | 1.1 | paper-card@doi or ref@doi fails ^10\.\d{4,9}/[-._;()/:A-Za-z0-9]+$ |
| `INVALID_STAGES` | 1.1 | nested <stages> inside <stage>, missing stages/stage @id, missing stage @title, invalid @gated, or group outside 1..8 <stage> |
| `INVALID_QUIZ` | 1.1 | missing @id, question count != 1, option outside 2..8, zero correct="true" options, >1 explain, or child XML tags inside fields |
| `INVALID_TLDR` | 1.1 | <tldr> outside body/section/stage, >1 in same container, bad placement, 0 or >7 <point>, >1 <verdict>, or >300 Unicode runes |
| `INVALID_NEWS` | 1.1 | <news-item> missing title, invalid importance, >1 <detail>, summary >600 runes, or <detail> >8 KB |
| `INVALID_CHECK` | 1.1 | <checklist> empty or <check status> not in done|todo|blocked|risk |
| `INVALID_SEARCH_LOG` | 1.1 | <search-log> missing <query>, >1 <criteria>/<stop>, or non-integer negative hits/kept |

## Minimal Example (`version="1.1"`)

```xml
<?xml version="1.0" encoding="UTF-8"?>
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
</document>
```
