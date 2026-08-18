package audit

import (
	"context"

	"github.com/dbpilot/dbpilot/internal/persistence"
	"github.com/dbpilot/dbpilot/internal/redact"
)

type Entry struct {
	ID           int64  `json:"id"`
	Actor        string `json:"actor"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Result       string `json:"result"`
	RemoteIP     string `json:"remote_ip"`
	Detail       string `json:"detail"`
	CreatedAt    string `json:"created_at"`
}

const (
	ResultSuccess = "success"
	ResultFailure = "failure"
)

type Service struct {
	db *persistence.DB
}

func NewService(db *persistence.DB) *Service {
	return &Service{db: db}
}

// Record writes an audit entry. Detail is passed through redaction as a
// defense-in-depth measure; callers must never include secrets in detail.
func (s *Service) Record(ctx context.Context, actor, action, resourceType, resourceID, result, remoteIP, detail string) {
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO audit_logs(actor, action, resource_type, resource_id, result, remote_ip, detail)
		VALUES(?, ?, ?, ?, ?, ?, ?)`,
		actor, action, resourceType, resourceID, result, remoteIP, redact.String(detail))
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]Entry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, actor, action, resource_type, resource_id, result, remote_ip, detail, created_at
		FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.ResourceType, &e.ResourceID, &e.Result, &e.RemoteIP, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
