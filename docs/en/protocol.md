English | [简体中文](../zh-CN/protocol.md)

# XML protocol overview

Aumbry reports are authored as UTF-8 XML documents validated by Aumbry's shared schema rules. The offline CLI validator (`aumbry validate`), the server ingest endpoint (`POST /api/v1/ingest` plus full-text indexing), and the web reader all interpret the exact same schema rules.

## Protocol version

Documents declare `<document version="1.2">` at the root (legacy `<document version="1.1">` documents continue to be accepted). Using `<audio>`, `<attachment>`, or a remote `http(s)://` `<figure src>` inside `version="1.1"` fails with `INVALID_AUDIO`, `INVALID_ATTACHMENT`, or `INVALID_ASSET_PATH` along with a hint to upgrade to `version="1.2"`; any other value or missing `version` fails with `UNSUPPORTED_VERSION`.

| Area | Rule |
| :--- | :--- |
| `<body>` direct children | Any top-level block tag directly (`<md>`, `<claim>`, `<metrics>`, `<chart>`, `<stages>`, `<tldr>`, `<quiz>`, `<news-item>`, `<audio>`, `<attachment>`, etc.) |
| `<section>` nesting depth | Up to `3` `<section>` levels (`<stage>` ancestors do not count toward section depth) |
| `<collapse>` children | Any leaf block (including `<audio>`) plus `<search-log>`, inside `<body>`, `<section>`, or `<stage>` |
| Media & attachment scope | Any descendant `<figure src>` (local `.png`/`.jpg`/`.jpeg`/`.webp`/`.svg` ≤ `20 MiB`, plus public `http(s)://` URLs auto-localized on ingest in `v1.2`; failures yield `REMOTE_ASSET_BLOCKED` / `REMOTE_ASSET_FETCH_FAILED`), `<audio src>` (local `.mp3`/`.wav`/`.m4a` ≤ `100 MiB`), and `<attachment type="pdf" src>` (local `.pdf` ≤ `50 MiB`) |
| `<audio>` rules (`v1.2`) | Up to `4` `<audio>` blocks per document inside `body`/`section`/`stage`/`collapse`/`tab`/`step`; `0..100` `<chapter start title target?>` (`HH:MM:SS` non-decreasing) and `0..1` `<transcript>` (`<= 512 KB`); title, chapter titles, and transcript are indexed |
| `<attachment>` rules (`v1.2`) | Up to `4` `<attachment>` blocks per document inside `body`/`section`/`stage` (`type="pdf"`, required `src` and `title`, manual `pages` attribute rejected); pure-Go server inspection records real `page_count` (accepts owner-password-only PDFs, rejects user-password-encrypted or corrupt PDFs with `INVALID_ATTACHMENT`); title is indexed |
| Document-wide block caps | `audio <= 4`, `attachment <= 4`, `chart <= 10`, `mermaid <= 10`, `stages <= 8`, `stage <= 40`, `sections <= 3 levels`, `distinct remote figures <= 20` |
| Shared anchor IDs & links | Single document-wide `^[a-z0-9-]{1,32}$` namespace across `section`, `stages`, `stage`, `quiz`, `figure`, `equation`, `ref`, `audio`, `attachment`; `<next-steps>` and `<chapter target>` links use `#s-`/`#stages-`/`#stage-`/`#quiz-`/`#fig-`/`#eq-`/`#ref-`/`#audio-`/`#att-` (PDF blocks accept `#att-<id>:p=<N>` page anchors checked against `page_count` on ingest; out-of-range returns `INVALID_TARGET`) |
| Markdown rendering | CommonMark (raw HTML disabled), KaTeX math (`$...$` / `\(...\)` inline, `$$...$$` / `\[...\]` block), syntax-highlighted code blocks, per-document isolated footnotes, read-only task lists, subscript (`~`), and superscript (`^`) |

## Inspect the specification and validate locally

Print the generated protocol tables or machine-readable JSON directly from the CLI:

```sh
aumbry schema --format md
aumbry schema --format json
```

Excerpt from `aumbry schema --format md`:

```markdown
# Aumbry XML Protocol Specification

Protocol version: `1.2` (also accepts legacy `1.1` documents without v1.2 elements).

## Structure Rules

- Section depth: 3 (at most 3 nested <section> levels counting only <section> ancestors (<stage> does not count))
- `<body>` direct children: section, sources, md, figure, audio, paper-card, repo-card, compare-table, ...
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
- Structural examples: `one-pager.xml`, `paper-reading.xml`, `compare-repro.xml`, `news-digest.xml`, `news-detail.xml`, and `v12-features.xml` under `skills/aumbry/assets/examples/`, plus the starter files `examples/minimal-report.xml` and `examples/full-report.xml`
