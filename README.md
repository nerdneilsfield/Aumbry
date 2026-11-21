# Aumbry

Aumbry is a Go monorepo for a private, searchable hub for AI-agent research reports. It follows the `nerdneilsfield/go-template` conventions for Cobra commands, structured logging, build metadata, Go tooling, and release builds. The server and CLI share XML schema code, SQLite is the application database, and a Vue frontend is embedded in the server binary.

## Requirements

- Go 1.27.1+
- Node.js 22+ and npm
- Docker (optional)

## Local development

```sh
cp .env.example .env
# Fill in all required settings in .env.
make setup
make test
make build
make run
```

The server listens on `AUMBRY_LISTEN_ADDR` (default `:8080`) and exposes `GET /healthz`. `AUMBRY_DATA_DIR` must be writable. Admin credentials, API tokens, a 32-byte cookie secret, and private WebDAV credentials are required by the PRD; the example file intentionally contains no secrets. The `.env` file is ignored by Git.

## Server operations

### Configuration

| Variable | Notes |
| :-- | :-- |
| `AUMBRY_LISTEN_ADDR` | default `:8080` |
| `AUMBRY_DATA_DIR` | holds `aumbry.db` (SQLite, WAL); back up this file |
| `AUMBRY_ADMIN_USER`, `AUMBRY_ADMIN_PASSWORD_HASH` | admin login; the hash must be bcrypt, the server exits at startup otherwise |
| `AUMBRY_API_TOKENS` | comma separated bearer tokens for `POST /api/v1/ingest` |
| `AUMBRY_COOKIE_SECRET` | at least 32 bytes; the cookie encryption and signing keys are derived from it. Changing it signs everyone out |
| `AUMBRY_WEBDAV_URL`, `AUMBRY_WEBDAV_USER`, `AUMBRY_WEBDAV_PASS` | private object store for `/xml/{id}.xml` and `/assets/{sha256}.{ext}`. The server is its only reader and writer |

Generate the admin password hash with Apache's `htpasswd` (`apache2-utils` / `httpd-tools`):

```sh
htpasswd -nbBC 10 '' 'your-password' | cut -d: -f2
```

The hash contains `$` characters. In `.env` wrap it in single quotes, for example `AUMBRY_ADMIN_PASSWORD_HASH='$2y$10$...'`, otherwise `make run` (which sources the file) and `docker compose` (which interpolates `$VARIABLE`) will corrupt it. If you put it in an `environment:` block of `docker-compose.yml` instead, write every `$` as `$$`.

### Security behaviour

- Visitors unlock a password-protected directory with `POST /api/v1/dirs/{id}/unlock`; the unlocked set lives in an encrypted, signed, `HttpOnly`, `SameSite=Lax` cookie (30 days; admin session 7 days). `Secure` is set when the request arrives over TLS or with `X-Forwarded-Proto: https`.
- Changing or clearing a directory password invalidates the sessions that were issued for the old password.
- Unlock and admin login are limited to 10 requests per minute per client IP (in memory, per process, so it resets on restart).
- Uploaded figures are served through the server only, with `Cache-Control: private`, `X-Content-Type-Options: nosniff` and a `sandbox` CSP so an SVG opened directly cannot run scripts.

### Behind a reverse proxy

The rate limiter keys on the TCP peer (`r.RemoteAddr`). Behind a reverse proxy every visitor therefore shares the proxy's address and one bucket. Forwarding headers are deliberately **not** trusted by default, because a client could forge them to evade the limit. If your proxy is trusted, replace `middleware.ClientIPFromRemoteAddr` in `internal/api/router.go` with the chi `middleware.ClientIPFrom*` that matches it (for example `ClientIPFromXFF("10.0.0.0/8")` or `ClientIPFromHeader("X-Real-IP")` for a proxy that overwrites that header). Do not use chi's `RealIP`, which trusts the headers from anyone. Terminate TLS at the proxy and forward `X-Forwarded-Proto` so cookies get the `Secure` flag.

### Limits and notes

- `POST /api/v1/ingest` accepts at most 100 MB per request, 10 MB for the XML and 20 MB per figure (png, jpeg, webp or svg, detected from the file content). The CLI sends each figure's `src` in a repeated `asset_srcs` field in the same order as the `assets[]` files.
- Search uses SQLite FTS5 with the trigram tokenizer. Query terms shorter than three characters (typical for two-character Chinese words) cannot use the index and fall back to a `LIKE` scan.
- Images that no item references any more stay in the private storage; there is no garbage collection in v1.
- The SQLite pool uses a single connection, which serializes writers; this is intended for a personal or small-team instance.

To run the CLI:

```sh
./aumbry validate examples/minimal-report.xml
```

The validator emits JSON by default; `--format text` selects human-readable output.

## Container

After completing `.env`, run `docker compose up --build`. SQLite data is stored in the `aumbry-data` volume. Configure a private WebDAV service separately; the compose file does not expose or provision one.

## Repository layout

```text
main.go              Template-style CLI entry point and signal handling
cmd/                 Cobra command tree, version, validate, config, and push
cmd/server/          HTTP server entry point
internal/api/        HTTP router, ingest, directory, item, asset, search and admin handlers
internal/auth/       API tokens, admin login and signed session cookies
internal/buildinfo/  version, commit, and date injected at link time
internal/config/     Environment configuration and validation
internal/db/         SQLite connection and initial migrations
internal/logging/    CLI Zap logger writing to stderr
internal/storage/    Storage interface and the private WebDAV adapter
internal/webembed/   Embedded frontend filesystem
pkg/schema/          Shared XML types, validation, and text extraction
web/                 Vue 3 + TypeScript + Vite application
```
