# Aumbry

Aumbry is a Go monorepo for a private, searchable hub for AI-agent research reports. It follows the `nerdneilsfield/go-template` conventions for Cobra commands, structured logging, build metadata, Go tooling, and release builds. The server and CLI share XML schema code, SQLite is the application database, and a Vue frontend is embedded in the server binary.

## Requirements

- Go 1.27.1+
- Node.js 22+ and npm
- Docker (optional)

## Local development

```sh
cp config.example.toml aumbry.toml && chmod 600 aumbry.toml
# Fill in all required settings in aumbry.toml (for local runs, set data_dir = "./data").
make setup
make test
make build
./aumbry-server check-config
./aumbry-server
```

Env-only local runs are still supported via `.env.example`:

```sh
cp .env.example .env
# Fill in all required settings in .env.
make run
```

The server listens on `listen_addr` / `AUMBRY_LISTEN_ADDR` (default `:8080`) and exposes `GET /healthz` (lightweight liveness) and `GET /readyz` (readiness probe checking SQLite and WebDAV with a 2s timeout and 5s WebDAV cache). `data_dir` / `AUMBRY_DATA_DIR` must be writable. Both `aumbry.toml` and `.env` are ignored by Git (keep permissions restricted with `chmod 600`).

## Server operations

### Configuration

Configuration can be supplied via TOML, environment variables, or both.

- **TOML discovery order** (first match wins): `--config <path>` -> `AUMBRY_CONFIG` -> `./aumbry.toml` -> `/etc/aumbry/config.toml`. Explicit paths (`--config` or `AUMBRY_CONFIG`) must exist; if no implicit path exists, the server runs in env-only mode.
- **Precedence**: environment variables override TOML keys, which override built-in defaults (`listen_addr = ":8080"`).
- **Strict validation**: unknown TOML keys are rejected at startup. All missing required keys are reported together. Secrets readable by group/other emit a startup warning (`chmod 600` recommended).
- **Validate without starting**: `aumbry-server check-config [--config path]` prints the effective configuration with secrets masked to their last 4 characters and exits without opening SQLite or connecting to WebDAV.

| TOML key | Environment variable | Required | Notes |
| :-- | :-- | :-- | :-- |
| `listen_addr` | `AUMBRY_LISTEN_ADDR` | No | default `:8080` |
| `data_dir` | `AUMBRY_DATA_DIR` | Yes | holds `aumbry.db` (SQLite, WAL) |
| `admin.user` | `AUMBRY_ADMIN_USER` | Yes | admin login username |
| `admin.password_hash` | `AUMBRY_ADMIN_PASSWORD_HASH` | Yes | valid bcrypt hash; validated at startup |
| `api_tokens` | `AUMBRY_API_TOKENS` | Yes | TOML array or comma-separated env bearer tokens for `POST /api/v1/ingest` |
| `cookie_secret` | `AUMBRY_COOKIE_SECRET` | Yes | at least 32 bytes; changing it signs everyone out |
| `webdav.url` | `AUMBRY_WEBDAV_URL` | Yes | private WebDAV root URL |
| `webdav.user` | `AUMBRY_WEBDAV_USER` | Yes | WebDAV username |
| `webdav.password` | `AUMBRY_WEBDAV_PASS` | Yes | WebDAV password |
| `trusted_proxies` | `AUMBRY_TRUSTED_PROXIES` | No | TOML array or comma-separated IP/CIDR list; default empty |

Generate the admin password hash with Apache's `htpasswd` (`apache2-utils` / `httpd-tools`):

```sh
htpasswd -nbBC 10 '' 'your-password' | cut -d: -f2
```

Inside TOML double-quoted strings (`"..."`), `$` does not need escaping. In `.env`, wrap the hash in single quotes (`AUMBRY_ADMIN_PASSWORD_HASH='$2y$10$...'`), otherwise `make run` and `docker compose` will expand `$VARIABLES`. If you put it in an `environment:` block of `docker-compose.yml`, write every `$` as `$$`.

### Security behaviour

- All HTTP responses include `Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff`, and `X-Frame-Options: DENY`.
- Visitors unlock a password-protected directory with `POST /api/v1/dirs/{id}/unlock`; the unlocked set lives in an encrypted, signed, `HttpOnly`, `SameSite=Lax` cookie (30 days; admin session 7 days). `Secure` is set only when the connection arrives over TLS or via a `trusted_proxies` peer carrying `X-Forwarded-Proto: https`.
- Changing or clearing a directory password invalidates the sessions that were issued for the old password.
- Unlock and admin login are limited to 10 requests per minute per client IP (in memory, per process).
- Write endpoints (`POST /api/v1/ingest` and future agent endpoints) are rate-limited to 120 requests per minute per client IP, and `POST /api/v1/ingest` caps concurrent in-flight uploads at 4.
- Uploaded figures and item XML are served through the server only, with `Cache-Control: private`, `X-Content-Type-Options: nosniff`, and `Content-Security-Policy: default-src 'none'; sandbox` so an SVG or XML opened directly cannot run scripts.

### Behind a reverse proxy

By default `trusted_proxies` is empty, so `X-Forwarded-For` and `X-Forwarded-Proto` are ignored and the direct TCP peer (`r.RemoteAddr`) is used.

When running behind a reverse proxy, set `trusted_proxies` (or `AUMBRY_TRUSTED_PROXIES`) to the proxy's IP(s) or CIDR(s), e.g. `trusted_proxies = ["10.0.0.0/8"]`. Only requests whose direct TCP peer falls inside `trusted_proxies` have their `X-Forwarded-For` (walked right-to-left skipping trusted hops) and `X-Forwarded-Proto: https` honored; untrusted peers cannot spoof these headers.

