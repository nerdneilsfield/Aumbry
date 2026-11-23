English | [简体中文](../zh-CN/configuration.md)

# Server configuration reference

`aumbry-server` and its maintenance subcommands (`check-config`, `backup`, `verify-storage`, `gc`, `reindex`) share one strict configuration loader. Settings can come from a TOML file, environment variables, or a mix of both.

## Discovery order and precedence

The loader selects at most one TOML file before applying environment overrides:

1. `--config <path>` flag on `aumbry-server` (must exist; startup fails if missing).
2. `AUMBRY_CONFIG` environment variable (must exist; startup fails if missing).
3. `./aumbry.toml` in the current working directory (optional).
4. `/etc/aumbry/config.toml` (optional).
5. Environment-only mode if neither explicit path is set and neither implicit file exists.

When both TOML values and environment variables are present, precedence flows strictly as:

```text
Environment variables  >  TOML file  >  Built-in defaults
```

Loader guarantees:

- **Strict TOML schema:** Unknown keys inside TOML fail startup (`unknown configuration key(s) ...`).
- **Complete error reporting:** All missing or invalid required keys are validated and reported together in one error output.
- **Misconfiguration guard:** If `storage` (`AUMBRY_STORAGE`) is unset, setting any `[webdav]`, `[s3]`, or `[r2]` value fails startup with a prompt to set `storage`, preventing silent fallback to an empty local directory.
- **Selected-backend validation:** Only the selected storage backend is validated.
- **Whitespace trimming:** Surrounding whitespace is trimmed on every string value and comma-separated list item **except** `AUMBRY_COOKIE_SECRET` and `AUMBRY_WEBDAV_PASS`, which are preserved verbatim.
- **Permission warning:** If the loaded TOML file is readable by group or other users, the server logs a warning (`consider chmod 600`).

## Configuration parameters

### Core settings

| TOML key | Environment variable | Type | Required | Default | Description and constraints |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `listen_addr` | `AUMBRY_LISTEN_ADDR` | string | Yes (auto-default) | `":8080"` | TCP address bound by the HTTP server (`host:port` or `:port`). Trimmed; empty falls back to `":8080"`. |
| `data_dir` | `AUMBRY_DATA_DIR` | string | Yes | None | Directory holding `aumbry.db` and `aumbry.lock`. Created with mode `0700` on first start; must be writable. |
| `storage` | `AUMBRY_STORAGE` | string | No | `"local"` | Object storage backend: `local`, `webdav`, `s3`, or `r2`. |
| `admin.user` | `AUMBRY_ADMIN_USER` | string | Yes | None | Username required by `POST /api/v1/admin/login`. Non-empty after trimming. |
| `admin.password_hash` | `AUMBRY_ADMIN_PASSWORD_HASH` | string | Yes | None | Valid bcrypt hash (`$2$`, `$2a$`, `$2b$`, `$2x$`, or `$2y$`) with valid cost and 53-char bcrypt-base64 salt+digest body. |
| `api_tokens` | `AUMBRY_API_TOKENS` | list of string (TOML array or CSV env) | Yes | None | Non-empty list of Bearer tokens accepted on `/api/v1/ingest` and `/api/v1/agent/*`. Blank entries are discarded. |
| `cookie_secret` | `AUMBRY_COOKIE_SECRET` | string | Yes | None | Root secret used to sign and encrypt session cookies. Minimum length: `32` bytes; rotating invalidates all existing admin and visitor sessions. |
| `trusted_proxies` | `AUMBRY_TRUSTED_PROXIES` | list of string (TOML array or CSV env) | No | `[]` | Single IPs or CIDRs (`127.0.0.1`, `10.0.0.0/8`) allowed to supply `X-Forwarded-For` and `X-Forwarded-Proto`. Invalid entries fail startup. |
| `public_url` | `AUMBRY_PUBLIC_URL` | string | No | `""` | Canonical origin (`http(s)://host[:port]`, no path, query, fragment, or userinfo) for `/feeds/` RSS/Podcast links and the authoritative `Host`/`Origin` allowlist for non-loopback `/mcp` access. When empty, `/feeds/` derives scheme/host from forwarded headers while `/mcp` permits loopback `Host` headers only (`403` on non-loopback hosts). |
| `revisions.max_per_item` | `AUMBRY_REVISIONS_MAX_PER_ITEM` | integer | No | `20` | Maximum published revisions retained per item; pruned oldest-first (`rev ASC`). `0` disables pruning; negative values fail startup. |
| `ingest.max_body_mb` | `AUMBRY_INGEST_MAX_BODY_MB` | integer | No | `200` | Maximum multipart body size in MiB for `POST /api/v1/ingest`. Minimum `20` (`1..19` fails startup; `<= 0` uses default `200`). Reserve at least `4 × max_body_mb` temp disk space. |
| `ingest.allow_remote_images` | `AUMBRY_INGEST_ALLOW_REMOTE_IMAGES` | boolean | No | `true` | Enables SSRF-safe server localization of `http(s)://` remote `<figure src>` URLs in `v1.2` documents during `POST /api/v1/ingest` and MCP `push_item`; set `false` to reject any remote `<figure src>` with `422 REMOTE_ASSET_BLOCKED`. |

