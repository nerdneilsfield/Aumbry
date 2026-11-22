English | [简体中文](../zh-CN/deployment.md)

# Deployment and operations guide

This guide covers deploying `aumbry-server` with Docker Compose or a host binary, configuring reverse proxies, checking service readiness, and running backup, storage verification, and garbage collection commands.

> **Caution — Single instance only:** Aumbry opens one SQLite connection (`SetMaxOpenConns(1)`), holds an advisory file lock (`aumbry.lock`) on `data_dir`, and stores rate-limit counters in process memory. Do not run two `aumbry-server` processes against the same `data_dir` or private WebDAV root; concurrent writers cause lock contention and broken rate limiting.

## Prerequisites

- **WebDAV backend:** A private HTTP/HTTPS WebDAV server reachable from `aumbry-server`. Visitors do not connect to WebDAV directly; `aumbry-server` proxies all XML and image downloads.
- **Container hosts:** Docker Engine and Docker Compose v2 (`ghcr.io/nerdneilsfield/aumbry:latest` supports `linux/amd64` and `linux/arm64`).
- **Source builds:** Go `1.27.1+`, Node.js `22+`, and `npm`. GoReleaser also publishes prebuilt `aumbry` and `aumbry-server` archives for `linux`, `darwin`, and `windows` (`amd64`, `arm64`).
- **Credentials:** An admin password bcrypt hash (`htpasswd -nbBC 10 '' '<your-password>' | cut -d: -f2`), one or more random API tokens, and a cookie secret of at least 32 bytes (`openssl rand -hex 32`).

## Deploy with Docker Compose

Two standalone compose files ship in the repository root. Pick the `.env` workflow for default environment configuration or `docker-compose.toml.yml` if you prefer mounting `aumbry.toml`. Neither compose file deploys a WebDAV container.

### Option A: `.env` workflow (default `docker-compose.yml`)

1. Copy the template file and restrict permissions:

   ```sh
   cp .env.example .env
   chmod 600 .env
   ```

2. Edit `.env` and fill in `AUMBRY_ADMIN_PASSWORD_HASH`, `AUMBRY_API_TOKENS`, `AUMBRY_COOKIE_SECRET`, `AUMBRY_WEBDAV_URL`, `AUMBRY_WEBDAV_USER`, and `AUMBRY_WEBDAV_PASS`. Single-quote values containing `$` (such as `AUMBRY_ADMIN_PASSWORD_HASH='$2y$10$...'`) so `docker compose` does not interpret `$2y` as an environment variable. (`docker-compose.yml` pins `AUMBRY_DATA_DIR=/data` on the service.)

3. Validate the rendered compose configuration and start the service:

   ```sh
   docker compose config > /dev/null
   docker compose up -d --build
   ```

   (`docker compose config` was verified against a dummy `.env`; `docker compose up` was traced in [docker-compose.yml](../../docker-compose.yml) and [Dockerfile](../../Dockerfile) and was not executed because no container runtime daemon was started during verification.)

### Option B: TOML workflow (`docker-compose.toml.yml`)

> **Warning — Secret permissions for container UID `65532`:** `aumbry-server` inside the container runs as `nonroot` (UID/GID `65532`). A host config file with mode `0600` owned by your user cannot be read by UID `65532`. Grant read access only to GID `65532` instead of `0644` so other local users on the host cannot read `cookie_secret` or WebDAV credentials.

1. Copy `config.example.toml` (`data_dir = "/data"` is already set for the container volume `/data`) and fill in all required keys:

   ```sh
   cp config.example.toml aumbry.toml
   ```

2. Restrict ownership and permissions for UID/GID `65532`:

   ```sh
   sudo chgrp 65532 aumbry.toml
   chmod 640 aumbry.toml
   ```

   (`aumbry-server` logs a startup warning whenever the config file is group-readable; if you prefer zero warnings, run `sudo chown 65532:65532 aumbry.toml && chmod 600 aumbry.toml`.)

3. Validate and launch with `docker-compose.toml.yml`:

   ```sh
   docker compose -f docker-compose.toml.yml config > /dev/null
   docker compose -f docker-compose.toml.yml up -d --build
   ```

   (`docker compose -f docker-compose.toml.yml config` exited `0`; `up -d --build` was traced in [docker-compose.toml.yml](../../docker-compose.toml.yml).)

