package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/dbpilot/dbpilot/internal/provider"
)

// IsProtectedAccount reports whether an account is a system role that must
// never be deleted or have its login disabled.
func IsProtectedAccount(name string) bool {
	return name == "postgres" || strings.HasPrefix(name, "pg_")
}

func (p *Postgres) ListAccounts(ctx context.Context, params provider.ConnParams) ([]provider.Account, error) {
	conn, err := p.connect(ctx, params)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	var currentUser string
	if err := conn.QueryRow(ctx, `SELECT current_user`).Scan(&currentUser); err != nil {
		return nil, err
	}

	rows, err := conn.Query(ctx, `
		SELECT r.rolname,
		       r.rolcanlogin,
		       r.rolsuper,
		       r.rolcreatedb,
		       r.rolcreaterole,
		       r.rolconnlimit,
		       COALESCE(r.rolvaliduntil::text, ''),
		       (SELECT count(*) FROM pg_catalog.pg_database d WHERE d.datdba = r.oid)
		FROM pg_catalog.pg_roles r
		ORDER BY r.rolname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []provider.Account
	for rows.Next() {
		var a provider.Account
		if err := rows.Scan(&a.Name, &a.CanLogin, &a.Superuser, &a.CreateDB, &a.CreateRole, &a.ConnLimit, &a.ValidUntil, &a.OwnedDBs); err != nil {
			return nil, err
		}
		a.SystemRole = IsProtectedAccount(a.Name)
		a.Protected = a.SystemRole || a.Superuser || a.Name == currentUser
		out = append(out, a)
	}
	return out, rows.Err()
}

func (p *Postgres) accountExists(ctx context.Context, conn *pgx.Conn, name string) (bool, error) {
	var n int
	err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_roles WHERE rolname = $1`, name).Scan(&n)
	return n > 0, err
}

func (p *Postgres) CreateAccount(ctx context.Context, params provider.ConnParams, req provider.CreateAccountRequest) error {
	if req.Name == "" {
		return errors.New("account name is required")
	}
	if IsProtectedAccount(req.Name) {
		return provider.ErrAccountProtected
	}
	conn, err := p.connect(ctx, params)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	exists, err := p.accountExists(ctx, conn, req.Name)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("account already exists")
	}

	var sb strings.Builder
	sb.WriteString("CREATE ROLE " + QuoteIdent(req.Name) + " WITH")
	if req.Login {
		sb.WriteString(" LOGIN")
	} else {
		sb.WriteString(" NOLOGIN")
	}
	sb.WriteString(" PASSWORD " + QuoteLiteral(req.Password))
	if req.Superuser {
		sb.WriteString(" SUPERUSER")
	} else {
		sb.WriteString(" NOSUPERUSER")
	}
	if req.CreateDB {
		sb.WriteString(" CREATEDB")
	} else {
		sb.WriteString(" NOCREATEDB")
	}
	if req.CreateRole {
		sb.WriteString(" CREATEROLE")
	} else {
		sb.WriteString(" NOCREATEROLE")
	}
	if req.ConnLimitSet || req.ConnLimit != 0 {
		sb.WriteString(fmt.Sprintf(" CONNECTION LIMIT %d", req.ConnLimit))
	}
	if req.ValidUntil != "" {
		sb.WriteString(" VALID UNTIL " + QuoteLiteral(req.ValidUntil))
	}
	_, err = conn.Exec(ctx, sb.String())
	return err
}

