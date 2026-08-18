package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dbpilot/dbpilot/internal/instance"
	"github.com/dbpilot/dbpilot/internal/provider"
	"github.com/dbpilot/dbpilot/internal/provider/postgres"
	"github.com/dbpilot/dbpilot/internal/runner"
	"github.com/dbpilot/dbpilot/internal/storage"
	"github.com/dbpilot/dbpilot/internal/storageregistry"
)

var versionRe = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

func parseVersion(s string) string {
	return versionRe.FindString(s)
}

// checkVersionCompat refuses to run when the dump tool is from an older major
// version than the server (which pg_dump cannot handle). A newer tool against
// an older server is allowed (downgrade dump) but logged as a warning above.
func checkVersionCompat(pgDumpVersion, serverVersion string, globals bool) error {
	if pgDumpVersion == "" || serverVersion == "" {
		return nil
	}
	dumpMajor, _, errDump := parseMajorMinor(pgDumpVersion)
	serverMajor, _, errServer := parseMajorMinor(serverVersion)
	if errDump != nil || errServer != nil {
		return nil
	}
	tool := "pg_dump"
	if globals {
		tool = "pg_dumpall"
	}
	if dumpMajor < serverMajor {
		return fmt.Errorf("%s version %d is older than server version %d; install a matching PostgreSQL client", tool, dumpMajor, serverMajor)
	}
	return nil
}

func parseMajorMinor(v string) (int, int, error) {
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid version %q", v)
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("invalid version %q", v)
	}
	return major, minor, nil
}

type Manifest struct {
	Database        string `json:"database"`
	BackupType      string `json:"backup_type"`
	Format          string `json:"format"`
	PostgresVersion string `json:"postgres_version"`
	PgDumpVersion   string `json:"pg_dump_version"`
	SizeBytes       int64  `json:"size_bytes"`
	SHA256          string `json:"sha256"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at"`
}

type Request struct {
	InstanceID int64
	Database   string
	Globals    bool
	StorageIDs []int64
	CreatedBy  string
}

// TaskPayload is the JSON payload carried by a "backup" task.
type TaskPayload struct {
	InstanceID     int64   `json:"instance_id"`
	Database       string  `json:"database"`
	Globals        bool    `json:"globals"`
	StorageIDs     []int64 `json:"storage_ids"`
	CreatedBy      string  `json:"created_by"`
	RetentionCount int     `json:"retention_count"`
}

type TargetResult struct {
	StorageID int64
	Status    string
	Error     string
}

type Result struct {
	Record  *Record
	Targets []TargetResult
}

type Engine struct {
	runner    runner.Runner
	tmpDir    string
	instances *instance.Store
	targets   *storage.TargetStore
	store     *Store
	pg        *postgres.Postgres
}

func NewEngine(r runner.Runner, tmpDir string, instances *instance.Store, targets *storage.TargetStore, store *Store, pg *postgres.Postgres) *Engine {
	if r == nil {
		r = runner.ExecRunner{}
	}
	return &Engine{runner: r, tmpDir: tmpDir, instances: instances, targets: targets, store: store, pg: pg}
}

