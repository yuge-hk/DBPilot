package secret

import (
	"context"
	"database/sql"
	"errors"

	"github.com/dbpilot/dbpilot/internal/persistence"
)

// SQLiteStore stores secrets encrypted with a master key.
type SQLiteStore struct {
	db  *persistence.DB
	key MasterKey
}

func NewSQLiteStore(db *persistence.DB, key MasterKey) *SQLiteStore {
	return &SQLiteStore{db: db, key: key}
}

func (s *SQLiteStore) Set(ctx context.Context, name, value string) error {
	enc, err := s.key.Encrypt(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO secrets(name, value, updated_at) VALUES(?, ?, strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		ON CONFLICT(name) DO UPDATE SET value = excluded.value, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ','now')`,
		name, enc)
	return err
}

func (s *SQLiteStore) Get(ctx context.Context, name string) (string, error) {
	var enc string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM secrets WHERE name = ?`, name).Scan(&enc)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return s.key.Decrypt(enc)
}

func (s *SQLiteStore) Delete(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM secrets WHERE name = ?`, name)
	return err
}
