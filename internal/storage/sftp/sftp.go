package sftp

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/dbpilot/dbpilot/internal/storage"
)

type Client struct {
	cfg      storage.SFTPConfig
	password string
	callback ssh.HostKeyCallback
	timeout  time.Duration
}

func New(cfg storage.SFTPConfig, password string) (*Client, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	if cfg.Host == "" || strings.ContainsAny(cfg.Host, " \t\r\n/") {
		return nil, errors.New("invalid sftp host")
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, errors.New("invalid sftp port")
	}
	if cfg.Username == "" {
		return nil, errors.New("sftp username is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30
	}

	base := strings.TrimSpace(cfg.BasePath)
	if base == "" || base == "." {
		base = ""
	} else {
		base = path.Clean(base)
		if base == ".." || strings.HasPrefix(base, "../") {
			return nil, errors.New("sftp base path must not escape the remote home directory")
		}
		base = strings.TrimRight(base, "/")
		if base == "" {
			base = "/"
		}
	}
	cfg.BasePath = base

	fingerprint := strings.TrimSpace(cfg.HostKeyFingerprint)
	if fingerprint == "" {
		return nil, errors.New("sftp host key fingerprint is required")
	}
	if !strings.HasPrefix(fingerprint, "SHA256:") {
		return nil, errors.New("sftp host key fingerprint must be a SHA256 fingerprint")
	}
	callback, err := hostKeyCallback(fingerprint)
	if err != nil {
		return nil, err
	}

	return &Client{
		cfg:      cfg,
		password: password,
		callback: callback,
		timeout:  time.Duration(cfg.Timeout) * time.Second,
	}, nil
}

func hostKeyCallback(expected string) (ssh.HostKeyCallback, error) {
	if expected == "" {
		return nil, errors.New("empty host key fingerprint")
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		got := ssh.FingerprintSHA256(key)
		if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			return fmt.Errorf("host key mismatch for %s: got %s", hostname, got)
		}
		return nil
	}, nil
}

func (c *Client) connect(ctx context.Context) (*ssh.Client, *sftp.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	addr := net.JoinHostPort(c.cfg.Host, strconv.Itoa(c.cfg.Port))
	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("sftp dial: %w", err)
	}
	sshCfg := &ssh.ClientConfig{
		User: c.cfg.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(c.password),
			ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = c.password
				}
				return answers, nil
			}),
		},
		HostKeyCallback: c.callback,
		Timeout:         c.timeout,
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshCfg)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("sftp ssh handshake: %w", err)
	}
	sshClient := ssh.NewClient(sshConn, chans, reqs)
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, nil, fmt.Errorf("sftp handshake: %w", err)
	}
	return sshClient, sftpClient, nil
}

func (c *Client) resolve(name string) (string, error) {
	if name == "" {
		return "", errors.New("empty path")
	}
	clean := path.Clean(name)
	if path.IsAbs(clean) {
		return "", errors.New("absolute paths are not allowed")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path traversal is not allowed")
	}
	if c.cfg.BasePath == "" {
		return clean, nil
	}
	full := path.Join(c.cfg.BasePath, clean)
	if c.cfg.BasePath == "/" {
		if !strings.HasPrefix(full, "/") {
			return "", errors.New("path escapes base directory")
		}
		return full, nil
	}
	if full != c.cfg.BasePath && !strings.HasPrefix(full, c.cfg.BasePath+"/") {
		return "", errors.New("path escapes base directory")
	}
	return full, nil
}

func (c *Client) root() string {
	if c.cfg.BasePath == "" {
		return "."
	}
	return c.cfg.BasePath
}