## Deploy as a host binary

1. Build both binaries (`make frontend` compiles `web/` into `internal/webembed/dist/`):

   ```sh
   make setup
   make build
   ```

2. Create `aumbry.toml`, set `data_dir = "./data"` (or another writable host directory), and lock file permissions:

   ```sh
   cp config.example.toml aumbry.toml
   chmod 600 aumbry.toml
   ```

3. Validate the configuration without opening SQLite or connecting to WebDAV:

   ```sh
   ./aumbry-server check-config --config ./aumbry.toml
   ```

   Expected output resembles:

   ```toml
   listen_addr = ':8080'
   data_dir = '/data'
   api_tokens = ['****abcd']
   cookie_secret = '****cdef'
   trusted_proxies = []

   [admin]
   user = 'admin'
   password_hash = '****lhWy'

   [webdav]
   url = 'https://dav.example.com/aumbry'
   user = 'aumbry'
   password = '****9999'
   ```

   Any syntax error, unknown TOML key, malformed bcrypt hash, or missing required setting exits `1` before the server binds a port.

4. Start the server:

   ```sh
   ./aumbry-server --config ./aumbry.toml
   ```

   Send `SIGINT` or `SIGTERM` for graceful shutdown (10 s shutdown timeout).

## Storage layout, limits, and security headers

- **`data_dir` contents:** `aumbry-server` creates `data_dir` with mode `0700`, acquires `aumbry.lock` via `flock`, and initializes `aumbry.db` with `WAL` journal mode, `foreign_keys=1`, and `busy_timeout=5000` (goose migrations apply automatically on startup).
- **WebDAV object layout and timeouts:** Item XML files live at `/xml/<12-char-id>-<64-hex>.xml`; uploaded images live at `/assets/<64-hex>.<ext>` (`.png`, `.jpg`, `.webp`, `.svg` detected by content MIME sniffing). WebDAV connections time out after `5 s` on TCP dial and `30 s` waiting for response headers.
- **Upload and rate bounds:** `POST /api/v1/ingest` enforces `100 MiB` total request body, `10 MiB` XML payload, `20 MiB` per figure, max `4` concurrent ingest requests, and `120 requests/min` per client IP across `/api/v1/ingest` and `/api/v1/agent/*`. Admin login and directory password unlock endpoints allow `10 requests/min` per client IP.
- **Response headers:** Every HTTP response includes `Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff`, and `X-Frame-Options: DENY`. Raw XML and asset downloads also send `Cache-Control: private` and `Content-Security-Policy: default-src 'none'; sandbox`.

## Health and readiness endpoints

- `GET /healthz`: lightweight liveness endpoint; returns HTTP `200` and `{"status":"ok"}`.
- `GET /readyz`: concurrently probes SQLite (`PingContext`) and WebDAV (`PROPFIND /`) with a `2 s` timeout and caches WebDAV probe results for `5 s` via `singleflight`. Returns HTTP `200` when both report `"ok"`, or HTTP `503` (`"status":"unavailable"`) when either reports `"auth"`, `"timeout"`, or `"unreachable"`:

  ```sh
  curl -s http://127.0.0.1:8080/readyz
  ```

  ```json
  {"checks":{"sqlite":"ok","webdav":"ok"},"status":"ok"}
  ```

## Reverse proxy and `trusted_proxies`

By default `trusted_proxies` is empty (`[]`), so `aumbry-server` ignores `X-Forwarded-For` and `X-Forwarded-Proto` and uses the direct TCP peer IP. Session cookies (`aumbry_admin` 7 days; `aumbry_unlocked` 30 days) omit the `Secure` attribute unless the request arrives over direct TLS or from a trusted proxy sending `X-Forwarded-Proto: https`.

To run behind Nginx, Caddy, or Traefik:

