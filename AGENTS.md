# AGENTS.md

Guidance for coding agents (OpenCode, Codex, etc.) working on this repository.

## What This Project Is

DBPilot is a Linux-native, self-hosted web management console for PostgreSQL,
similar in spirit to a lightweight RDS console. It is **not** pgAdmin, a SQL
IDE, 1Panel, a Docker manager, or a Kubernetes platform.

- Single static Go binary; frontend is embedded via `go:embed`
- Runs without Docker / Kubernetes / Node.js at runtime
- Chinese-first UI with full i18n
- Metadata stored in SQLite (DBPilot's own state only, never a copy of PG data)

## Tech Stack

- **Backend**: Go (`net/http` + `chi`, `pgx/v5`, `modernc.org/sqlite` pure-Go
  driver, `go:embed`, `slog`, `yaml.v3`, `x/crypto/bcrypt`). No CGO. Linux-only.
- **Frontend**: Vue 3 + TypeScript + Vite + Vue Router + Pinia + Vue I18n + Naive UI
- **Persistence**: SQLite with versioned migrations (`internal/persistence`)
- **CLI**: `dbpilot version | run | install [--dry-run] | status`

## Repository Layout

```
cmd/dbpilot/            CLI entrypoint (single main.go)
internal/api/           HTTP handlers, chi router, middleware, error codes
internal/audit/         audit log service
internal/auth/          DBPilot admin auth (bcrypt, sessions)
internal/backup/        backup engine, restore, retention, policy, backup store
internal/config/        YAML config loading + defaults
internal/instance/      persisted PostgreSQL instance store
internal/persistence/   SQLite open + migrations (3 versions)
internal/provider/      database Provider interface
internal/provider/postgres/  PostgreSQL Provider implementation
internal/redact/        secret redaction helpers
internal/runner/        CommandRunner abstraction over os/exec
internal/scheduler/     backup policy scheduler (daily/weekly/monthly)
internal/secret/        SecretStore interface + encrypted SQLite impl
internal/storage/       Storage interface + TargetStore
internal/storage/local/     local / mounted directory backend
internal/storage/webdav/    WebDAV backend
internal/storageregistry/   builds a Storage from a target definition
internal/task/          task model/store + background task manager
internal/version/       version string
web/                    Vue 3 frontend (embed.go + src/)
deploy/                 systemd unit
.github/workflows/      CI + release
```

## Architecture Boundaries

Only abstract what actually needs to be swapped. Do not build a universal
framework for hypothetical future databases.

- `provider.Provider` — database operations (`TestConnection`, `ListDatabases`,
  `ListAccounts`, `CreateAccount`, `UpdateAccount`, `DeleteAccount`,
  `DeleteDatabase`, `CreateDatabaseWithAccount`). V0.1 ships only the
  PostgreSQL implementation.
- `storage.Storage` — backup destination (`Test`, `Put`, `Open`, `Delete`,
  `Stat`, `List`). Implementations: local and WebDAV.
- `secret.Store` — centralized secret persistence (`Set` / `Get` / `Delete`),
  AES-GCM encrypted, backed by SQLite. All credentials (PG passwords, WebDAV
  passwords) go through here; never store them in business tables.
- `runner.Runner` — external command execution boundary
  (`Run(ctx, name, args, env)`). All `pg_dump`/`pg_restore`/`psql` calls go
  through it so they are testable.
- `backup.Engine` — orchestrates dump → SHA256 → manifest → upload to N targets
  → retention; also restore and backup deletion.

Keep database-specific logic inside `internal/provider/postgres` and
`internal/backup`. Do not scatter Postgres SQL into unrelated packages.

## Backend Rules

- Respect existing package boundaries; avoid import cycles. The layering is
  roughly: `api` → provider/backup/storage/secret/persistence.
- Keep HTTP handlers thin; put behavior in the provider/engine, not inline SQL
  in handlers.
- Errors use a stable `code` + `message` shape (`internal/api/response.go`).
  The `code` is a machine-readable identifier (e.g. `database_not_found`); the
  frontend maps it to localized text. Never hardcode end-user Chinese strings
  into backend business logic.
- Propagate errors; do not swallow them. No placeholder success, no
  `TODO`-as-done.
- New persisted fields require a new migration in
  `internal/persistence/persistence.go` (append a `Migration` with a unique
  `Version`, run in order, fail-stop). Never edit an already-applied migration
  to "fix" a schema issue on existing installs.

### PostgreSQL Provider Rules

- Always quote identifiers with `QuoteIdent` (`internal/provider/postgres/sql.go`)
  and string literals with `QuoteLiteral`. Never concatenate user input into
  identifiers or SQL.
- Do not claim a PostgreSQL feature works just because the SQL looks correct;
  verify against a real server.
- Compatibility target: PostgreSQL 16 / 17 / 18. 17 and 18 are covered by
  integration tests; 16 is not yet tested. Use stable catalogs
  (`pg_database`, `pg_roles`, `pg_auth_members`, `pg_shdepend`,
  `pg_stat_activity`) and standard DDL.
- Permission templates: Owner (native ownership), Read-Write, Read-Only.
  Read-Write/Read-Only must handle existing objects **and** future objects via
  `ALTER DEFAULT PRIVILEGES`, including sequences and schema usage.
- `pg_dump` major version must not be lower than the server major version;
  detect and reject incompatible tools (`internal/backup/engine.go`).
- Deleting a role must check dependencies (owned databases, shared objects,
  membership, active sessions) and refuse with reasons. Never default to
  `DROP OWNED ... CASCADE`. Never allow dropping protected/system roles
  (`postgres`, `pg_*`).
- One-click create must roll back cleanly on partial failure — no orphan
  roles or databases.

### Backup / Restore / Storage Rules

- Use official CLI tools (`pg_dump`, `pg_restore`, `pg_dumpall`); never
  reimplement the dump protocol.
- Default format is `pg_dump -Fc` (custom). Every backup produces SHA256 and a
  `manifest.json` that matches the artifact.
- Execute commands with `runner.Runner` and argv arrays. **Never** `sh -c` /
  `bash -c` with user input, and never put a password in argv (use `PGPASSWORD`
  via the `env` map).
- Backup success = actual command exit 0 + real artifact + verified hash. No
  fake success.
- Multi-target backups must record per-target status. One target failing must
  not delete another target's success; overall status is
  `success` / `partial_success` / `failed`.
- Restore verifies SHA256 before restoring; on mismatch, refuse. Restore only
  into a new database by default. Do not add destructive overwrite-restore
  without explicit user request.
- Retention (`Keep Last N`) deletes storage objects first, then records. If a
  storage delete fails, keep the record for retry — never orphan files.
- Local storage must defend against path traversal and be careful with
  symlinks. Temp files: restricted path, random names, safe permissions,
  cleanup on failure.

### Secret / Security Rules (non-negotiable)

Never:
- log passwords / connection-string passwords
- record secrets in audit entries
- put secrets in URL query strings
- return stored plaintext passwords from the API
- commit real credentials
- leave real production passwords in test fixtures
- hardcode AES keys / session secrets / WebDAV credentials in source
- build shell commands by concatenating user input
- disable CSRF / auth / rate limiting "to make a test pass"
- bypass a security check to work around an error

When editing auth / secret / restore / backup / SQL-execution code, prioritize
security over making a test pass quickly. If a protection blocks a feature,
understand why it exists before changing it, then implement a compatible
solution. Use `internal/redact` for defense-in-depth on log/audit output.

## Frontend Rules

- Vue 3 + TypeScript + Naive UI + Vue I18n. Default locale `zh-CN`.
- No hardcoded user-visible strings in components — add them to
  `web/src/locales/zh-CN.ts` and reference via `t('...')`.
- Preserve dark/light theme support (`web/src/stores/theme.ts`).
- Do not introduce another UI framework, and do not restyle into a generic
  admin template. Keep a database-console / RDS-console information structure
  (left nav, dense tables, clear status tags, warning/danger for destructive
  actions).
- Map API error `code`s to localized messages in the frontend.
- Do not use `any` to dodge TypeScript errors. Run `npm run typecheck`.

## Testing & Definition of Done

Backend (run after every Go change):

```bash
gofmt -l .        # must print nothing
go vet ./...
go test ./...
go build ./...
```

Frontend (run after every frontend change):

```bash
cd web && npm run typecheck && npm run build
```

PostgreSQL Provider / permissions / backup / restore changes additionally
require the integration tests against a real PostgreSQL:

```bash
DBPILOT_TEST_PG=1 go test ./internal/provider/postgres/ ./internal/backup/
# optional: DBPILOT_TEST_PG_PORT=5433 to target a non-default port
```

The integration tests default to `127.0.0.1:5432`, user/password
`postgres`/`postgres`, sslmode `disable`, and skip when `DBPILOT_TEST_PG` is
unset. If you cannot run them, report **`NOT RUN`**, never `PASSED`.

Permission tests must verify the negative cases too: Read-Only must be denied
INSERT/UPDATE/DELETE (not just allow SELECT); Read-Write must actually exercise
CRUD; Owner must exercise DDL. Backup/restore tests must cover failure paths
(pg_dump failure, storage failure, partial success, checksum mismatch), not
only happy paths.

Before declaring a task done:

1. review `git diff`
2. check for accidentally generated/untracked files
3. check no secrets leaked
4. confirm tests and builds pass
5. confirm no placeholder / mock success remains

## Scope Guardrails (Non-goals)

Do not add any of these without an explicit user request:

- MySQL / MariaDB / Redis
- S3 / MinIO
- Kubernetes; Docker as a required runtime dependency
- HA / Patroni / replication management / PITR / WAL archiving / pgBackRest
- SQL IDE / ER diagrams / graphical schema editor
- File manager / web terminal / SSH client / app store
- Prometheus / Grafana / large monitoring system
- `postgresql.conf` / `pg_hba.conf` visual editors
- Extension marketplace

Keep reasonable `Provider`/`Storage` abstraction, but avoid large-scale
over-abstraction for "future MySQL". Implement the boundary needed today.

## Git & Commit Rules

Never commit: `.env`, secrets, API keys, production credentials, `*.db`,
SQLite WAL/SHM (`*.db-wal`, `*.db-shm`), backups, tmp files, logs,
`node_modules`, frontend build artifacts (except `web/dist/index.html`
placeholder), Go binaries, IDE files, or test-generated data.

An agent must not, unless explicitly asked: `git push`, force push, rebase
public history, delete remote branches, or create releases. Creating a local
commit is allowed when requested.

## Shell / Tool Usage

- Read a file's latest content before editing. If an `edit` `oldString` does
  not match, re-read the file instead of retrying the same string.
- Keep edits small and unique. Run `gofmt` after editing Go files.
- Do not edit `node_modules` or commit build artifacts.
- Do not read large backup files or binaries as if they were source.
- When piping build/test output (`... | tail`), preserve exit status — use
  `set -o pipefail` or otherwise avoid masking a failing command's exit code.

## Documentation Rules

Do not create per-session files like `PHASE1.md`, `PROGRESS.md`, `NOTES.md`,
`SUMMARY.md`, `TEMP.md` in the repo root. Long-lived content belongs in
`README.md`, `CHANGELOG.md`, `CONTRIBUTING.md`, `SECURITY.md`, or `docs/`.
`AGENTS.md` describes how to modify the project; it is not a README duplicate.
