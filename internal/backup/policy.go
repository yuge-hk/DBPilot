package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dbpilot/dbpilot/internal/persistence"
)

type Schedule struct {
	Type string `json:"type"` // daily | weekly | monthly
	Time string `json:"time"` // "HH:MM"
	Day  int    `json:"day"`  // weekly: 0(Sun)-6(Sat); monthly: 1-31
}

type Policy struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	InstanceID     int64    `json:"instance_id"`
	DatabaseName   string   `json:"database_name"`
	Enabled        bool     `json:"enabled"`
	Schedule       Schedule `json:"schedule"`
	RetentionCount int      `json:"retention_count"`
	GlobalsEnabled bool     `json:"globals_enabled"`
	StorageTargets []int64  `json:"storage_targets"`
	LastRunAt      string   `json:"last_run_at"`
	NextRunAt      string   `json:"next_run_at"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

var ErrPolicyNotFound = errors.New("backup policy not found")

type PolicyStore struct {
	db *persistence.DB
}

func NewPolicyStore(db *persistence.DB) *PolicyStore {
	return &PolicyStore{db: db}
}

func (s *PolicyStore) Create(ctx context.Context, p *Policy) (*Policy, error) {
	sched, _ := json.Marshal(p.Schedule)
	targets, _ := json.Marshal(p.StorageTargets)
	enabled := 0
	if p.Enabled {
		enabled = 1
	}
	globals := 0
	if p.GlobalsEnabled {
		globals = 1
	}
	next := NextRun(p.Schedule, time.Now())
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO backup_policies(name, instance_id, database_name, enabled, schedule, retention_count, globals_enabled, storage_targets, next_run_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.InstanceID, p.DatabaseName, enabled, string(sched), p.RetentionCount, globals, string(targets), next.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	p.ID = id
	p.NextRunAt = next.Format(time.RFC3339)
	return p, nil
}

func (s *PolicyStore) Update(ctx context.Context, p *Policy) error {
	sched, _ := json.Marshal(p.Schedule)
	targets, _ := json.Marshal(p.StorageTargets)
	enabled := 0
	if p.Enabled {
		enabled = 1
	}
	globals := 0
	if p.GlobalsEnabled {
		globals = 1
	}
	next := NextRun(p.Schedule, time.Now())
	_, err := s.db.ExecContext(ctx, `
		UPDATE backup_policies SET name=?, instance_id=?, database_name=?, enabled=?, schedule=?, retention_count=?, globals_enabled=?, storage_targets=?, next_run_at=?, updated_at=strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE id=?`,
		p.Name, p.InstanceID, p.DatabaseName, enabled, string(sched), p.RetentionCount, globals, string(targets), next.Format(time.RFC3339), p.ID)
	return err
}

func (s *PolicyStore) List(ctx context.Context) ([]Policy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, instance_id, database_name, enabled, schedule, retention_count, globals_enabled, storage_targets, last_run_at, next_run_at, created_at, updated_at
		FROM backup_policies ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPolicies(rows)
}

func scanPolicies(rows *sql.Rows) ([]Policy, error) {
	var out []Policy
	for rows.Next() {
		var p Policy
		var schedRaw, targetsRaw string
		var enabled, globals int
		if err := rows.Scan(&p.ID, &p.Name, &p.InstanceID, &p.DatabaseName, &enabled, &schedRaw, &p.RetentionCount, &globals, &targetsRaw, &p.LastRunAt, &p.NextRunAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(schedRaw), &p.Schedule)
		_ = json.Unmarshal([]byte(targetsRaw), &p.StorageTargets)
		p.Enabled = enabled == 1
		p.GlobalsEnabled = globals == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PolicyStore) Get(ctx context.Context, id int64) (*Policy, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, instance_id, database_name, enabled, schedule, retention_count, globals_enabled, storage_targets, last_run_at, next_run_at, created_at, updated_at
		FROM backup_policies WHERE id = ?`, id)
	var p Policy
	var schedRaw, targetsRaw string
	var enabled, globals int
	if err := row.Scan(&p.ID, &p.Name, &p.InstanceID, &p.DatabaseName, &enabled, &schedRaw, &p.RetentionCount, &globals, &targetsRaw, &p.LastRunAt, &p.NextRunAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPolicyNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(schedRaw), &p.Schedule)
	_ = json.Unmarshal([]byte(targetsRaw), &p.StorageTargets)
	p.Enabled = enabled == 1
	p.GlobalsEnabled = globals == 1
	return &p, nil
}

func (s *PolicyStore) Delete(ctx context.Context, id int64) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM backup_policies WHERE id = ?`, id)
	return err
}

// MarkRun records a successful scheduler trigger and advances next_run_at.
func (s *PolicyStore) MarkRun(ctx context.Context, id int64, next time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE backup_policies SET last_run_at = strftime('%Y-%m-%dT%H:%M:%SZ','now'), next_run_at = ? WHERE id = ?`,
		next.Format(time.RFC3339), id)
	return err
}

// NextRun computes the next occurrence of a schedule strictly after `after`.
func NextRun(sched Schedule, after time.Time) time.Time {
	hour, min := 0, 0
	if _, err := fmt.Sscanf(sched.Time, "%d:%d", &hour, &min); err != nil {
		hour, min = 3, 0
	}
	switch sched.Type {
	case "weekly":
		day := sched.Day % 7
		if day < 0 {
			day += 7
		}
		return nextWeekly(after, time.Weekday(day), hour, min)
	case "monthly":
		day := sched.Day
		if day < 1 {
			day = 1
		}
		if day > 31 {
			day = 31
		}
		return nextMonthly(after, day, hour, min)
	default: // daily
		return nextDaily(after, hour, min)
	}
}

func nextDaily(after time.Time, hour, min int) time.Time {
	cand := time.Date(after.Year(), after.Month(), after.Day(), hour, min, 0, 0, after.Location())
	if !cand.After(after) {
		cand = cand.AddDate(0, 0, 1)
	}
	return cand
}

func nextWeekly(after time.Time, day time.Weekday, hour, min int) time.Time {
	cand := time.Date(after.Year(), after.Month(), after.Day(), hour, min, 0, 0, after.Location())
	diff := (int(day) - int(cand.Weekday()) + 7) % 7
	cand = cand.AddDate(0, 0, diff)
	if !cand.After(after) {
		cand = cand.AddDate(0, 0, 7)
	}
	return cand
}

func nextMonthly(after time.Time, day, hour, min int) time.Time {
	year, month, _ := after.Date()
	cand := time.Date(year, month, 1, hour, min, 0, 0, after.Location())
	last := time.Date(year, month+1, 0, hour, min, 0, 0, after.Location()).Day()
	d := day
	if d > last {
		d = last
	}
	cand = time.Date(year, month, d, hour, min, 0, 0, after.Location())
	if !cand.After(after) {
		// next month
		year, month, _ = time.Date(year, month+1, 1, 0, 0, 0, 0, after.Location()).Date()
		last = time.Date(year, month+1, 0, 0, 0, 0, 0, after.Location()).Day()
		d = day
		if d > last {
			d = last
		}
		cand = time.Date(year, month, d, hour, min, 0, 0, after.Location())
	}
	return cand
}