func (p *Postgres) UpdateAccount(ctx context.Context, params provider.ConnParams, req provider.UpdateAccountRequest) error {
	if req.Name == "" {
		return errors.New("account name is required")
	}
	conn, err := p.connect(ctx, params)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	exists, err := p.accountExists(ctx, conn, req.Name)
	if err != nil {
		return err
	}
	if !exists {
		return provider.ErrAccountNotFound
	}

	var targetSuper bool
	if err := conn.QueryRow(ctx, `SELECT rolsuper FROM pg_catalog.pg_roles WHERE rolname = $1`, req.Name).Scan(&targetSuper); err != nil {
		return err
	}
	var currentUser string
	if err := conn.QueryRow(ctx, `SELECT current_user`).Scan(&currentUser); err != nil {
		return err
	}
	protected := IsProtectedAccount(req.Name) || targetSuper || req.Name == currentUser
	if protected && req.Login != nil && !*req.Login {
		return provider.ErrAccountProtected
	}
	if targetSuper && req.Superuser != nil && !*req.Superuser {
		return provider.ErrAccountProtected
	}
	if req.Name == currentUser && req.NewPassword != nil {
		return provider.ErrCurrentAccountChange
	}

	parts := []string{"ALTER ROLE " + QuoteIdent(req.Name)}
	if req.NewPassword != nil {
		parts = append(parts, "PASSWORD "+QuoteLiteral(*req.NewPassword))
	}
	if req.Login != nil {
		if *req.Login {
			parts = append(parts, "LOGIN")
		} else {
			parts = append(parts, "NOLOGIN")
		}
	}
	if req.Superuser != nil {
		if *req.Superuser {
			parts = append(parts, "SUPERUSER")
		} else {
			parts = append(parts, "NOSUPERUSER")
		}
	}
	if req.CreateDB != nil {
		if *req.CreateDB {
			parts = append(parts, "CREATEDB")
		} else {
			parts = append(parts, "NOCREATEDB")
		}
	}
	if req.CreateRole != nil {
		if *req.CreateRole {
			parts = append(parts, "CREATEROLE")
		} else {
			parts = append(parts, "NOCREATEROLE")
		}
	}
	if req.ConnLimit != nil {
		parts = append(parts, fmt.Sprintf("CONNECTION LIMIT %d", *req.ConnLimit))
	}
	if req.ValidUntil != nil {
		if *req.ValidUntil == "" {
			parts = append(parts, "VALID UNTIL 'infinity'")
		} else {
			parts = append(parts, "VALID UNTIL "+QuoteLiteral(*req.ValidUntil))
		}
	}
	if len(parts) == 1 {
		return errors.New("no changes requested")
	}
	_, err = conn.Exec(ctx, strings.Join(parts, " "))
	return err
}

// DeleteAccount refuses to drop a role that still owns databases, owns
// shared objects, has membership, or has active sessions. It never runs
// DROP OWNED ... CASCADE.
func (p *Postgres) DeleteAccount(ctx context.Context, params provider.ConnParams, name string) error {
	if IsProtectedAccount(name) {
		return provider.ErrAccountProtected
	}
	conn, err := p.connect(ctx, params)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	exists, err := p.accountExists(ctx, conn, name)
	if err != nil {
		return err
	}
	if !exists {
		return provider.ErrAccountNotFound
	}

	var targetSuper bool
	if err := conn.QueryRow(ctx, `SELECT rolsuper FROM pg_catalog.pg_roles WHERE rolname = $1`, name).Scan(&targetSuper); err != nil {
		return err
	}
	var currentUser string
	if err := conn.QueryRow(ctx, `SELECT current_user`).Scan(&currentUser); err != nil {
		return err
	}
	if targetSuper || name == currentUser {
		return provider.ErrAccountProtected
	}

	var reasons []string

	var ownedDBs int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_database WHERE datdba = (SELECT oid FROM pg_catalog.pg_roles WHERE rolname = $1)`, name).Scan(&ownedDBs); err != nil {
		return err
	}
	if ownedDBs > 0 {
		reasons = append(reasons, fmt.Sprintf("owns %d database(s)", ownedDBs))
	}

	var sharedDeps int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_shdepend WHERE refclassid = 'pg_catalog.pg_authid'::regclass AND refobjid = (SELECT oid FROM pg_catalog.pg_roles WHERE rolname = $1)`, name).Scan(&sharedDeps); err != nil {
		return err
	}
	if sharedDeps > 0 {
		reasons = append(reasons, fmt.Sprintf("has %d shared object dependencie(s)", sharedDeps))
	}

	var membership int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM pg_catalog.pg_auth_members
		WHERE roleid = (SELECT oid FROM pg_catalog.pg_roles WHERE rolname = $1)
		   OR member = (SELECT oid FROM pg_catalog.pg_roles WHERE rolname = $1)`, name).Scan(&membership); err != nil {
		return err
	}
	if membership > 0 {
		reasons = append(reasons, "has role membership")
	}

	var sessions int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_stat_activity WHERE usename = $1`, name).Scan(&sessions); err != nil {
		return err
	}
	if sessions > 0 {
		reasons = append(reasons, fmt.Sprintf("has %d active session(s)", sessions))
	}

	if len(reasons) > 0 {
		return fmt.Errorf("%w: %s", provider.ErrAccountHasDeps, strings.Join(reasons, "; "))
	}

	_, err = conn.Exec(ctx, `DROP ROLE `+QuoteIdent(name))
	return err
}
