package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/dbpilot/dbpilot/internal/provider"
)

var validPermissions = map[string]bool{
	"owner":      true,
	"read_write": true,
	"read_only":  true,
}

// CreateDatabaseWithAccount implements the one-click "database + account +
// permission" flow. It rolls back the newly created role/database if any
// later step fails, so a partial failure never leaves orphaned objects.
func (p *Postgres) CreateDatabaseWithAccount(ctx context.Context, params provider.ConnParams, req provider.CreateDatabaseRequest) (*provider.CreateResult, error) {
	if req.Database == "" {
		return nil, errors.New("database name is required")
	}
	if req.Account == "" {
		return nil, errors.New("account name is required")
	}
	if !validPermissions[req.Permission] {
		return nil, fmt.Errorf("invalid permission: %s", req.Permission)
	}
	if req.AccountMode != "create" && req.AccountMode != "existing" {
		return nil, fmt.Errorf("invalid account mode: %s", req.AccountMode)
	}

	conn, err := p.connect(ctx, params)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	createdRole := false
	createdDB := false
	cleanup := func() {
		if createdDB {
			_, _ = conn.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, req.Database)
			_, _ = conn.Exec(ctx, `DROP DATABASE IF EXISTS `+QuoteIdent(req.Database))
		}
		if createdRole {
			_, _ = conn.Exec(ctx, `DROP ROLE IF EXISTS `+QuoteIdent(req.Account))
		}
	}

	// 1. database must not already exist
	exists, err := p.DatabaseExists(ctx, params, req.Database)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("database already exists")
	}

	// 2. account handling
	if req.AccountMode == "create" {
		if IsProtectedAccount(req.Account) {
			return nil, provider.ErrAccountProtected
		}
		var n int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_roles WHERE rolname = $1`, req.Account).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, errors.New("account already exists")
		}
		stmt := `CREATE ROLE ` + QuoteIdent(req.Account) + ` WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD ` + QuoteLiteral(req.Password)
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return nil, fmt.Errorf("create account: %w", err)
		}
		createdRole = true
	} else {
		var n int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_roles WHERE rolname = $1`, req.Account).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, provider.ErrAccountNotFound
		}
	}

	// 3. create database
	if req.Permission == "owner" {
		_, err = conn.Exec(ctx, `CREATE DATABASE `+QuoteIdent(req.Database)+` OWNER `+QuoteIdent(req.Account))
	} else {
		_, err = conn.Exec(ctx, `CREATE DATABASE `+QuoteIdent(req.Database))
	}
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("create database: %w", err)
	}
	createdDB = true

	// 4. grant permissions (non-owner modes)
	if req.Permission != "owner" {
		if err := p.grantPrivileges(ctx, params, req.Database, req.Account, req.Permission); err != nil {
			cleanup()
			return nil, fmt.Errorf("grant privileges: %w", err)
		}
	}

	// 5. verify connection (create mode only, we know the password)
	if req.AccountMode == "create" {
		verifyParams := params
		verifyParams.Username = req.Account
		verifyParams.Password = req.Password
		vconn, verr := p.connectDB(ctx, verifyParams, req.Database)
		if verr != nil {
			cleanup()
			return nil, fmt.Errorf("verify connection: %w", verr)
		}
		vconn.Close(ctx)
	}

	host := params.Host
	if host == "" {
		host = "127.0.0.1"
	}
	if params.UnixSocket != "" {
		host = params.UnixSocket
	}
	return &provider.CreateResult{
		Database:   req.Database,
		Account:    req.Account,
		Host:       host,
		Port:       params.Port,
		Permission: req.Permission,
	}, nil
}

func (p *Postgres) grantPrivileges(ctx context.Context, params provider.ConnParams, database, account, permission string) error {
	conn, err := p.connectDB(ctx, params, database)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	// CONNECT must be granted from any database; do it on the new database.
	stmts := []string{
		`GRANT CONNECT ON DATABASE ` + QuoteIdent(database) + ` TO ` + QuoteIdent(account),
		`GRANT USAGE ON SCHEMA public TO ` + QuoteIdent(account),
	}

	var tablePriv, seqPriv, futureTablePriv, futureSeqPriv string
	switch permission {
	case "read_write":
		tablePriv = "SELECT, INSERT, UPDATE, DELETE"
		seqPriv = "USAGE, SELECT, UPDATE"
		futureTablePriv = "SELECT, INSERT, UPDATE, DELETE"
		futureSeqPriv = "USAGE, SELECT, UPDATE"
	case "read_only":
		tablePriv = "SELECT"
		seqPriv = "USAGE, SELECT"
		futureTablePriv = "SELECT"
		futureSeqPriv = "USAGE, SELECT"
	default:
		return fmt.Errorf("unsupported permission: %s", permission)
	}

	// Read-Write may create its own objects in the public schema (migrations).
	if permission == "read_write" {
		stmts = append(stmts, `GRANT CREATE ON SCHEMA public TO `+QuoteIdent(account))
	}

	stmts = append(stmts,
		`GRANT `+tablePriv+` ON ALL TABLES IN SCHEMA public TO `+QuoteIdent(account),
		`GRANT `+seqPriv+` ON ALL SEQUENCES IN SCHEMA public TO `+QuoteIdent(account),
		`ALTER DEFAULT PRIVILEGES FOR ROLE `+QuoteIdent(params.Username)+` IN SCHEMA public GRANT `+futureTablePriv+` ON TABLES TO `+QuoteIdent(account),
		`ALTER DEFAULT PRIVILEGES FOR ROLE `+QuoteIdent(params.Username)+` IN SCHEMA public GRANT `+futureSeqPriv+` ON SEQUENCES TO `+QuoteIdent(account),
	)

	for _, s := range stmts {
		if _, err := conn.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}
