English | [简体中文](../zh-CN/cli.md)

# CLI reference

The `aumbry` CLI validates XML documents offline and pushes reports, lists directories or items, and manages owned items through the server API. Authentication uses a Bearer token listed in the server's `api_tokens` setting; this token is independent of directory passwords and admin web login credentials.

## Configure authentication and profiles

Choose environment variables for stateless runs or named YAML profiles for persistent local workstations.

> **Warning — Plaintext token storage:** `aumbry config set` writes your API token in plaintext inside `$XDG_CONFIG_HOME/aumbry/config.yaml` (or `~/.config/aumbry/config.yaml` on every OS when `XDG_CONFIG_HOME` is unset). Aumbry creates the parent directory with mode `0700` and enforces mode `0600` on the config file on every save. Avoid committing or backing up this file unencrypted; prefer environment variables on CI runners.

### Method A: Environment variables

Set `AUMBRY_SERVER` and `AUMBRY_TOKEN` (no config file is required when neither `active_profile` nor `--profile` specifies a missing profile):

```sh
export AUMBRY_SERVER="http://127.0.0.1:8080"
export AUMBRY_TOKEN="amb_demo_token_abcd1234"
export AUMBRY_DIRS="DRTxD0OSpDPZ"
```

`AUMBRY_DIRS` accepts a comma-separated list of directory IDs with whitespace trimmed around each entry.

### Method B: `aumbry config` profiles

Create or update the active profile (defaults to `default` on first run; only passed flags change):

```sh
aumbry config set --server http://127.0.0.1:8080 --token amb_demo_token_abcd1234 --dir DRTxD0OSpDPZ
```

Expected output:

```text
profile "default" saved
```

Inspect the effective profile (the token is masked to its last 4 characters):

```sh
aumbry config show
```

Expected output:

```yaml
profile: default
server: http://127.0.0.1:8080
token: '****1234'
default_dirs:
    - DRTxD0OSpDPZ
```

Manage multiple servers with named profiles (`--dir` is repeatable and replaces `default_dirs` when passed):

```sh
aumbry config set --profile staging --server https://staging.aumbry.example --token amb_staging_9999 --dir 4ho9VKNPRj2h
aumbry config use staging
```

### Precedence rules

Effective settings for every remote command (`push`, `dirs`, `list`, `unpublish`, `delete`) resolve in this order:

```text
CLI flags (--dir, --profile)  >  Environment variables (AUMBRY_SERVER, AUMBRY_TOKEN, AUMBRY_DIRS)  >  Selected profile
```

`--profile <name>` selects the profile for that command without changing `active_profile`, and environment variables still override the non-empty fields inside that profile. Naming a profile that does not exist in `config.yaml` fails with exit code `1`.

## Global flags and exit codes

- `-v`, `--verbose`: writes debug logs to `stderr`.
- `--version`: prints CLI version, short commit hash, and build time, then exits.
- Exit code `0`: command succeeded (or document is valid).
- Exit code `1`: invalid arguments, local/remote validation failure, network error, or server error response.

## Commands

### `aumbry validate`

Validate an Aumbry XML file (`version="1.2"` or legacy `version="1.1"`) offline. Relative `<figure src>`, `<audio src>`, and `<attachment type="pdf" src>` paths resolve against the XML file's parent directory (not your current working directory), and symlinks pointing outside that directory are rejected. For `v1.2` remote `http(s)://` `<figure src>` URLs, **`aumbry validate` stays 100% offline** (no HTTP or DNS calls) and syntactically blocks `userinfo`, `localhost`, loopback, link-local (`169.254.*`), private, or non-canonical numeric hosts with `REMOTE_ASSET_BLOCKED`.

```sh
aumbry validate <file.xml> [--format json|text]
```

Default JSON output:

```sh
aumbry validate examples/minimal-report.xml
```

```json
{
  "valid": true,
  "file": "examples/minimal-report.xml",
  "errors": []
}
```

Human-readable text output:

```sh
aumbry validate examples/minimal-report.xml --format text
```

```text
valid: examples/minimal-report.xml
```

### `aumbry push`

Validate an XML document locally and stream it together with referenced local images, audio files, and `.pdf` attachments to `POST /api/v1/ingest` (CLI default upload timeout: `5m`). Remote `http(s)://` `<figure src>` URLs in `version="1.2"` documents are skipped during local multipart file collection and retained in the uploaded XML so the server localizes them safely through its proxy-free SSRF guard (while keeping `<meta><canonical-url>` untouched).

```sh
aumbry push <file.xml> [--dir <dir-id>]... [--key <external_key>] [--id <item-id>] [--draft] [--create-only] [--dry-run] [--profile <name>]
```

Key flags and constraints:

