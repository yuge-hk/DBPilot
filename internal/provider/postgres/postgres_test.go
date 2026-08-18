package postgres

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/dbpilot/dbpilot/internal/provider"
)

func TestBuildDSN(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		dsn := BuildDSN(provider.ConnParams{Host: "192.168.0.20", Port: 5432, Username: "postgres", Password: "secret", SSLMode: "disable"})
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("parse dsn: %v", err)
		}
		if u.Host != "192.168.0.20:5432" {
			t.Errorf("host = %q", u.Host)
		}
		if u.User.Username() != "postgres" {
			t.Errorf("user = %q", u.User.Username())
		}
		pw, _ := u.User.Password()
		if pw != "secret" {
			t.Errorf("password = %q", pw)
		}
		if u.Query().Get("sslmode") != "disable" {
			t.Errorf("sslmode = %q", u.Query().Get("sslmode"))
		}
	})

	t.Run("special characters are escaped", func(t *testing.T) {
		dsn := BuildDSN(provider.ConnParams{Host: "localhost", Port: 5432, Username: "u@ser", Password: "p@ss w/rd?#", SSLMode: "require"})
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("parse dsn: %v", err)
		}
		pw, _ := u.User.Password()
		if pw != "p@ss w/rd?#" {
			t.Errorf("password roundtrip failed: %q", pw)
		}
		if u.User.Username() != "u@ser" {
			t.Errorf("username roundtrip failed: %q", u.User.Username())
		}
	})

	t.Run("unix socket", func(t *testing.T) {
		dsn := BuildDSN(provider.ConnParams{UnixSocket: "/var/run/postgresql", Username: "postgres", Password: "x", SSLMode: "disable"})
		u, _ := url.Parse(dsn)
		if u.Query().Get("host") != "/var/run/postgresql" {
			t.Errorf("socket host = %q", u.Query().Get("host"))
		}
	})
}

type fakeRunner struct {
	fn func(ctx context.Context, name string, args []string, env map[string]string) ([]byte, []byte, error)
}

func (f *fakeRunner) Run(ctx context.Context, name string, args []string, env map[string]string) ([]byte, []byte, error) {
	return f.fn(ctx, name, args, env)
}

func TestDetectTools(t *testing.T) {
	versions := map[string]string{
		"pg_dump":    "pg_dump (PostgreSQL) 16.2",
		"pg_restore": "pg_restore (PostgreSQL) 16.2",
		"pg_dumpall": "pg_dumpall (PostgreSQL) 16.2",
		"psql":       "psql (PostgreSQL) 16.2",
	}
	r := &fakeRunner{fn: func(ctx context.Context, name string, args []string, env map[string]string) ([]byte, []byte, error) {
		if v, ok := versions[name]; ok {
			return []byte(v), nil, nil
		}
		return nil, nil, errors.New("not found")
	}}
	p := &Postgres{
		runner: r,
		lookPath: func(name string) (string, bool) {
			if name == "pg_dumpall" {
				return "", false
			}
			return "/usr/bin/" + name, true
		},
	}

	tools := p.DetectTools(context.Background())
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}
	byName := map[string]provider.ToolInfo{}
	for _, ti := range tools {
		byName[ti.Name] = ti
	}
	if !byName["pg_dump"].Found || byName["pg_dump"].Version != "pg_dump (PostgreSQL) 16.2" {
		t.Errorf("pg_dump detection wrong: %+v", byName["pg_dump"])
	}
	if byName["pg_dumpall"].Found {
		t.Errorf("pg_dumpall should be missing")
	}
}
