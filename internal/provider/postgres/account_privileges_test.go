package postgres

import (
	"context"
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

func findLevel(privs []provider.DatabasePrivilege, db string) string {
	for _, p := range privs {
		if p.Database == db {
			return p.Level
		}
	}
	return ""
}
