English | [简体中文](../zh-CN/protocol.md)

# XML protocol overview

Aumbry reports are authored as UTF-8 XML documents validated by the shared Go schema package (`pkg/schema`). The offline CLI validator (`aumbry validate`), the server ingest handler (`POST /api/v1/ingest` plus SQLite FTS5 indexing), and the Vue 3 reader all interpret the exact same schema rules.

## Protocol versions (`1.0` vs `1.1`)

Every document declares `<document version="1.0">` or `<document version="1.1">` at the root.

| Area | `version="1.0"` | `version="1.1"` |
| :--- | :--- | :--- |
| `<body>` direct children | Only `<section>` and `<sources>` | Any top-level block tag directly (`<md>`, `<claim>`, `<metrics>`, `<chart>`, `<stages>`, `<tldr>`, `<quiz>`, `<news-item>`, etc.) |
| `<section>` nesting depth | Top-level only (`depth = 1`) | Up to `3` `<section>` levels (`<stage>` ancestors do not count toward section depth) |
| `<collapse>` children | `<md>`, `<figure>` inside `<section>` only | Any leaf block plus `<search-log>` inside `<body>`, `<section>`, or `<stage>` |
| `<figure>` asset scope | Direct child of `<section>` or `<section>/<collapse>` (`figureXPathV10`) | Any descendant `<figure[@src]>` (`figureXPathV11`) |
| Document-wide block caps | None beyond payload sizes | `chart <= 10`, `mermaid <= 10`, `stages <= 8`, `stage <= 40`, `sections <= 3 levels` |
| Shared anchor IDs & links | None | Single document-wide `^[a-z0-9-]{1,32}$` namespace across `section`, `stages`, `stage`, `quiz`, `figure`, `equation`, `ref`; `<next-steps>` targets use `#s-`/`#stage-`/`#fig-`/`#eq-`/`#ref-` |
| Markdown rendering | CommonMark + KaTeX `$` math + Shiki code highlighting (`html: false`) | Same pipeline plus `markdown-it-footnote` (`env.docId` isolated), read-only `markdown-it-task-lists`, `sub`, `sup` |
| Version gating | Using any v1.1 tag or attribute fails with `TAG_REQUIRES_1_1` | All v1.0 and v1.1 elements/attributes allowed |

## Inspect the specification and validate locally

Print the generated protocol tables or machine-readable JSON directly from the CLI:

```sh
aumbry schema --format md
aumbry schema --format json
```

Excerpt from `aumbry schema --format md`:

```markdown
# Aumbry XML Protocol Specification

## Version Comparison (1.0 vs 1.1)

### Version `1.0`

- Section depth: 1 (top-level <section> inside <body> only; no nested <section>)
- `<body>` direct children: section, sources
```

Validate any XML document locally before pushing:

```sh
aumbry validate examples/minimal-report.xml --format text
```

Expected output when valid:

```text
valid: examples/minimal-report.xml
```

When invalid, default JSON output includes exact line numbers, error codes (such as `INVALID_XML`, `TAG_REQUIRES_1_1`, `DUPLICATE_ID`, or `INVALID_CHART`), offending fields, and fix hints.

## Full tag reference and examples

Do not duplicate the full element list here; use the auto-generated reference checked by CI (`make skill-check`):

- Complete element, attribute, container, chart JSON schema, and error-code tables: [skills/aumbry/references/xml-tags.md](../../skills/aumbry/references/xml-tags.md)
- Troubleshooting guide by error code: [skills/aumbry/references/troubleshooting.md](../../skills/aumbry/references/troubleshooting.md)
- Structural v1.1 examples: `one-pager.xml`, `paper-reading.xml`, `compare-repro.xml`, `news-digest.xml`, and `news-detail.xml` under `skills/aumbry/assets/examples/`, plus v1.0 starter files in `examples/minimal-report.xml` and `examples/full-report.xml`