### Storage backend settings

| TOML key | Environment variable | Backend | Required | Default / constraint |
| :--- | :--- | :--- | :--- | :--- |
| `local.path` | `AUMBRY_LOCAL_PATH` | `local` | No | Object root directory; defaults to `<data_dir>/objects`. Relative paths resolve against the server working directory. |
| `webdav.url` | `AUMBRY_WEBDAV_URL` | `webdav` | Yes | Absolute URL with scheme and host pointing to the private WebDAV root. |
| `webdav.user` | `AUMBRY_WEBDAV_USER` | `webdav` | Yes | HTTP Basic auth username for WebDAV. |
| `webdav.password` | `AUMBRY_WEBDAV_PASS` | `webdav` | Yes | HTTP Basic auth password for WebDAV (`AUMBRY_WEBDAV_PASS` preserved verbatim). |
| `s3.endpoint` | `AUMBRY_S3_ENDPOINT` | `s3` | No | Optional `http://` or `https://` origin (e.g. `http://minio:9000`) without path, query, or userinfo. Leave empty for AWS S3. |
| `s3.region` | `AUMBRY_S3_REGION` | `s3` | When endpoint is empty | Defaults to `"auto"` when `s3.endpoint` is set and region is omitted (for MinIO, set e.g. `"us-east-1"`). |
| `s3.bucket` | `AUMBRY_S3_BUCKET` | `s3` | Yes | Existing bucket dedicated to this Aumbry database; Aumbry does not create buckets. |
| `s3.access_key_id` | `AUMBRY_S3_ACCESS_KEY_ID` | `s3` | Yes | Static access key ID (masked by `check-config`). |
| `s3.secret_access_key` | `AUMBRY_S3_SECRET_ACCESS_KEY` | `s3` | Yes | Static secret access key (masked by `check-config`). |
| `s3.path_style` | `AUMBRY_S3_PATH_STYLE` | `s3` | No | Default `false`. Self-hosted MinIO typically requires `true`. |
| `r2.account_id` | `AUMBRY_R2_ACCOUNT_ID` | `r2` | Yes | 32-digit hex Cloudflare account ID; derives endpoint `https://<account_id>.r2.cloudflarestorage.com` and region `auto`. For jurisdictional endpoints (such as `.eu.r2.cloudflarestorage.com`), use `storage = "s3"` and set `s3.endpoint`. |
| `r2.bucket` | `AUMBRY_R2_BUCKET` | `r2` | Yes | Existing R2 bucket dedicated to this Aumbry database. |
| `r2.access_key_id` | `AUMBRY_R2_ACCESS_KEY_ID` | `r2` | Yes | R2 S3 API access key ID (masked by `check-config`). |
| `r2.secret_access_key` | `AUMBRY_R2_SECRET_ACCESS_KEY` | `r2` | Yes | R2 S3 API secret access key (masked by `check-config`). |

