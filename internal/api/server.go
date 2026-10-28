package api

import (
	"context"
	"crypto/subtle"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/dbpilot/dbpilot/internal/audit"
	"github.com/dbpilot/dbpilot/internal/auth"
	"github.com/dbpilot/dbpilot/internal/backup"
	"github.com/dbpilot/dbpilot/internal/config"
	"github.com/dbpilot/dbpilot/internal/instance"
	"github.com/dbpilot/dbpilot/internal/persistence"
	"github.com/dbpilot/dbpilot/internal/provider/postgres"
	"github.com/dbpilot/dbpilot/internal/runner"
	"github.com/dbpilot/dbpilot/internal/storage"
	"github.com/dbpilot/dbpilot/internal/task"
	"github.com/dbpilot/dbpilot/internal/version"
)

const (
	SessionCookieName = "dbpilot_session"
	CSRFHeader        = "X-CSRF-Token"
	csrfTokenLen      = 32
)

type ctxKey string

const (
	ctxUser ctxKey = "user"
	ctxSess ctxKey = "session"
)

type Server struct {
	cfg       *config.Config
	db        *persistence.DB
	auth      *auth.Service
	staticFS  fs.FS
	rateLimit *loginRateLimiter
	instances *instance.Store
	pg        *postgres.Postgres
	targets   *storage.TargetStore
	backups   *backup.Store
	policies  *backup.PolicyStore
	tasks     *task.Manager
	engine    *backup.Engine
	audit     *audit.Service
	runner    runner.Runner
}

func NewServer(cfg *config.Config, db *persistence.DB, authSvc *auth.Service, static fs.FS) *Server {
	return &Server{
		cfg:       cfg,
		db:        db,
		auth:      authSvc,
		staticFS:  static,
		rateLimit: newLoginRateLimiter(5, time.Minute),
	}
}

func (s *Server) SetInstanceStore(store *instance.Store) {
	s.instances = store
}

func (s *Server) SetPostgresProvider(p *postgres.Postgres) {
	s.pg = p
}

func (s *Server) SetStorageTargets(store *storage.TargetStore) {
	s.targets = store
}

func (s *Server) SetBackupStore(store *backup.Store) {
	s.backups = store
}

func (s *Server) SetPolicyStore(store *backup.PolicyStore) {
	s.policies = store
}

func (s *Server) SetTaskManager(m *task.Manager) {
	s.tasks = m
}

func (s *Server) SetBackupEngine(e *backup.Engine) {
	s.engine = e
}

func (s *Server) SetAuditService(a *audit.Service) {
	s.audit = a
}

func (s *Server) SetRunner(r runner.Runner) {
	s.runner = r
}

// auditRecord writes an audit entry for the current request (no secrets).
func (s *Server) auditRecord(r *http.Request, action, resourceType, resourceID, result, detail string) {
	if s.audit == nil {
		return
	}
	actor := ""
	if u := currentUser(r); u != nil {
		actor = u.Username
	}
	ip := r.RemoteAddr
	s.audit.Record(r.Context(), actor, action, resourceType, resourceID, result, ip, detail)
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(s.requestLogger)

	r.Get("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, map[string]string{"status": "ok", "version": version.String()})
	})

	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Get("/status", s.handleAuthStatus)
		r.Post("/setup", s.rateLimit.middleware(s.handleSetup))
		r.Post("/login", s.rateLimit.middleware(s.handleLogin))
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Use(s.requireCSRF)
			r.Get("/me", s.handleMe)
			r.Post("/logout", s.handleLogout)
			r.Post("/change-password", s.handleChangePassword)
		})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Use(s.requireCSRF)
		s.routes(r)
	})

	s.serveStatic(r)
	return r
}

