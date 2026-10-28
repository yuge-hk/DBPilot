package sftp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	psftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/dbpilot/dbpilot/internal/storage"
)

func TestResolve(t *testing.T) {
	c := &Client{cfg: storage.SFTPConfig{BasePath: "/backups/dbpilot"}}
	got, err := c.resolve("a/b.txt")
	if err != nil || got != "/backups/dbpilot/a/b.txt" {
		t.Fatalf("resolve = %q, %v", got, err)
	}
	for _, bad := range []string{"", "/etc/passwd", "../x", "a/../../x"} {
		if _, err := c.resolve(bad); err == nil {
			t.Errorf("expected %q to be rejected", bad)
		}
	}

	root := &Client{cfg: storage.SFTPConfig{BasePath: "/"}}
	got, err = root.resolve("a.txt")
	if err != nil || got != "/a.txt" {
		t.Fatalf("root resolve = %q, %v", got, err)
	}
}

func TestNewValidation(t *testing.T) {
	base := storage.SFTPConfig{
		Host: "127.0.0.1", Port: 22, Username: "u",
		HostKeyFingerprint: "SHA256:abc", Timeout: 5,
	}
	if _, err := New(base, "p"); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	noFingerprint := base
	noFingerprint.HostKeyFingerprint = ""
	if _, err := New(noFingerprint, "p"); err == nil {
		t.Error("missing fingerprint should be rejected")
	}
	badBase := base
	badBase.BasePath = "../escape"
	if _, err := New(badBase, "p"); err == nil {
		t.Error("escaping base path should be rejected")
	}
}

func TestClientLifecycle(t *testing.T) {
	addr, fingerprint, cleanup := startTestServer(t)
	defer cleanup()

	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	client, err := New(storage.SFTPConfig{
		Host: host, Port: port, Username: "test", BasePath: "backups",
		HostKeyFingerprint: fingerprint, Timeout: 5,
	}, "secret")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx := context.Background()
	if err := client.Test(ctx); err != nil {
		t.Fatalf("test: %v", err)
	}
	if err := client.Put(ctx, "a/b.txt", strings.NewReader("hello sftp")); err != nil {
		t.Fatalf("put: %v", err)
	}
	info, err := client.Stat(ctx, "a/b.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size != int64(len("hello sftp")) {
		t.Errorf("size = %d", info.Size)
	}
	rc, err := client.Open(ctx, "a/b.txt")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil || string(data) != "hello sftp" {
		t.Fatalf("read = %q, %v", data, err)
	}
	objects, err := client.List(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, obj := range objects {
		if obj.Path == "a/b.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("listed objects missing a/b.txt: %+v", objects)
	}
	if err := client.Delete(ctx, "a/b.txt"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := client.Stat(ctx, "a/b.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stat after delete = %v, want os.ErrNotExist", err)
	}
}

func startTestServer(t *testing.T) (string, string, func()) {
	t.Helper()
	dir := t.TempDir()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	sshCfg := &ssh.ServerConfig{
		PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if meta.User() == "test" && string(password) == "secret" {
				return nil, nil
			}
			return nil, fmt.Errorf("denied")
		},
	}
	sshCfg.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveSSHConn(conn, sshCfg, dir)
		}
	}()

	return listener.Addr().String(), ssh.FingerprintSHA256(signer.PublicKey()), func() {
		listener.Close()
		<-done
	}
}

func serveSSHConn(conn net.Conn, cfg *ssh.ServerConfig, dir string) {
	defer conn.Close()
	serverConn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer serverConn.Close()
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer channel.Close()
			for req := range requests {
				if req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp" {
					_ = req.Reply(true, nil)
					server, err := psftp.NewServer(channel, psftp.WithServerWorkingDirectory(dir))
					if err != nil {
						return
					}
					_ = server.Serve()
					_ = server.Close()
					return
				}
				_ = req.Reply(false, nil)
			}
		}()
	}
}