- `--dir <id>`: directory ID to mount the item in (repeatable). Passing `--dir` overrides `default_dirs` / `AUMBRY_DIRS`.
- `--draft`: upload as an unmounted draft (`dirs: []`); **ignores** configured `default_dirs` and `AUMBRY_DIRS`. Cannot be combined with `--dir`, and returns `409 already_published` if the target item is already mounted in any directory (run `aumbry unpublish <id>` first).
- `--create-only`: fail with `409 key_exists` if `external_key` already exists. Requires `--key`; cannot be combined with `--id`.
- `--dry-run`: validates XML and images locally, then verifies server connectivity, token auth, and target directory existence via `GET /api/v1/agent/dirs` without uploading objects.
- `--key <external_key>`: idempotency key for creates or updates (`1` to `200` Unicode runes after trimming, no control characters for new keys). Reuse the same `--key` across updates of the same document.
- `--id <item-id>`: update an existing item by its 12-character server ID.

> **Caution — Pushing without `--dir`:** If you omit `--dir` and neither `AUMBRY_DIRS` nor `default_dirs` is set (and you do not pass `--draft`), `aumbry push` sends an empty directory list, leaving the item unmounted (`403 forbidden` to visitors). Confirm target directories with `aumbry dirs` before publishing.

Verify without uploading (`--dry-run`):

```sh
aumbry push examples/minimal-report.xml --key demo:report --dir DRTxD0OSpDPZ --dry-run
```

```json
{
  "assets": 0,
  "create_only": false,
  "dirs": [
    "DRTxD0OSpDPZ"
  ],
  "draft": false,
  "dry_run": true,
  "file": "examples/minimal-report.xml",
  "type": "report",
  "valid": true
}
```

Save an unmounted draft (`--draft`):

```sh
aumbry push examples/minimal-report.xml --key demo:draft --draft
```

```json
{
  "assets": 0,
  "created": true,
  "dirs": [],
  "draft": true,
  "full_url": "http://127.0.0.1:8080/r/VFlSwVxYYaCN",
  "id": "VFlSwVxYYaCN",
  "published": false,
  "type": "report",
  "url": "/r/VFlSwVxYYaCN"
}
```

Create once with `--create-only --key` and update later by `--id`:

```sh
aumbry push examples/minimal-report.xml --key demo:report --create-only --dir DRTxD0OSpDPZ
```

```json
{
  "assets": 0,
  "created": true,
  "dirs": [
    "DRTxD0OSpDPZ"
  ],
  "draft": false,
  "full_url": "http://127.0.0.1:8080/r/MDUnpJpiAjMo",
  "id": "MDUnpJpiAjMo",
  "published": true,
  "type": "report",
  "url": "/r/MDUnpJpiAjMo"
}
```

```sh
aumbry push examples/minimal-report.xml --id MDUnpJpiAjMo --dir DRTxD0OSpDPZ
```

```json
{
  "assets": 0,
  "created": false,
  "dirs": [
    "DRTxD0OSpDPZ"
  ],
  "draft": false,
  "full_url": "http://127.0.0.1:8080/r/MDUnpJpiAjMo",
  "id": "MDUnpJpiAjMo",
  "published": true,
  "type": "report",
  "url": "/r/MDUnpJpiAjMo"
}
```

### `aumbry dirs`

List server directories visible to your API token (`GET /api/v1/agent/dirs`). Note that tokens can inspect directory metadata and passwords do not hide directory structure from `aumbry dirs`; inspect `protected` (not `has_password`) to check whether visitors need a password.

```sh
aumbry dirs [--format json|tree] [--profile <name>]
```

JSON example:

```sh
aumbry dirs
```

```json
[
  {
    "id": "DRTxD0OSpDPZ",
    "name": "Public Reports",
    "path": "Public Reports",
    "parent_id": null,
    "has_password": false,
    "protected": false,
    "item_count": 0
  },
  {
    "id": "4ho9VKNPRj2h",
    "name": "Team Private",
    "path": "Team Private",
    "parent_id": null,
    "has_password": true,
    "protected": true,
    "item_count": 0
  },
  {
    "id": "50h0cpGGYRkM",
    "name": "Weekly Briefs",
    "path": "Team Private / Weekly Briefs",
    "parent_id": "4ho9VKNPRj2h",
    "has_password": false,
    "protected": true,
    "item_count": 0
  }
]
```

Tree example:

```sh
aumbry dirs --format tree
```

```text
- Public Reports (id=DRTxD0OSpDPZ, items=0, public)
- Team Private (id=4ho9VKNPRj2h, items=0, password)
  - Weekly Briefs (id=50h0cpGGYRkM, items=0, protected)
```

### `aumbry list`

List items with optional filtering (`GET /api/v1/agent/items`). `--key` performs an exact match on `external_key`, `--q` matches substrings across `title` and `external_key` only (not body text), `--dir` filters by mounted directory ID, and `--limit`/`--offset` paginate (default `20`, server max `100`). `"owned": true` indicates the item was created by your current API token.

```sh
aumbry list [--key <key>] [--q <query>] [--dir <dir-id>] [--limit <n>] [--offset <n>] [--profile <name>]
```

Example:

```sh
aumbry list --key demo:report --limit 20
```

```json
{
  "items": [
    {
      "id": "MDUnpJpiAjMo",
      "type": "report",
      "title": "Aumbry example report",
      "date": "2026-11-20",
      "external_key": "demo:report",
      "url": "/r/MDUnpJpiAjMo",
      "dirs": [
        "DRTxD0OSpDPZ"
      ],
      "owned": true
    }
  ],
  "limit": 20,
  "offset": 0
}
```

