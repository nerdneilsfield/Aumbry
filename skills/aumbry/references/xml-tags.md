# Aumbry XML Protocol Specification

Protocol version: `1.2` (also accepts legacy `1.1` documents without v1.2 elements).

## Structure Rules

- Section depth: 3 (at most 3 nested <section> levels counting only <section> ancestors (<stage> does not count))
- `<body>` direct children: section, sources, md, figure, audio, paper-card, repo-card, compare-table, callout, collapse, claim, equation, mermaid, chart, metrics, steps, tabs, timeline, glossary, references, figure-group, code-block, quote, checklist, search-log, stages, quiz, next-steps, tldr, news-item
- Document-wide caps: `audio` <= 4, `chart` <= 10, `mermaid` <= 10, `stage` <= 40, `stages` <= 8
- Position rules:
  - <audio> requires <document version="1.2"> and at most 4 blocks per document (INVALID_AUDIO)
  - <stages> may appear multiple times in <body> or <section>, but NEVER inside any <stage> descendent (INVALID_STAGES)
  - <tldr> must be a direct child of <body>, <section>, or <stage>, at most 1 per container (INVALID_TLDR)
  - <search-log> is allowed inside <collapse> as well as <body>/<section>/<stage>, but not <tab> or <step>
  - <section> nesting depth <= 3 counts only <section> ancestors (<stage> does not increment section depth)
  - <steps interactive="true"> requires >= 2 <step> elements
  - all IDs across section, stages, stage, quiz, figure, audio, equation, and ref share ONE document-wide namespace (DUPLICATE_ID)
  - positional #sec-<N> anchors cannot be used as <next-steps> or <chapter target> targets (INVALID_TARGET)

| Container | Allowed Parents | Allowed Children | Bounds | Notes |
| :--- | :--- | :--- | :--- | :--- |
| `<audio>` | body, section, stage, collapse, tab, step | chapter, transcript | - | requires <document version="1.2">; 0..100 <chapter> (non-decreasing HH:MM:SS), 0..1 <transcript> (<= 512 KB) |
| `<body>` | document | section, sources, md, figure, audio, paper-card, repo-card, compare-table, callout, collapse, claim, equation, mermaid, chart, metrics, steps, tabs, timeline, glossary, references, figure-group, code-block, quote, checklist, search-log, stages, quiz, next-steps, tldr, news-item | - |  |
| `<checklist>` | body, section, stage | check | 1..n |  |
| `<collapse>` | body, section, stage | md, figure, audio, callout, claim, equation, mermaid, chart, code-block, quote, search-log | - | allows search-log in addition to leaf blocks |
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
| `<section>` | body, section, stage | section, sources, md, figure, audio, paper-card, repo-card, compare-table, callout, collapse, claim, equation, mermaid, chart, metrics, steps, tabs, timeline, glossary, references, figure-group, code-block, quote, checklist, search-log, stages, quiz, next-steps, tldr, news-item | - | section depth <= 3 |
| `<sources>` | body, section, stage | source | 1..n |  |
| `<stage>` | stages | section, sources, md, figure, audio, paper-card, repo-card, compare-table, callout, collapse, claim, equation, mermaid, chart, metrics, steps, tabs, timeline, glossary, references, figure-group, code-block, quote, checklist, search-log, quiz, next-steps, tldr, news-item | - | allows all body/section blocks and <section>, except nested <stages> |
| `<stages>` | body, section | stage | 1..8 | cannot be nested inside any stage descendent |
| `<step>` | steps | md, figure, audio, callout, claim, equation, mermaid, chart, code-block, quote | - |  |
| `<steps>` | body, section, stage | step | 1..n | interactive="true" requires >= 2 <step> |
| `<tab>` | tabs | md, figure, audio, callout, claim, equation, mermaid, chart, code-block, quote | - |  |
| `<tabs>` | body, section, stage | tab | 2..n |  |
| `<timeline>` | body, section, stage | event | 1..n |  |
| `<tldr>` | body, section, stage | point, verdict | 1..n | direct child of body/section/stage only; at most 1 per container; 1..7 <point>; 0..1 <verdict> |

## Elements & Attributes

