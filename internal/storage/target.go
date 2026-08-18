package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dbpilot/dbpilot/internal/persistence"
	"github.com/dbpilot/dbpilot/internal/secret"
)

type LocalConfig struct {
	Path string `json:"path"`
}

type WebDAVConfig struct {
	Endpoint  string `json:"endpoint"`
	Username  string `json:"username"`
	BasePath  string `json:"base_path"`
	TLSVerify bool   `json:"tls_verify"`
	Timeout   int    `json:"timeout"`
}

type Target struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Config    json.RawMessage `json:"config"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

var ErrTargetNotFound = errors.New("storage target not found")

type TargetStore struct {
	db     *persistence.DB
	secret secret.Store
}

func NewTargetStore(db *persistence.DB, s secret.Store) *TargetStore {
	return &TargetStore{db: db, secret: s}
}

func secretRef(id int64) string {
	return fmt.Sprintf("storage:%d:password", id)
}

func (s *TargetStore) Create(ctx context.Context, name, typ string, config any, password string) (*Target, error) {
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO storage_targets(name, type, config) VALUES(?, ?, ?)`, name, typ, string(raw))
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if password != "" {
		if err := s.secret.Set(ctx, secretRef(id), password); err != nil {
			return nil, err
		}
	}
	return &Target{ID: id, Name: name, Type: typ, Config: raw}, nil
}

func (s *TargetStore) List(ctx context.Context) ([]Target, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, type, config, created_at, updated_at FROM storage_targets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Target
	for rows.Next() {
		var t Target
		var raw string
		if err := rows.Scan(&t.ID, &t.Name, &t.Type, &raw, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Config = json.RawMessage(raw)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *TargetStore) Get(ctx context.Context, id int64) (*Target, error) {
	var t Target
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT id, name, type, config, created_at, updated_at FROM storage_targets WHERE id = ?`, id).
		Scan(&t.ID, &t.Name, &t.Type, &raw, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTargetNotFound
		}
		return nil, err
	}
	t.Config = json.RawMessage(raw)
	return &t, nil
}

func (s *TargetStore) Update(ctx context.Context, id int64, name string, config any, password *string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE storage_targets SET name = ?, config = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id = ?`,
		name, string(raw), id); err != nil {
		return err
	}
	if password != nil && *password != "" {
		if err := s.secret.Set(ctx, secretRef(id), *password); err != nil {
			return err
		}
	}
	return nil
}

func (s *TargetStore) Delete(ctx context.Context, id int64) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM storage_targets WHERE id = ?`, id); err != nil {
		return err
	}
	return s.secret.Delete(ctx, secretRef(id))
}

func (s *TargetStore) Password(ctx context.Context, id int64) (string, error) {
	return s.secret.Get(ctx, secretRef(id))
}
