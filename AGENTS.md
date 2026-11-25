# AGENTS.md

Instructions for coding agents working in this repository, including this harness working on itself.

## Project
A coding-agent harness in Go, built for learning. Read `CONTEXT.md` for the current state and roadmap, and `README.md` for usage.

## Layout
- `cmd/ultrared/`: the CLI. Flags, prompt, terminal rendering, Ctrl-C. Only this package does terminal I/O.
- `internal/agent/`: the loop ("brain").
- `internal/tools/`: tool definitions.
- `internal/executor/`: the `Executor` interface ("hands") with `Local` and `Docker` implementations.

## Commands
Dev tasks live in `Taskfile.yml` (`task` lists them). Before finishing a change, run `task check` and make sure it passes; CI runs the same.
- Check: `task check` (gofmt check, `go vet`, `go test -race -short`; the tests use a fake Anthropic API)
- All tests: `task test-all` (adds the Docker executor test; it is skipped if Docker is down)
- Format: `task fmt`
- Build: `task build` (writes `bin/ultrared`)
- Run: `task run` (needs `ANTHROPIC_API_KEY`; tools run in the sandbox container on `.tmp/workspace`)
- WASM experiment: separate module in `experiments/wasm/`; `task wasm:check` builds it

Without Task, the plain Go commands work too: `go vet ./...`, `go test ./...`, `go run ./cmd/ultrared -dir <workspace>`.

Scratch output (workspaces, transcripts) belongs in `.tmp/`, never in the source tree.

## Conventions
- Standard library first. Current dependencies are `anthropic-sdk-go` and `invopop/jsonschema`. Ask before adding more, including dev tools pinned in `go.mod`.
- All side effects go through the `Executor` interface (`internal/executor/`). Tools must never call `os`/`exec` directly.
- `internal/agent` must not read stdin or print. It reports through callbacks (later: events), so that every client (CLI now, desktop and mobile later) can drive it.
- Tool errors are returned as `error` and become `is_error` tool results. Never `panic` in a tool.
- New tools need a tests-first case in `internal/tools/tools_test.go` or `internal/agent/agent_test.go`.
- Keep `CONTEXT.md` updated when a TODO is done or a design decision changes.

## Pull request reviews
Every push to a non-draft pull request gets an automated Codex review, posted as a PR comment (`.github/workflows/codex-review.yml`). When you work on a PR, read those comments, but verify each finding before acting on it; the reviewer can be wrong.

## Reviewing pull requests
When you are the reviewer, report real problems only, most severe first:
- Bugs, races and goroutine or context leaks, error handling that loses or hides errors, security problems. Tools run model-chosen commands, so pay attention to paths escaping the workspace and to sandbox (Docker flags) and approval bypasses.
- Breaks of the conventions above: side effects outside the `Executor`, `os`/`exec` in tools, terminal I/O in `internal/agent`, `panic` in a tool, a new tool without a test, an unapproved dependency.
- Docs that the change makes wrong: `README.md`, `CONTEXT.md` (a finished TODO or a changed decision not recorded), this file.

Skip formatting (CI runs gofmt and `go vet`), style and naming.
