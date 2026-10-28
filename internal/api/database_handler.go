package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/dbpilot/dbpilot/internal/provider"
)

func (s *Server) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	dbs, err := s.pg.ListDatabases(r.Context(), params)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if dbs == nil {
		dbs = []provider.Database{}
	}
	writeOK(w, dbs)
}

type createDatabaseRequest struct {
	Database      string `json:"database"`
	AccountMode   string `json:"account_mode"`
	Account       string `json:"account"`
	Password      string `json:"password"`
	Permission    string `json:"permission"`
	RemoteAccess  bool   `json:"remote_access"`
	RemoteAddress string `json:"remote_address"`
}

type createDatabaseResponse struct {
	provider.CreateResult
	RemoteAccess    bool   `json:"remote_access"`
	RemoteAddress   string `json:"remote_address,omitempty"`
	FirewallWarning string `json:"firewall_warning,omitempty"`
}

func (s *Server) handleCreateDatabase(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	var req createDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if apiErr := validateIdentifier(req.Database, "database"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if apiErr := validateIdentifier(req.Account, "account"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if req.AccountMode != "create" && req.AccountMode != "existing" {
		writeError(w, ErrBadRequest("invalid_request", "invalid account mode"))
		return
	}
	permissions := map[string]bool{"owner": true, "read_write": true, "read_only": true}
	if !permissions[req.Permission] {
		writeError(w, ErrBadRequest("invalid_request", "invalid permission"))
		return
	}
	if req.AccountMode == "create" && req.Password == "" {
		writeError(w, ErrBadRequest("invalid_password", "password is required"))
		return
	}
	if req.RemoteAccess && !validHbaAddress(req.RemoteAddress) {
		writeError(w, ErrBadRequest("invalid_address", "a valid remote address or CIDR is required"))
		return
	}
	if req.RemoteAccess && (req.Database == "all" || req.Account == "all") {
		writeError(w, ErrBadRequest("invalid_name", "database or account name \"all\" cannot be used for automatic remote access"))
		return
	}

	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	res, err := s.pg.CreateDatabaseWithAccount(r.Context(), params, provider.CreateDatabaseRequest{
		Database:    req.Database,
		AccountMode: req.AccountMode,
		Account:     req.Account,
		Password:    req.Password,
		Permission:  req.Permission,
	})
	if err != nil {
		s.auditRecord(r, "create_database", "database", req.Database, "failure", "")
		writeError(w, createDBError(err))
		return
	}
	auditDetail := "account=" + req.Account + " permission=" + req.Permission
	response := createDatabaseResponse{
		CreateResult:  *res,
		RemoteAccess:  req.RemoteAccess,
		RemoteAddress: req.RemoteAddress,
	}
	if inst, ierr := s.instances.Get(r.Context(), id); ierr == nil && inst.ConnectionHost != "" {
		response.Host = inst.ConnectionHost
		response.UnixSocket = ""
		if response.Port == 0 {
			response.Port = inst.Port
			if response.Port == 0 {
				response.Port = 5432
			}
		}
	}
	if req.RemoteAccess {
		auditDetail += " remote=" + req.RemoteAddress
		firewallReq := addFirewallRuleRequest{
			Database: req.Database,
			User:     req.Account,
			Address:  req.RemoteAddress,
			Method:   "scram-sha-256",
		}
		if ferr := s.addFirewallRule(r.Context(), id, params, firewallReq); ferr != nil {
			response.FirewallWarning = ferr.Error()
			s.auditRecord(r, "add_firewall_rule", "instance", params.Host, "failure", "address="+req.RemoteAddress)
		} else {
			s.auditRecord(r, "add_firewall_rule", "instance", params.Host, "success", "address="+req.RemoteAddress)
			response.FirewallWarning = s.remoteAccessWarning(r.Context(), params)
		}
	}
	s.auditRecord(r, "create_database", "database", req.Database, "success", auditDetail)
	writeJSON(w, http.StatusCreated, response)
}

func createDBError(err error) *APIError {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "database already exists"):
		return NewError(http.StatusConflict, "database_exists", "database already exists")
	case strings.Contains(msg, "account already exists"):
		return NewError(http.StatusConflict, "account_exists", "account already exists")
	case errors.Is(err, provider.ErrAccountProtected):
		return NewError(http.StatusForbidden, "protected_account", "this account is protected")
	case errors.Is(err, provider.ErrAccountNotFound):
		return ErrNotFound("account_not_found", "account not found")
	case errors.Is(err, provider.ErrAccountCannotLogin):
		return ErrBadRequest("account_cannot_login", "account cannot login")
	case errors.Is(err, provider.ErrAccountOwnsDatabase):
		return NewError(http.StatusConflict, "account_owns_database", "account owns the database")
	default:
		return NewError(http.StatusConflict, "create_database_failed", err.Error())
	}
}

func (s *Server) handleDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	name := pathParam(r, "name")
	if apiErr := validateIdentifier(name, "database"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if err := s.pg.DeleteDatabase(r.Context(), params, name); err != nil {
		s.auditRecord(r, "delete_database", "database", name, "failure", "")
		writeError(w, dbDeleteError(err))
		return
	}
	s.auditRecord(r, "delete_database", "database", name, "success", "")
	writeOK(w, map[string]string{"status": "ok"})
}

func dbDeleteError(err error) *APIError {
	switch {
	case errors.Is(err, provider.ErrDatabaseProtected):
		return NewError(http.StatusForbidden, "protected_database", "this database is protected")
	case errors.Is(err, provider.ErrDatabaseNotFound):
		return ErrNotFound("database_not_found", "database not found")
	default:
		return NewError(http.StatusConflict, "delete_failed", err.Error())
	}
}