### `aumbry unpublish`

Clear all directory mounts for an item (`DELETE /api/v1/agent/items/{id}/mounts`), returning it to unmounted draft status (`403` to visitors) while preserving its ID, metadata, and XML content. Only the token that originally created the item can unpublish it (other tokens and items without a recorded owner return HTTP `403 not_owner`).

```sh
aumbry unpublish MDUnpJpiAjMo
```

```json
{
  "id": "MDUnpJpiAjMo",
  "published": false
}
```

### `aumbry delete`

> **Caution — Permanent deletion:** `aumbry delete <id> --yes` permanently deletes the item's metadata, search index entry, revision records, and directory mounts (`DELETE /api/v1/agent/items/{id}`). This cannot be undone; orphaned XML and asset objects are reclaimed by offline `aumbry-server gc`. Without `--yes`, the CLI refuses to execute locally. Only the creating API token is authorized (`403 not_owner` otherwise).

```sh
aumbry delete MDUnpJpiAjMo --yes
```

```json
{
  "deleted": true,
  "id": "MDUnpJpiAjMo"
}
```

### `aumbry schema`, `aumbry version`, and `aumbry status`

- `aumbry schema [--format md|json]`: prints the built-in XML protocol specification (`md` default).
- `aumbry version [--json]` (or `aumbry --version`): prints local binary version, 7-character short commit hash, build timestamp, Go toolchain version, OS, and architecture (pass `--json` for structured output):

  ```sh
  aumbry version
  ```

  ```text
  aumbry dev
  commit: none
  built: unknown
  go: go1.27.4 linux/amd64
  ```

- `aumbry status [--profile <name>] [--json]`: calls `GET /api/v1/agent/status` to inspect the remote server's version/build metadata, health and dependency checks, uptime, CPU usage and memory (process RSS / Go heap / goroutines), SQLite footprint, directory/item/revision/asset counts, and live ingest/MCP session concurrency (automatically notes CLI vs server version mismatches; pass `--json` for raw JSON):

  ```sh
  aumbry status
  ```

  ```text
  Server:    http://127.0.0.1:8080
  Health:    ok (sqlite=ok storage=ok)
  Uptime:    1h 2m 5s (started 2026-11-23T11:27:55Z)
  ServerVer: v0.2.1 (commit abcdef1, built 2026-11-23T12:30:00Z)
  CLIVer:    v0.2.1 (commit abcdef1, built 2026-11-23T12:30:00Z)
  Runtime:   go1.27.4 linux/amd64, goroutines=14, gc=5
  CPU:       3.5% across 8 cores (1.25s total)
  Memory:    rss 24.0 MB (heap 4.00 MB alloc / 6.00 MB in-use / 32.0 MB sys)
  Storage:   local (sqlite 128 KB)
  Content:   dirs=5 (2 protected), items=9 (7 published, 2 drafts), revisions=14, assets=6 (3.00 MB), dir_tokens=1
  Activity:  ingest=1/4, mcp_sessions=3 (anon=2, anon_posts=0)
  Limits:    max_ingest=200 MB, max_revisions=20, remote_images=true, public_url=true, trusted_proxies=1
  ```

## MCP server endpoint (`/mcp`)

`aumbry-server` serves a Streamable HTTP MCP endpoint at **`/mcp`** (`POST`, `GET` SSE, `DELETE`), enabling MCP-capable agents to discover, search, read, validate, and publish text or remote-figure XML reports directly:

- **Three caller identities (headers only — browser cookies are ignored):**
  1. Unauthenticated (anonymous): public directories and their published items/revisions only.
  2. `Authorization: Bearer ad_<...>` (directory read token): subtree authorized for that token without penetrating child passwords.
  3. `Authorization: Bearer <api_token>` with optional `X-Aumbry-Dir-Tokens: ad_...,ad_...`: public directories, token-authorized subtrees, plus the caller token's own drafts, protected items, and revisions (`created_by == digest`); **`push_item` requires an API token**.
- **Available tools (`7`):** `server_info`, `list_dirs`, `list_items`, `search_items`, `get_item`, `validate_xml`, `push_item`.
  - `server_info` reports server version, short commit hash, build timestamp, supported XML protocol versions (`1.1`, `1.2`), caller role (`anonymous`, `dir_token`, `api_token`), `can_push`, and active size/revision limits; callers using an API token also receive live dependency health and uptime.
  - `list_items` and `search_items` omit `xml` and `plain_text`, strip unreadable mounts from `dirs`, and intersect explicit `dir_id` filters with caller visibility (so drafts or out-of-subtree items never bypass `dir_id`).
  - `push_item` shares `/api/v1/ingest` idempotency keys, rate limits, and SSRF-safe remote image localization under two separate size caps (`16 MiB` raw JSON-RPC POST body vs. `10 MiB` decoded XML); use CLI `aumbry push` for documents referencing local relative binary assets (`invalid_request`) or heavily JSON-escaped XML approaching `10 MiB` whose encoded JSON exceeds `16 MiB` (HTTP `413`).