func (c *Client) Test(ctx context.Context) error {
	sshC, sc, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer closeAll(sc, sshC)

	if c.cfg.BasePath != "" && c.cfg.BasePath != "/" {
		if err := sc.MkdirAll(c.cfg.BasePath); err != nil {
			return fmt.Errorf("create sftp base directory: %w", err)
		}
	}

	probe, err := c.resolve(".dbpilot_test")
	if err != nil {
		return err
	}
	content := []byte("dbpilot")
	f, err := sc.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("write sftp test file: %w", err)
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		_ = sc.Remove(probe)
		return fmt.Errorf("write sftp test file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = sc.Remove(probe)
		return fmt.Errorf("close sftp test file: %w", err)
	}

	rf, err := sc.Open(probe)
	if err != nil {
		_ = sc.Remove(probe)
		return fmt.Errorf("read sftp test file: %w", err)
	}
	got, err := io.ReadAll(rf)
	rf.Close()
	_ = sc.Remove(probe)
	if err != nil {
		return fmt.Errorf("read sftp test file: %w", err)
	}
	if string(got) != string(content) {
		return errors.New("sftp test file content mismatch")
	}
	return nil
}

func (c *Client) Put(ctx context.Context, name string, reader io.Reader) error {
	full, err := c.resolve(name)
	if err != nil {
		return err
	}
	sshC, sc, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer closeAll(sc, sshC)

	dir := path.Dir(full)
	if dir != "." && dir != "/" {
		if err := sc.MkdirAll(dir); err != nil {
			return fmt.Errorf("create sftp directory: %w", err)
		}
	}

	tmp := full + ".tmp"
	f, err := sc.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, reader); err != nil {
		f.Close()
		_ = sc.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = sc.Remove(tmp)
		return err
	}
	if err := sc.Rename(tmp, full); err != nil {
		_ = sc.Remove(full)
		if err := sc.Rename(tmp, full); err != nil {
			_ = sc.Remove(tmp)
			return fmt.Errorf("rename sftp temp file: %w", err)
		}
	}
	return nil
}

func (c *Client) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	full, err := c.resolve(name)
	if err != nil {
		return nil, err
	}
	sshC, sc, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	f, err := sc.Open(full)
	if err != nil {
		closeAll(sc, sshC)
		return nil, err
	}
	return &readCloser{file: f, sftp: sc, ssh: sshC}, nil
}

func (c *Client) Delete(ctx context.Context, name string) error {
	full, err := c.resolve(name)
	if err != nil {
		return err
	}
	sshC, sc, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer closeAll(sc, sshC)
	if err := sc.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (c *Client) Stat(ctx context.Context, name string) (*storage.ObjectInfo, error) {
	full, err := c.resolve(name)
	if err != nil {
		return nil, err
	}
	sshC, sc, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer closeAll(sc, sshC)
	info, err := sc.Stat(full)
	if err != nil {
		return nil, err
	}
	return &storage.ObjectInfo{Path: name, Size: info.Size(), ModTime: info.ModTime()}, nil
}

func (c *Client) List(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	root := c.root()
	if prefix != "" {
		resolved, err := c.resolve(prefix)
		if err != nil {
			return nil, err
		}
		root = resolved
	}
	sshC, sc, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer closeAll(sc, sshC)

	info, err := sc.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []storage.ObjectInfo{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return []storage.ObjectInfo{{Path: prefix, Size: info.Size(), ModTime: info.ModTime()}}, nil
	}

	var out []storage.ObjectInfo
	var walk func(string) error
	walk = func(dir string) error {
		entries, err := sc.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			full := path.Join(dir, entry.Name())
			if entry.IsDir() {
				if err := walk(full); err != nil {
					return err
				}
				continue
			}
			rel := full
			if c.cfg.BasePath != "" && c.cfg.BasePath != "/" {
				rel = strings.TrimPrefix(full, c.cfg.BasePath+"/")
			} else if c.cfg.BasePath == "/" {
				rel = strings.TrimPrefix(full, "/")
			}
			out = append(out, storage.ObjectInfo{Path: rel, Size: entry.Size(), ModTime: entry.ModTime()})
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	return out, nil
}

type readCloser struct {
	file *sftp.File
	sftp *sftp.Client
	ssh  *ssh.Client
}

func (r *readCloser) Read(p []byte) (int, error) { return r.file.Read(p) }

func (r *readCloser) Close() error {
	return errors.Join(r.file.Close(), r.sftp.Close(), r.ssh.Close())
}

func closeAll(sc *sftp.Client, sshC *ssh.Client) {
	if sc != nil {
		_ = sc.Close()
	}
	if sshC != nil {
		_ = sshC.Close()
	}
}
