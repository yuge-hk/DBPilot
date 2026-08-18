package secret

import (
	"context"
	"errors"
)

// Store is the unified interface for persisting sensitive values
// (PostgreSQL passwords, WebDAV passwords, etc.). Callers must never
// store secrets directly in business tables.
type Store interface {
	Set(ctx context.Context, name, value string) error
	Get(ctx context.Context, name string) (string, error)
	Delete(ctx context.Context, name string) error
}

var ErrNotFound = errors.New("secret not found")
