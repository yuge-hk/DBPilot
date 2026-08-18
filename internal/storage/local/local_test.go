package local

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalPutOpenDelete(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	l := New(dir)

	if err := l.Test(ctx); err != nil {
		t.Fatalf("test: %v", err)
	}

	data := []byte("hello backup world")
	if err := l.Put(ctx, "backups/1/db/foo.dump", bytes.NewReader(data)); err != nil {
		t.Fatalf("put: %v", err)
	}

	r, err := l.Open(ctx, "backups/1/db/foo.dump")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if !bytes.Equal(got, data) {
		t.Errorf("content mismatch: %q", got)
	}

	info, err := l.Stat(ctx, "backups/1/db/foo.dump")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size != int64(len(data)) {
		t.Errorf("size = %d", info.Size)
	}

	objs, err := l.List(ctx, "backups/1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}

	if err := l.Delete(ctx, "backups/1/db/foo.dump"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := l.Stat(ctx, "backups/1/db/foo.dump"); err == nil {
		t.Error("expected stat to fail after delete")
	}
}

func TestLocalPathTraversal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	l := New(dir)

	// outside file must not be written
	outside := filepath.Join(t.TempDir(), "evil.txt")
	if err := l.Put(ctx, "../evil.txt", strings.NewReader("x")); err == nil {
		t.Error("expected traversal to be rejected")
	}
	if err := l.Put(ctx, "a/../../evil.txt", strings.NewReader("x")); err == nil {
		t.Error("expected traversal to be rejected")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Error("traversal wrote outside base dir")
	}
}