| Element | Parents | Attributes | Limits & Notes |
| :--- | :--- | :--- | :--- |
| `<document>` | (root) | `version*=[1.1|1.2]` | Root element of every Aumbry XML document. |
| `<meta>` | document | - | children: 5..n — Required document metadata. |
| `<title>` | meta | - | Document title (required). |
| `<type>` | meta | - | Document type: "report" or "post" (required). |
| `<date>` | meta | - | ISO YYYY-MM-DD calendar date (required). |
| `<summary>` | meta | - | One-line card/search summary (required). |
| `<source-agent>` | meta | - | Origin agent identifier (required). |
| `<canonical-url>` | meta | - | Optional http(s) canonical URL. |
| `<tags>` | meta | - | Optional list of topic tags. |
| `<tag>` | tags | - | Non-empty tag string. |
| `<related-ids>` | meta | - | Optional list of related 12-char item IDs. |
| `<id>` | related-ids | - | 12-char item ID matching ^[A-Za-z0-9_-]{12}$. |
| `<body>` | document | - | Document body (allows all block tags directly). |
| `<section>` | body, section, stage | `title*`, `id` | Titled section (nestable up to 3 section levels). |
| `<md>` | body, section, stage, collapse, tab, step | - | Rich Markdown content block (wrap in CDATA). |
| `<figure>` | body, section, stage, collapse, tab, step, figure-group | `src*`, `caption`, `id`, `alt` | Local image asset reference (.png/.jpg/.jpeg/.webp/.svg). |
| `<audio>` | body, section, stage, collapse, tab, step | `src*`, `title*`, `id` | doc max: 4 — Local audio asset reference (.mp3/.wav/.m4a); requires <document version="1.2">. Title and transcript/chapters are indexed. |
| `<chapter>` | audio | `start*`, `title*`, `target` | Timestamped chapter marker inside <audio> (0-100 per block; timestamps must be non-decreasing). |
| `<transcript>` | audio | - | <= 524288 B — Optional full audio transcript (0-1 per <audio>; non-empty plain text or CDATA <= 512 KB; indexed). |
| `<paper-card>` | body, section, stage | `title*`, `url*`, `authors`, `date`, `related-id`, `venue`, `doi`, `code-url`, `read-depth=[full|abstract|snippet]` | Academic paper summary card. |
| `<repo-card>` | body, section, stage | `name*`, `url*`, `stars`, `lang`, `license`, `commit` | Open-source repository summary card. |
| `<compare-table>` | body, section, stage | `caption` | Structured comparison table. |
| `<columns>` | compare-table | - | children: 1..n — Column header container. |
| `<col>` | columns | - | Table column header (leaf element; disallows child elements). |
| `<row>` | compare-table | - | Table row matching column count. |
| `<cell>` | row | `tag=[claim|evidence|inference|na|unreported|approx]` | Table cell (leaf element; disallows child elements). |
| `<callout>` | body, section, stage, collapse, tab, step | `type*=[info|warn|insight]`, `title` | Highlighted callout box. |
| `<collapse>` | body, section, stage | `title*` | Collapsible block (leaf blocks + search-log). |
| `<sources>` | body, section, stage | - | children: 1..n — External link list. |
| `<source>` | sources | `title*`, `url*` | External source link (leaf element; disallows child elements). |
| `<claim>` | body, section, stage, collapse, tab, step | `level*=[claim|evidence|inference|unverified]`, `source` | Annotated assertion/evidence/inference/unverified block. |
| `<equation>` | body, section, stage, collapse, tab, step | `id`, `number`, `caption` | <= 4096 B — Display LaTeX equation (non-empty, <= 4 KB; body excluded from search). |
| `<mermaid>` | body, section, stage, collapse, tab, step | `caption` | doc max: 10; <= 20480 B — Mermaid diagram source (non-empty, <= 20 KB, max 10 per document; source excluded from search). |
| `<chart>` | body, section, stage, collapse, tab, step | `type*=[bar|line|pie|scatter]`, `caption` | doc max: 10 — Minimal chart JSON validated against chart_schema.json + cross-field checks (<= 20 series, <= 2000 total points, max 10 charts per document; JSON excluded from search). |
| `<metrics>` | body, section, stage | - | children: 1..12 — KPI metric card grid (1-12 items). |
| `<metric>` | metrics | `label*`, `value*`, `unit`, `delta`, `trend=[up|down|flat]` | Single KPI metric. |
| `<steps>` | body, section, stage | `interactive=[true|false]` | children: 1..n — Numbered workflow or reproduction steps. |
| `<step>` | steps | `title*` | Single step container. |
| `<tabs>` | body, section, stage | - | children: 2..n — Tabbed container requiring at least 2 uniquely titled tabs. |
| `<tab>` | tabs | `title*` | Single tab panel. |
| `<timeline>` | body, section, stage | - | children: 1..n — Chronological event timeline. |
| `<event>` | timeline | `date*`, `title*` | Timeline entry. |
| `<glossary>` | body, section, stage | - | children: 1..n — Definition list. |
| `<term>` | glossary | `name*` | Glossary entry definition. |
| `<references>` | body, section, stage | - | children: 1..n — Numbered bibliography referenced via #ref-<id>. |
| `<ref>` | references | `id*`, `title*`, `authors`, `year`, `venue`, `url`, `doi`, `read-depth=[full|abstract|snippet]` | Academic bibliographic entry. |
| `<figure-group>` | body, section, stage | `cols*=[2|3]`, `caption` | children: 2..6 — Multi-column figure grid (2-6 figures). |
| `<code-block>` | body, section, stage, collapse, tab, step | `lang`, `file`, `lines`, `commit`, `caption`, `eq`, `highlight`, `diff=[true|false]` | <= 32768 B — Syntax-highlighted code or unified diff (<= 32 KB; only file/caption indexed). |
| `<quote>` | body, section, stage, collapse, tab, step | `source`, `author`, `url` | Verbatim excerpt distinct from analytical prose. |
| `<checklist>` | body, section, stage | - | children: 1..n — Read-only verification checklist. |
| `<check>` | checklist | `status*=[done|todo|blocked|risk]`, `detail` | Single checklist item. |
| `<search-log>` | body, section, stage, collapse | `date` | Audit log for literature search workflows. |
| `<criteria>` | search-log | - | At most 1 inclusion/exclusion criteria group. |
| `<include>` | criteria | - | Inclusion criterion (indexed). |
| `<exclude>` | criteria | - | Exclusion criterion (indexed). |
| `<query>` | search-log | `source`, `date`, `hits`, `kept` | Executed search query (>= 1 required inside <search-log>). |
| `<excluded>` | search-log | `title`, `reason` | Excluded candidate item (title and reason are indexed). |
| `<gap>` | search-log | - | Identified literature gap (indexed). |
| `<stop>` | search-log | - | Stopping rule (at most 1 inside <search-log>). |
| `<stages>` | body, section | `id*`, `gated=[true|false]` | children: 1..8; doc max: 8 — Independent progressive-reading group (1-8 stages; max 8 groups per document; never nested inside stage). |
| `<stage>` | stages | `id*`, `title*`, `level` | doc max: 40 — Single progressive stage (max 40 total across document). |
| `<quiz>` | body, section, stage | `id*` | Inline interactive self-check quiz (options and correctness excluded from search). |
| `<question>` | quiz | - | Exactly 1 non-empty plain-text question inside <quiz> (indexed). |
| `<option>` | quiz, next-steps | `correct=[true|false]`, `target` | Quiz answer choice (2-8 per quiz; >=1 correct="true"; excluded from search) OR non-empty navigation button inside <next-steps> (1-5 per block; excluded from search). |
| `<explain>` | quiz | - | Optional explanation shown after quiz submission (0-1 per quiz; indexed). |
| `<next-steps>` | body, section, stage | - | children: 1..5 — In-document action buttons targeting anchored sections/stages/figures/equations/quizzes/refs. |
| `<tldr>` | body, section, stage | `title`, `placement=[top|bottom]` | children: 1..n — Key takeaways card (direct child of body/section/stage only; at most 1 per container). |
| `<point>` | tldr | - | <= 300 runes — Single takeaway point (1-7 per tldr, non-empty, one line, <= 300 Unicode runes). |
| `<verdict>` | tldr | - | <= 300 runes — Optional summary verdict (0-1 per tldr, non-empty, <= 300 Unicode runes). |
| `<news-item>` | body, section, stage | `title*`, `source`, `url`, `published`, `importance=[high|normal]`, `detail-id` | children: 0..1; <= 600 runes — News digest entry with inline summary (<= 600 Unicode runes) and optional expandable <detail>. |
| `<detail>` | news-item | - | <= 8192 B — Optional inline expandable detail Markdown (<= 8 KB; indexed). |

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
| `audio` | `#audio-<id>` | true |
| `equation` | `#eq-<id>` | true |
| `ref` | `#ref-<id>` | true |
| `section (without id)` | `#sec-<N>` | false |

