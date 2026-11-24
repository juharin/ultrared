# AGENTS.md

Instructions for coding agents working in this repository, including this harness working on itself.

## Project
A coding-agent harness in Go, built for learning. Read `CONTEXT.md` for the current state and roadmap, and `README.md` for usage.

## Layout
- `cmd/ultrared/`: the CLI. Flags, prompt, terminal rendering, Ctrl-C. Only this package does terminal I/O.
- `internal/agent/`: the loop ("brain").
- `internal/tools/`: tool definitions.
- `internal/executor/`: the `Executor` interface ("hands") with `Local` and `Docker` implementations.
- `internal/session/`: the events a session produces (`Event`, one type per kind of event).

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
- `internal/agent` must not read stdin or print. It reports everything as `session.Event`s through `Agent.Emit`, and approvals are `approval_requested` events answered with `Agent.Resolve`, so that every client (CLI now, desktop and mobile later) can drive it. A new kind of thing the loop reports needs a new event type in `internal/session/`, not a new callback.
- Tool errors are returned as `error` and become `is_error` tool results. Never `panic` in a tool.
- New tools need a tests-first case in `internal/tools/tools_test.go` or `internal/agent/agent_test.go`.
- Keep `CONTEXT.md` updated when a TODO is done or a design decision changes.
