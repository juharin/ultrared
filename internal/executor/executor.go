// Package executor is the "hands" of the agent: the only place where side
// effects happen. The agent loop (the "brain") never touches the filesystem or
// a shell directly. That one seam is what lets you move execution into a
// container, a remote VM, or another machine without touching the loop or the
// tools. Pi calls this seam "Operations"; Anthropic's Managed Agents reduces it
// to execute(name, input) -> string.
package executor

import (
	"context"
	"time"
)

type Executor interface {
	Run(ctx context.Context, command string) (stdout string, exitCode int, err error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
	Close() error
}

const cmdTimeout = 120 * time.Second
