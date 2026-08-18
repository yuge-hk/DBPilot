package scheduler

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/dbpilot/dbpilot/internal/backup"
	"github.com/dbpilot/dbpilot/internal/task"
)

// Scheduler runs backup policies on an internal ticker. No cron is required.
type Scheduler struct {
	policies *backup.PolicyStore
	tasks    *task.Manager
	interval time.Duration
}

func New(policies *backup.PolicyStore, tasks *task.Manager) *Scheduler {
	return &Scheduler{policies: policies, tasks: tasks, interval: 30 * time.Second}
}

func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		s.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()
}

func (s *Scheduler) tick(ctx context.Context) {
	policies, err := s.policies.List(ctx)
	if err != nil {
		slog.Error("scheduler list policies", "error", err)
		return
	}
	now := time.Now()
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		if p.NextRunAt == "" {
			// first run: compute from schedule, but don't fire immediately if
			// the computed time is in the past — just schedule for the future.
			next := backup.NextRun(p.Schedule, now)
			_ = s.policies.MarkRun(ctx, p.ID, next) // only advances next_run_at
			continue
		}
		next, err := time.Parse(time.RFC3339, p.NextRunAt)
		if err != nil {
			continue
		}
		if now.Before(next) {
			continue
		}
		// fire
		payload := backup.TaskPayload{
			InstanceID:     p.InstanceID,
			Database:       p.DatabaseName,
			Globals:        p.GlobalsEnabled,
			StorageIDs:     p.StorageTargets,
			CreatedBy:      "scheduler",
			RetentionCount: p.RetentionCount,
		}
		if _, err := s.tasks.Enqueue(ctx, "backup", "database", "policy-"+strconv.FormatInt(p.ID, 10), payload); err != nil {
			slog.Error("scheduler enqueue", "policy_id", p.ID, "error", err)
			continue
		}
		_ = s.policies.MarkRun(ctx, p.ID, backup.NextRun(p.Schedule, now))
		slog.Info("scheduled backup triggered", "policy_id", p.ID, "database", p.DatabaseName)
	}
}
