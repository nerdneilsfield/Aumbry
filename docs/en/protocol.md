English | [简体中文](../zh-CN/protocol.md)

# XML protocol overview

Aumbry reports are authored as UTF-8 XML documents validated by Aumbry's shared schema rules. The offline CLI validator (`aumbry validate`), the server ingest endpoint (`POST /api/v1/ingest` plus full-text indexing), and the web reader all interpret the exact same schema rules.

## Protocol version

Every document declares `<document version="1.1">` at the root. Aumbry accepts no other version. A different value, or a missing `version` attribute, fails with `UNSUPPORTED_VERSION`.

| Area | Rule |
| :--- | :--- |
| `<body>` direct children | Any top-level block tag directly (`<md>`, `<claim>`, `<metrics>`, `<chart>`, `<stages>`, `<tldr>`, `<quiz>`, `<news-item>`, etc.) |
| `<section>` nesting depth | Up to `3` `<section>` levels (`<stage>` ancestors do not count toward section depth) |
| `<collapse>` children | Any leaf block plus `<search-log>`, inside `<body>`, `<section>`, or `<stage>` |
| `<figure>` asset scope | Any descendant `<figure>` with a `src` attribute |
| Document-wide block caps | `chart <= 10`, `mermaid <= 10`, `stages <= 8`, `stage <= 40`, `sections <= 3 levels` |
| Shared anchor IDs & links | Single document-wide `^[a-z0-9-]{1,32}$` namespace across `section`, `stages`, `stage`, `quiz`, `figure`, `equation`, `ref`; `<next-steps>` targets use `#s-`/`#stage-`/`#fig-`/`#eq-`/`#ref-` |
| Markdown rendering | CommonMark (raw HTML disabled), KaTeX `$` math, syntax-highlighted code blocks, per-document isolated footnotes, read-only task lists, subscript (`~`), and superscript (`^`) |

## Inspect the specification and validate locally

Print the generated protocol tables or machine-readable JSON directly from the CLI:

```sh
aumbry schema --format md
aumbry schema --format json
```

Excerpt from `aumbry schema --format md`:

```markdown
# Aumbry XML Protocol Specification

Protocol version: `1.1` (the only accepted value of `<document version>`).

## Structure Rules

- Section depth: 3 (at most 3 nested <section> levels counting only <section> ancestors (<stage> does not count))
- `<body>` direct children: section, sources, md, figure, paper-card, repo-card, compare-table, ...
```

Validate any XML document locally before pushing:

```sh
aumbry validate examples/minimal-report.xml --format text
```

Expected output when valid:

```text
valid: examples/minimal-report.xml
```

When invalid, default JSON output includes exact line numbers, error codes (such as `INVALID_XML`, `UNSUPPORTED_VERSION`, `DUPLICATE_ID`, or `INVALID_CHART`), offending fields, and fix hints.

## Full tag reference and examples

Do not duplicate the full element list here; use the auto-generated reference checked by CI (`make skill-check`):

- Complete element, attribute, container, chart JSON schema, and error-code tables: [skills/aumbry/references/xml-tags.md](../../skills/aumbry/references/xml-tags.md)
- Troubleshooting guide by error code: [skills/aumbry/references/troubleshooting.md](../../skills/aumbry/references/troubleshooting.md)
- Structural examples: `one-pager.xml`, `paper-reading.xml`, `compare-repro.xml`, `news-digest.xml`, and `news-detail.xml` under `skills/aumbry/assets/examples/`, plus the starter files `examples/minimal-report.xml` and `examples/full-report.xml`
