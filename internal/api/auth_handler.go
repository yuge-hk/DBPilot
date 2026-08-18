package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dbpilot/dbpilot/internal/auth"
)

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.auth.CountUsers(r.Context())
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	writeOK(w, map[string]bool{"initialized": n > 0})
}

type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if err := validateUsername(req.Username); err != nil {
		writeError(w, err)
		return
	}
	if err := validatePassword(req.Password); err != nil {
		writeError(w, err)
		return
	}
	n, err := s.auth.CountUsers(r.Context())
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if n > 0 {
		writeError(w, ErrConflict("already_initialized", "already initialized"))
		return
	}
	user, err := s.auth.InitAdmin(r.Context(), req.Username, req.Password)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			// concurrent double-submit raced; treat as already initialized
			writeError(w, ErrConflict("already_initialized", "already initialized"))
			return
		}
		writeError(w, ErrInternal(err))
		return
	}
	s.issueSession(w, r, user)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	user, err := s.auth.Authenticate(r.Context(), req.Username, req.Password)
	if err != nil {
		s.auditRecord(r, "login", "user", req.Username, "failure", "")
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, ErrUnauthorized())
			return
		}
		writeError(w, ErrInternal(err))
		return
	}
	s.auditRecord(r, "login", "user", req.Username, "success", "")
	s.issueSession(w, r, user)
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, user *auth.User) {
	token, csrf, err := s.auth.CreateSession(r.Context(), user.ID, auth.SessionTTL)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	s.setSessionCookie(w, token, auth.SessionTTL)
	writeOK(w, map[string]any{
		"username":   user.Username,
		"csrf_token": csrf,
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	sess, _ := r.Context().Value(ctxSess).(*auth.Session)
	csrf := ""
	if sess != nil {
		csrf = sess.CSRFToken
	}
	writeOK(w, map[string]any{"id": user.ID, "username": user.Username, "csrf_token": csrf})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := s.sessionToken(r)
	if token != "" {
		_ = s.auth.DeleteSession(r.Context(), token)
	}
	s.clearSessionCookie(w)
	writeOK(w, map[string]string{"status": "ok"})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if err := validatePassword(req.NewPassword); err != nil {
		writeError(w, err)
		return
	}
	user := currentUser(r)
	full, err := s.auth.FindByUsername(r.Context(), user.Username)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if !auth.VerifyPassword(full.PasswordHash, req.CurrentPassword) {
		writeError(w, ErrBadRequest("invalid_current_password", "current password is incorrect"))
		return
	}
	if err := s.auth.UpdatePassword(r.Context(), user.ID, req.NewPassword); err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	_ = s.auth.DeleteAllSessionsForUser(r.Context(), user.ID)
	writeOK(w, map[string]string{"status": "ok"})
}

func currentUser(r *http.Request) *auth.User {
	u, _ := r.Context().Value(ctxUser).(*auth.User)
	return u
}

func validateUsername(u string) error {
	if u == "" {
		return ErrBadRequest("invalid_username", "username is required")
	}
	if utf8.RuneCountInString(u) > 64 {
		return ErrBadRequest("invalid_username", "username is too long")
	}
	for _, c := range u {
		if unicode.IsSpace(c) || unicode.IsControl(c) {
			return ErrBadRequest("invalid_username", "username contains invalid characters")
		}
	}
	return nil
}

func validatePassword(p string) error {
	if len(p) < 8 || len(p) > 128 {
		return ErrBadRequest("invalid_password", "password must be 8-128 characters")
	}
	return nil
}
