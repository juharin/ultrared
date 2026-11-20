# AGENTS.md

Instructions for coding agents working in this repository, including this harness working on itself.

## Project
A coding-agent harness in Go, built for learning. Read `CONTEXT.md` for the current state and roadmap, and `README.md` for usage.

## Commands
- Build: `go build .`
- Test: `go test ./...` (uses a fake Anthropic API; the Docker test is skipped if Docker is down)
- Vet / format: `go vet ./...` and `gofmt -l .` (the output must be empty)
- WASM experiment: separate module in `experiments/wasm/`; see its README

## Conventions
- Standard library first. Current dependencies are `anthropic-sdk-go` and `invopop/jsonschema`. Ask before adding more.
- All side effects go through the `Executor` interface (`executor.go`). Tools must never call `os`/`exec` directly.
- Tool errors are returned as `error` and become `is_error` tool results. Never `panic` in a tool.
- New tools need a tests-first case in `agent_test.go` or a focused unit test.
- Keep `CONTEXT.md` updated when a TODO is done or a design decision changes.
