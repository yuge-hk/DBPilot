package backup

import (
	"context"
	"testing"
)

// Flow I (retention): create 5 backups, keep last 2, verify 3 oldest deleted.
func TestRetentionKeepLastN(t *testing.T) {
	engine, instStore, _, backupStore := newTestEngine(t, &fakeRunner{version: "16.2", dumpContent: []byte("x")}, []string{t.TempDir()})
	list, _ := engine.targets.List(context.Background())
	insts, _ := instStore.List(context.Background())

	for i := 0; i < 5; i++ {
		_, err := engine.Backup(context.Background(), Request{
			InstanceID: insts[0].ID, Database: "mydb", StorageIDs: []int64{list[0].ID}, CreatedBy: "admin",
		}, func(string, ...any) {})
		if err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
	}

	deleted, err := engine.ApplyRetention(context.Background(), insts[0].ID, "mydb", 2)
	if err != nil {
		t.Fatalf("retention: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deleted, got %d", deleted)
	}

	remaining, _ := backupStore.ListByInstance(context.Background(), insts[0].ID, "mydb", 100)
	successCount := 0
	for _, r := range remaining {
		if r.Status == "success" {
			successCount++
		}
	}
	if successCount != 2 {
		t.Fatalf("expected 2 remaining successful backups, got %d", successCount)
	}
}
