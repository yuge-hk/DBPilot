package webdav

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dbpilot/dbpilot/internal/storage"
)

type fakeWebDAV struct {
	mu          sync.Mutex
	objects     map[string][]byte
	username    string
	password    string
	requireAuth bool
}

func newFakeWebDAV() *fakeWebDAV {
	return &fakeWebDAV{objects: map[string][]byte{}}
}

func (f *fakeWebDAV) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.requireAuth {
			u, p, ok := r.BasicAuth()
			if !ok || u != f.username || p != f.password {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		switch r.Method {
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case "PUT":
			body, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			f.objects[r.URL.Path] = body
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		case "GET":
			f.mu.Lock()
			b, ok := f.objects[r.URL.Path]
			f.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write(b)
		case "DELETE":
			f.mu.Lock()
			delete(f.objects, r.URL.Path)
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case "PROPFIND":
			body := `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>` + r.URL.Path + `</d:href>
    <d:propstat>
      <d:prop>
        <d:getcontentlength>123</d:getcontentlength>
        <d:getlastmodified>Wed, 01 Jan 2025 00:00:00 GMT</d:getlastmodified>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(body))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func testClient(t *testing.T, srv *httptest.Server, username, password string) *Client {
	t.Helper()
	c, err := New(storage.WebDAVConfig{
		Endpoint: srv.URL, Username: username, BasePath: "dbpilot", TLSVerify: true, Timeout: 10,
	}, password)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

func TestWebDAVPutGetDelete(t *testing.T) {
	f := newFakeWebDAV()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	c := testClient(t, srv, "", "")
	ctx := context.Background()

	data := []byte("backup data")
	if err := c.Put(ctx, "backups/1/db/x.dump", bytes.NewReader(data)); err != nil {
		t.Fatalf("put: %v", err)
	}

	r, err := c.Open(ctx, "backups/1/db/x.dump")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if !bytes.Equal(got, data) {
		t.Errorf("content mismatch")
	}

	info, err := c.Stat(ctx, "backups/1/db/x.dump")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size != 123 {
		t.Errorf("size = %d", info.Size)
	}

	if err := c.Delete(ctx, "backups/1/db/x.dump"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestWebDAVAuthFailure(t *testing.T) {
	f := newFakeWebDAV()
	f.requireAuth = true
	f.username = "alist"
	f.password = "correct"
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	c := testClient(t, srv, "alist", "wrong")
	err := c.Put(context.Background(), "x.dump", strings.NewReader("x"))
	if err == nil {
		t.Fatal("expected auth failure")
	}

	c2 := testClient(t, srv, "alist", "correct")
	if err := c2.Put(context.Background(), "x.dump", strings.NewReader("x")); err != nil {
		t.Fatalf("put with correct auth should work: %v", err)
	}
}

func TestWebDAVTestRoundtrip(t *testing.T) {
	f := newFakeWebDAV()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	c := testClient(t, srv, "", "")
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("test: %v", err)
	}
	// probe files must be cleaned up
	f.mu.Lock()
	defer f.mu.Unlock()
	for p := range f.objects {
		if strings.Contains(p, "probe") {
			t.Errorf("probe file not cleaned up: %s", p)
		}
	}
}

func TestWebDAVRejectsNonHTTPScheme(t *testing.T) {
	_, err := New(storage.WebDAVConfig{Endpoint: "ftp://example.com"}, "")
	if err == nil {
		t.Fatal("expected error for non-http scheme")
	}
}
