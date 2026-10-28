package instance

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/dbpilot/dbpilot/internal/persistence"
	"github.com/dbpilot/dbpilot/internal/secret"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := persistence.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	key := secret.MasterKey(make([]byte, 32))
	return NewStore(db, secret.NewSQLiteStore(db, key))
}

func TestInstanceCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	inst := &Instance{Name: "prod", Host: "127.0.0.1", Port: 5432, Username: "postgres", SSLMode: "disable", ConnectionHost: "192.168.1.10"}
	if err := s.Create(ctx, inst, "supersecret"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if inst.ID == 0 {
		t.Fatal("expected id assigned")
	}

	// password is retrievable for connection use
	pw, err := s.Password(ctx, inst.ID)
	if err != nil {
		t.Fatalf("password: %v", err)
	}
	if pw != "supersecret" {
		t.Fatalf("password mismatch: %q", pw)
	}

	// listing must not expose password
	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(list))
	}
	if list[0].Username != "postgres" {
		t.Errorf("username = %q", list[0].Username)
	}
	if list[0].ConnectionHost != "192.168.1.10" {
		t.Errorf("connection_host = %q", list[0].ConnectionHost)
	}

	// update without password keeps existing
	if err := s.Update(ctx, &Instance{ID: inst.ID, Name: "prod2", Host: "127.0.0.1", Port: 5433, Username: "postgres", SSLMode: "require", ConnectionHost: "db.example.com"}, nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := s.Get(ctx, inst.ID)
	if got.Name != "prod2" || got.Port != 5433 || got.ConnectionHost != "db.example.com" {
		t.Errorf("update not applied: %+v", got)
	}
	pw2, _ := s.Password(ctx, inst.ID)
	if pw2 != "supersecret" {
		t.Errorf("password should be preserved, got %q", pw2)
	}

	// update with new password
	newPw := "newsecret"
	if err := s.Update(ctx, &Instance{ID: inst.ID, Name: "prod2", Host: "127.0.0.1", Port: 5433, Username: "postgres", SSLMode: "require"}, &newPw); err != nil {
		t.Fatalf("update password: %v", err)
	}
	pw3, _ := s.Password(ctx, inst.ID)
	if pw3 != "newsecret" {
		t.Errorf("password not updated: %q", pw3)
	}

	// delete removes instance and secret
	if err := s.Delete(ctx, inst.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(ctx, inst.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if _, err := s.Password(ctx, inst.ID); !errors.Is(err, secret.ErrNotFound) {
		t.Errorf("expected secret deleted, got %v", err)
	}
}
