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
- Build: `go build ./...` (the binary: `go build ./cmd/ultrared`)
- Run: `go run ./cmd/ultrared -dir <workspace> [-docker IMAGE]`
- Test: `go test ./...` (uses a fake Anthropic API; the Docker test is skipped if Docker is down)
- Vet / format: `go vet ./...` and `gofmt -l .` (the output must be empty)
- WASM experiment: separate module in `experiments/wasm/`; see its README

## Conventions
- Standard library first. Current dependencies are `anthropic-sdk-go` and `invopop/jsonschema`. Ask before adding more.
- All side effects go through the `Executor` interface (`internal/executor/`). Tools must never call `os`/`exec` directly.
- `internal/agent` must not read stdin or print. It reports through callbacks (later: events), so that every client (CLI now, desktop and mobile later) can drive it.
- Tool errors are returned as `error` and become `is_error` tool results. Never `panic` in a tool.
- New tools need a tests-first case in `internal/tools/tools_test.go` or `internal/agent/agent_test.go`.
- Keep `CONTEXT.md` updated when a TODO is done or a design decision changes.
