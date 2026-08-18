package instance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dbpilot/dbpilot/internal/persistence"
	"github.com/dbpilot/dbpilot/internal/provider"
	"github.com/dbpilot/dbpilot/internal/secret"
)

type Instance struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	SSLMode     string `json:"ssl_mode"`
	UnixSocket  string `json:"unix_socket"`
	SystemdUnit string `json:"systemd_unit"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

var ErrNotFound = errors.New("instance not found")

type Store struct {
	db     *persistence.DB
	secret secret.Store
}

func NewStore(db *persistence.DB, s secret.Store) *Store {
	return &Store{db: db, secret: s}
}

func secretRef(id int64) string {
	return fmt.Sprintf("instance:%d:password", id)
}

func (s *Store) Create(ctx context.Context, inst *Instance, password string) error {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO instances(name, host, port, username, secret_ref, ssl_mode, unix_socket, systemd_unit)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		inst.Name, inst.Host, inst.Port, inst.Username, "", inst.SSLMode, inst.UnixSocket, inst.SystemdUnit)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	inst.ID = id
	if err := s.secret.Set(ctx, secretRef(id), password); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE instances SET secret_ref = ? WHERE id = ?`, secretRef(id), id)
	return err
}

func (s *Store) List(ctx context.Context) ([]Instance, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, host, port, username, ssl_mode, unix_socket, systemd_unit, created_at, updated_at
		FROM instances ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Instance
	for rows.Next() {
		var i Instance
		if err := rows.Scan(&i.ID, &i.Name, &i.Host, &i.Port, &i.Username, &i.SSLMode, &i.UnixSocket, &i.SystemdUnit, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (*Instance, error) {
	var i Instance
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, host, port, username, ssl_mode, unix_socket, systemd_unit, created_at, updated_at
		FROM instances WHERE id = ?`, id).
		Scan(&i.ID, &i.Name, &i.Host, &i.Port, &i.Username, &i.SSLMode, &i.UnixSocket, &i.SystemdUnit, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &i, nil
}

func (s *Store) Update(ctx context.Context, inst *Instance, newPassword *string) error {
	if _, err := s.Get(ctx, inst.ID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE instances SET name=?, host=?, port=?, username=?, ssl_mode=?, unix_socket=?, systemd_unit=?,
		updated_at = strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id = ?`,
		inst.Name, inst.Host, inst.Port, inst.Username, inst.SSLMode, inst.UnixSocket, inst.SystemdUnit, inst.ID)
	if err != nil {
		return err
	}
	if newPassword != nil && *newPassword != "" {
		if err := s.secret.Set(ctx, secretRef(inst.ID), *newPassword); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM instances WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return s.secret.Delete(ctx, secretRef(id))
}

func (s *Store) Password(ctx context.Context, id int64) (string, error) {
	return s.secret.Get(ctx, secretRef(id))
}

func (s *Store) ConnParams(ctx context.Context, id int64) (provider.ConnParams, error) {
	inst, err := s.Get(ctx, id)
	if err != nil {
		return provider.ConnParams{}, err
	}
	password, err := s.Password(ctx, id)
	if err != nil {
		return provider.ConnParams{}, err
	}
	return provider.ConnParams{
		Host:       inst.Host,
		Port:       inst.Port,
		Username:   inst.Username,
		Password:   password,
		SSLMode:    inst.SSLMode,
		UnixSocket: inst.UnixSocket,
	}, nil
}
