package postgres

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/dbpilot/dbpilot/internal/provider"
)

func integrationParams(t *testing.T) provider.ConnParams {
	t.Helper()
	if os.Getenv("DBPILOT_TEST_PG") == "" {
		t.Skip("set DBPILOT_TEST_PG=1 to run against a local PostgreSQL")
	}
	port := 5432
	if p := os.Getenv("DBPILOT_TEST_PG_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}
	return provider.ConnParams{
		Host:     "127.0.0.1",
		Port:     port,
		Username: "postgres",
		Password: "postgres",
		SSLMode:  "disable",
	}
}

func TestAccountLifecycle(t *testing.T) {
	params := integrationParams(t)
	ctx := context.Background()
	p := New(nil)

	const name = "dbpilot_it_user"

	// cleanup from previous runs
	_ = p.DeleteAccount(ctx, params, name)

	if err := p.CreateAccount(ctx, params, provider.CreateAccountRequest{
		Name: name, Password: "testpassword123", Login: true,
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}

	accounts, err := p.ListAccounts(ctx, params)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	found := false
	for _, a := range accounts {
		if a.Name == name {
			found = true
			if !a.CanLogin {
				t.Error("account should be able to login")
			}
			if a.SystemRole {
				t.Error("new account should not be system role")
			}
		}
	}
	if !found {
		t.Fatal("created account not found in list")
	}

	// duplicate creation must fail
	err = p.CreateAccount(ctx, params, provider.CreateAccountRequest{Name: name, Password: "x", Login: true})
	if err == nil {
		t.Error("duplicate account creation should fail")
	}

	// update password + disable login
	newPw := "newpassword456"
	loginFalse := false
	if err := p.UpdateAccount(ctx, params, provider.UpdateAccountRequest{
		Name: name, NewPassword: &newPw, Login: &loginFalse,
	}); err != nil {
		t.Fatalf("update account: %v", err)
	}

	// deleting a protected account must be refused
	err = p.DeleteAccount(ctx, params, "postgres")
	if !errors.Is(err, provider.ErrAccountProtected) {
		t.Errorf("deleting postgres should be protected, got %v", err)
	}

	// delete clean account
	if err := p.DeleteAccount(ctx, params, name); err != nil {
		t.Fatalf("delete account: %v", err)
	}
}

func TestDeleteDatabaseProtection(t *testing.T) {
	params := integrationParams(t)
	ctx := context.Background()
	p := New(nil)

	err := p.DeleteDatabase(ctx, params, "postgres")
	if !errors.Is(err, provider.ErrDatabaseProtected) {
		t.Errorf("deleting postgres database should be protected, got %v", err)
	}
}
