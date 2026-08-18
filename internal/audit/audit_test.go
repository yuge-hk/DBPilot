package audit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dbpilot/dbpilot/internal/persistence"
)

func TestAuditRedactsSecrets(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewService(db)

	// a detail string that accidentally contains a connection string + password
	secretDetail := "connected to postgresql://admin:supersecret@10.0.0.1:5432/appdb with password=othersecret"
	svc.Record(context.Background(), "admin", "test", "database", "db", "success", "127.0.0.1", secretDetail)

	entries, err := svc.List(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	detail := entries[0].Detail
	if strings.Contains(detail, "supersecret") {
		t.Errorf("secret leaked into audit detail: %q", detail)
	}
	if strings.Contains(detail, "othersecret") {
		t.Errorf("password leaked into audit detail: %q", detail)
	}
}
