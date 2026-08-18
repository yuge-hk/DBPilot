package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/dbpilot/dbpilot/internal/backup"
)

type policyRequest struct {
	Name           string          `json:"name"`
	InstanceID     int64           `json:"instance_id"`
	DatabaseName   string          `json:"database_name"`
	Enabled        bool            `json:"enabled"`
	Schedule       backup.Schedule `json:"schedule"`
	RetentionCount int             `json:"retention_count"`
	GlobalsEnabled bool            `json:"globals_enabled"`
	StorageTargets []int64         `json:"storage_targets"`
}

func (r *policyRequest) validate() *APIError {
	if r.Name == "" {
		return ErrBadRequest("invalid_name", "name is required")
	}
	if r.InstanceID <= 0 {
		return ErrBadRequest("invalid_request", "instance_id is required")
	}
	if r.DatabaseName == "" && !r.GlobalsEnabled {
		return ErrBadRequest("invalid_request", "database_name is required")
	}
	if len(r.StorageTargets) == 0 {
		return ErrBadRequest("invalid_request", "at least one storage target is required")
	}
	if r.Schedule.Type == "" {
		r.Schedule.Type = "daily"
	}
	if r.Schedule.Type != "daily" && r.Schedule.Type != "weekly" && r.Schedule.Type != "monthly" {
		return ErrBadRequest("invalid_request", "invalid schedule type")
	}
	if r.Schedule.Time == "" {
		r.Schedule.Time = "03:00"
	}
	return nil
}

func (s *Server) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := s.policies.List(r.Context())
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if policies == nil {
		policies = []backup.Policy{}
	}
	writeOK(w, policies)
}

func (s *Server) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	var req policyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if apiErr := req.validate(); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	p := &backup.Policy{
		Name:           req.Name,
		InstanceID:     req.InstanceID,
		DatabaseName:   req.DatabaseName,
		Enabled:        req.Enabled,
		Schedule:       req.Schedule,
		RetentionCount: req.RetentionCount,
		GlobalsEnabled: req.GlobalsEnabled,
		StorageTargets: req.StorageTargets,
	}
	created, err := s.policies.Create(r.Context(), p)
	if err != nil {
		s.auditRecord(r, "create_backup_policy", "policy", req.Name, "failure", "")
		writeError(w, ErrInternal(err))
		return
	}
	s.auditRecord(r, "create_backup_policy", "policy", req.Name, "success", "database="+req.DatabaseName)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	var req policyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if apiErr := req.validate(); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	p := &backup.Policy{
		ID:             id,
		Name:           req.Name,
		InstanceID:     req.InstanceID,
		DatabaseName:   req.DatabaseName,
		Enabled:        req.Enabled,
		Schedule:       req.Schedule,
		RetentionCount: req.RetentionCount,
		GlobalsEnabled: req.GlobalsEnabled,
		StorageTargets: req.StorageTargets,
	}
	if err := s.policies.Update(r.Context(), p); err != nil {
		s.auditRecord(r, "update_backup_policy", "policy", req.Name, "failure", "")
		writeError(w, policyNotFoundErr(err))
		return
	}
	s.auditRecord(r, "update_backup_policy", "policy", req.Name, "success", "")
	writeOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	if err := s.policies.Delete(r.Context(), id); err != nil {
		writeError(w, policyNotFoundErr(err))
		return
	}
	s.auditRecord(r, "delete_backup_policy", "policy", strconv.FormatInt(id, 10), "success", "")
	writeOK(w, map[string]string{"status": "ok"})
}

func policyNotFoundErr(err error) *APIError {
	if errors.Is(err, backup.ErrPolicyNotFound) {
		return ErrNotFound("policy_not_found", "backup policy not found")
	}
	return ErrInternal(err)
}
