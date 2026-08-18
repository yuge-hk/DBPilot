package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dbpilot/dbpilot/internal/auth"
	"github.com/dbpilot/dbpilot/internal/config"
	"github.com/dbpilot/dbpilot/internal/persistence"
)

func newTestServer(t *testing.T) (*Server, *auth.Service) {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Server.Addr = "127.0.0.1"
	cfg.Server.Port = 0

	db, err := persistence.Open(filepath.Join(cfg.DataDir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := auth.NewService(db)
	srv := NewServer(cfg, db, svc, nil)
	return srv, svc
}

func doJSON(t *testing.T, srv *Server, method, path, token string, csrf string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	}
	if csrf != "" {
		req.Header.Set(CSRFHeader, csrf)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestAuthFlow(t *testing.T) {
	srv, _ := newTestServer(t)

	// status: not initialized
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/auth/status", "", "", nil)
	if rec.Code != 200 {
		t.Fatalf("status code: %d", rec.Code)
	}
	var st map[string]bool
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st["initialized"] {
		t.Fatal("expected not initialized")
	}

	// setup admin
	rec = doJSON(t, srv, http.MethodPost, "/api/v1/auth/setup", "", "", map[string]string{
		"username": "admin", "password": "admin12345",
	})
	if rec.Code != 200 {
		t.Fatalf("setup code: %d body=%s", rec.Code, rec.Body.String())
	}
	var setupResp struct {
		CSRFToken string `json:"csrf_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &setupResp)
	if setupResp.CSRFToken == "" {
		t.Fatal("setup should return csrf token")
	}
	token := rec.Result().Cookies()[0].Value
	if token == "" {
		t.Fatal("setup should set session cookie")
	}

	// me with token
	rec = doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", token, "", nil)
	if rec.Code != 200 {
		t.Fatalf("me code: %d", rec.Code)
	}

	// duplicate setup should conflict
	rec = doJSON(t, srv, http.MethodPost, "/api/v1/auth/setup", "", "", map[string]string{
		"username": "other", "password": "other12345",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup should conflict, got %d", rec.Code)
	}

	// change password without csrf -> forbidden
	rec = doJSON(t, srv, http.MethodPost, "/api/v1/auth/change-password", token, "", map[string]string{
		"current_password": "admin12345", "new_password": "newpassword1",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("change password without csrf should be forbidden, got %d", rec.Code)
	}

	// change password with csrf -> ok
	rec = doJSON(t, srv, http.MethodPost, "/api/v1/auth/change-password", token, setupResp.CSRFToken, map[string]string{
		"current_password": "admin12345", "new_password": "newpassword1",
	})
	if rec.Code != 200 {
		t.Fatalf("change password: %d body=%s", rec.Code, rec.Body.String())
	}

	// login with old password fails
	rec = doJSON(t, srv, http.MethodPost, "/api/v1/auth/login", "", "", map[string]string{
		"username": "admin", "password": "admin12345",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old password login should be unauthorized, got %d", rec.Code)
	}

	// login with new password works
	rec = doJSON(t, srv, http.MethodPost, "/api/v1/auth/login", "", "", map[string]string{
		"username": "admin", "password": "newpassword1",
	})
	if rec.Code != 200 {
		t.Fatalf("new password login should work, got %d", rec.Code)
	}
}

func TestUnauthorizedAccess(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", "", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
