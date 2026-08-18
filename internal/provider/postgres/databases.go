package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/dbpilot/dbpilot/internal/provider"
)

func (p *Postgres) connect(ctx context.Context, params provider.ConnParams) (*pgx.Conn, error) {
	return pgx.Connect(ctx, BuildDSN(params))
}

func (p *Postgres) connectDB(ctx context.Context, params provider.ConnParams, database string) (*pgx.Conn, error) {
	return pgx.Connect(ctx, buildDSN(params, database))
}

// ListDatabases returns user databases with owner, encoding, collation,
// size and current connection count.
func (p *Postgres) ListDatabases(ctx context.Context, params provider.ConnParams) ([]provider.Database, error) {
	conn, err := p.connect(ctx, params)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT d.datname,
		       pg_catalog.pg_get_userbyid(d.datdba),
		       pg_catalog.pg_encoding_to_char(d.encoding),
		       d.datcollate,
		       pg_catalog.pg_database_size(d.datname),
		       (SELECT count(*) FROM pg_catalog.pg_stat_activity a WHERE a.datname = d.datname)
		FROM pg_catalog.pg_database d
		WHERE d.datallowconn
		ORDER BY d.datname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []provider.Database
	for rows.Next() {
		var d provider.Database
		if err := rows.Scan(&d.Name, &d.Owner, &d.Encoding, &d.Collation, &d.SizeBytes, &d.ConnCount); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DatabaseExists reports whether a database with the given name exists.
func (p *Postgres) DatabaseExists(ctx context.Context, params provider.ConnParams, name string) (bool, error) {
	conn, err := p.connect(ctx, params)
	if err != nil {
		return false, err
	}
	defer conn.Close(ctx)
	var n int
	err = conn.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_database WHERE datname = $1`, name).Scan(&n)
	return n > 0, err
}

// CreateDatabaseRaw creates a database with a safely quoted identifier.
func (p *Postgres) CreateDatabaseRaw(ctx context.Context, params provider.ConnParams, name string) error {
	conn, err := p.connect(ctx, params)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, `CREATE DATABASE `+QuoteIdent(name))
	return err
}

// DropDatabaseRaw drops a database after terminating its active connections.
func (p *Postgres) DropDatabaseRaw(ctx context.Context, params provider.ConnParams, name string) error {
	conn, err := p.connect(ctx, params)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, _ = conn.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, name)
	_, err = conn.Exec(ctx, `DROP DATABASE IF EXISTS `+QuoteIdent(name))
	return err
}

var protectedDatabases = map[string]bool{
	"postgres":  true,
	"template0": true,
	"template1": true,
}

// DeleteDatabase drops a database. It refuses to drop protected databases
// and refuses when there are active connections.
func (p *Postgres) DeleteDatabase(ctx context.Context, params provider.ConnParams, name string) error {
	if protectedDatabases[name] {
		return provider.ErrDatabaseProtected
	}
	conn, err := p.connect(ctx, params)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	var exists bool
	if err := conn.QueryRow(ctx, `SELECT count(*) = 1 FROM pg_catalog.pg_database WHERE datname = $1`, name).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return provider.ErrDatabaseNotFound
	}

	var conns int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_stat_activity WHERE datname = $1`, name).Scan(&conns); err != nil {
		return err
	}
	if conns > 0 {
		return errors.New("database has active connections")
	}

	_, err = conn.Exec(ctx, `DROP DATABASE `+QuoteIdent(name))
	return err
}
