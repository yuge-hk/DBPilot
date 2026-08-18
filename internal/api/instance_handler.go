package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/dbpilot/dbpilot/internal/instance"
	"github.com/dbpilot/dbpilot/internal/provider"
)

type instanceRequest struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	SSLMode     string `json:"ssl_mode"`
	UnixSocket  string `json:"unix_socket"`
	SystemdUnit string `json:"systemd_unit"`
}

var validSSLModes = map[string]bool{
	"disable": true, "allow": true, "prefer": true, "require": true, "verify-ca": true, "verify-full": true,
}

func (r *instanceRequest) validate() *APIError {
	if r.Name == "" {
		return ErrBadRequest("invalid_name", "name is required")
	}
	if len(r.Name) > 64 {
		return ErrBadRequest("invalid_name", "name too long")
	}
	if r.UnixSocket == "" && r.Host == "" {
		return ErrBadRequest("invalid_host", "host is required")
	}
	if r.Port == 0 {
		r.Port = 5432
	}
	if r.Port < 1 || r.Port > 65535 {
		return ErrBadRequest("invalid_port", "invalid port")
	}
	if r.Username == "" {
		return ErrBadRequest("invalid_username", "username is required")
	}
	if r.SSLMode == "" {
		r.SSLMode = "prefer"
	}
	if !validSSLModes[r.SSLMode] {
		return ErrBadRequest("invalid_ssl_mode", "invalid ssl mode")
	}
	return nil
}

func (r *instanceRequest) toParams() provider.ConnParams {
	return provider.ConnParams{
		Host:       r.Host,
		Port:       r.Port,
		Username:   r.Username,
		Password:   r.Password,
		SSLMode:    r.SSLMode,
		UnixSocket: r.UnixSocket,
	}
}

func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	insts, err := s.instances.List(r.Context())
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if insts == nil {
		insts = []instance.Instance{}
	}
	writeOK(w, insts)
}

func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	inst, err := s.instances.Get(r.Context(), id)
	if err != nil {
		writeError(w, instanceNotFoundErr(err))
		return
	}
	writeOK(w, inst)
}

func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	var req instanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if apiErr := req.validate(); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if req.Password == "" {
		writeError(w, ErrBadRequest("invalid_password", "password is required"))
		return
	}

	// Test connection before persisting.
	testRes, err := s.pg.TestConnection(r.Context(), req.toParams())
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if !testRes.OK {
		writeError(w, NewError(http.StatusBadRequest, "connection_failed", testRes.Error))
		return
	}

	inst := &instance.Instance{
		Name:        req.Name,
		Host:        req.Host,
		Port:        req.Port,
		Username:    req.Username,
		SSLMode:     req.SSLMode,
		UnixSocket:  req.UnixSocket,
		SystemdUnit: req.SystemdUnit,
	}
	if err := s.instances.Create(r.Context(), inst, req.Password); err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	created, err := s.instances.Get(r.Context(), inst.ID)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateInstance(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	var req instanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if apiErr := req.validate(); apiErr != nil {
		writeError(w, apiErr)
		return
	}

	inst := &instance.Instance{
		ID:          id,
		Name:        req.Name,
		Host:        req.Host,
		Port:        req.Port,
		Username:    req.Username,
		SSLMode:     req.SSLMode,
		UnixSocket:  req.UnixSocket,
		SystemdUnit: req.SystemdUnit,
	}

	var newPassword *string
	if req.Password != "" {
		newPassword = &req.Password
	}

	// Verify new connection params before saving.
	params := req.toParams()
	if newPassword != nil {
		params.Password = *newPassword
	} else {
		existing, err := s.instances.Password(r.Context(), id)
		if err != nil {
			writeError(w, ErrInternal(err))
			return
		}
		params.Password = existing
	}
	testRes, err := s.pg.TestConnection(r.Context(), params)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if !testRes.OK {
		writeError(w, NewError(http.StatusBadRequest, "connection_failed", testRes.Error))
		return
	}

	if err := s.instances.Update(r.Context(), inst, newPassword); err != nil {
		writeError(w, instanceNotFoundErr(err))
		return
	}
	writeOK(w, inst)
}

func (s *Server) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	if err := s.instances.Delete(r.Context(), id); err != nil {
		writeError(w, instanceNotFoundErr(err))
		return
	}
	writeOK(w, map[string]string{"status": "ok"})
}

type testConnectionRequest struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	SSLMode    string `json:"ssl_mode"`
	UnixSocket string `json:"unix_socket"`
}

func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	var req testConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if req.Port == 0 {
		req.Port = 5432
	}
	if req.SSLMode == "" {
		req.SSLMode = "prefer"
	}
	if !validSSLModes[req.SSLMode] {
		writeError(w, ErrBadRequest("invalid_ssl_mode", "invalid ssl mode"))
		return
	}
	res, err := s.pg.TestConnection(r.Context(), provider.ConnParams{
		Host:       req.Host,
		Port:       req.Port,
		Username:   req.Username,
		Password:   req.Password,
		SSLMode:    req.SSLMode,
		UnixSocket: req.UnixSocket,
	})
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	writeOK(w, res)
}

func (s *Server) handleTestInstance(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	params, err := s.instances.ConnParams(r.Context(), id)
	if err != nil {
		writeError(w, instanceNotFoundErr(err))
		return
	}
	res, err := s.pg.TestConnection(r.Context(), params)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	writeOK(w, res)
}

func (s *Server) handleDetectTools(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	writeOK(w, s.pg.DetectTools(ctx))
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func instanceNotFoundErr(err error) *APIError {
	if errors.Is(err, instance.ErrNotFound) {
		return ErrNotFound("instance_not_found", "instance not found")
	}
	return ErrInternal(err)
}
