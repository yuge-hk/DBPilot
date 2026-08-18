package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/dbpilot/dbpilot/internal/provider"
	"github.com/dbpilot/dbpilot/internal/runner"
	"github.com/dbpilot/dbpilot/internal/storageregistry"
)

// RestoreRequest describes a restore-as-new-database operation.
type RestoreRequest struct {
	InstanceID     int64  `json:"instance_id"`
	BackupID       int64  `json:"backup_id"`
	TargetDatabase string `json:"target_database"`
}

// Restore downloads the backup, verifies its SHA256 against the manifest,
// checks pg_restore, and restores into a brand-new database. It never
// overwrites an existing database.
func (e *Engine) Restore(ctx context.Context, req RestoreRequest, logf func(string, ...any)) error {
	params, err := e.instances.ConnParams(ctx, req.InstanceID)
	if err != nil {
		return fmt.Errorf("resolve instance: %w", err)
	}

	rec, err := e.store.Get(ctx, req.BackupID)
	if err != nil {
		return err
	}
	if rec.BackupType != TypeDatabase {
		return errors.New("only database backups can be restored")
	}
	if rec.Status != "success" {
		return errors.New("backup is not in a restorable state")
	}

	// find a successful target entry
	entries, err := e.store.ListTargetEntries(ctx, req.BackupID)
	if err != nil {
		return err
	}
	var entry *TargetEntry
	for i := range entries {
		if entries[i].Status == "success" {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return errors.New("no successful storage target for this backup")
	}

	target, err := e.targets.Get(ctx, entry.StorageID)
	if err != nil {
		return err
	}
	password, _ := e.targets.Password(ctx, entry.StorageID)
	st, err := storageregistry.Build(ctx, target, password)
	if err != nil {
		return err
	}

	// check pg_restore
	if _, ok := runner.LookPath("pg_restore"); !ok {
		return errors.New("pg_restore not found; install postgresql-client")
	}

	// download to temp file while computing sha256
	if err := os.MkdirAll(e.tmpDir, 0o700); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(e.tmpDir, "restore-*.dump")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	logf("downloading backup object %s", entry.ObjectPath+"/"+rec.ObjectName())
	reader, err := st.Open(ctx, entry.ObjectPath+"/"+rec.ObjectName())
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("open backup object: %w", err)
	}
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmpFile, h), reader)
	reader.Close()
	tmpFile.Close()
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if rec.SHA256 != "" && sum != rec.SHA256 {
		return errors.New("checksum mismatch: backup file may be corrupted")
	}
	logf("downloaded %d bytes, sha256 verified", size)

	// validate target database name + availability
	if err := e.validateTargetName(ctx, params, req.TargetDatabase); err != nil {
		return err
	}

	// create the target database
	logf("creating database %s", req.TargetDatabase)
	if err := e.createDatabase(ctx, params, req.TargetDatabase); err != nil {
		return err
	}
	cleanupNeeded := true
	defer func() {
		if cleanupNeeded {
			_ = e.dropDatabase(ctx, params, req.TargetDatabase)
		}
	}()

	// pg_restore
	logf("restoring via pg_restore")
	env := map[string]string{
		"PGPASSWORD": params.Password,
		"PGUSER":     params.Username,
		"PGSSLMODE":  params.SSLMode,
	}
	if params.UnixSocket != "" {
		env["PGHOST"] = params.UnixSocket
	} else {
		env["PGHOST"] = params.Host
		env["PGPORT"] = strconv.Itoa(params.Port)
	}
	args := []string{"--format=custom", "--no-owner", "--dbname=" + req.TargetDatabase, tmpPath}
	_, stderr, err := e.runner.Run(ctx, "pg_restore", args, env)
	if err != nil {
		return fmt.Errorf("pg_restore failed: %s", string(stderr))
	}

	cleanupNeeded = false
	logf("restore completed: %s", req.TargetDatabase)
	return nil
}

func (e *Engine) validateTargetName(ctx context.Context, params provider.ConnParams, name string) error {
	if name == "" {
		return errors.New("target database name is required")
	}
	exists, err := e.pg.DatabaseExists(ctx, params, name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("target database %q already exists", name)
	}
	return nil
}

func (e *Engine) createDatabase(ctx context.Context, params provider.ConnParams, name string) error {
	return e.pg.CreateDatabaseRaw(ctx, params, name)
}

func (e *Engine) dropDatabase(ctx context.Context, params provider.ConnParams, name string) error {
	return e.pg.DropDatabaseRaw(ctx, params, name)
}
