package main

// The Executor is the "hands" of the agent: the only place where side effects
// happen. The agent loop (the "brain") never touches the filesystem or a shell
// directly. That one seam is what lets you move execution into a container,
// a remote VM, or another machine without touching the loop or the tools.
// Pi calls this seam "Operations"; Anthropic's Managed Agents reduces it to
// execute(name, input) -> string.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Executor interface {
	Run(ctx context.Context, command string) (stdout string, exitCode int, err error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
	Close() error
}

const cmdTimeout = 120 * time.Second

// ---------------------------------------------------------------------------
// LocalExecutor: runs directly on the host. No isolation at all.

type LocalExecutor struct{ Root string }

func (l *LocalExecutor) Run(ctx context.Context, command string) (string, int, error) {
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

func (l *LocalExecutor) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(l.Root, p)
}

func (l *LocalExecutor) ReadFile(_ context.Context, p string) ([]byte, error) {
	return os.ReadFile(l.abs(p))
}

func (l *LocalExecutor) WriteFile(_ context.Context, p string, data []byte) error {
	full := l.abs(p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

func (l *LocalExecutor) Close() error { return nil }

// ---------------------------------------------------------------------------
// DockerExecutor: one long-lived container per session. The workspace is
// bind-mounted, networking is off by default, and capabilities are dropped.
// Set runtime to "runsc" if gVisor is installed for a stronger boundary.

type DockerExecutor struct {
	ID      string
	Workdir string
}

func NewDockerExecutor(ctx context.Context, image, hostDir, runtime string, network bool) (*DockerExecutor, error) {
	abs, err := filepath.Abs(hostDir)
	if err != nil {
		return nil, err
	}
	args := []string{"run", "-d", "--rm", "--init",
		"--cap-drop=ALL", "--security-opt=no-new-privileges",
		"--pids-limit=512", "--memory=2g", "--cpus=2",
		// Run as the host user so files created in the bind mount are not root-owned.
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-v", abs + ":/workspace", "-w", "/workspace"}
	if !network {
		args = append(args, "--network=none")
	}
	if runtime != "" {
		args = append(args, "--runtime="+runtime)
	}
	args = append(args, image, "sleep", "infinity")
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker run: %v: %s", err, out)
	}
	return &DockerExecutor{ID: strings.TrimSpace(string(out)), Workdir: "/workspace"}, nil
}

func (d *DockerExecutor) exec(ctx context.Context, stdin []byte, argv ...string) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	args := append([]string{"exec", "-i", "-w", d.Workdir, d.ID}, argv...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return out, ee.ExitCode(), nil
	}
	return out, 0, err
}

func (d *DockerExecutor) Run(ctx context.Context, command string) (string, int, error) {
	out, code, err := d.exec(ctx, nil, "sh", "-c", command)
	return string(out), code, err
}

func (d *DockerExecutor) ReadFile(ctx context.Context, p string) ([]byte, error) {
	out, code, err := d.exec(ctx, nil, "cat", "--", p)
	if err == nil && code != 0 {
		err = fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return out, err
}

func (d *DockerExecutor) WriteFile(ctx context.Context, p string, data []byte) error {
	// Path is passed as $1 to avoid shell injection through the filename.
	out, code, err := d.exec(ctx, data, "sh", "-c", `mkdir -p "$(dirname "$1")" && cat > "$1"`, "sh", p)
	if err == nil && code != 0 {
		err = fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return err
}

func (d *DockerExecutor) Close() error {
	return exec.Command("docker", "rm", "-f", d.ID).Run()
}
