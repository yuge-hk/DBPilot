package storage

import (
	"context"
	"io"
	"time"
)

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// Storage is the abstraction over a backup destination. V0.1 implements
// local/mounted directories and WebDAV; the interface leaves room for an
// S3Storage without touching callers.
type Storage interface {
	Test(ctx context.Context) error
	Put(ctx context.Context, path string, reader io.Reader) error
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	Stat(ctx context.Context, path string) (*ObjectInfo, error)
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

type Type string

const (
	TypeLocal  Type = "local"
	TypeWebDAV Type = "webdav"
)
