package postgres

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/dbpilot/dbpilot/internal/provider"
	"github.com/dbpilot/dbpilot/internal/runner"
)

type Postgres struct {
	runner   runner.Runner
	lookPath func(string) (string, bool)
}

func New(r runner.Runner) *Postgres {
	if r == nil {
		r = runner.ExecRunner{}
	}
	return &Postgres{runner: r, lookPath: runner.LookPath}
}

// BuildDSN constructs a safe DSN from connection parameters, correctly
// escaping the password and username so special characters cannot break
// the connection string or inject parameters.
func BuildDSN(p provider.ConnParams) string {
	return buildDSN(p, "postgres")
}

func buildDSN(p provider.ConnParams, database string) string {
	u := &url.URL{Scheme: "postgres", Path: "/" + database}
	u.User = url.UserPassword(p.Username, p.Password)
	q := url.Values{}
	if p.UnixSocket != "" {
		q.Set("host", p.UnixSocket)
	} else {
		host := p.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := p.Port
		if port == 0 {
			port = 5432
		}
		u.Host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	mode := p.SSLMode
	if mode == "" {
		mode = "prefer"
	}
	q.Set("sslmode", mode)
	q.Set("connect_timeout", "10")
	u.RawQuery = q.Encode()
	return u.String()
}

func (p *Postgres) TestConnection(ctx context.Context, params provider.ConnParams) (*provider.TestResult, error) {
	res := &provider.TestResult{}
	dsn := BuildDSN(params)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, nil
	}
	defer conn.Close(ctx)

	var version, serverAddr, currentUser string
	if err := conn.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, nil
	}
	if err := conn.QueryRow(ctx, "SELECT inet_server_addr()::text").Scan(&serverAddr); err != nil {
		serverAddr = ""
	}
	if err := conn.QueryRow(ctx, "SELECT current_user").Scan(&currentUser); err != nil {
		currentUser = ""
	}
	var dbCount int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_database WHERE datallowconn").Scan(&dbCount); err != nil {
		dbCount = 0
	}

	res.OK = true
	res.Version = version
	res.ServerAddress = serverAddr
	res.CurrentUser = currentUser
	res.DatabaseCount = dbCount
	return res, nil
}

// ServerVersion returns the server's version string (e.g. "16.2").
func (p *Postgres) ServerVersion(ctx context.Context, params provider.ConnParams) string {
	conn, err := p.connect(ctx, params)
	if err != nil {
		return ""
	}
	defer conn.Close(ctx)
	var v string
	if err := conn.QueryRow(ctx, "SHOW server_version").Scan(&v); err != nil {
		return ""
	}
	return v
}

// ServerConfig returns the server's listen_addresses and config file paths.
type ServerConfig struct {
	ListenAddresses string
	HbaFile         string
	ConfigFile      string
	DataDirectory   string
}

func (p *Postgres) ServerConfig(ctx context.Context, params provider.ConnParams) (*ServerConfig, error) {
	conn, err := p.connect(ctx, params)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	cfg := &ServerConfig{}
	_ = conn.QueryRow(ctx, "SHOW listen_addresses").Scan(&cfg.ListenAddresses)
	_ = conn.QueryRow(ctx, "SHOW hba_file").Scan(&cfg.HbaFile)
	_ = conn.QueryRow(ctx, "SHOW config_file").Scan(&cfg.ConfigFile)
	_ = conn.QueryRow(ctx, "SHOW data_directory").Scan(&cfg.DataDirectory)
	return cfg, nil
}

var toolNames = []string{"pg_dump", "pg_restore", "pg_dumpall", "psql"}

// DetectTools locates and versions the PostgreSQL client tools used by the
// backup/restore engine. It never downloads binaries.
func (p *Postgres) DetectTools(ctx context.Context) []provider.ToolInfo {
	out := make([]provider.ToolInfo, 0, len(toolNames))
	for _, name := range toolNames {
		info := provider.ToolInfo{Name: name}
		path, ok := p.lookPath(name)
		if !ok {
			info.Found = false
			out = append(out, info)
			continue
		}
		info.Found = true
		info.Path = path
		stdout, _, err := p.runner.Run(ctx, name, []string{"--version"}, nil)
		if err == nil {
			info.Version = strings.TrimSpace(string(stdout))
		}
		out = append(out, info)
	}
	return out
}
