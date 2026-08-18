package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/dbpilot/dbpilot/internal/backup"
	"github.com/dbpilot/dbpilot/internal/task"
)

type backupCreateRequest struct {
	InstanceID int64   `json:"instance_id"`
	Database   string  `json:"database"`
	StorageIDs []int64 `json:"storage_ids"`
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	instanceID, _ := strconv.ParseInt(q.Get("instance_id"), 10, 64)
	database := q.Get("database")

	var recs []backup.Record
	var err error
	if instanceID > 0 {
		recs, err = s.backups.ListByInstance(r.Context(), instanceID, database, 100)
	} else {
		recs, err = s.backups.List(r.Context(), 100, 0)
	}
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if recs == nil {
		recs = []backup.Record{}
	}
	writeOK(w, recs)
}

func (s *Server) handleGetBackup(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	rec, err := s.backups.Get(r.Context(), id)
	if err != nil {
		writeError(w, backupNotFoundErr(err))
		return
	}
	entries, err := s.backups.ListTargetEntries(r.Context(), id)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if entries == nil {
		entries = []backup.TargetEntry{}
	}
	writeOK(w, map[string]any{"backup": rec, "targets": entries})
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req backupCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if req.InstanceID <= 0 {
		writeError(w, ErrBadRequest("invalid_request", "instance_id is required"))
		return
	}
	if req.Database == "" {
		writeError(w, ErrBadRequest("invalid_request", "database is required"))
		return
	}
	if len(req.StorageIDs) == 0 {
		writeError(w, ErrBadRequest("invalid_request", "at least one storage target is required"))
		return
	}
	t, err := s.tasks.Enqueue(r.Context(), "backup", "database", strconv.FormatInt(req.InstanceID, 10), backup.TaskPayload{
		InstanceID: req.InstanceID,
		Database:   req.Database,
		StorageIDs: req.StorageIDs,
		CreatedBy:  user.Username,
	})
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	s.auditRecord(r, "create_backup", "database", req.Database, "success", "")
	writeJSON(w, http.StatusAccepted, t)
}

func (s *Server) handleCreateGlobalsBackup(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req backupCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if req.InstanceID <= 0 {
		writeError(w, ErrBadRequest("invalid_request", "instance_id is required"))
		return
	}
	if len(req.StorageIDs) == 0 {
		writeError(w, ErrBadRequest("invalid_request", "at least one storage target is required"))
		return
	}
	t, err := s.tasks.Enqueue(r.Context(), "backup", "globals", strconv.FormatInt(req.InstanceID, 10), backup.TaskPayload{
		InstanceID: req.InstanceID,
		Globals:    true,
		StorageIDs: req.StorageIDs,
		CreatedBy:  user.Username,
	})
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	writeJSON(w, http.StatusAccepted, t)
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	if err := s.engine.DeleteBackup(r.Context(), id); err != nil {
		if errors.Is(err, backup.ErrNotFound) {
			writeError(w, ErrNotFound("backup_not_found", "backup not found"))
			return
		}
		s.auditRecord(r, "delete_backup", "backup", strconv.FormatInt(id, 10), "failure", "")
		writeError(w, NewError(http.StatusConflict, "delete_backup_failed", err.Error()))
		return
	}
	s.auditRecord(r, "delete_backup", "backup", strconv.FormatInt(id, 10), "success", "")
	writeOK(w, map[string]string{"status": "ok"})
}

func backupNotFoundErr(err error) *APIError {
	if errors.Is(err, backup.ErrNotFound) {
		return ErrNotFound("backup_not_found", "backup not found")
	}
	return ErrInternal(err)
}

// registerTaskHandlers wires background task handlers to the task manager.
func (s *Server) registerTaskHandlers() {
	s.tasks.Register("backup", s.runBackupTask)
	s.tasks.Register("restore", s.runRestoreTask)
}

// RegisterTaskHandlers is the exported entry point for wiring task handlers.
func (s *Server) RegisterTaskHandlers() {
	s.registerTaskHandlers()
}

func (s *Server) runBackupTask(ctx context.Context, t *task.Task, payload []byte, log task.Logger) error {
	var p backup.TaskPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	req := backup.Request{
		InstanceID: p.InstanceID,
		Database:   p.Database,
		Globals:    p.Globals,
		StorageIDs: p.StorageIDs,
		CreatedBy:  p.CreatedBy,
	}
	logf := func(format string, args ...any) { log.Infof(format, args...) }
	result, err := s.engine.Backup(ctx, req, logf)
	if err != nil {
		return err
	}

	// apply retention after a successful database backup
	if result.Record.Status == "success" && !p.Globals && p.RetentionCount > 0 {
		if deleted, err := s.engine.ApplyRetention(ctx, p.InstanceID, p.Database, p.RetentionCount); err != nil {
			log.Errorf("retention failed: %v", err)
		} else if deleted > 0 {
			log.Infof("retention deleted %d old backup(s)", deleted)
		}
	}

	switch result.Record.Status {
	case "success":
		return nil
	case "partial_success":
		return &task.PartialError{Err: errors.New("some storage targets failed")}
	default:
		return errors.New(result.Record.ErrorMessage)
	}
}
