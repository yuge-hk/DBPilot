package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dbpilot/dbpilot/internal/persistence"
)

const (
	DefaultUsername = "admin"
	SessionTTL      = 24 * time.Hour
)

var ErrUserNotFound = errors.New("user not found")

type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

type Service struct {
	db *persistence.DB
}

func NewService(db *persistence.DB) *Service {
	return &Service{db: db}
}

func (s *Service) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Service) InitAdmin(ctx context.Context, username, password string) (*User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO users(username, password_hash) VALUES(?, ?)`, username, hash)
	if err != nil {
		return nil, fmt.Errorf("create admin: %w", err)
	}
	id, _ := res.LastInsertId()
	return &User{ID: id, Username: username}, nil
}

func (s *Service) FindByUsername(ctx context.Context, username string) (*User, error) {
	var (
		u    User
		hash string
	)
	err := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.PasswordHash = hash
	return &u, nil
}

func (s *Service) Authenticate(ctx context.Context, username, password string) (*User, error) {
	u, err := s.FindByUsername(ctx, username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if !VerifyPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

func (s *Service) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (token, csrf string, err error) {
	token, err = NewToken()
	if err != nil {
		return "", "", err
	}
	csrf, err = NewToken()
	if err != nil {
		return "", "", err
	}
	expires := time.Now().Add(ttl).UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions(token_hash, user_id, csrf_token, expires_at) VALUES(?, ?, ?, ?)`,
		HashToken(token), userID, csrf, expires)
	if err != nil {
		return "", "", err
	}
	return token, csrf, nil
}

type Session struct {
	ID        int64
	UserID    int64
	CSRFToken string
	ExpiresAt time.Time
}

func (s *Service) GetSession(ctx context.Context, token string) (*Session, error) {
	var sess Session
	var expires string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, csrf_token, expires_at FROM sessions WHERE token_hash = ?`,
		HashToken(token)).Scan(&sess.ID, &sess.UserID, &sess.CSRFToken, &expires)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	sess.ExpiresAt, _ = time.Parse(time.RFC3339, expires)
	if time.Now().After(sess.ExpiresAt) {
		_ = s.DeleteSession(ctx, token)
		return nil, nil
	}
	return &sess, nil
}

func (s *Service) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, HashToken(token))
	return err
}

func (s *Service) GetUser(ctx context.Context, id int64) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id, username FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *Service) UpdatePassword(ctx context.Context, userID int64, newPassword string) error {
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id = ?`,
		hash, userID)
	return err
}

func (s *Service) DeleteAllSessionsForUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Service) CleanExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}
