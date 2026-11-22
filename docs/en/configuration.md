English | [简体中文](../zh-CN/configuration.md)

# Server configuration reference

`aumbry-server` and its maintenance subcommands (`check-config`, `backup`, `verify-storage`, `gc`) share one strict configuration loader defined in `internal/config/config.go`. Settings can come from a TOML file, environment variables, or a mix of both.

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
- **Whitespace trimming:** Surrounding whitespace is trimmed on every string value and comma-separated list item **except** `AUMBRY_COOKIE_SECRET` and `AUMBRY_WEBDAV_PASS`, which are preserved verbatim.
- **Permission warning:** If the loaded TOML file is readable by group or others (`mode & 0o044 != 0`), the loader emits a warning (`consider chmod 600`).

## Configuration parameters

| TOML key | Environment variable | Type | Required | Default | Description and constraints |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `listen_addr` | `AUMBRY_LISTEN_ADDR` | string | Yes (auto-default) | `":8080"` | TCP address bound by the HTTP server (`host:port` or `:port`). Trimmed; empty falls back to `":8080"`. |
| `data_dir` | `AUMBRY_DATA_DIR` | string | Yes | None | Directory holding `aumbry.db` and `aumbry.lock`. Created with mode `0700` on first start; must be writable. |
| `admin.user` | `AUMBRY_ADMIN_USER` | string | Yes | None | Username required by `POST /api/v1/admin/login`. Non-empty after trimming. |
| `admin.password_hash` | `AUMBRY_ADMIN_PASSWORD_HASH` | string | Yes | None | Valid bcrypt hash (`$2$`, `$2a$`, `$2b$`, `$2x$`, or `$2y$`) with valid cost and 53-char bcrypt-base64 salt+digest body. |
| `api_tokens` | `AUMBRY_API_TOKENS` | list of string (TOML array or CSV env) | Yes | None | Non-empty list of Bearer tokens accepted on `/api/v1/ingest` and `/api/v1/agent/*`. Blank entries are discarded. |
| `cookie_secret` | `AUMBRY_COOKIE_SECRET` | string | Yes | None | Secret used to derive HMAC/AES cookie keys via HKDF-SHA256. Minimum length: `32` bytes; rotating invalidates all cookies. |
| `webdav.url` | `AUMBRY_WEBDAV_URL` | string | Yes | None | Absolute URL with scheme and host pointing to the private WebDAV root. |
| `webdav.user` | `AUMBRY_WEBDAV_USER` | string | Yes | None | Username for HTTP Basic authentication against WebDAV. Non-empty after trimming. |
| `webdav.password` | `AUMBRY_WEBDAV_PASS` | string | Yes | None | Password for HTTP Basic authentication against WebDAV. Used verbatim in env (`AUMBRY_WEBDAV_PASS`). |
| `trusted_proxies` | `AUMBRY_TRUSTED_PROXIES` | list of string (TOML array or CSV env) | No | `[]` | Single IPs or CIDRs (`127.0.0.1`, `10.0.0.0/8`) allowed to supply `X-Forwarded-For` and `X-Forwarded-Proto`. Invalid entries fail startup. |

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

Run `check-config` to verify syntax, required fields, bcrypt validity, proxy CIDRs, and effective overrides without opening SQLite or contacting WebDAV:

```sh
aumbry-server check-config --config ./aumbry.toml
```

Expected output prints the effective configuration in TOML with secrets masked (`****` plus up to the last 4 characters):

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

If configuration warnings exist (for example a group-readable TOML file), `check-config` prints them to `stderr` while still exiting `0` when all fields are valid.