1. Set `trusted_proxies` to your proxy IPs or CIDRs, for example `trusted_proxies = ["127.0.0.1", "10.0.0.0/8"]` (or `AUMBRY_TRUSTED_PROXIES=127.0.0.1,10.0.0.0/8`). Only requests whose direct TCP peer matches an entry in `trusted_proxies` honor `X-Forwarded-For` (resolved right-to-left skipping trusted proxies) and `X-Forwarded-Proto: https`.
2. Ensure the reverse proxy passes `X-Forwarded-For`, `X-Forwarded-Proto`, and `Host`, and allows request bodies of at least `100 MiB` (for example `client_max_body_size 100m;` in Nginx) so multi-figure `aumbry push` requests are not truncated before reaching Aumbry.

## Backup, storage verification, and maintenance

Every maintenance command uses the same config resolution (`--config`, `AUMBRY_CONFIG`, `./aumbry.toml`, `/etc/aumbry/config.toml`, or environment variables).

### Back up SQLite safely online (`backup`)

> **Caution — Do not copy `aumbry.db` directly:** In SQLite WAL mode, uncheckpointed transactions live in `aumbry.db-wal`. Copying `aumbry.db` directly can produce an inconsistent snapshot. `aumbry-server backup` runs SQLite `VACUUM INTO` into a pre-created `0600` file, which is safe while `aumbry-server` runs, and cleans up the target file if backup fails.

1. Run an online database snapshot to a non-existent destination path:

   ```sh
   aumbry-server backup --config ./aumbry.toml --out /var/backups/aumbry-$(date +%Y%m%d).db
   ```

   Expected output:

   ```text
   backup written to /var/backups/aumbry-20261122.db
   ```

2. `backup` captures SQLite metadata and full-text search only; updating an item later deletes its superseded XML object from WebDAV. Pair every SQLite snapshot with a simultaneous WebDAV snapshot to achieve full point-in-time recovery.

3. After restoring a backup and WebDAV snapshot, verify integrity against WebDAV before serving traffic:

   ```sh
   aumbry-server verify-storage --config ./aumbry.toml
   ```

### Verify WebDAV objects (`verify-storage`)

`verify-storage` checks that every `items.xml_storage_path` and `assets.storage_path` recorded in `aumbry.db` exists on WebDAV:

```sh
aumbry-server verify-storage --config ./aumbry.toml
```

Expected output when healthy (exits non-zero listing missing paths otherwise):

```text
storage OK: verified 1 xml and 0 asset objects
```

### Garbage collect orphan objects (`gc`)

When an `ingest` request fails after uploading XML to WebDAV (or when legacy assets become unreferenced), unreferenced objects remain on WebDAV so concurrent uploads cannot delete live objects. `gc` restricts deletion to paths matching Aumbry's generated naming patterns (`/xml/<12-char-id>-<64-hex>.xml` and `/assets/<64-hex>.(png|jpg|webp|svg)`); manual files such as `/xml/README.txt` or `/assets/manual.png` are excluded and left untouched.

1. List orphan objects safely while `aumbry-server` is running:

   ```sh
   aumbry-server gc --config ./aumbry.toml --dry-run
   ```

   Expected output resembles:

   ```text
   dry-run: 0 orphan xml, 0 orphan remote assets, 0 orphan db asset rows (0 db-only)
   ```

2. Stop `aumbry-server` before running destructive `gc`. Running destructive `gc` while the server holds `aumbry.lock` fails immediately:

   ```text
   refusing to run destructive gc while aumbry-server is running; stop the server first or pass --dry-run
   ```

3. Once `aumbry-server` has stopped cleanly, delete unreferenced remote objects and orphan `assets` rows:

   ```sh
   aumbry-server gc --config ./aumbry.toml
   ```

   Expected output resembles:

   ```text
   gc complete: deleted 0 xml, 0 remote assets, 0 db asset rows
   ```

4. Restart `aumbry-server`.

### Search index and reindexing

Full-text search queries the `items_fts` SQLite FTS5 virtual table (`trigram` tokenizer) for tokens of 3 or more characters and falls back to escaped `LIKE` predicates for shorter tokens. `aumbry-server` updates `items_fts` transactionally on every `POST /api/v1/ingest` and item deletion. **There is no `reindex` subcommand** in `aumbry-server`; to rebuild the search entry for an item, re-push the XML document with its existing `--key` or `--id`.
