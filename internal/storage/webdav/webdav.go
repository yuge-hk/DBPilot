package webdav

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dbpilot/dbpilot/internal/storage"
)

// Client is a minimal WebDAV client targeting generic WebDAV servers
// (AList/OpenList, Nextcloud, NAS WebDAV, ...). No vendor-specific API.
type Client struct {
	endpoint  string
	basePath  string
	username  string
	password  string
	tlsVerify bool
	timeout   time.Duration
	http      *http.Client
}

func New(cfg storage.WebDAVConfig, password string) (*Client, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("endpoint must be http or https")
	}
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: !cfg.TLSVerify},
	}
	return &Client{
		endpoint:  strings.TrimSuffix(cfg.Endpoint, "/"),
		basePath:  strings.Trim(cfg.BasePath, "/"),
		username:  cfg.Username,
		password:  password,
		tlsVerify: cfg.TLSVerify,
		timeout:   timeout,
		http:      &http.Client{Timeout: timeout, Transport: transport},
	}, nil
}

func (c *Client) objectURL(path string) string {
	base := c.endpoint
	if c.basePath != "" {
		base += "/" + c.basePath
	}
	return base + "/" + strings.TrimPrefix(path, "/")
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.objectURL(path), body)
	if err != nil {
		return nil, err
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	return req, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("webdav error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func (c *Client) Test(ctx context.Context) error {
	dir := ".dbpilot_test"
	file := dir + "/probe.txt"
	content := "dbpilot-probe"

	// MKCOL
	req, _ := c.newRequest(ctx, "MKCOL", dir, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("mkcol: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusMethodNotAllowed {
		return fmt.Errorf("mkcol failed: %d", resp.StatusCode)
	}

	// PUT
	req, _ = c.newRequest(ctx, "PUT", file, strings.NewReader(content))
	req.ContentLength = int64(len(content))
	if resp, err = c.do(req); err != nil {
		c.cleanupTest(ctx, file, dir)
		return fmt.Errorf("put: %w", err)
	}
	resp.Body.Close()

	// GET + verify
	req, _ = c.newRequest(ctx, "GET", file, nil)
	resp, err = c.do(req)
	if err != nil {
		c.cleanupTest(ctx, file, dir)
		return fmt.Errorf("get: %w", err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(got) != content {
		c.cleanupTest(ctx, file, dir)
		return errors.New("content mismatch")
	}

	// DELETE file + dir
	c.cleanupTest(ctx, file, dir)
	return nil
}

func (c *Client) cleanupTest(ctx context.Context, file, dir string) {
	req, _ := c.newRequest(ctx, "DELETE", file, nil)
	if resp, err := c.http.Do(req); err == nil {
		resp.Body.Close()
	}
	req, _ = c.newRequest(ctx, "DELETE", dir, nil)
	if resp, err := c.http.Do(req); err == nil {
		resp.Body.Close()
	}
}

func (c *Client) Put(ctx context.Context, path string, reader io.Reader) error {
	req, err := c.newRequest(ctx, "PUT", path, reader)
	if err != nil {
		return err
	}
	if rs, ok := reader.(io.ReadSeeker); ok {
		if size, err := seekSize(rs); err == nil {
			req.ContentLength = size
			_, _ = rs.Seek(0, io.SeekStart)
		}
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	req, err := c.newRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) Delete(ctx context.Context, path string) error {
	req, err := c.newRequest(ctx, "DELETE", path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("delete failed: %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) Stat(ctx context.Context, path string) (*storage.ObjectInfo, error) {
	entries, err := c.propfind(ctx, path, 0)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("object not found")
	}
	e := entries[0]
	return &storage.ObjectInfo{Path: path, Size: e.size, ModTime: e.modTime}, nil
}

func (c *Client) List(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	entries, err := c.propfind(ctx, prefix, 1)
	if err != nil {
		return nil, err
	}
	var out []storage.ObjectInfo
	for _, e := range entries {
		out = append(out, storage.ObjectInfo{Path: e.path, Size: e.size, ModTime: e.modTime})
	}
	return out, nil
}

type propEntry struct {
	path    string
	size    int64
	modTime time.Time
}

func (c *Client) propfind(ctx context.Context, path string, depth int) ([]propEntry, error) {
	body := `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:getcontentlength/>
    <d:getlastmodified/>
  </d:prop>
</d:propfind>`
	req, err := c.newRequest(ctx, "PROPFIND", path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", fmt.Sprintf("%d", depth))
	req.ContentLength = int64(len(body))
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var multi struct {
		Responses []struct {
			Href     string `xml:"href"`
			Propstat []struct {
				Prop struct {
					Length   string `xml:"getcontentlength"`
					Modified string `xml:"getlastmodified"`
				} `xml:"prop"`
			} `xml:"propstat"`
		} `xml:"response"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&multi); err != nil {
		return nil, fmt.Errorf("parse propfind: %w", err)
	}

	var entries []propEntry
	for _, r := range multi.Responses {
		e := propEntry{path: c.trimObjectPath(r.Href)}
		for _, ps := range r.Propstat {
			if ps.Prop.Length != "" {
				var n int64
				fmt.Sscanf(ps.Prop.Length, "%d", &n)
				e.size = n
			}
			if ps.Prop.Modified != "" {
				if tm, err := http.ParseTime(ps.Prop.Modified); err == nil {
					e.modTime = tm
				}
			}
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (c *Client) trimObjectPath(href string) string {
	h := strings.TrimPrefix(href, c.endpointPrefix())
	return strings.TrimPrefix(h, "/")
}

func (c *Client) endpointPrefix() string {
	if c.basePath != "" {
		return c.endpoint + "/" + c.basePath
	}
	return c.endpoint
}

func seekSize(rs io.ReadSeeker) (int64, error) {
	cur, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	end, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}
	_, _ = rs.Seek(cur, io.SeekStart)
	return end, nil
}
