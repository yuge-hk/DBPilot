package runner

import (
	"context"
	"os"
	"os/exec"
)

// Runner abstracts external command execution so that pg_dump, psql and
// friends can be tested without a real PostgreSQL installation.
type Runner interface {
	Run(ctx context.Context, name string, args []string, env map[string]string) (stdout []byte, stderr []byte, err error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args []string, env map[string]string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	var stdout, stderr buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.b, stderr.b, err
}

type buffer struct {
	b []byte
}

func (b *buffer) Write(p []byte) (int, error) {
	b.b = append(b.b, p...)
	return len(p), nil
}

// LookPath reports the absolute path of a binary if found.
func LookPath(name string) (string, bool) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return p, true
}
