package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/dbpilot/dbpilot/internal/backup"
	"github.com/dbpilot/dbpilot/internal/task"
)

type restoreRequest struct {
	BackupID       int64  `json:"backup_id"`
	TargetDatabase string `json:"target_database"`
}

func (s *Server) handleCreateRestore(w http.ResponseWriter, r *http.Request) {
	var req restoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if req.BackupID <= 0 {
		writeError(w, ErrBadRequest("invalid_request", "backup_id is required"))
		return
	}
	if apiErr := validateIdentifier(req.TargetDatabase, "database"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	rec, err := s.backups.Get(r.Context(), req.BackupID)
	if err != nil {
		writeError(w, backupNotFoundErr(err))
		return
	}
	t, err := s.tasks.Enqueue(r.Context(), "restore", "database", req.TargetDatabase, backup.RestoreRequest{
		InstanceID:     rec.InstanceID,
		BackupID:       req.BackupID,
		TargetDatabase: req.TargetDatabase,
	})
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	s.auditRecord(r, "restore", "database", req.TargetDatabase, "success", "backup_id="+strconv.FormatInt(req.BackupID, 10))
	writeJSON(w, http.StatusAccepted, t)
}

func (s *Server) runRestoreTask(ctx context.Context, t *task.Task, payload []byte, log task.Logger) error {
	var req backup.RestoreRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return err
	}
	logf := func(format string, args ...any) { log.Infof(format, args...) }
	return s.engine.Restore(ctx, req, logf)
}
