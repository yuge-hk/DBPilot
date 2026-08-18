package storageregistry

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dbpilot/dbpilot/internal/storage"
	"github.com/dbpilot/dbpilot/internal/storage/local"
	"github.com/dbpilot/dbpilot/internal/storage/webdav"
)

// Build constructs a concrete Storage from a target definition and its
// resolved secret (if any).
func Build(ctx context.Context, t *storage.Target, password string) (storage.Storage, error) {
	switch t.Type {
	case string(storage.TypeLocal):
		var cfg storage.LocalConfig
		if err := json.Unmarshal(t.Config, &cfg); err != nil {
			return nil, fmt.Errorf("parse local config: %w", err)
		}
		if cfg.Path == "" {
			return nil, fmt.Errorf("local storage path is empty")
		}
		return local.New(cfg.Path), nil
	case string(storage.TypeWebDAV):
		var cfg storage.WebDAVConfig
		if err := json.Unmarshal(t.Config, &cfg); err != nil {
			return nil, fmt.Errorf("parse webdav config: %w", err)
		}
		return webdav.New(cfg, password)
	default:
		return nil, fmt.Errorf("unknown storage type: %s", t.Type)
	}
}
