//go:build linux

package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"time"

	"github.com/dbpilot/dbpilot/internal/api"
	"github.com/dbpilot/dbpilot/internal/audit"
	"github.com/dbpilot/dbpilot/internal/auth"
	"github.com/dbpilot/dbpilot/internal/backup"
	"github.com/dbpilot/dbpilot/internal/config"
	"github.com/dbpilot/dbpilot/internal/instance"
	"github.com/dbpilot/dbpilot/internal/persistence"
	"github.com/dbpilot/dbpilot/internal/provider/postgres"
	"github.com/dbpilot/dbpilot/internal/runner"
	"github.com/dbpilot/dbpilot/internal/scheduler"
	"github.com/dbpilot/dbpilot/internal/secret"
	"github.com/dbpilot/dbpilot/internal/storage"
	"github.com/dbpilot/dbpilot/internal/task"
	"github.com/dbpilot/dbpilot/internal/version"
	webui "github.com/dbpilot/dbpilot/web"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println(version.String())
	case "run":
		runCmd(os.Args[2:])
	case "install":
		installCmd(os.Args[2:])
	case "status":
		statusCmd(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`dbpilot - PostgreSQL management console

Usage:
  dbpilot run [--config /etc/dbpilot/config.yaml]
  dbpilot install [--dry-run] [--config /etc/dbpilot/config.yaml]
  dbpilot status
  dbpilot version`)
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "/etc/dbpilot/config.yaml", "path to config file")
	_ = fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	setupLogger(cfg)

	if err := ensureDirs(cfg); err != nil {
		slog.Error("ensure dirs", "error", err)
		os.Exit(1)
	}

	db, err := persistence.Open(cfg.DBPath())
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		slog.Error("migrate", "error", err)
		os.Exit(1)
	}

	staticFS, _ := webui.DistFS()
	authSvc := auth.NewService(db)

	key, err := secret.LoadOrCreateKey(cfg.KeyPath())
	if err != nil {
		slog.Error("load secret key", "error", err)
		os.Exit(1)
	}
	secretStore := secret.NewSQLiteStore(db, key)
	instanceStore := instance.NewStore(db, secretStore)
	pgProvider := postgres.New(runner.ExecRunner{})
	targetStore := storage.NewTargetStore(db, secretStore)
	backupStore := backup.NewStore(db)
	policyStore := backup.NewPolicyStore(db)
	taskManager := task.NewManager(db)
	engine := backup.NewEngine(runner.ExecRunner{}, cfg.TmpDir(), instanceStore, targetStore, backupStore, pgProvider)
	auditService := audit.NewService(db)

	srv := api.NewServer(cfg, db, authSvc, staticFS)
	srv.SetInstanceStore(instanceStore)
	srv.SetPostgresProvider(pgProvider)
	srv.SetStorageTargets(targetStore)
	srv.SetBackupStore(backupStore)
	srv.SetPolicyStore(policyStore)
	srv.SetTaskManager(taskManager)
	srv.SetBackupEngine(engine)
	srv.SetAuditService(auditService)
	srv.RegisterTaskHandlers()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	taskManager.Start(ctx, 1)
	scheduler.New(policyStore, taskManager).Start(ctx)

	addr := cfg.ListenAddr()
	slog.Info("dbpilot starting", "version", version.String(), "addr", addr)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := httpSrv.ListenAndServe(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func setupLogger(cfg *config.Config) {
	level := slog.LevelInfo
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	var handler slog.Handler
	if cfg.Log.File != "" {
		f, err := os.OpenFile(cfg.Log.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
		} else {
			handler = slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})
		}
	} else {
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}
	slog.SetDefault(slog.New(handler))
}

func ensureDirs(cfg *config.Config) error {
	for _, dir := range []string{cfg.DataDir, cfg.TmpDir(), cfg.BackupsDir(), cfg.LogDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	if _, err := os.Stat(cfg.KeyPath()); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(cfg.KeyPath()), 0o700); err != nil {
				return err
			}
		}
	}
	return nil
}

const systemdUnit = `[Unit]
Description=DBPilot PostgreSQL management console
After=network.target postgresql.service

[Service]
Type=simple
User=dbpilot
Group=dbpilot
ExecStart=/usr/local/bin/dbpilot run --config /etc/dbpilot/config.yaml
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=full
ReadWritePaths=/var/lib/dbpilot /var/log/dbpilot

[Install]
WantedBy=multi-user.target
`

