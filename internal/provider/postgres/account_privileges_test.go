package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/dbpilot/dbpilot/internal/provider"
)

func TestGrantRevokeDatabasePrivileges(t *testing.T) {
	params := integrationParams(t)
	ctx := context.Background()
	p := New(nil)

	const db = "dbpilot_it_priv"
	const acc = "dbpilot_it_priv_user"

	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, acc)

	// create db (owned by admin) + account
	_ = p.CreateDatabaseRaw(ctx, params, db)
	if err := p.CreateAccount(ctx, params, provider.CreateAccountRequest{Name: acc, Password: "privpass123", Login: true}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	defer p.DeleteDatabase(ctx, params, db)
	defer p.DeleteAccount(ctx, params, acc)

	// seed a table as admin
	execOn(t, ctx, params, db, "CREATE TABLE t(id int, v text); INSERT INTO t VALUES (1,'a');")

	// grant read_only
	if err := p.GrantDatabasePrivileges(ctx, params, acc, db, "read_only"); err != nil {
		t.Fatalf("grant read_only: %v", err)
	}
	privs, err := p.ListDatabasePrivileges(ctx, params, acc)
	if err != nil {
		t.Fatalf("list privileges: %v", err)
	}
	if got := findLevel(privs, db); got != "read_only" {
		t.Errorf("expected read_only, got %q", got)
	}

	c := connectAs(t, ctx, acc, "privpass123", db)
	var n int
	if err := c.QueryRow(ctx, "SELECT count(*) FROM t").Scan(&n); err != nil {
		t.Errorf("read_only select failed: %v", err)
	}
	if _, err := c.Exec(ctx, "INSERT INTO t VALUES (2,'b')"); err == nil {
		t.Errorf("read_only insert should fail")
	}
	c.Close(ctx)

	// upgrade to read_write
	if err := p.GrantDatabasePrivileges(ctx, params, acc, db, "read_write"); err != nil {
		t.Fatalf("grant read_write: %v", err)
	}
	c = connectAs(t, ctx, acc, "privpass123", db)
	if _, err := c.Exec(ctx, "INSERT INTO t VALUES (2,'b')"); err != nil {
		t.Errorf("read_write insert should succeed: %v", err)
	}
	c.Close(ctx)

	// revoke -> no table access (connect stays via PUBLIC grant, but schema
	// usage and table privileges are removed)
	if err := p.RevokeDatabasePrivileges(ctx, params, acc, db); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	c = connectAs(t, ctx, acc, "privpass123", db)
	if _, err := c.Exec(ctx, "SELECT count(*) FROM t"); err == nil {
		t.Errorf("revoke should deny table access")
	}
	c.Close(ctx)

	privs, _ = p.ListDatabasePrivileges(ctx, params, acc)
	if got := findLevel(privs, db); got != "none" {
		t.Errorf("expected none after revoke, got %q", got)
	}
}

func TestGrantDowngradeReadWriteToReadOnly(t *testing.T) {
	params := integrationParams(t)
	ctx := context.Background()
	p := New(nil)

	const db = "dbpilot_it_downgrade"
	const acc = "dbpilot_it_downgrade_u"

	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, acc)

	_ = p.CreateDatabaseRaw(ctx, params, db)
	if err := p.CreateAccount(ctx, params, provider.CreateAccountRequest{Name: acc, Password: "downgradepass", Login: true}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	execOn(t, ctx, params, db, "CREATE TABLE t(id int)")

	if err := p.GrantDatabasePrivileges(ctx, params, acc, db, "read_write"); err != nil {
		t.Fatalf("grant read_write: %v", err)
	}
	if err := p.GrantDatabasePrivileges(ctx, params, acc, db, "read_only"); err != nil {
		t.Fatalf("downgrade to read_only: %v", err)
	}

	privs, err := p.ListDatabasePrivileges(ctx, params, acc)
	if err != nil {
		t.Fatalf("list privileges: %v", err)
	}
	if got := findLevel(privs, db); got != "read_only" {
		t.Errorf("expected read_only after downgrade, got %q", got)
	}

	c := connectAs(t, ctx, acc, "downgradepass", db)
	if _, err := c.Exec(ctx, "INSERT INTO t VALUES (1)"); err == nil {
		t.Errorf("insert should be denied after downgrade to read_only")
	}
	c.Close(ctx)

	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, acc)
}