## Storage backend examples and permissions

Each Aumbry database must have exclusive ownership of its configured directory or bucket; sharing one directory or bucket across multiple Aumbry databases causes `gc` to delete live objects belonging to another instance. Dynamic IAM role chains and `AWS_*` environment variables are not used; only the static keys configured above are read.

**Local directory (default):**

```toml
storage = "local"

[local]
path = "/data/objects" # optional; defaults to <data_dir>/objects
```

**Cloudflare R2** *(checked against official Cloudflare R2 documentation; not verified on a live R2 account)*:

```toml
storage = "r2"

[r2]
account_id = "0123456789abcdef0123456789abcdef"
bucket = "aumbry"
access_key_id = "..."
secret_access_key = "..."
```

**Self-hosted MinIO (`s3`):**

```toml
storage = "s3"

[s3]
endpoint = "http://minio:9000"
region = "us-east-1"
bucket = "aumbry"
path_style = true
access_key_id = "..."
secret_access_key = "..."
```

**AWS S3:**

```toml
storage = "s3"

[s3]
region = "us-east-1"
bucket = "aumbry"
access_key_id = "..."
secret_access_key = "..."
```

**Private WebDAV:**

```toml
storage = "webdav"

[webdav]
url = "https://dav.example.com/aumbry"
user = "aumbry"
password = "replace-with-webdav-password"
```

### Required S3 and R2 permissions

Grant the static credential read, write, delete, and list permissions on the target bucket:

- **AWS S3 / MinIO IAM actions:** `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject` on `arn:aws:s3:::<bucket>/*`, and `s3:ListBucket` on `arn:aws:s3:::<bucket>`.
  If `s3:ListBucket` is missing on AWS S3, `GET`/`HEAD` requests for absent objects return `403 Forbidden` rather than `404 Not Found`; Aumbry treats `403` as `auth` failure rather than missing-object `404`.
- **Cloudflare R2:** Issue an S3 API token with **Object Read & Write** permissions scoped to the target bucket *(checked against official Cloudflare R2 documentation; not verified on a live R2 account)*.

## Generating secrets and escaping `$` in bcrypt hashes

Generate a 32-byte hex cookie secret and an admin bcrypt hash:

```sh
openssl rand -hex 32
htpasswd -nbBC 10 '' '<your-password>' | cut -d: -f2
```

(`openssl rand` was executed directly; `htpasswd` is from Apache `apache2-utils`/`httpd-tools`, traced from `config.example.toml` and `.env.example`.)

Follow format-specific quoting rules for bcrypt hashes containing `$`:

- **TOML double-quoted strings:** Write the hash as-is (`password_hash = "$2y$10$..."`); TOML does not expand `$`.
- **`.env` files (`docker compose` / `make run`):** Wrap the hash in single quotes (`AUMBRY_ADMIN_PASSWORD_HASH='$2y$10$...'`) so `$2y` and `$10` are not expanded as empty shell variables.
- **`docker-compose.yml` inline `environment:` block:** Escape every `$` as `$$` (`$$2y$$10$$...`).

## Validate configuration without starting (`check-config`)

Run `check-config` to verify syntax, required fields, bcrypt validity, proxy CIDRs, and effective overrides without opening SQLite or connecting to object storage:

```sh
aumbry-server check-config --config ./aumbry.toml
```

Expected output prints the effective configuration in TOML with secrets masked (`****` plus up to the last 4 characters, including `s3.access_key_id`, `s3.secret_access_key`, `r2.access_key_id`, `r2.secret_access_key`, and `webdav.password`):

```toml
listen_addr = ':8080'
data_dir = '/data'
storage = 'local'
api_tokens = ['****abcd']
cookie_secret = '****cdef'
trusted_proxies = []

[admin]
user = 'admin'
password_hash = '****lhWy'

[local]
path = '/data/objects'
```

If configuration warnings exist (for example a group-readable TOML file), `check-config` prints them to `stderr` while still exiting `0` when all fields are valid.