func installCmd(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "print actions without executing")
	configPath := fs.String("config", "/etc/dbpilot/config.yaml", "path to config file")
	_ = fs.Parse(args)

	steps := []string{
		"create user/group dbpilot",
		"create directories /etc/dbpilot /var/lib/dbpilot /var/log/dbpilot",
		"write default config /etc/dbpilot/config.yaml",
		"initialize SQLite database",
		"set ownership for dbpilot user",
		"install systemd unit /etc/systemd/system/dbpilot.service",
		"enable and start dbpilot.service",
	}

	if *dryRun {
		fmt.Println("Dry run - the following steps would be executed:")
		for i, s := range steps {
			fmt.Printf("  %d. %s\n", i+1, s)
		}
		return
	}

	run := func(name string, fn func() error) {
		if err := fn(); err != nil {
			fmt.Fprintf(os.Stderr, "FAILED: %s: %v\n", name, err)
			os.Exit(1)
		}
		fmt.Printf("OK: %s\n", name)
	}

	run("create user/group dbpilot", func() error {
		if _, err := user.Lookup("dbpilot"); err == nil {
			return nil
		}
		return exec.Command("useradd", "--system", "--no-create-home", "--shell", "/usr/sbin/nologin", "dbpilot").Run()
	})

	run("create directories", func() error {
		for _, d := range []string{"/var/lib/dbpilot", "/var/lib/dbpilot/tmp", "/var/lib/dbpilot/backups", "/var/log/dbpilot"} {
			if err := os.MkdirAll(d, 0o750); err != nil {
				return err
			}
		}
		// /etc/dbpilot holds only a non-secret config; the service user must be
		// able to traverse it (o+x) to read config.yaml.
		if err := os.MkdirAll("/etc/dbpilot", 0o755); err != nil {
			return err
		}
		return os.Chmod("/etc/dbpilot", 0o755)
	})

	run("write default config", func() error {
		if _, err := os.Stat(*configPath); err == nil {
			fmt.Println("  config already exists, keeping it")
			return nil
		}
		content := `server:
  addr: 127.0.0.1
  port: 8080
data_dir: /var/lib/dbpilot
log_dir: /var/log/dbpilot
log:
  level: info
`
		// 0644: the config carries no secrets (passwords live in the encrypted
		// SQLite store), and the dbpilot service user must be able to read it.
		return os.WriteFile(*configPath, []byte(content), 0o644)
	})

	run("initialize SQLite database", func() error {
		cfg, err := config.Load(*configPath)
		if err != nil {
			return err
		}
		db, err := persistence.Open(cfg.DBPath())
		if err != nil {
			return err
		}
		defer db.Close()
		if err := db.Migrate(context.Background()); err != nil {
			return err
		}
		return nil
	})

	run("set ownership for dbpilot user", func() error {
		uid, gid := lookupDuid()
		if uid < 0 {
			return nil
		}
		// the data dir was initialized as root above; give the service user
		// full ownership of everything it writes (db, secret.key, tmp, backups).
		for _, d := range []string{"/var/lib/dbpilot", "/var/lib/dbpilot/tmp", "/var/lib/dbpilot/backups", "/var/log/dbpilot"} {
			if err := chownRecursive(d, uid, gid); err != nil {
				return err
			}
		}
		return nil
	})

	run("install systemd unit", func() error {
		return os.WriteFile("/etc/systemd/system/dbpilot.service", []byte(systemdUnit), 0o644)
	})

	run("enable and start dbpilot.service", func() error {
		if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
			return err
		}
		if err := exec.Command("systemctl", "enable", "dbpilot.service").Run(); err != nil {
			return err
		}
		return exec.Command("systemctl", "start", "dbpilot.service").Run()
	})

	fmt.Println("\nDBPilot installed. Open http://127.0.0.1:8080 to complete initial setup.")
}

func chownRecursive(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(p, uid, gid)
	})
}

func lookupDuid() (int, int) {
	u, err := user.Lookup("dbpilot")
	if err != nil {
		return -1, -1
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return uid, gid
}

func statusCmd(args []string) {
	cfg, err := config.Load("/etc/dbpilot/config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Version:   %s\n", version.String())
	fmt.Printf("Data dir:  %s\n", cfg.DataDir)
	fmt.Printf("Log dir:   %s\n", cfg.LogDir)
	fmt.Printf("Listen:    %s\n", cfg.ListenAddr())

	_, err = os.Stat(cfg.DBPath())
	if err != nil {
		fmt.Println("Database:  not initialized")
		os.Exit(1)
	}
	fmt.Println("Database:  initialized")

	out, err := exec.Command("systemctl", "is-active", "dbpilot.service").Output()
	if err == nil {
		fmt.Printf("Service:   %s", string(out))
	} else {
		fmt.Println("Service:   unknown (not running via systemd)")
	}
}