func TestFreshReadOnlyLevel(t *testing.T) {
	params := integrationParams(t)
	ctx := context.Background()
	p := New(nil)

	const db = "dbpilot_it_fresh_ro"
	const acc = "dbpilot_it_fresh_ro_u"

	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, acc)

	if _, err := p.CreateDatabaseWithAccount(ctx, params, provider.CreateDatabaseRequest{
		Database: db, AccountMode: "create", Account: acc, Password: "freshropass", Permission: "read_only",
	}); err != nil {
		t.Fatalf("create read_only database: %v", err)
	}

	privs, err := p.ListDatabasePrivileges(ctx, params, acc)
	if err != nil {
		t.Fatalf("list privileges: %v", err)
	}
	if got := findLevel(privs, db); got != "read_only" {
		t.Errorf("fresh read_only database should report read_only, got %q", got)
	}

	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, acc)
}

func TestRevokeOwnerIsRefused(t *testing.T) {
	params := integrationParams(t)
	ctx := context.Background()
	p := New(nil)

	const db = "dbpilot_it_owner_revoke"
	const acc = "dbpilot_it_owner_revoke_u"

	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, acc)

	if _, err := p.CreateDatabaseWithAccount(ctx, params, provider.CreateDatabaseRequest{
		Database: db, AccountMode: "create", Account: acc, Password: "ownerrevokepass", Permission: "owner",
	}); err != nil {
		t.Fatalf("create owner database: %v", err)
	}

	if err := p.RevokeDatabasePrivileges(ctx, params, acc, db); !errors.Is(err, provider.ErrAccountOwnsDatabase) {
		t.Errorf("revoking owner privileges should return ErrAccountOwnsDatabase, got %v", err)
	}

	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, acc)
}

func findLevel(privs []provider.DatabasePrivilege, db string) string {
	for _, p := range privs {
		if p.Database == db {
			return p.Level
		}
	}
	return ""
}

func TestReadOnlySeesReadWriteFutureTables(t *testing.T) {
	params := integrationParams(t)
	ctx := context.Background()
	p := New(nil)

	const db = "dbpilot_it_future_objects"
	const rw = "dbpilot_it_future_rw"
	const ro = "dbpilot_it_future_ro"

	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, rw)
	_ = p.DeleteAccount(ctx, params, ro)

	if err := p.CreateDatabaseRaw(ctx, params, db); err != nil {
		t.Fatalf("create database: %v", err)
	}
	if err := p.CreateAccount(ctx, params, provider.CreateAccountRequest{Name: rw, Password: "rwfuturepass", Login: true}); err != nil {
		t.Fatalf("create rw account: %v", err)
	}
	if err := p.CreateAccount(ctx, params, provider.CreateAccountRequest{Name: ro, Password: "rofuturepass", Login: true}); err != nil {
		t.Fatalf("create ro account: %v", err)
	}
	if err := p.GrantDatabasePrivileges(ctx, params, rw, db, "read_write"); err != nil {
		t.Fatalf("grant rw: %v", err)
	}
	if err := p.GrantDatabasePrivileges(ctx, params, ro, db, "read_only"); err != nil {
		t.Fatalf("grant ro: %v", err)
	}

	rwConn := connectAs(t, ctx, rw, "rwfuturepass", db)
	if _, err := rwConn.Exec(ctx, "CREATE TABLE rw_future (id int); INSERT INTO rw_future VALUES (1)"); err != nil {
		t.Fatalf("rw create table: %v", err)
	}

	roConn := connectAs(t, ctx, ro, "rofuturepass", db)
	var n int
	if err := roConn.QueryRow(ctx, "SELECT count(*) FROM rw_future").Scan(&n); err != nil {
		t.Errorf("read-only should see tables created by read-write: %v", err)
	}
	if _, err := roConn.Exec(ctx, "INSERT INTO rw_future VALUES (2)"); err == nil {
		t.Errorf("read-only insert should be denied")
	}
	roConn.Close(ctx)

	if err := p.RevokeDatabasePrivileges(ctx, params, ro, db); err != nil {
		t.Fatalf("revoke ro: %v", err)
	}
	if _, err := rwConn.Exec(ctx, "CREATE TABLE rw_future_after_revoke (id int)"); err != nil {
		t.Fatalf("rw create table after revoke: %v", err)
	}
	rwConn.Close(ctx)

	adminConn, err := p.connectDB(ctx, params, db)
	if err != nil {
		t.Fatalf("connect as admin: %v", err)
	}
	var allowed bool
	if err := adminConn.QueryRow(ctx, `SELECT has_table_privilege($1, 'public.rw_future_after_revoke', 'SELECT')`, ro).Scan(&allowed); err != nil {
		t.Fatalf("check privilege: %v", err)
	}
	adminConn.Close(ctx)
	if allowed {
		t.Errorf("revoked read-only account should not get SELECT on future tables")
	}

	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, rw)
	_ = p.DeleteAccount(ctx, params, ro)
}
