package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dbpilot/dbpilot/internal/storage"
)

// Local stores objects under a base directory. It supports any mounted
// filesystem (NFS, SMB/CIFS, ZFS, CephFS, SSHFS) transparently.
type Local struct {
	base string
}

func New(base string) *Local {
	return &Local{base: filepath.Clean(base)}
}

// resolve maps an object path onto the base directory and rejects any path
// that would escape the base directory (path traversal defense).
func (l *Local) resolve(path string) (string, error) {
	if path == "" {
		return "", errors.New("empty path")
	}
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) {
		return "", errors.New("absolute paths are not allowed")
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path traversal is not allowed")
	}
	full := filepath.Join(l.base, clean)
	rel, err := filepath.Rel(l.base, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes base directory")
	}
	return full, nil
}

func (l *Local) Test(ctx context.Context) error {
	// create, write, read, remove a temp file
	probe, err := l.resolve(".dbpilot_test")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.base, 0o750); err != nil {
		return fmt.Errorf("create base dir: %w", err)
	}
	if err := os.WriteFile(probe, []byte("dbpilot"), 0o600); err != nil {
		return fmt.Errorf("write test file: %w", err)
	}
	data, err := os.ReadFile(probe)
	if err != nil {
		return fmt.Errorf("read test file: %w", err)
	}
	if string(data) != "dbpilot" {
		return errors.New("test file content mismatch")
	}
	if err := os.Remove(probe); err != nil {
		return fmt.Errorf("remove test file: %w", err)
	}
	return nil
}

func (l *Local) Put(ctx context.Context, path string, reader io.Reader) error {
	full, err := l.resolve(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return err
	}
	tmp := full + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, reader); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, full); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func (l *Local) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	full, err := l.resolve(path)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

func (l *Local) Delete(ctx context.Context, path string) error {
	full, err := l.resolve(path)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (l *Local) Stat(ctx context.Context, path string) (*storage.ObjectInfo, error) {
	full, err := l.resolve(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	return &storage.ObjectInfo{Path: path, Size: info.Size(), ModTime: info.ModTime()}, nil
}

func (l *Local) List(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	dir, err := l.resolve(prefix)
	if err != nil {
		return nil, err
	}
	var out []storage.ObjectInfo
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(l.base, p)
		if err != nil {
			return err
		}
		out = append(out, storage.ObjectInfo{Path: rel, Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
