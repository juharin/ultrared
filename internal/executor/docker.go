package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Docker uses one long-lived container per session. The workspace is
// bind-mounted, networking is off by default, and capabilities are dropped.
// Set runtime to "runsc" if gVisor is installed for a stronger boundary.
type Docker struct {
	ID      string
	Workdir string
}

func NewDocker(ctx context.Context, image, hostDir, runtime string, network bool) (*Docker, error) {
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
	return &Docker{ID: strings.TrimSpace(string(out)), Workdir: "/workspace"}, nil
}

func (d *Docker) exec(ctx context.Context, stdin []byte, argv ...string) ([]byte, int, error) {
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

func (d *Docker) Run(ctx context.Context, command string) (string, int, error) {
	out, code, err := d.exec(ctx, nil, "sh", "-c", command)
	return string(out), code, err
}

func (d *Docker) ReadFile(ctx context.Context, p string) ([]byte, error) {
	out, code, err := d.exec(ctx, nil, "cat", "--", p)
	if err == nil && code != 0 {
		err = fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return out, err
}

func (d *Docker) WriteFile(ctx context.Context, p string, data []byte) error {
	// Path is passed as $1 to avoid shell injection through the filename.
	out, code, err := d.exec(ctx, data, "sh", "-c", `mkdir -p "$(dirname "$1")" && cat > "$1"`, "sh", p)
	if err == nil && code != 0 {
		err = fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return err
}

func (d *Docker) Close() error {
	return exec.Command("docker", "rm", "-f", d.ID).Run()
}