func (s *Server) routes(r chi.Router) {
	r.Route("/instances", func(r chi.Router) {
		r.Get("/", s.handleListInstances)
		r.Post("/", s.handleCreateInstance)
		r.Post("/test", s.handleTestConnection)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetInstance)
			r.Put("/", s.handleUpdateInstance)
			r.Delete("/", s.handleDeleteInstance)
			r.Post("/test", s.handleTestInstance)
			r.Get("/tools", s.handleDetectTools)

			r.Get("/databases", s.handleListDatabases)
			r.Post("/databases", s.handleCreateDatabase)
			r.Delete("/databases/{name}", s.handleDeleteDatabase)

			r.Get("/firewall", s.handleGetFirewall)
			r.Post("/firewall/rules", s.handleAddFirewallRule)
			r.Delete("/firewall/rules", s.handleRemoveFirewallRule)

			r.Get("/accounts", s.handleListAccounts)
			r.Post("/accounts", s.handleCreateAccount)
			r.Put("/accounts/{name}", s.handleUpdateAccount)
			r.Delete("/accounts/{name}", s.handleDeleteAccount)
			r.Post("/accounts/{name}/privileges", s.handleGrantPrivileges)
			r.Get("/accounts/{name}/privileges", s.handleListPrivileges)
			r.Delete("/accounts/{name}/privileges/{database}", s.handleRevokePrivileges)
		})
	})

	r.Route("/storages", func(r chi.Router) {
		r.Get("/", s.handleListStorages)
		r.Post("/", s.handleCreateStorage)
		r.Post("/test", s.handleTestStorage)
		r.Route("/{id}", func(r chi.Router) {
			r.Put("/", s.handleUpdateStorage)
			r.Delete("/", s.handleDeleteStorage)
		})
	})

	r.Route("/backups", func(r chi.Router) {
		r.Get("/", s.handleListBackups)
		r.Post("/", s.handleCreateBackup)
		r.Post("/globals", s.handleCreateGlobalsBackup)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetBackup)
			r.Delete("/", s.handleDeleteBackup)
		})
	})

	r.Route("/backup-policies", func(r chi.Router) {
		r.Get("/", s.handleListPolicies)
		r.Post("/", s.handleCreatePolicy)
		r.Route("/{id}", func(r chi.Router) {
			r.Put("/", s.handleUpdatePolicy)
			r.Delete("/", s.handleDeletePolicy)
		})
	})

	r.Route("/tasks", func(r chi.Router) {
		r.Get("/", s.handleListTasks)
		r.Get("/{id}", s.handleGetTask)
	})

	r.Route("/restores", func(r chi.Router) {
		r.Post("/", s.handleCreateRestore)
	})

	r.Route("/audit", func(r chi.Router) {
		r.Get("/", s.handleListAudit)
	})

	r.Get("/overview", s.handleOverview)
	r.Get("/system/addresses", s.handleSystemAddresses)
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := s.sessionToken(r)
		if token == "" {
			writeError(w, ErrUnauthorized())
			return
		}
		sess, err := s.auth.GetSession(r.Context(), token)
		if err != nil {
			writeError(w, ErrInternal(err))
			return
		}
		if sess == nil {
			s.clearSessionCookie(w)
			writeError(w, ErrUnauthorized())
			return
		}
		user, err := s.auth.GetUser(r.Context(), sess.UserID)
		if err != nil {
			writeError(w, ErrInternal(err))
			return
		}
		ctx := context.WithValue(r.Context(), ctxSess, sess)
		ctx = context.WithValue(ctx, ctxUser, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		sess, _ := r.Context().Value(ctxSess).(*auth.Session)
		if sess == nil {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get(CSRFHeader)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(sess.CSRFToken)) != 1 {
			writeError(w, NewError(http.StatusForbidden, "invalid_csrf_token", "invalid csrf token"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sessionToken(r *http.Request) string {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.Server.ExternalURL != "" && strings.HasPrefix(s.cfg.Server.ExternalURL, "https"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) serveStatic(r chi.Router) {
	if s.staticFS == nil {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, ErrNotFound("not_found", "not found"))
		})
		return
	}
	fileServer := http.FileServer(http.FS(s.staticFS))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, ErrNotFound("not_found", "not found"))
			return
		}
		if r.URL.Path != "/" {
			if _, err := fs.Stat(s.staticFS, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
				// SPA fallback
				index, err := fs.ReadFile(s.staticFS, "index.html")
				if err != nil {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write(index)
				return
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}

type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func newLoginRateLimiter(max int, window time.Duration) *loginRateLimiter {
	return &loginRateLimiter{attempts: make(map[string][]time.Time), max: max, window: window}
}

func (l *loginRateLimiter) middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if idx := strings.LastIndex(ip, ":"); idx != -1 {
			ip = ip[:idx]
		}
		l.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-l.window)
		var recent []time.Time
		for _, t := range l.attempts[ip] {
			if t.After(cutoff) {
				recent = append(recent, t)
			}
		}
		l.attempts[ip] = recent
		if len(recent) >= l.max {
			l.mu.Unlock()
			writeError(w, NewError(http.StatusTooManyRequests, "too_many_attempts", "too many login attempts"))
			return
		}
		l.attempts[ip] = append(recent, now)
		l.mu.Unlock()
		next.ServeHTTP(w, r)
	}
}