### Single-instance constraint, backup, and storage maintenance

- **Single instance only**: Aumbry uses a single SQLite writer connection and in-memory rate limiters. Run only one server instance against a given `data_dir` and WebDAV store; running multiple replicas causes database lock contention and inconsistent rate limiting.
- **WAL-safe backup (`backup`)**: Do **not** back up SQLite by copying `aumbry.db` directly while running in WAL mode, as uncheckpointed pages in `-wal` would be omitted. Use `aumbry-server backup --out <path>` instead (runs `VACUUM INTO`, safe online while the server is running). Note that `backup` captures only the SQLite database; updating an item later deletes the superseded XML from WebDAV. For a full point-in-time recovery, pair the database snapshot with a simultaneous WebDAV snapshot, run `aumbry-server verify-storage` after restoring, and open a sample report and figure to confirm.
- **Storage verification (`verify-storage`)**: `aumbry-server verify-storage` checks that every XML and asset path referenced in SQLite exists on WebDAV and exits non-zero if any object is missing.
- **Garbage collection (`gc`)**: `aumbry-server gc --dry-run` lists unreferenced WebDAV XML/asset objects (including failed-upload remnants) and orphan `assets` rows; safe while running. Destructive `aumbry-server gc` (without `--dry-run`) refuses to run while `aumbry-server` holds the `data_dir` lock—stop the server first to avoid deleting objects uploaded by an in-flight `POST /api/v1/ingest` transaction.
- `POST /api/v1/ingest` accepts at most 100 MB per request, 10 MB for the XML and 20 MB per figure (png, jpeg, webp or svg, detected from the file content). The CLI sends each figure's `src` in a repeated `asset_srcs` field in the same order as the `assets[]` files.
- WebDAV client requests bound dial (5s) and response-header wait (30s); `/readyz` uses a dedicated 2s probe deadline.
- Search uses SQLite FTS5 with the trigram tokenizer. Query terms shorter than three characters (typical for two-character Chinese words) cannot use the index and fall back to a `LIKE` scan.

To run the CLI:

```sh
./aumbry validate examples/minimal-report.xml
```

The validator emits JSON by default; `--format text` selects human-readable output. Run `./aumbry schema` to inspect the supported v1.0/v1.1 XML protocol reference.

## 安装 Skill

仓库内随附 Agent Skill 位于 [`skills/aumbry/`](skills/aumbry/SKILL.md)（每次 GitHub Release 压缩包也会在 `skills/aumbry/` 下打包同一份目录，包含 `references/` 与 `assets/examples/`）。

将 Skill 复制或软链到你的 Agent skills 目录（例如 `~/.agents/skills/aumbry`）：

```sh
# 方式一：软链仓库目录（随 git pull 同步）
mkdir -p ~/.agents/skills
ln -sfn "$(pwd)/skills/aumbry" ~/.agents/skills/aumbry

# 方式二：从仓库或 Release 解压目录复制
mkdir -p ~/.agents/skills
cp -R skills/aumbry ~/.agents/skills/aumbry
```

配置 CLI 连接与 Token 后即可让 Agent 按 Skill 自动发现目录、校验并推送：

```sh
aumbry config set --server https://your-aumbry.example --token amb_...
aumbry config show
```

## Container

Multi-architecture images (`linux/amd64`, `linux/arm64`) are published to GHCR at `ghcr.io/nerdneilsfield/aumbry` (with `aumbry-server` as `ENTRYPOINT` and `aumbry` at `/usr/local/bin/aumbry`). After the very first release push, set the package visibility to **public** once in GitHub package settings if anonymous pulls are desired.

Official images do not embed `AUMBRY_*` environment defaults, so `data_dir` / `AUMBRY_DATA_DIR` must be provided via `.env` or TOML. Two standalone compose setups are provided:

```sh
# 1. Env-only (v1.0 default: copy .env.example to .env, no aumbry.toml needed)
docker compose up --build

# 2. TOML-only (copy config.example.toml to aumbry.toml keeping data_dir = "/data";
#    grant read permission to container UID/GID 65532 instead of 0644 so secrets are not world-readable)
sudo chgrp 65532 aumbry.toml && chmod 640 aumbry.toml
docker compose -f docker-compose.toml.yml up --build
```

SQLite data is persisted in the `aumbry-data` volume. Configure a private WebDAV service separately; the compose file does not expose or provision one.

## Repository layout

```text
main.go              Template-style CLI entry point and signal handling
cmd/                 Cobra command tree (version, validate, schema, config, push, dirs, list, unpublish, delete)
cmd/server/          HTTP server entry point and server subcommands
internal/api/        HTTP router, proxy trust, readiness, ingest, agent, directory, item, asset, search and admin handlers
internal/auth/       API tokens, admin login and signed session cookies
internal/buildinfo/  version, commit, and date injected at link time
internal/config/     TOML + environment configuration loading and validation
internal/db/         SQLite connection and initial migrations
internal/logging/    CLI Zap logger writing to stderr
internal/storage/    Storage interface and the private WebDAV adapter
internal/webembed/   Embedded frontend filesystem
pkg/schema/          Shared XML types, v1.0/v1.1 validation, spec, and text extraction
skills/aumbry/       Bundled Agent Skill, generated schema reference, and 5 structural examples
web/                 Vue 3 + TypeScript + Vite application
```
