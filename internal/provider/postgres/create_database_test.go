package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/dbpilot/dbpilot/internal/provider"
)

func execOn(t *testing.T, ctx context.Context, params provider.ConnParams, db, sql string) {
	t.Helper()
	conn, err := pgx.Connect(ctx, buildDSN(params, db))
	if err != nil {
		t.Fatalf("connect %s: %v", db, err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, sql); err != nil {
		t.Fatalf("exec %s: %v", sql, err)
	}
}

func TestOneClickCreateOwner(t *testing.T) {
	params := integrationParams(t)
	ctx := context.Background()
	p := New(nil)

	const db = "dbpilot_it_owner"
	const acc = "dbpilot_it_owner_user"

	// cleanup previous runs (ignore errors)
	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, acc)

	res, err := p.CreateDatabaseWithAccount(ctx, params, provider.CreateDatabaseRequest{
		Database: db, AccountMode: "create", Account: acc,
		Password: "ownerpass123", Permission: "owner",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Database != db || res.Account != acc {
		t.Fatalf("unexpected result: %+v", res)
	}

	// owner can create a table and write
	execOn(t, ctx, provider.ConnParams{Host: "127.0.0.1", Port: integrationParams(t).Port, Username: acc, Password: "ownerpass123", SSLMode: "disable"},
		db, "CREATE TABLE t1 (id int); INSERT INTO t1 VALUES (1)")

	// cleanup
	if err := p.DeleteDatabase(ctx, params, db); err != nil {
		t.Fatalf("cleanup delete db: %v", err)
	}
	if err := p.DeleteAccount(ctx, params, acc); err != nil {
		t.Fatalf("cleanup delete account: %v", err)
	}
}

func TestOneClickReadOnlyAndReadWrite(t *testing.T) {
	params := integrationParams(t)
	ctx := context.Background()
	p := New(nil)

	const roDB = "dbpilot_it_perm_ro"
	const rwDB = "dbpilot_it_perm_rw"
	const ro = "dbpilot_it_ro"
	const rw = "dbpilot_it_rw"

	_ = p.DeleteDatabase(ctx, params, roDB)
	_ = p.DeleteDatabase(ctx, params, rwDB)
	_ = p.DeleteAccount(ctx, params, ro)
	_ = p.DeleteAccount(ctx, params, rw)

	// create a read-only database + account
	if _, err := p.CreateDatabaseWithAccount(ctx, params, provider.CreateDatabaseRequest{
		Database: roDB, AccountMode: "create", Account: ro, Password: "ropass123", Permission: "read_only",
	}); err != nil {
		t.Fatalf("create ro: %v", err)
	}
	// create a read-write database + account
	if _, err := p.CreateDatabaseWithAccount(ctx, params, provider.CreateDatabaseRequest{
		Database: rwDB, AccountMode: "create", Account: rw, Password: "rwpass123", Permission: "read_write",
	}); err != nil {
		t.Fatalf("create rw: %v", err)
	}

	// create a table as admin in each
	execOn(t, ctx, params, roDB, "CREATE TABLE items (id serial primary key, name text); INSERT INTO items(name) VALUES ('a')")
	execOn(t, ctx, params, rwDB, "CREATE TABLE items (id serial primary key, name text); INSERT INTO items(name) VALUES ('a')")

	// read-only: SELECT works, INSERT fails
	roConn := connectAs(t, ctx, ro, "ropass123", roDB)
	var n int
	if err := roConn.QueryRow(ctx, "SELECT count(*) FROM items").Scan(&n); err != nil {
		t.Errorf("ro select failed: %v", err)
	}
	if _, err := roConn.Exec(ctx, "INSERT INTO items(name) VALUES ('x')"); err == nil {
		t.Errorf("ro insert should fail")
	}
	roConn.Close(ctx)

	// read-write: SELECT/INSERT/UPDATE/DELETE works
	rwConn := connectAs(t, ctx, rw, "rwpass123", rwDB)
	var id int
	if err := rwConn.QueryRow(ctx, "INSERT INTO items(name) VALUES ('b') RETURNING id").Scan(&id); err != nil {
		t.Errorf("rw insert failed: %v", err)
	}
	if _, err := rwConn.Exec(ctx, "UPDATE items SET name='c' WHERE id=$1", id); err != nil {
		t.Errorf("rw update failed: %v", err)
	}
	if _, err := rwConn.Exec(ctx, "DELETE FROM items WHERE id=$1", id); err != nil {
		t.Errorf("rw delete failed: %v", err)
	}
	// read-write may create its own table (CREATE ON SCHEMA public)
	if _, err := rwConn.Exec(ctx, "CREATE TABLE rw_own (id serial primary key, v text)"); err != nil {
		t.Errorf("rw create table failed: %v", err)
	}
	if _, err := rwConn.Exec(ctx, "INSERT INTO rw_own(v) VALUES ('self')"); err != nil {
		t.Errorf("rw insert into own table failed: %v", err)
	}
	rwConn.Close(ctx)

	// cleanup
	_ = p.DeleteDatabase(ctx, params, roDB)
	_ = p.DeleteDatabase(ctx, params, rwDB)
	_ = p.DeleteAccount(ctx, params, ro)
	_ = p.DeleteAccount(ctx, params, rw)
}

func TestOneClickFailureCleanup(t *testing.T) {
	params := integrationParams(t)
	ctx := context.Background()
	p := New(nil)

	const db = "dbpilot_it_cleanup"
	const acc = "dbpilot_it_cleanup_user"

	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, acc)

	// create the account first (so db creation fails on account conflict,
	// but we want to test db-create failure triggers cleanup). Instead,
	// force a database-create failure via a duplicate.
	if _, err := p.CreateDatabaseWithAccount(ctx, params, provider.CreateDatabaseRequest{
		Database: db, AccountMode: "create", Account: acc, Password: "pass123", Permission: "owner",
	}); err != nil {
		t.Fatalf("first create should succeed: %v", err)
	}

	// second create with a NEW account must fail (db exists) and must not
	// leave the new account behind.
	_, err := p.CreateDatabaseWithAccount(ctx, params, provider.CreateDatabaseRequest{
		Database: db, AccountMode: "create", Account: "dbpilot_it_orphan", Password: "pass123", Permission: "owner",
	})
	if err == nil {
		t.Fatal("duplicate database should fail")
	}

	// orphan account must NOT exist
	accounts, _ := p.ListAccounts(ctx, params)
	for _, a := range accounts {
		if a.Name == "dbpilot_it_orphan" {
			t.Fatal("orphan account left behind after failed create")
		}
	}

	_ = p.DeleteDatabase(ctx, params, db)
	_ = p.DeleteAccount(ctx, params, acc)
}

func connectAs(t *testing.T, ctx context.Context, user, pass, db string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(ctx, buildDSN(provider.ConnParams{
		Host: "127.0.0.1", Port: integrationParams(t).Port, Username: user, Password: pass, SSLMode: "disable",
	}, db))
	if err != nil {
		t.Fatalf("connect as %s: %v", user, err)
	}
	return conn
}