// Backup runs a logical backup and uploads it to every requested storage
// target. It returns a Result with the persisted Record and per-target status.
func (e *Engine) Backup(ctx context.Context, req Request, logf func(format string, args ...any)) (*Result, error) {
	params, err := e.instances.ConnParams(ctx, req.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("resolve instance: %w", err)
	}

	backupType := TypeDatabase
	format := "custom"
	label := req.Database
	if req.Globals {
		backupType = TypeGlobals
		format = "plain"
		label = "globals"
	}

	logf("starting %s backup for %s", backupType, label)

	pgDumpVersion := e.pgDumpVersion(ctx)
	postgresVersion := e.serverVersion(ctx, params)

	// version compatibility check (§26): refuse silently-incompatible tools
	if err := checkVersionCompat(pgDumpVersion, postgresVersion, req.Globals); err != nil {
		return nil, err
	}
	if pgDumpVersion != "" && postgresVersion != "" && pgDumpVersion != postgresVersion {
		logf("warning: pg_dump version %s differs from server version %s", pgDumpVersion, postgresVersion)
	}

	rec := &Record{
		InstanceID:      req.InstanceID,
		Database:        req.Database,
		BackupType:      backupType,
		Format:          format,
		Status:          "running",
		PgDumpVersion:   pgDumpVersion,
		PostgresVersion: postgresVersion,
		CreatedBy:       req.CreatedBy,
	}
	rec, err = e.store.Create(ctx, rec)
	if err != nil {
		return nil, fmt.Errorf("create backup record: %w", err)
	}

	ts := time.Now().UTC().Format("20060102-150405")
	objectDir := fmt.Sprintf("backups/%d/%s/%s", req.InstanceID, label, ts)
	objectName := req.Database + ".dump"
	if req.Globals {
		objectName = "globals.sql"
	}
	rec.ManifestPath = objectDir + "/manifest.json"

	if err := os.MkdirAll(e.tmpDir, 0o700); err != nil {
		return nil, fmt.Errorf("create tmp dir: %w", err)
	}
	tmpFile, err := os.CreateTemp(e.tmpDir, "backup-*.dump")
	if err != nil {
		return nil, fmt.Errorf("create tmp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// run pg_dump / pg_dumpall
	started := time.Now()
	if err := e.dump(ctx, params, req, tmpPath); err != nil {
		rec.Status = "failed"
		rec.ErrorCode = "pg_dump_failed"
		rec.ErrorMessage = err.Error()
		rec.DurationMs = time.Since(started).Milliseconds()
		_ = e.store.Update(ctx, rec)
		return &Result{Record: rec}, fmt.Errorf("pg_dump failed: %w", err)
	}

	// compute size + sha256
	finalSize, sha, err := e.hashFile(tmpPath)
	if err != nil {
		rec.Status = "failed"
		rec.ErrorCode = "hash_failed"
		rec.ErrorMessage = err.Error()
		rec.DurationMs = time.Since(started).Milliseconds()
		_ = e.store.Update(ctx, rec)
		return &Result{Record: rec}, fmt.Errorf("hash failed: %w", err)
	}

	manifest := Manifest{
		Database:        req.Database,
		BackupType:      backupType,
		Format:          format,
		PostgresVersion: postgresVersion,
		PgDumpVersion:   pgDumpVersion,
		SizeBytes:       finalSize,
		SHA256:          sha,
		StartedAt:       started.UTC().Format(time.RFC3339),
		FinishedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	manifestFile := filepath.Join(e.tmpDir, fmt.Sprintf("manifest-%d.json", rec.ID))
	if err := os.WriteFile(manifestFile, manifestData, 0o600); err != nil {
		return nil, err
	}
	defer os.Remove(manifestFile)

	rec.OriginalSize = finalSize
	rec.FinalSize = finalSize
	rec.SHA256 = sha

	// upload to targets
	targetResults := e.upload(ctx, rec, req.StorageIDs, tmpPath, objectDir, objectName, manifestFile, logf)

	successCount := 0
	for _, tr := range targetResults {
		if tr.Status == "success" {
			successCount++
		}
	}
	rec.DurationMs = time.Since(started).Milliseconds()
	switch {
	case len(targetResults) == 0:
		rec.Status = "failed"
		rec.ErrorCode = "no_storage_targets"
		rec.ErrorMessage = "no storage targets configured"
	case successCount == len(targetResults):
		rec.Status = "success"
	case successCount > 0:
		rec.Status = "partial_success"
	default:
		rec.Status = "failed"
		rec.ErrorCode = "all_storage_targets_failed"
		rec.ErrorMessage = "all storage targets failed"
	}
	_ = e.store.Update(ctx, rec)

	return &Result{Record: rec, Targets: targetResults}, nil
}

func (e *Engine) dump(ctx context.Context, params provider.ConnParams, req Request, outputPath string) error {
	env := map[string]string{
		"PGPASSWORD": params.Password,
		"PGUSER":     params.Username,
		"PGSSLMODE":  params.SSLMode,
	}
	if params.UnixSocket != "" {
		env["PGHOST"] = params.UnixSocket
	} else {
		env["PGHOST"] = params.Host
		env["PGPORT"] = strconv.Itoa(params.Port)
	}

	var args []string
	if req.Globals {
		args = []string{"--globals-only", "--file=" + outputPath}
	} else {
		args = []string{"--format=custom", "--file=" + outputPath, req.Database}
	}

	bin := "pg_dump"
	if req.Globals {
		bin = "pg_dumpall"
	}
	_, stderr, err := e.runner.Run(ctx, bin, args, env)
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(stderr)))
	}
	return nil
}

func (e *Engine) pgDumpVersion(ctx context.Context) string {
	stdout, _, err := e.runner.Run(ctx, "pg_dump", []string{"--version"}, nil)
	if err != nil {
		return ""
	}
	return parseVersion(string(stdout))
}

func (e *Engine) serverVersion(ctx context.Context, params provider.ConnParams) string {
	if e.pg == nil {
		return ""
	}
	return parseVersion(e.pg.ServerVersion(ctx, params))
}

func (e *Engine) hashFile(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func (e *Engine) upload(ctx context.Context, rec *Record, storageIDs []int64, tmpPath, objectDir, objectName, manifestFile string, logf func(string, ...any)) []TargetResult {
	results := make([]TargetResult, 0, len(storageIDs))
	for _, sid := range storageIDs {
		tr := TargetResult{StorageID: sid}
		target, err := e.targets.Get(ctx, sid)
		if err != nil {
			tr.Status = "failed"
			tr.Error = "target not found: " + err.Error()
			results = append(results, tr)
			continue
		}
		password, _ := e.targets.Password(ctx, sid)
		st, err := storageregistry.Build(ctx, target, password)
		if err != nil {
			tr.Status = "failed"
			tr.Error = err.Error()
			results = append(results, tr)
			continue
		}

		if err := e.putFile(ctx, st, tmpPath, objectDir+"/"+objectName, logf); err != nil {
			tr.Status = "failed"
			tr.Error = err.Error()
			results = append(results, tr)
			continue
		}
		if err := e.putFile(ctx, st, manifestFile, objectDir+"/manifest.json", logf); err != nil {
			tr.Status = "failed"
			tr.Error = "manifest upload failed: " + err.Error()
			results = append(results, tr)
			continue
		}

		info, _ := os.Stat(tmpPath)
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		entry := &TargetEntry{
			BackupID:   rec.ID,
			StorageID:  sid,
			Status:     "success",
			ObjectPath: objectDir,
			Size:       size,
		}
		if err := e.store.AddTargetEntry(ctx, entry); err != nil {
			tr.Status = "failed"
			tr.Error = "record target entry: " + err.Error()
			results = append(results, tr)
			continue
		}
		tr.Status = "success"
		results = append(results, tr)
	}
	return results
}

func (e *Engine) putFile(ctx context.Context, st storage.Storage, localPath, remotePath string, logf func(string, ...any)) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := st.Put(ctx, remotePath, f); err != nil {
		return err
	}
	return nil
}
