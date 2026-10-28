package persistence

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Migration struct {
	Version int
	Name    string
	SQL     string
}

var migrations = []Migration{
	{
		Version: 1,
		Name:    "initial_schema",
		SQL: `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS sessions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash TEXT NOT NULL UNIQUE,
    user_id    INTEGER NOT NULL,
    csrf_token TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS secrets (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    value      TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS instances (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL,
    host         TEXT NOT NULL,
    port         INTEGER NOT NULL DEFAULT 5432,
    username     TEXT NOT NULL,
    secret_ref   TEXT NOT NULL,
    ssl_mode     TEXT NOT NULL DEFAULT 'disable',
    unix_socket  TEXT NOT NULL DEFAULT '',
    systemd_unit TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS storage_targets (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL,
    config     TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS backup_policies (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT NOT NULL,
    instance_id    INTEGER NOT NULL,
    database_name  TEXT NOT NULL,
    enabled        INTEGER NOT NULL DEFAULT 1,
    schedule       TEXT NOT NULL,
    retention_count INTEGER NOT NULL DEFAULT 14,
    globals_enabled INTEGER NOT NULL DEFAULT 0,
    storage_targets TEXT NOT NULL DEFAULT '[]',
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS backups (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id      INTEGER NOT NULL,
    database         TEXT NOT NULL,
    backup_type      TEXT NOT NULL DEFAULT 'database',
    format           TEXT NOT NULL DEFAULT 'custom',
    status           TEXT NOT NULL,
    started_at       TEXT NOT NULL,
    finished_at      TEXT NOT NULL DEFAULT '',
    duration_ms      INTEGER NOT NULL DEFAULT 0,
    original_size    INTEGER NOT NULL DEFAULT 0,
    final_size       INTEGER NOT NULL DEFAULT 0,
    sha256           TEXT NOT NULL DEFAULT '',
    pg_dump_version  TEXT NOT NULL DEFAULT '',
    postgres_version TEXT NOT NULL DEFAULT '',
    error_code       TEXT NOT NULL DEFAULT '',
    error_message    TEXT NOT NULL DEFAULT '',
    created_by       TEXT NOT NULL DEFAULT '',
    manifest_path    TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS backup_target_entries (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    backup_id   INTEGER NOT NULL,
    storage_id  INTEGER NOT NULL,
    status      TEXT NOT NULL,
    object_path TEXT NOT NULL DEFAULT '',
    size        INTEGER NOT NULL DEFAULT 0,
    error_code  TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS tasks (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    type         TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'queued',
    progress     INTEGER NOT NULL DEFAULT 0,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id  TEXT NOT NULL DEFAULT '',
    error_code   TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    log          TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    started_at   TEXT NOT NULL DEFAULT '',
    finished_at  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    actor         TEXT NOT NULL,
    action        TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   TEXT NOT NULL DEFAULT '',
    result        TEXT NOT NULL,
    remote_ip     TEXT NOT NULL DEFAULT '',
    detail        TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_backups_instance ON backups(instance_id, database);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at);
`,
	},
	{
		Version: 2,
		Name:    "task_payload",
		SQL: `
ALTER TABLE tasks ADD COLUMN payload TEXT NOT NULL DEFAULT '';
`,
	},
	{
		Version: 3,
		Name:    "backup_policy_schedule",
		SQL: `
ALTER TABLE backup_policies ADD COLUMN last_run_at TEXT NOT NULL DEFAULT '';
ALTER TABLE backup_policies ADD COLUMN next_run_at TEXT NOT NULL DEFAULT '';
`,
	},
	{
		Version: 4,
		Name:    "instance_connection_host",
		SQL: `
ALTER TABLE instances ADD COLUMN connection_host TEXT NOT NULL DEFAULT '';
`,
	},
}

type DB struct {
	*sql.DB
}

func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	raw.SetMaxOpenConns(1)
	if err := raw.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return &DB{raw}, nil
}

func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY, name TEXT NOT NULL,
    applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')))`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := d.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, name) VALUES(?, ?)`, m.Version, m.Name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
