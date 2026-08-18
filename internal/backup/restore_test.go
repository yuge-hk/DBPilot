package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A corrupted backup object must be rejected before any restore happens.
func TestRestoreChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	engine, instStore, _, _ := newTestEngine(t, &fakeRunner{version: "16.2", dumpContent: []byte("ORIGINAL")}, []string{dir})
	list, _ := engine.targets.List(context.Background())
	insts, _ := instStore.List(context.Background())

	res, err := engine.Backup(context.Background(), Request{
		InstanceID: insts[0].ID, Database: "mydb", StorageIDs: []int64{list[0].ID}, CreatedBy: "admin",
	}, func(string, ...any) {})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// corrupt the stored dump file
	entries, _ := engine.store.ListTargetEntries(context.Background(), res.Record.ID)
	dumpPath := filepath.Join(dir, entries[0].ObjectPath, "mydb.dump")
	if err := os.WriteFile(dumpPath, []byte("TAMPERED"), 0o600); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	err = engine.Restore(context.Background(), RestoreRequest{
		InstanceID: insts[0].ID, BackupID: res.Record.ID, TargetDatabase: "mydb_restored",
	}, func(string, ...any) {})
	if err == nil {
		t.Fatal("expected checksum error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}
