package task

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/dbpilot/dbpilot/internal/persistence"
)

type Status string

const (
	StatusQueued         Status = "queued"
	StatusRunning        Status = "running"
	StatusSuccess        Status = "success"
	StatusPartialSuccess Status = "partial_success"
	StatusFailed         Status = "failed"
	StatusCancelled      Status = "cancelled"
)

type Task struct {
	ID           int64  `json:"id"`
	Type         string `json:"type"`
	Status       Status `json:"status"`
	Progress     int    `json:"progress"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Log          string `json:"log"`
	CreatedAt    string `json:"created_at"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
}

var ErrNotFound = errors.New("task not found")

const maxLogBytes = 200 * 1024

type Store struct {
	db *persistence.DB
}

func NewStore(db *persistence.DB) *Store {
	return &Store{db: db}
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (s *Store) Create(ctx context.Context, typ, resourceType, resourceID, payload string) (*Task, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks(type, status, resource_type, resource_id, payload) VALUES(?, 'queued', ?, ?, ?)`,
		typ, resourceType, resourceID, payload)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Task{ID: id, Type: typ, Status: StatusQueued, ResourceType: resourceType, ResourceID: resourceID, CreatedAt: now()}, nil
}

func (s *Store) Get(ctx context.Context, id int64) (*Task, error) {
	var t Task
	err := s.db.QueryRowContext(ctx, `
		SELECT id, type, status, progress, resource_type, resource_id, error_code, error_message, log, created_at, started_at, finished_at
		FROM tasks WHERE id = ?`, id).
		Scan(&t.ID, &t.Type, &t.Status, &t.Progress, &t.ResourceType, &t.ResourceID, &t.ErrorCode, &t.ErrorMessage, &t.Log, &t.CreatedAt, &t.StartedAt, &t.FinishedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (s *Store) Payload(ctx context.Context, id int64) (string, error) {
	var p string
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM tasks WHERE id = ?`, id).Scan(&p)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return p, nil
}

func (s *Store) MarkRunning(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = 'running', started_at = ? WHERE id = ?`, now(), id)
	return err
}

func (s *Store) SetProgress(ctx context.Context, id, progress int) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET progress = ? WHERE id = ?`, progress, id)
	return err
}

func (s *Store) Finish(ctx context.Context, id int64, status Status, errorCode, errorMessage, log string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = ?, error_code = ?, error_message = ?, log = ?, finished_at = ?, progress = CASE WHEN ? = 'success' THEN 100 ELSE progress END WHERE id = ?`,
		string(status), errorCode, errorMessage, log, now(), string(status), id)
	return err
}

func (s *Store) AppendLog(ctx context.Context, id int64, add string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET log = substr(log || ?, -?) WHERE id = ?`, add, maxLogBytes, id)
	return err
}

func (s *Store) List(ctx context.Context, limit, offset int) ([]Task, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, status, progress, resource_type, resource_id, error_code, error_message, log, created_at, started_at, finished_at
		FROM tasks ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Type, &t.Status, &t.Progress, &t.ResourceType, &t.ResourceID, &t.ErrorCode, &t.ErrorMessage, &t.Log, &t.CreatedAt, &t.StartedAt, &t.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RecoverOnStartup re-queues tasks that were interrupted by a crash and
// marks orphaned running tasks as failed.
func (s *Store) RecoverOnStartup(ctx context.Context) ([]int64, error) {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = 'failed', error_code = 'interrupted', error_message = 'interrupted by shutdown', finished_at = ? WHERE status = 'running'`, now()); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM tasks WHERE status = 'queued' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
