English | [简体中文](../zh-CN/deployment.md)

# Deployment and operations guide

This guide covers deploying `aumbry-server` with Docker Compose or a host binary, configuring reverse proxies, checking service readiness, and running backup, storage verification, garbage collection, and search reindex commands.

> **Caution — Single instance only:** Aumbry serializes SQLite writes on a single connection, holds an advisory file lock (`aumbry.lock`) in `data_dir`, and stores rate-limit counters in process memory. Do not run two `aumbry-server` processes against the same `data_dir` or private WebDAV root; concurrent processes cause lock conflicts and broken rate limiting.

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

1. Build the frontend assets and both binaries:

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

- **`data_dir` contents:** `aumbry-server` creates `data_dir` with mode `0700`, locks `aumbry.lock` to prevent concurrent instances, and initializes `aumbry.db` in SQLite WAL mode (applying schema migrations automatically on startup).
- **WebDAV object layout and timeouts:** Item XML files live at `/xml/<12-char-id>-<64-hex>.xml`; uploaded images live at `/assets/<64-hex>.<ext>` (`.png`, `.jpg`, `.webp`, `.svg` detected by content MIME sniffing). WebDAV connections time out after `5 s` on TCP dial and `30 s` waiting for response headers.
- **Upload and rate bounds:** `POST /api/v1/ingest` enforces `100 MiB` total request body, `10 MiB` XML payload, `20 MiB` per figure, max `4` concurrent ingest requests, and `120 requests/min` per client IP across `/api/v1/ingest` and `/api/v1/agent/*`. Admin login and directory password unlock endpoints allow `10 requests/min` per client IP.
- **Response headers:** Every HTTP response includes `Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff`, and `X-Frame-Options: DENY`. Raw XML and asset downloads also send `Cache-Control: private` and `Content-Security-Policy: default-src 'none'; sandbox`.

## Health and readiness endpoints

- `GET /healthz`: lightweight liveness endpoint; returns HTTP `200` and `{"status":"ok"}`.
- `GET /readyz`: concurrently probes SQLite and WebDAV (`PROPFIND /`) with a `2 s` timeout, deduplicating concurrent checks and caching WebDAV probe results for `5 s`. Returns HTTP `200` when both report `"ok"`, or HTTP `503` (`"status":"unavailable"`) when either reports `"auth"`, `"timeout"`, or `"unreachable"`:

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

3. After restoring a database snapshot and its paired WebDAV snapshot, verify the restore before serving traffic:

   1. Run `verify-storage` (see below). It must exit `0`:

      ```sh
      aumbry-server verify-storage --config ./aumbry.toml
      ```

   2. Start `aumbry-server`, sign in to the admin page, and open one report, preferably one that contains figures. Confirm that the report text renders and that at least one image displays. `verify-storage` only checks that objects exist; this step confirms that they are readable through the server.

### Verify WebDAV objects (`verify-storage`)

`verify-storage` checks that every item XML file and referenced image asset recorded in `aumbry.db` exists on WebDAV:

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

3. Once `aumbry-server` has stopped cleanly, delete unreferenced remote objects and orphan database asset records:

   ```sh
   aumbry-server gc --config ./aumbry.toml
   ```

   Expected output resembles:

   ```text
   gc complete: deleted 0 xml, 0 remote assets, 0 db asset rows
   ```

4. Restart `aumbry-server`.

### Rebuild the search index (`reindex`)

Search terms of 3 or more characters use the SQLite full-text index (`items_fts`); shorter terms use substring matching on `items.plain_text`. Every `POST /api/v1/ingest` writes both in one transaction. `reindex` re-reads the stored XML of every item from WebDAV, recomputes the indexed text with the rules of the installed version, and rewrites both `items.plain_text` and `items_fts`, so the two search paths stay consistent.

Run `reindex` in either of these cases:

- You upgraded to a version that changes which text is indexed. Documents ingested earlier keep the old index text until you rebuild it.
- Search results disagree with the content of a report, for example after restoring a database snapshot or repairing the database by hand.

Normal ingest, deletion, and `backup` do not require it.

A writing run takes the same `aumbry.lock` as `gc`, so stop `aumbry-server` first. `--dry-run` only reads, so you can run it while the server is up.

1. Preview the run. `--dry-run` reads and parses every item but writes nothing:

   ```sh
   aumbry-server reindex --config ./aumbry.toml --dry-run
   ```

   Expected output resembles:

   ```text
   dry-run: would reindex 12 of 12 items (3 with changed plain_text), 0 failed
   ```

   `with changed plain_text` counts the items whose stored `plain_text` differs from the recomputed text. A real run rewrites the `items_fts` row of every item it can read, whether or not `plain_text` changed.

2. Stop `aumbry-server`. If the server still holds `aumbry.lock`, a writing run exits at once:

   ```text
   refusing to run reindex while aumbry-server is running; stop the server first or pass --dry-run
   ```

3. Run the rebuild. It updates items in batches of 100, one transaction per batch:

   ```sh
   aumbry-server reindex --config ./aumbry.toml
   ```

   Expected output resembles:

   ```text
   reindexed 12 of 12 items (3 with changed plain_text), 0 failed
   ```

4. Restart `aumbry-server`.

If an item's XML cannot be read from WebDAV or is no longer valid XML, `reindex` prints `error: item <id> (<xml path>): ...`, leaves that item's search data unchanged, and continues with the other items. After the last item, the command prints the summary and exits `1`. Fix the cause (run `verify-storage` to find missing objects), then run `reindex` again; repeated runs are safe.

## First release: make the GHCR package public

This section is for maintainers and applies once, after the first release that pushes `ghcr.io/nerdneilsfield/aumbry`. GitHub creates a new container package as private, so `docker pull` from a host that is not signed in to `ghcr.io` fails until you change the package visibility. Later releases keep the setting.

1. After the release workflow succeeds, open the owner's **Packages** tab on GitHub and select `aumbry`.
2. Check that the package page shows the repository `nerdneilsfield/Aumbry`. The image carries the `org.opencontainers.image.source` label, which normally links it automatically. If no repository is shown, select **Connect repository** and choose `nerdneilsfield/Aumbry`.
3. Select **Package settings**, scroll to **Danger Zone**, and select **Change visibility**.
4. Choose **Public**, type the package name `aumbry` to confirm, and select **I understand the consequences, change package visibility**.
5. From a host that is not signed in to `ghcr.io`, pull the released tag, for example `docker pull ghcr.io/nerdneilsfield/aumbry:<version>`. The pull succeeds without credentials.

These steps follow the GitHub package settings pages; they were not run during verification, and GitHub may rename the labels.
