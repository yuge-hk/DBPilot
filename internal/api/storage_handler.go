package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/dbpilot/dbpilot/internal/storage"
	"github.com/dbpilot/dbpilot/internal/storageregistry"
)

type storageRequest struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Config   json.RawMessage `json:"config"`
	Password string          `json:"password"`
}

func (s *Server) handleListStorages(w http.ResponseWriter, r *http.Request) {
	targets, err := s.targets.List(r.Context())
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if targets == nil {
		targets = []storage.Target{}
	}
	writeOK(w, targets)
}

func (s *Server) handleCreateStorage(w http.ResponseWriter, r *http.Request) {
	var req storageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if apiErr := validateStorageRequest(&req); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	cfg, err := parseStorageConfig(&req)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_request", err.Error()))
		return
	}
	target, err := s.targets.Create(r.Context(), req.Name, req.Type, cfg, req.Password)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	s.auditRecord(r, "create_storage", "storage", req.Name, "success", "type="+req.Type)
	writeJSON(w, http.StatusCreated, target)
}

func (s *Server) handleUpdateStorage(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	var req storageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if apiErr := validateStorageRequest(&req); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	cfg, err := parseStorageConfig(&req)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_request", err.Error()))
		return
	}
	var newPassword *string
	if req.Password != "" {
		newPassword = &req.Password
	}
	if err := s.targets.Update(r.Context(), id, req.Name, cfg, newPassword); err != nil {
		writeError(w, storageNotFoundErr(err))
		return
	}
	s.auditRecord(r, "update_storage", "storage", req.Name, "success", "")
	writeOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteStorage(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	if err := s.targets.Delete(r.Context(), id); err != nil {
		writeError(w, storageNotFoundErr(err))
		return
	}
	s.auditRecord(r, "delete_storage", "storage", strconv.FormatInt(id, 10), "success", "")
	writeOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleTestStorage(w http.ResponseWriter, r *http.Request) {
	var req storageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if apiErr := validateStorageRequest(&req); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	cfg, err := parseStorageConfig(&req)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_request", err.Error()))
		return
	}
	target := &storage.Target{Type: req.Type, Config: mustJSON(cfg)}
	st, err := storageregistry.Build(r.Context(), target, req.Password)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_request", err.Error()))
		return
	}
	if err := st.Test(r.Context()); err != nil {
		writeOK(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func validateStorageRequest(req *storageRequest) *APIError {
	if req.Name == "" {
		return ErrBadRequest("invalid_name", "name is required")
	}
	if req.Type != string(storage.TypeLocal) && req.Type != string(storage.TypeWebDAV) {
		return ErrBadRequest("invalid_request", "unknown storage type")
	}
	return nil
}

func parseStorageConfig(req *storageRequest) (any, error) {
	switch req.Type {
	case string(storage.TypeLocal):
		var cfg storage.LocalConfig
		if err := json.Unmarshal(req.Config, &cfg); err != nil {
			return nil, err
		}
		if cfg.Path == "" {
			return nil, errors.New("path is required")
		}
		return cfg, nil
	case string(storage.TypeWebDAV):
		var cfg storage.WebDAVConfig
		if err := json.Unmarshal(req.Config, &cfg); err != nil {
			return nil, err
		}
		if cfg.Endpoint == "" {
			return nil, errors.New("endpoint is required")
		}
		if cfg.Timeout <= 0 {
			cfg.Timeout = 30
		}
		return cfg, nil
	default:
		return nil, errors.New("unknown storage type")
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func storageNotFoundErr(err error) *APIError {
	if errors.Is(err, storage.ErrTargetNotFound) {
		return ErrNotFound("storage_not_found", "storage target not found")
	}
	return ErrInternal(err)
}
