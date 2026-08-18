# DBPilot

A lightweight, self-hosted web management console for Linux-native PostgreSQL.

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev)
[![Vue](https://img.shields.io/badge/Vue-3-42b883.svg)](https://vuejs.org)

DBPilot brings the simplicity of a cloud database console — think AWS RDS,
Aliyun RDS, or TencentDB — to PostgreSQL that you run and own yourself.
Create databases, manage accounts, grant permissions, and back up / restore,
all from a clean web UI.

> [简体中文](README-zh.md)

DBPilot is **not** pgAdmin, a SQL IDE, 1Panel, a Docker manager, or a
Kubernetes platform.

## Highlights

- **Linux Native** — built for self-hosted PostgreSQL, not a container orchestration platform
- **Single Binary** — one static Go binary with the frontend embedded via `go:embed`
- **No Docker Required** — runs directly on Linux with `systemd`; no Node.js or nginx at runtime
- **Chinese-first UI** — default `zh-CN`, with a complete i18n architecture ready for more locales
- **Backup anywhere** — Local directory, NFS / SMB mounts, and WebDAV (AList, Nextcloud, NAS, ...)

## Features

- PostgreSQL instance management with connection testing and client-tool detection
- Database and account (role) management with system-role protection
- **One-click create** — database + dedicated account + permission template in a single step, with a connection string and safe rollback on partial failure
- Permission templates: **Owner**, **Read-Write**, **Read-Only** (existing *and* future objects via `ALTER DEFAULT PRIVILEGES`)
- Logical backup with `pg_dump` (custom `-Fc` format) + SHA256 + `manifest.json`
- Multi-target backup with per-target status (`success` / `partial_success` / `failed`)
- Scheduled backups (daily / weekly / monthly) and `Keep Last N` retention
- Restore into a new database with checksum and manifest verification
- Background task system and audit log

## Supported PostgreSQL

| Version | Status |
|---|---|
| PostgreSQL 16 | target |
| PostgreSQL 17 | tested |
| PostgreSQL 18 | tested |

DBPilot uses stable system catalogs (`pg_database`, `pg_roles`, ...) and
standard DDL, so it targets PostgreSQL 16 / 17 / 18.

> Backup and restore rely on the system `pg_dump` / `pg_restore`. Install a
> client matching your server's major version, otherwise DBPilot refuses to
> run and reports the mismatch:
>
> ```bash
> apt install postgresql-client-17   # for PostgreSQL 17
> apt install postgresql-client-18   # for PostgreSQL 18
> ```

## Quick Start

```bash
# Install (creates the dbpilot user, directories, systemd unit, and starts it)
sudo dbpilot install

# Or run directly
dbpilot run --config /etc/dbpilot/config.yaml
```

Open `http://127.0.0.1:8080`, complete the admin setup, then:

```text
Add a PostgreSQL instance (test the connection)
→ one-click create a database + dedicated account (Owner)
→ copy the connection string
→ configure a backup target (/mnt/nas/postgres or WebDAV)
→ schedule daily backups with retention
```

## CLI

```bash
dbpilot version              # print version
dbpilot run                  # run the server
dbpilot install [--dry-run]  # system user, dirs, config, systemd unit
dbpilot status               # runtime status
```

## Development

Backend:

```bash
go build ./...
go test ./...
go vet ./...
```

Frontend:

```bash
cd web
npm install
npm run typecheck
npm run build
```

Integration tests (require a local PostgreSQL; default `127.0.0.1:5432`,
`postgres` / `postgres`):

```bash
DBPILOT_TEST_PG=1 go test ./internal/provider/postgres/ ./internal/backup/
# optional: DBPILOT_TEST_PG_PORT=5433 to target a non-default port
```

## Architecture

- **Backend** — Go (`net/http` + `chi`, `pgx/v5`, `modernc.org/sqlite`,
  `go:embed`, `slog`), no CGO, Linux-only
- **Frontend** — Vue 3 + TypeScript + Vite + Pinia + Vue I18n + Naive UI
- **Persistence** — SQLite (DBPilot metadata only) with versioned migrations
- **Credentials** — encrypted centrally through a `SecretStore` (AES-GCM),
  never stored in business tables

## Security

Passwords are never logged, audited, or returned by the API. SQL identifiers
are always quoted; external tools are invoked with argv arrays, never through
a shell. Sessions use HttpOnly + SameSite cookies with CSRF protection and
login rate limiting. See [SECURITY.md](SECURITY.md) for more.

## Acknowledgments

DBPilot is developed with the assistance of
[DeepSeek](https://www.deepseek.com) and [OpenCode](https://opencode.ai).

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[Apache-2.0](LICENSE)
