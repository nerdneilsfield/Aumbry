English | [简体中文](README.zh-CN.md)

# Aumbry

Aumbry is a self-hosted hub for publishing, organizing, and searching structured XML research reports produced by AI agents or people.

[![CI](https://github.com/nerdneilsfield/Aumbry/actions/workflows/ci.yml/badge.svg)](https://github.com/nerdneilsfield/Aumbry/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/github/license/nerdneilsfield/Aumbry)](LICENSE)

## Highlights

- **Single server binary with embedded web reader**: `aumbry-server` bundles the web reader and admin console in one binary, serving `/d/<id>`, `/r/<id>`, `/p/<id>`, `/admin`, RSS/Podcast feeds under `/feeds/`, and a Streamable HTTP MCP server at `/mcp`.
- **Shared XML protocol (`1.2`) and offline validator**: The `aumbry` CLI and `aumbry-server` enforce the same XML schema rules (`version="1.2"`, with full backward compatibility for `1.1`), supporting `<audio>` players/chapters/transcripts, optional `<attachment type="pdf">` Canvas viewing (`pdfjs-dist` range loading, pure-Go `pdfcpu` page validation), SSRF-guarded remote `<figure src="http(s)://...">` localization, and symlink escape checks.
- **SQLite metadata, revisions, backlinks, and full-text search**: Metadata, directory mounts, published revision snapshots (with client-side block diffs), briefing backlinks, and FTS5 indexes live in `<data_dir>/aumbry.db` (SQLite WAL mode), supporting Chinese search from two characters up (single characters work too), full-text queries for other terms of 3+ characters, and substring matching for shorter terms.
- **Directory access control, read tokens, and private object storage**: Unmounted items return HTTP `403` to visitors, directories inherit ancestor passwords across the tree, directory-scoped read tokens (`ad_...`) power protected `/feeds/` RSS/Podcast subscriptions and `/mcp` access without penetrating independently locked child directories, and objects stored in the private backend (`local`, `webdav`, `s3`, or `r2`) are served via authenticated handlers (`Range` supported) with `Content-Security-Policy: default-src 'none'; sandbox`.
- **Bundled Agent Skill & MCP endpoint**: [skills/aumbry/](skills/aumbry/SKILL.md) ships inside the repository and release archives alongside the built-in `/mcp` endpoint (`list_dirs`, `list_items`, `search_items`, `get_item`, `validate_xml`, and `push_item`).

## Overview

Aumbry separates authoring from hosting so research agents do not write directly to your database or expose files on a public bucket. An agent writes a structured XML report alongside local images, audio, or PDFs, runs `aumbry validate` offline, and pushes via `aumbry push` (`POST /api/v1/ingest`) or the `/mcp` `push_item` tool using a Bearer token from `api_tokens`. Visitors receive links to individual directories (`/d/<dir-id>`), items (`/r/<id>`, `/p/<id>`), or feeds (`/feeds/<dir-id>/rss.xml` and `podcast.xml`); the root page (`/`) displays no public index.

Aumbry is open-source under the MIT license (`github.com/nerdneilsfield/aumbry`), with protocol `1.2` on `master`. It runs as a single-instance private research hub rather than a public multi-user wiki or a distributed cluster: one `aumbry-server` process owns a given `data_dir` and object store.

## Installation

Choose the prebuilt container image or build from source. Prebuilt CLI and server archives are also released for `linux`, `darwin`, and `windows` (`amd64` and `arm64`) through GoReleaser.

**Container (`linux/amd64`, `linux/arm64`)** — requires Docker Engine and Docker Compose v2 (uses `local` storage inside the `/data` volume by default, or connect `webdav` / `s3` / `r2`):

```sh
docker pull ghcr.io/nerdneilsfield/aumbry:latest
```

The image runs as `nonroot` (UID/GID `65532`), sets `aumbry-server` as the entrypoint, and installs `aumbry` at `/usr/local/bin/aumbry`.

**Homebrew** installs both `aumbry` and `aumbry-server`:

```sh
brew install --cask nerdneilsfield/apps/aumbry
```

**Scoop (Windows)** installs both executables from the same bucket:

```powershell
scoop bucket add nerdneilsfield https://github.com/nerdneilsfield/homebrew-apps
scoop install nerdneilsfield/aumbry
```

**From source** — requires Go `1.27.1` or later and Node.js `22` or later with `npm`:

```sh
git clone https://github.com/nerdneilsfield/Aumbry.git
cd Aumbry
make setup
make build
```

This writes `./aumbry-server` and `./aumbry` into the repository root.

## Quick start

This example starts Aumbry from a `.env` file with Docker Compose and validates the bundled sample report with the CLI.

> **Prerequisite:** Prepare a bcrypt hash of your admin password using `htpasswd -nbBC 10 '' '<your-password>' | cut -d: -f2` before starting the server. By default `storage` is `local` (storing objects in `/data/objects`); if you use `webdav`, `s3`, or `r2`, provision that endpoint or bucket first.

1. Copy the environment template and fill in every required variable:

   ```sh
   cp .env.example .env
   chmod 600 .env
   ```

   Single-quote `AUMBRY_ADMIN_PASSWORD_HASH` in `.env` (for example `AUMBRY_ADMIN_PASSWORD_HASH='$2y$10$...'`) so `docker compose` does not expand `$` tokens as variables. Keep `AUMBRY_DATA_DIR=/data` for the container volume.

2. Start the server:

   ```sh
   docker compose up -d --build
   ```

   (Traced from `docker-compose.yml` and `Dockerfile`; run on a host with Docker daemon access.)

3. Verify liveness and readiness (wait until storage and SQLite report `ok`):

   ```sh
   curl -s http://127.0.0.1:8080/healthz
   curl -s http://127.0.0.1:8080/readyz
   ```

   Expected responses:

   ```json
   {"status":"ok"}
   ```

   ```json
   {"checks":{"sqlite":"ok","storage":"ok"},"status":"ok"}
   ```

4. Validate the bundled sample XML offline (works before setting up a server):

   ```sh
   ./aumbry validate examples/minimal-report.xml --format text
   ```

   Expected output:

   ```text
   valid: examples/minimal-report.xml
   ```

5. Open `http://127.0.0.1:8080/admin` in your browser, sign in with your configured admin credentials, and select **新建根目录** to create your first directory (note its 12-character ID or fetch it with `aumbry dirs`).

For TOML container configuration (`docker-compose.toml.yml`), host binary runs, backups, and offline garbage collection, see [Deployment guide](docs/en/deployment.md). Compose examples for running behind Traefik, nginx, or Caddy are in [`examples/deploy/`](examples/deploy/).

## Usage

**Configure the CLI and push a report:**

> **Warning:** `aumbry config set` stores the Bearer `token` in plaintext inside `$XDG_CONFIG_HOME/aumbry/config.yaml` (or `~/.config/aumbry/config.yaml`) with mode `0600`. Use environment variables (`AUMBRY_SERVER`, `AUMBRY_TOKEN`) in ephemeral CI runners.

```sh
aumbry config set --server http://127.0.0.1:8080 --token <your-api-token>
aumbry dirs --format tree
aumbry push examples/minimal-report.xml --key demo:report --dir <dir-id>
```

Example `aumbry push` output:

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

**Install the Agent Skill:**

Requires Node.js `22.20+` (`vercel-labs/skills` CLI). Install globally across projects:

```sh
npx skills add nerdneilsfield/aumbry --skill aumbry -g
```

The agent also needs the `aumbry` CLI binary on `PATH` (see [Installation](#installation) for prebuilt Release archives or building from source). See [Agents and the Aumbry Skill](docs/en/agents-and-skill.md) for agent-specific flags, non-interactive installs, offline/archive installation, and directory selection rules.

## Configuration

Server configuration loads from TOML (`--config` -> `AUMBRY_CONFIG` -> `./aumbry.toml` -> `/etc/aumbry/config.toml`) and overrides any key with environment variables. Unknown TOML keys fail startup. Validate merged settings without starting the server via `aumbry-server check-config`.

| TOML key | Environment variable | Required | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `listen_addr` | `AUMBRY_LISTEN_ADDR` | Yes (auto-default) | `":8080"` | HTTP bind address |
| `data_dir` | `AUMBRY_DATA_DIR` | Yes | None | Directory holding `aumbry.db` (WAL) |
| `storage` | `AUMBRY_STORAGE` | No | `"local"` | Storage backend: `local`, `webdav`, `s3`, or `r2` |
| `local.path` | `AUMBRY_LOCAL_PATH` | No | `<data_dir>/objects` | Object root when `storage = "local"` |
| `admin.user` | `AUMBRY_ADMIN_USER` | Yes | None | Admin login username |
| `admin.password_hash` | `AUMBRY_ADMIN_PASSWORD_HASH` | Yes | None | Validated bcrypt password hash |
| `api_tokens` | `AUMBRY_API_TOKENS` | Yes | None | Bearer tokens for CLI and agent API |
| `cookie_secret` | `AUMBRY_COOKIE_SECRET` | Yes | None | Cookie signing secret (>= 32 bytes) |
| `webdav.*` | `AUMBRY_WEBDAV_*` | When `storage = "webdav"` | None | Private WebDAV URL, user, password |
| `s3.*` | `AUMBRY_S3_*` | When `storage = "s3"` | None | Endpoint, region, bucket, static keys, `path_style` |
| `r2.*` | `AUMBRY_R2_*` | When `storage = "r2"` | None | Cloudflare R2 `account_id`, bucket, static keys |
| `trusted_proxies` | `AUMBRY_TRUSTED_PROXIES` | No | `[]` | Reverse proxy IPs/CIDRs for forwarding headers |
| `public_url` | `AUMBRY_PUBLIC_URL` | No | `""` | Canonical `http(s)://host[:port]` for feeds and non-loopback `/mcp` Host/Origin checks |
| `revisions.max_per_item` | `AUMBRY_REVISIONS_MAX_PER_ITEM` | No | `20` | Published revisions retained per item (`0` = unlimited) |
| `ingest.max_body_mb` | `AUMBRY_INGEST_MAX_BODY_MB` | No | `200` | Max `/api/v1/ingest` request size in MiB (min `20`) |
| `ingest.allow_remote_images` | `AUMBRY_INGEST_ALLOW_REMOTE_IMAGES` | No | `true` | Localize public `http(s)://` `<figure src>` URLs in `v1.2` |

For all validation rules, secret masking rules, and container UID `65532` file permission settings, see [Server configuration reference](docs/en/configuration.md).

## Documentation

| Guide | Covers |
| :--- | :--- |
| [Deployment guide](docs/en/deployment.md) | Docker Compose (`.env` and TOML), binary runs, reverse proxies (`/mcp` + `/feeds/`), WAL-safe `backup`, `verify-storage`, `gc`, `reindex`, single-instance bounds |
| [Server configuration reference](docs/en/configuration.md) | Config lookup order, env overrides, complete key/type table, `htpasswd` escaping, `check-config` |
| [Directories and access control](docs/en/directories-and-access.md) | Directory tree, password inheritance, `protected` vs `has_password`, `ad_` tokens, dual-gate revision history, `/mcp` auth matrix |
| [CLI reference](docs/en/cli.md) | Profiles, env overrides, `validate`, `push`, `dirs`, `list`, `unpublish`, `delete`, `/mcp` endpoint & tools |
| [Agents and the Aumbry Skill](docs/en/agents-and-skill.md) | Skill installation, target-directory decision flow, draft vs publish authorization, MCP usage & token scoping |
| [XML protocol overview](docs/en/protocol.md) | Protocol purpose, `1.2` / `1.1` rules (`<audio>`, `<attachment type="pdf">`, remote figures), `aumbry schema`, pointer to full generated reference |
| [Known issues](docs/en/known-issues.md) | Understood problems that are not fixed yet, with when you hit them and what to do |

The repository issue tracker and discussions live on GitHub (`github.com/nerdneilsfield/Aumbry`).

## Security and operational notes

- **Single instance only**: Aumbry serializes SQLite writes on a single connection and tracks login/unlock/write/MCP rate limits in process memory (`10 req/min` on unlock/login; `120 req/min` and max `4` concurrent `/ingest` uploads). Run one replica per `data_dir` and object store.
- **Reverse proxies, cookies, and `/mcp`**: `trusted_proxies` is empty by default, ignoring `X-Forwarded-For` and `X-Forwarded-Proto`. Configure the proxy IP/CIDR so client IPs are resolved right-to-left and `Secure` session cookies (`aumbry_admin` 7 days, `aumbry_unlocked` 30 days, `HttpOnly`, `SameSite=Lax`) are issued over HTTPS. For non-loopback `/mcp` access behind a proxy, configure `public_url` (`http(s)://host[:port]`) and forward the full `Host` including port (`proxy_set_header Host $http_host;`).
- **SSRF-safe remote image localization**: When `ingest.allow_remote_images` is enabled (default), remote `<figure src>` downloads explicitly set `Proxy: nil` (ignoring `HTTP_PROXY`/`HTTPS_PROXY`), validate socket IPs on connect and across redirects (`control` + `.Unmap()`), and parse SVGs strictly without DTDs.
- **Online backup vs offline GC**: Snapshot SQLite with `aumbry-server backup --out <path>` (`VACUUM INTO`) rather than copying `aumbry.db` directly in WAL mode. Run destructive `aumbry-server gc` only after stopping `aumbry-server`; `gc` deletes only unreferenced Aumbry-named XML and asset objects.

## Development

Build, lint, test, and validate skill examples against the Go schema:

```sh
make fmt-check
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
make skill-check
```

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE).
