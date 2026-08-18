package backup

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/dbpilot/dbpilot/internal/storage"
)

type memWebDAV struct {
	mu      sync.Mutex
	objects map[string][]byte
	fail    bool
}

func (m *memWebDAV) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch r.Method {
		case "PUT":
			b, _ := io.ReadAll(r.Body)
			m.mu.Lock()
			m.objects[r.URL.Path] = b
			m.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case "GET":
			m.mu.Lock()
			b, ok := m.objects[r.URL.Path]
			m.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write(b)
		case "DELETE":
			m.mu.Lock()
			delete(m.objects, r.URL.Path)
			m.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
}

func addWebDAVTarget(t *testing.T, engine *Engine, endpoint string) int64 {
	t.Helper()
	target, err := engine.targets.Create(context.Background(), "webdav", "webdav", storage.WebDAVConfig{
		Endpoint: endpoint, Username: "", BasePath: "dbpilot", TLSVerify: true, Timeout: 10,
	}, "")
	if err != nil {
		t.Fatalf("create webdav target: %v", err)
	}
	return target.ID
}

// Flow F: same backup -> local success + webdav success
func TestBackupLocalAndWebDAV(t *testing.T) {
	m := &memWebDAV{objects: map[string][]byte{}}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	content := []byte("FLOW_F_DATA")
	engine, instStore, _, backupStore := newTestEngine(t, &fakeRunner{version: "16.2", dumpContent: content}, []string{t.TempDir()})
	localID, _ := engine.targets.List(context.Background())
	webdavID := addWebDAVTarget(t, engine, srv.URL)

	insts, _ := instStore.List(context.Background())
	res, err := engine.Backup(context.Background(), Request{
		InstanceID: insts[0].ID, Database: "mydb", StorageIDs: []int64{localID[0].ID, webdavID}, CreatedBy: "admin",
	}, func(string, ...any) {})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if res.Record.Status != "success" {
		t.Fatalf("Flow F: status = %s, want success", res.Record.Status)
	}
	if len(m.objects) < 2 {
		t.Errorf("expected dump + manifest on webdav, got %d objects", len(m.objects))
	}
	_ = backupStore
}

// Flow G: webdav fails, local still succeeds -> partial_success
func TestBackupPartialWebDAVFails(t *testing.T) {
	m := &memWebDAV{objects: map[string][]byte{}, fail: true}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	content := []byte("FLOW_G_DATA")
	engine, instStore, _, _ := newTestEngine(t, &fakeRunner{version: "16.2", dumpContent: content}, []string{t.TempDir()})
	localList, _ := engine.targets.List(context.Background())
	localID := localList[0].ID
	webdavID := addWebDAVTarget(t, engine, srv.URL)

	insts, _ := instStore.List(context.Background())
	res, err := engine.Backup(context.Background(), Request{
		InstanceID: insts[0].ID, Database: "mydb", StorageIDs: []int64{localID, webdavID}, CreatedBy: "admin",
	}, func(string, ...any) {})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if res.Record.Status != "partial_success" {
		t.Fatalf("Flow G: status = %s, want partial_success", res.Record.Status)
	}
}
