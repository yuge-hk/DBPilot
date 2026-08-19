package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/dbpilot/dbpilot/internal/provider"
)

// GrantDatabasePrivileges grants a permission level (owner / read_write /
// read_only) on a database to an account. It handles existing and future
// objects via ALTER DEFAULT PRIVILEGES for the non-owner levels.
func (p *Postgres) GrantDatabasePrivileges(ctx context.Context, params provider.ConnParams, account, database, permission string) error {
	if account == "" || database == "" {
		return errors.New("account and database are required")
	}
	if !validPermissions[permission] {
		return fmt.Errorf("invalid permission: %s", permission)
	}

	if permission == "owner" {
		conn, err := p.connect(ctx, params)
		if err != nil {
			return err
		}
		defer conn.Close(ctx)
		_, err = conn.Exec(ctx, `ALTER DATABASE `+QuoteIdent(database)+` OWNER TO `+QuoteIdent(account))
		return err
	}

	// CONNECT must be granted from any database; schema/object grants need a
	// connection to the target database.
	conn, err := p.connect(ctx, params)
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, `GRANT CONNECT ON DATABASE `+QuoteIdent(database)+` TO `+QuoteIdent(account))
	conn.Close(ctx)
	if err != nil {
		return err
	}

	return p.grantPrivileges(ctx, params, database, account, permission)
}

// ListDatabasePrivileges reports, for each user database, the account's
// effective privilege level (owner / read_write / read_only / none).
func (p *Postgres) ListDatabasePrivileges(ctx context.Context, params provider.ConnParams, account string) ([]provider.DatabasePrivilege, error) {
	conn, err := p.connect(ctx, params)
	if err != nil {
		return nil, err
	}

	// owner map
	type dbInfo struct {
		name  string
		owner bool
	}
	var dbs []dbInfo
	rows, err := conn.Query(ctx, `
		SELECT datname, (pg_get_userbyid(datdba) = $1) AS is_owner
		FROM pg_catalog.pg_database
		WHERE datallowconn AND datname NOT IN ('template0', 'template1')
		ORDER BY datname`, account)
	if err != nil {
		conn.Close(ctx)
		return nil, err
	}
	for rows.Next() {
		var d dbInfo
		if err := rows.Scan(&d.name, &d.owner); err != nil {
			rows.Close()
			conn.Close(ctx)
			return nil, err
		}
		dbs = append(dbs, d)
	}
	rows.Close()
	conn.Close(ctx)

	out := make([]provider.DatabasePrivilege, 0, len(dbs))
	for _, d := range dbs {
		priv := provider.DatabasePrivilege{Database: d.name, Owner: d.owner}
		if d.owner {
			priv.Level = "owner"
		} else {
			priv.Level = p.accountLevel(ctx, params, account, d.name)
		}
		out = append(out, priv)
	}
	return out, nil
}

func (p *Postgres) accountLevel(ctx context.Context, params provider.ConnParams, account, database string) string {
	conn, err := p.connectDB(ctx, params, database)
	if err != nil {
		return "none"
	}
	defer conn.Close(ctx)

	var canWrite, canRead, canCreate bool
	_ = conn.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM information_schema.role_table_grants WHERE grantee = $1 AND privilege_type = 'INSERT'),
			EXISTS (SELECT 1 FROM information_schema.role_table_grants WHERE grantee = $1 AND privilege_type = 'SELECT'),
			has_schema_privilege($1, 'public', 'CREATE')`,
		account).Scan(&canWrite, &canRead, &canCreate)

	switch {
	case canWrite || canCreate:
		return "read_write"
	case canRead:
		return "read_only"
	default:
		return "none"
	}
}

// RevokeDatabasePrivileges removes an account's privileges (CONNECT, schema
// usage, table/sequence access and default privileges) on a database. It does
// not touch the database owner.
func (p *Postgres) RevokeDatabasePrivileges(ctx context.Context, params provider.ConnParams, account, database string) error {
	if account == "" || database == "" {
		return errors.New("account and database are required")
	}

	conn, err := p.connect(ctx, params)
	if err != nil {
		return err
	}
	_, _ = conn.Exec(ctx, `REVOKE CONNECT ON DATABASE `+QuoteIdent(database)+` FROM `+QuoteIdent(account))
	conn.Close(ctx)

	conn, err = p.connectDB(ctx, params, database)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	stmts := []string{
		`REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM ` + QuoteIdent(account),
		`REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM ` + QuoteIdent(account),
		`REVOKE ALL PRIVILEGES ON SCHEMA public FROM ` + QuoteIdent(account),
		`ALTER DEFAULT PRIVILEGES FOR ROLE ` + QuoteIdent(params.Username) + ` IN SCHEMA public REVOKE ALL ON TABLES FROM ` + QuoteIdent(account),
		`ALTER DEFAULT PRIVILEGES FOR ROLE ` + QuoteIdent(params.Username) + ` IN SCHEMA public REVOKE ALL ON SEQUENCES FROM ` + QuoteIdent(account),
	}
	for _, s := range stmts {
		if _, err := conn.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}
