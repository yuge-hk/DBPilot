package backup

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/dbpilot/dbpilot/internal/persistence"
)

const (
	TypeDatabase = "database"
	TypeGlobals  = "globals"
)

type Record struct {
	ID              int64  `json:"id"`
	InstanceID      int64  `json:"instance_id"`
	Database        string `json:"database"`
	BackupType      string `json:"backup_type"`
	Format          string `json:"format"`
	Status          string `json:"status"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at"`
	DurationMs      int64  `json:"duration_ms"`
	OriginalSize    int64  `json:"original_size"`
	FinalSize       int64  `json:"final_size"`
	SHA256          string `json:"sha256"`
	PgDumpVersion   string `json:"pg_dump_version"`
	PostgresVersion string `json:"postgres_version"`
	ErrorCode       string `json:"error_code"`
	ErrorMessage    string `json:"error_message"`
	CreatedBy       string `json:"created_by"`
	ManifestPath    string `json:"manifest_path"`
	CreatedAt       string `json:"created_at"`
}

type TargetEntry struct {
	ID           int64  `json:"id"`
	BackupID     int64  `json:"backup_id"`
	StorageID    int64  `json:"storage_id"`
	Status       string `json:"status"`
	ObjectPath   string `json:"object_path"`
	Size         int64  `json:"size"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

var ErrNotFound = errors.New("backup not found")

// ObjectName returns the dump object filename for a backup record.
func (r *Record) ObjectName() string {
	if r.BackupType == TypeGlobals {
		return "globals.sql"
	}
	return r.Database + ".dump"
}

type Store struct {
	db *persistence.DB
}

func NewStore(db *persistence.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, r *Record) (*Record, error) {
	r.StartedAt = time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO backups(instance_id, database, backup_type, format, status, started_at, pg_dump_version, postgres_version, created_by, manifest_path)
		VALUES(?, ?, ?, ?, 'running', ?, ?, ?, ?, ?)`,
		r.InstanceID, r.Database, r.BackupType, r.Format, r.StartedAt, r.PgDumpVersion, r.PostgresVersion, r.CreatedBy, r.ManifestPath)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	r.ID = id
	return r, nil
}

func (s *Store) Update(ctx context.Context, r *Record) error {
	r.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		UPDATE backups SET status=?, finished_at=?, duration_ms=?, original_size=?, final_size=?, sha256=?, pg_dump_version=?, postgres_version=?, error_code=?, error_message=?, manifest_path=?
		WHERE id = ?`,
		r.Status, r.FinishedAt, r.DurationMs, r.OriginalSize, r.FinalSize, r.SHA256, r.PgDumpVersion, r.PostgresVersion, r.ErrorCode, r.ErrorMessage, r.ManifestPath, r.ID)
	return err
}

func (s *Store) Get(ctx context.Context, id int64) (*Record, error) {
	var r Record
	err := s.db.QueryRowContext(ctx, `
		SELECT id, instance_id, database, backup_type, format, status, started_at, finished_at, duration_ms, original_size, final_size, sha256, pg_dump_version, postgres_version, error_code, error_message, created_by, manifest_path, created_at
		FROM backups WHERE id = ?`, id).
		Scan(&r.ID, &r.InstanceID, &r.Database, &r.BackupType, &r.Format, &r.Status, &r.StartedAt, &r.FinishedAt, &r.DurationMs, &r.OriginalSize, &r.FinalSize, &r.SHA256, &r.PgDumpVersion, &r.PostgresVersion, &r.ErrorCode, &r.ErrorMessage, &r.CreatedBy, &r.ManifestPath, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) List(ctx context.Context, limit, offset int) ([]Record, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, instance_id, database, backup_type, format, status, started_at, finished_at, duration_ms, original_size, final_size, sha256, pg_dump_version, postgres_version, error_code, error_message, created_by, manifest_path, created_at
		FROM backups ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.InstanceID, &r.Database, &r.BackupType, &r.Format, &r.Status, &r.StartedAt, &r.FinishedAt, &r.DurationMs, &r.OriginalSize, &r.FinalSize, &r.SHA256, &r.PgDumpVersion, &r.PostgresVersion, &r.ErrorCode, &r.ErrorMessage, &r.CreatedBy, &r.ManifestPath, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListByInstance(ctx context.Context, instanceID int64, database string, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT id, instance_id, database, backup_type, format, status, started_at, finished_at, duration_ms, original_size, final_size, sha256, pg_dump_version, postgres_version, error_code, error_message, created_by, manifest_path, created_at
		FROM backups WHERE instance_id = ?`
	args := []any{instanceID}
	if database != "" {
		q += ` AND database = ?`
		args = append(args, database)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.InstanceID, &r.Database, &r.BackupType, &r.Format, &r.Status, &r.StartedAt, &r.FinishedAt, &r.DurationMs, &r.OriginalSize, &r.FinalSize, &r.SHA256, &r.PgDumpVersion, &r.PostgresVersion, &r.ErrorCode, &r.ErrorMessage, &r.CreatedBy, &r.ManifestPath, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) AddTargetEntry(ctx context.Context, e *TargetEntry) error {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO backup_target_entries(backup_id, storage_id, status, object_path, size) VALUES(?, ?, ?, ?, ?)`,
		e.BackupID, e.StorageID, e.Status, e.ObjectPath, e.Size)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	e.ID = id
	return nil
}

func (s *Store) UpdateTargetEntry(ctx context.Context, e *TargetEntry) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE backup_target_entries SET status=?, object_path=?, size=?, error_code=?, error_message=? WHERE id = ?`,
		e.Status, e.ObjectPath, e.Size, e.ErrorCode, e.ErrorMessage, e.ID)
	return err
}

func (s *Store) ListTargetEntries(ctx context.Context, backupID int64) ([]TargetEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, backup_id, storage_id, status, object_path, size, error_code, error_message
		FROM backup_target_entries WHERE backup_id = ? ORDER BY id`, backupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TargetEntry
	for rows.Next() {
		var e TargetEntry
		if err := rows.Scan(&e.ID, &e.BackupID, &e.StorageID, &e.Status, &e.ObjectPath, &e.Size, &e.ErrorCode, &e.ErrorMessage); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM backup_target_entries WHERE backup_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, id)
	return err
}

// LastSuccessful returns the most recent successful backup for a database.
func (s *Store) LastSuccessful(ctx context.Context, instanceID int64, database string) (*Record, error) {
	var r Record
	err := s.db.QueryRowContext(ctx, `
		SELECT id, instance_id, database, backup_type, format, status, started_at, finished_at, duration_ms, original_size, final_size, sha256, pg_dump_version, postgres_version, error_code, error_message, created_by, manifest_path, created_at
		FROM backups WHERE instance_id = ? AND database = ? AND backup_type = 'database' AND status = 'success'
		ORDER BY id DESC LIMIT 1`, instanceID, database).
		Scan(&r.ID, &r.InstanceID, &r.Database, &r.BackupType, &r.Format, &r.Status, &r.StartedAt, &r.FinishedAt, &r.DurationMs, &r.OriginalSize, &r.FinalSize, &r.SHA256, &r.PgDumpVersion, &r.PostgresVersion, &r.ErrorCode, &r.ErrorMessage, &r.CreatedBy, &r.ManifestPath, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}
