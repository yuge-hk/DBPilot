package provider

import (
	"context"
	"errors"
)

// ConnParams describes how to reach a PostgreSQL server. Password is kept
// separate from the persisted instance and resolved via the secret store.
type ConnParams struct {
	Host       string
	Port       int
	Username   string
	Password   string
	SSLMode    string
	UnixSocket string
}

type TestResult struct {
	OK            bool   `json:"ok"`
	Version       string `json:"version"`
	ServerAddress string `json:"server_address"`
	CurrentUser   string `json:"current_user"`
	DatabaseCount int    `json:"database_count"`
	Error         string `json:"error,omitempty"`
}

type ToolInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Version string `json:"version"`
	Found   bool   `json:"found"`
}

type Database struct {
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Encoding  string `json:"encoding"`
	Collation string `json:"collation"`
	SizeBytes int64  `json:"size_bytes"`
	ConnCount int    `json:"conn_count"`
}

type Account struct {
	Name       string `json:"name"`
	CanLogin   bool   `json:"can_login"`
	Superuser  bool   `json:"superuser"`
	CreateDB   bool   `json:"create_db"`
	CreateRole bool   `json:"create_role"`
	ConnLimit  int    `json:"conn_limit"`
	ValidUntil string `json:"valid_until"`
	OwnedDBs   int    `json:"owned_dbs"`
	SystemRole bool   `json:"system_role"`
}

type CreateAccountRequest struct {
	Name       string
	Password   string
	Login      bool
	Superuser  bool
	CreateDB   bool
	CreateRole bool
	ConnLimit  int
	ValidUntil string
}

type UpdateAccountRequest struct {
	Name        string
	NewPassword *string
	Login       *bool
	Superuser   *bool
	CreateDB    *bool
	CreateRole  *bool
	ConnLimit   *int
	ValidUntil  *string
}

var (
	ErrAccountNotFound   = errors.New("account not found")
	ErrDatabaseNotFound  = errors.New("database not found")
	ErrAccountProtected  = errors.New("account is protected")
	ErrAccountHasDeps    = errors.New("account has dependencies")
	ErrDatabaseProtected = errors.New("database is protected")
)

type CreateDatabaseRequest struct {
	Database    string
	AccountMode string // "create" | "existing"
	Account     string
	Password    string
	Permission  string // "owner" | "read_write" | "read_only"
}

type CreateResult struct {
	Database   string `json:"database"`
	Account    string `json:"account"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Permission string `json:"permission"`
}

// Provider abstracts database-specific operations. V0.1 ships a PostgreSQL
// implementation; a future MySQL provider can implement the same interface.
type Provider interface {
	TestConnection(ctx context.Context, p ConnParams) (*TestResult, error)
	ListDatabases(ctx context.Context, p ConnParams) ([]Database, error)
	ListAccounts(ctx context.Context, p ConnParams) ([]Account, error)
	CreateAccount(ctx context.Context, p ConnParams, req CreateAccountRequest) error
	UpdateAccount(ctx context.Context, p ConnParams, req UpdateAccountRequest) error
	DeleteAccount(ctx context.Context, p ConnParams, name string) error
	DeleteDatabase(ctx context.Context, p ConnParams, name string) error
	CreateDatabaseWithAccount(ctx context.Context, p ConnParams, req CreateDatabaseRequest) (*CreateResult, error)
}
