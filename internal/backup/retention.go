package backup

import (
	"context"
	"errors"

	"github.com/dbpilot/dbpilot/internal/storageregistry"
)

// DeleteBackup removes a backup's objects from every storage target and then
// deletes the database record. If any object deletion fails, the record is
// kept so the operation can be retried (no orphan files).
func (e *Engine) DeleteBackup(ctx context.Context, backupID int64) error {
	rec, err := e.store.Get(ctx, backupID)
	if err != nil {
		return err
	}
	entries, err := e.store.ListTargetEntries(ctx, backupID)
	if err != nil {
		return err
	}

	var failures []string
	for _, ent := range entries {
		target, err := e.targets.Get(ctx, ent.StorageID)
		if err != nil {
			failures = append(failures, "target not found: "+err.Error())
			continue
		}
		password, _ := e.targets.Password(ctx, ent.StorageID)
		st, err := storageregistry.Build(ctx, target, password)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if err := st.Delete(ctx, ent.ObjectPath+"/"+rec.ObjectName()); err != nil {
			failures = append(failures, err.Error())
		}
		if err := st.Delete(ctx, ent.ObjectPath+"/manifest.json"); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return errors.New("failed to delete some storage objects: " + failures[0])
	}
	return e.store.Delete(ctx, backupID)
}

// ApplyRetention keeps only the most recent `keepN` successful backups for a
// database, deleting the oldest ones (storage objects first, then records).
func (e *Engine) ApplyRetention(ctx context.Context, instanceID int64, database string, keepN int) (int, error) {
	if keepN <= 0 {
		return 0, nil
	}
	recs, err := e.store.ListByInstance(ctx, instanceID, database, 0)
	if err != nil {
		return 0, err
	}
	var successful []Record
	for _, r := range recs {
		if r.BackupType == TypeDatabase && r.Status == "success" {
			successful = append(successful, r)
		}
	}
	if len(successful) <= keepN {
		return 0, nil
	}
	deleted := 0
	for _, r := range successful[keepN:] { // oldest are at the end (list is DESC)
		if err := e.DeleteBackup(ctx, r.ID); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}
