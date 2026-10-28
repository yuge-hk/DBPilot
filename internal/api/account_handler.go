package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dbpilot/dbpilot/internal/provider"
	"github.com/dbpilot/dbpilot/internal/provider/postgres"
)

type accountRequest struct {
	Name       string  `json:"name"`
	Password   string  `json:"password"`
	Login      *bool   `json:"login"`
	Superuser  *bool   `json:"superuser"`
	CreateDB   *bool   `json:"create_db"`
	CreateRole *bool   `json:"create_role"`
	ConnLimit  *int    `json:"conn_limit"`
	ValidUntil *string `json:"valid_until"`
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
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
	accounts, err := s.pg.ListAccounts(r.Context(), params)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if accounts == nil {
		accounts = []provider.Account{}
	}
	writeOK(w, accounts)
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	var req accountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if apiErr := validateIdentifier(req.Name, "account"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if postgres.IsProtectedAccount(req.Name) {
		writeError(w, NewError(http.StatusForbidden, "protected_account", "this account is protected"))
		return
	}
	if req.Password == "" {
		writeError(w, ErrBadRequest("invalid_password", "password is required"))
		return
	}
	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	login := true
	if req.Login != nil {
		login = *req.Login
	}
	connLimit := -1
	if req.ConnLimit != nil {
		connLimit = *req.ConnLimit
	}
	validUntil := ""
	if req.ValidUntil != nil {
		validUntil = *req.ValidUntil
	}
	creq := provider.CreateAccountRequest{
		Name:         req.Name,
		Password:     req.Password,
		Login:        login,
		Superuser:    boolVal(req.Superuser),
		CreateDB:     boolVal(req.CreateDB),
		CreateRole:   boolVal(req.CreateRole),
		ConnLimit:    connLimit,
		ConnLimitSet: req.ConnLimit != nil,
		ValidUntil:   validUntil,
	}
	if err := s.pg.CreateAccount(r.Context(), params, creq); err != nil {
		s.auditRecord(r, "create_account", "account", req.Name, "failure", "")
		writeError(w, accountError(err))
		return
	}
	s.auditRecord(r, "create_account", "account", req.Name, "success", "")
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name})
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	name := pathParam(r, "name")
	if apiErr := validateIdentifier(name, "account"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	var req accountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	ureq := provider.UpdateAccountRequest{Name: name}
	if req.Password != "" {
		ureq.NewPassword = &req.Password
	}
	ureq.Login = req.Login
	ureq.Superuser = req.Superuser
	ureq.CreateDB = req.CreateDB
	ureq.CreateRole = req.CreateRole
	ureq.ConnLimit = req.ConnLimit
	ureq.ValidUntil = req.ValidUntil
	if err := s.pg.UpdateAccount(r.Context(), params, ureq); err != nil {
		s.auditRecord(r, "update_account", "account", name, "failure", "")
		writeError(w, accountError(err))
		return
	}
	s.auditRecord(r, "update_account", "account", name, "success", "")
	writeOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	name := pathParam(r, "name")
	if apiErr := validateIdentifier(name, "account"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if err := s.pg.DeleteAccount(r.Context(), params, name); err != nil {
		s.auditRecord(r, "delete_account", "account", name, "failure", "")
		writeError(w, accountError(err))
		return
	}
	s.auditRecord(r, "delete_account", "account", name, "success", "")
	writeOK(w, map[string]string{"status": "ok"})
}

func boolVal(b *bool) bool {
	return b != nil && *b
}

type grantPrivilegeRequest struct {
	Database   string `json:"database"`
	Permission string `json:"permission"`
}

func (s *Server) handleListPrivileges(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	name := pathParam(r, "name")
	if apiErr := validateIdentifier(name, "account"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	privs, err := s.pg.ListDatabasePrivileges(r.Context(), params, name)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if privs == nil {
		privs = []provider.DatabasePrivilege{}
	}
	writeOK(w, privs)
}

func (s *Server) handleGrantPrivileges(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	name := pathParam(r, "name")
	if apiErr := validateIdentifier(name, "account"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	var req grantPrivilegeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if apiErr := validateIdentifier(req.Database, "database"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if req.Permission != "owner" && req.Permission != "read_write" && req.Permission != "read_only" {
		writeError(w, ErrBadRequest("invalid_request", "invalid permission"))
		return
	}
	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if err := s.pg.GrantDatabasePrivileges(r.Context(), params, name, req.Database, req.Permission); err != nil {
		s.auditRecord(r, "grant_privileges", "account", name, "failure", "database="+req.Database+" permission="+req.Permission)
		writeError(w, privilegeError(err, "grant_failed", "grant privileges failed"))
		return
	}
	s.auditRecord(r, "grant_privileges", "account", name, "success", "database="+req.Database+" permission="+req.Permission)
	writeOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleRevokePrivileges(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	name := pathParam(r, "name")
	database := pathParam(r, "database")
	if apiErr := validateIdentifier(name, "account"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if apiErr := validateIdentifier(database, "database"); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if err := s.pg.RevokeDatabasePrivileges(r.Context(), params, name, database); err != nil {
		s.auditRecord(r, "revoke_privileges", "account", name, "failure", "database="+database)
		writeError(w, privilegeError(err, "revoke_failed", "revoke privileges failed"))
		return
	}
	s.auditRecord(r, "revoke_privileges", "account", name, "success", "database="+database)
	writeOK(w, map[string]string{"status": "ok"})
}

func accountError(err error) *APIError {
	switch {
	case errors.Is(err, provider.ErrAccountProtected):
		return NewError(http.StatusForbidden, "protected_account", "this account is protected")
	case errors.Is(err, provider.ErrAccountNotFound):
		return ErrNotFound("account_not_found", "account not found")
	case errors.Is(err, provider.ErrAccountHasDeps):
		return NewError(http.StatusConflict, "account_has_dependencies", err.Error())
	case errors.Is(err, provider.ErrAccountOwnsDatabase):
		return NewError(http.StatusConflict, "account_owns_database", "account owns the database")
	case errors.Is(err, provider.ErrCurrentAccountChange):
		return NewError(http.StatusConflict, "current_account_password", "change the current connection password in instance settings")
	default:
		return NewError(http.StatusConflict, "account_operation_failed", err.Error())
	}
}

func privilegeError(err error, fallbackCode, fallbackMessage string) *APIError {
	switch {
	case errors.Is(err, provider.ErrAccountOwnsDatabase):
		return NewError(http.StatusConflict, "account_owns_database", "account owns the database")
	case errors.Is(err, provider.ErrAccountNotFound):
		return ErrNotFound("account_not_found", "account not found")
	case errors.Is(err, provider.ErrDatabaseNotFound):
		return ErrNotFound("database_not_found", "database not found")
	default:
		return NewError(http.StatusConflict, fallbackCode, fallbackMessage+": "+err.Error())
	}
}
