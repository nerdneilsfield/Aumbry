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
internal/api/        HTTP router and health endpoint
internal/auth/       Session and authorization contracts
internal/buildinfo/  version, commit, and date injected at link time
internal/config/     Environment configuration and validation
internal/db/         SQLite connection and initial migrations
internal/logging/    CLI Zap logger writing to stderr
internal/storage/    Storage interface for the private WebDAV adapter
internal/webembed/   Embedded frontend filesystem
pkg/schema/          Shared XML types, validation, and text extraction
web/                 Vue 3 + TypeScript + Vite application
```

## Scaffold status

The scaffold currently provides environment validation, SQLite schema initialization, the health endpoint, the shared XML metadata validator, and frontend/CLI build entry points. Ingest and push, WebDAV operations, sessions and authorization, directory/item/search APIs, and the full XML renderer are not implemented yet; those API areas must not be used as if they were secured or production-ready.