## Error Codes

| Code | Trigger |
| :--- | :--- |
| `INVALID_XML` | malformed XML or disallowed DOCTYPE; includes CDATA hint when unescaped < or & triggers inside text blocks |
| `INVALID_ROOT` | root element is not <document> |
| `UNSUPPORTED_VERSION` | document @version is missing or is not "1.1" or "1.2" |
| `UNEXPECTED_TEXT` | non-whitespace text placed directly inside structural containers |
| `MISSING_META` | missing <meta> element |
| `MISSING_BODY` | missing <body> element |
| `DUPLICATE_META` | more than one <meta> element |
| `DUPLICATE_BODY` | more than one <body> element |
| `REQUIRED_FIELD` | missing required <meta> child (<title>, <type>, <date>, <summary>, <source-agent>) |
| `INVALID_TYPE` | <meta>/<type> is not report or post |
| `INVALID_DATE` | invalid date on <meta>/<date>, <event>@date, <news-item>@published, <search-log>@date, or <query>@date |
| `INVALID_TAG` | unexpected or empty element inside <meta>/<tags> |
| `INVALID_ID` | invalid 12-char item id (<meta>/<related-ids>/<id>, paper-card@related-id, news-item@detail-id) or invalid anchor id (^[a-z0-9-]{1,32}$) |
| `UNKNOWN_TAG` | unexpected element inside a container or inside a leaf block |
| `MISSING_ATTR` | required attribute missing or blank |
| `INVALID_ATTR` | invalid enum attribute value, empty <next-steps>/<option> text, or container count constraint (callout@type, claim@level, cell@tag, read-depth, figure-group, tabs count, steps count, glossary term uniqueness, next-steps option count) |
| `INVALID_URL` | URL is not an absolute http(s) URL (canonical-url, paper-card@url|code-url, repo-card@url, source@url, ref@url, quote@url, news-item@url) |
| `INVALID_ASSET_PATH` | <figure src> or <audio src> is not a clean relative path inside document directory |
| `INVALID_ASSET_TYPE` | <figure src> is not .png/.jpg/.jpeg/.webp/.svg, or <audio src> is not .mp3/.wav/.m4a |
| `MISSING_ASSET` | referenced local <figure src> or <audio src> file does not exist |
| `INVALID_AUDIO` | <audio> used in version="1.1", missing src/title, title >200 runes, >4 audio blocks, >100 chapters, invalid/decreasing <chapter start>, blank/empty <transcript>, or <transcript> >512 KB |
| `TABLE_NO_COLUMNS` | <compare-table> missing <columns>/<col> |
| `TABLE_NO_ROWS` | <compare-table> has no <row> |
| `TABLE_COL_MISMATCH` | <row> <cell> count differs from <columns>/<col> count |
| `MISSING_SOURCE` | <sources> has no <source> |
| `SECTION_TOO_DEEP` | <section> nested deeper than 3 <section> levels |
| `TOO_MANY_BLOCKS` | document exceeds caps (>10 mermaid, >10 chart, >8 stages groups, >40 total stage elements) |
| `DUPLICATE_ID` | duplicate anchor id across the document-wide shared id namespace |
| `DUPLICATE_TAB` | duplicate <tab title> within the same <tabs> |
| `INVALID_TARGET` | <next-steps>/<option target="#..."> or <audio>/<chapter target="#..."> fails to resolve or uses #sec-N; <code-block eq="..."> missing matching <equation id> |
| `INVALID_CHART` | bad chart@type, JSON syntax error, JSON Schema failure, length mismatch, non-finite NaN/Inf, >2000 total points, or invalid series[].err (reported with JSON path) |
| `INVALID_EQUATION` | <equation> empty or exceeds 4 KB |
| `INVALID_MERMAID` | <mermaid> empty or exceeds 20 KB |
| `INVALID_CODE_BLOCK` | <code-block> empty, exceeds 32 KB, invalid diff flag, or invalid highlight line spec |
| `INVALID_METRIC` | <metric> missing/empty label or value, bad trend, or <metrics> outside 1..12 items |
| `INVALID_DOI` | paper-card@doi or ref@doi fails ^10\.\d{4,9}/[-._;()/:A-Za-z0-9]+$ |
| `INVALID_STAGES` | nested <stages> inside <stage>, missing stages/stage @id, missing stage @title, invalid @gated, or group outside 1..8 <stage> |
| `INVALID_QUIZ` | missing @id, question count != 1, option outside 2..8, zero correct="true" options, >1 explain, or child XML tags inside fields |
| `INVALID_TLDR` | <tldr> outside body/section/stage, >1 in same container, bad placement, 0 or >7 <point>, >1 <verdict>, a <point> that spans several lines, or >300 Unicode runes |
| `INVALID_NEWS` | <news-item> missing title, invalid importance, >1 <detail>, summary >600 runes, or <detail> >8 KB |
| `INVALID_CHECK` | <checklist> empty or <check status> not in done|todo|blocked|risk |
| `INVALID_SEARCH_LOG` | <search-log> missing <query>, >1 <criteria>/<stop>, or non-integer negative hits/kept |

## Minimal Example (`version="1.2"`)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<document version="1.2">
  <meta>
    <title>Minimal Document</title>
    <type>report</type>
    <date>2026-11-20</date>
    <summary>One-line summary for cards and search results.</summary>
    <source-agent>research-agent</source-agent>
  </meta>
  <body>
    <md><![CDATA[Hello **Aumbry**.]]></md>
  </body>
</document>
```
