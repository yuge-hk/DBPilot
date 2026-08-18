package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dbpilot/dbpilot/internal/instance"
	"github.com/dbpilot/dbpilot/internal/persistence"
	"github.com/dbpilot/dbpilot/internal/secret"
	"github.com/dbpilot/dbpilot/internal/storage"
)

type fakeRunner struct {
	pgDumpErr   error
	version     string
	dumpContent []byte
	lastEnv     map[string]string
}

func (f *fakeRunner) Run(ctx context.Context, name string, args []string, env map[string]string) ([]byte, []byte, error) {
	f.lastEnv = env
	switch name {
	case "pg_dump", "pg_dumpall":
		if args[0] == "--version" {
			return []byte(f.version), nil, nil
		}
		if f.pgDumpErr != nil {
			return nil, []byte(f.pgDumpErr.Error()), f.pgDumpErr
		}
		for _, a := range args {
			if strings.HasPrefix(a, "--file=") {
				_ = os.WriteFile(a[len("--file="):], f.dumpContent, 0o600)
			}
		}
		return nil, nil, nil
	}
	return nil, nil, errors.New("unknown command " + name)
}

func newTestEngine(t *testing.T, r *fakeRunner, storagePaths []string) (*Engine, *instance.Store, *storage.TargetStore, *Store) {
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
	secretStore := secret.NewSQLiteStore(db, key)
	instStore := instance.NewStore(db, secretStore)
	targetStore := storage.NewTargetStore(db, secretStore)
	backupStore := NewStore(db)

	inst := &instance.Instance{Name: "test", Host: "127.0.0.1", Port: 5432, Username: "postgres", SSLMode: "disable"}
	if err := instStore.Create(context.Background(), inst, "secretpw"); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	var ids []int64
	for _, p := range storagePaths {
		target, err := targetStore.Create(context.Background(), "target", "local", storage.LocalConfig{Path: p}, "")
		if err != nil {
			t.Fatalf("create target: %v", err)
		}
		ids = append(ids, target.ID)
	}

	engine := NewEngine(r, t.TempDir(), instStore, targetStore, backupStore, nil)
	return engine, instStore, targetStore, backupStore
}

func shaOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestBackupSuccess(t *testing.T) {
	content := []byte("FAKE_DUMP_DATA")
	dir := t.TempDir()
	engine, instStore, _, backupStore := newTestEngine(t, &fakeRunner{version: "pg_dump (PostgreSQL) 16.2", dumpContent: content}, []string{dir})

	insts, _ := instStore.List(context.Background())
	ts := engine.targets
	list, _ := ts.List(context.Background())
	storageID := list[0].ID

	res, err := engine.Backup(context.Background(), Request{
		InstanceID: insts[0].ID, Database: "mydb", StorageIDs: []int64{storageID}, CreatedBy: "admin",
	}, func(format string, args ...any) {})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if res.Record.Status != "success" {
		t.Fatalf("status = %s", res.Record.Status)
	}
	if res.Record.SHA256 != shaOf(content) {
		t.Errorf("sha256 mismatch")
	}
	if res.Record.Database != "mydb" {
		t.Errorf("database = %s", res.Record.Database)
	}

	// manifest should exist on disk in the storage dir
	entries, _ := backupStore.ListTargetEntries(context.Background(), res.Record.ID)
	if len(entries) != 1 {
		t.Fatalf("expected 1 target entry, got %d", len(entries))
	}
	manifestPath := filepath.Join(dir, entries[0].ObjectPath, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Errorf("manifest missing: %v", err)
	}
}

func TestBackupPgDumpFailure(t *testing.T) {
	dir := t.TempDir()
	engine, instStore, _, _ := newTestEngine(t, &fakeRunner{pgDumpErr: errors.New("connection refused")}, []string{dir})

	ts := engine.targets
	list, _ := ts.List(context.Background())
	insts, _ := instStore.List(context.Background())

	res, err := engine.Backup(context.Background(), Request{
		InstanceID: insts[0].ID, Database: "mydb", StorageIDs: []int64{list[0].ID}, CreatedBy: "admin",
	}, func(format string, args ...any) {})
	if err == nil {
		t.Fatal("expected error")
	}
	if res.Record.Status != "failed" {
		t.Errorf("status = %s, want failed", res.Record.Status)
	}
	if res.Record.ErrorCode != "pg_dump_failed" {
		t.Errorf("error code = %s", res.Record.ErrorCode)
	}
}

func TestBackupPartialSuccess(t *testing.T) {
	content := []byte("DATA")
	goodDir := t.TempDir()
	// a path under a regular file makes MkdirAll fail -> storage target fails
	blocker := filepath.Join(t.TempDir(), "blocker")
	_ = os.WriteFile(blocker, []byte("x"), 0o600)
	badDir := filepath.Join(blocker, "sub")

	engine, instStore, _, _ := newTestEngine(t, &fakeRunner{version: "16.2", dumpContent: content}, []string{goodDir, badDir})

	ts := engine.targets
	list, _ := ts.List(context.Background())
	insts, _ := instStore.List(context.Background())

	res, err := engine.Backup(context.Background(), Request{
		InstanceID: insts[0].ID, Database: "mydb", StorageIDs: []int64{list[0].ID, list[1].ID}, CreatedBy: "admin",
	}, func(format string, args ...any) {})
	if err != nil {
		t.Fatalf("backup should not hard-fail: %v", err)
	}
	if res.Record.Status != "partial_success" {
		t.Errorf("status = %s, want partial_success", res.Record.Status)
	}
}
