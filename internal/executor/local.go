package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

// Local runs directly on the host. No isolation at all.
type Local struct{ Root string }

func (l *Local) Run(ctx context.Context, command string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = l.Root
	out, err := cmd.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return string(out), ee.ExitCode(), nil
	}
	return string(out), 0, err
}

func (l *Local) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(l.Root, p)
}

func (l *Local) ReadFile(_ context.Context, p string) ([]byte, error) {
	return os.ReadFile(l.abs(p))
}

func (l *Local) WriteFile(_ context.Context, p string, data []byte) error {
	full := l.abs(p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

func (l *Local) Close() error { return nil }
