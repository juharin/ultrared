# AGENTS.md

Instructions for coding agents working in this repository, including this harness working on itself.

## Project
A coding-agent harness in Go, built for learning. Read `CONTEXT.md` for the current state and roadmap, and `README.md` for usage.

## Layout
- `cmd/ultrared/`: the CLI. Flags, prompt, terminal rendering, Ctrl-C. Only this package does terminal I/O.
- `internal/agent/`: the loop ("brain").
- `internal/tools/`: tool definitions.
- `internal/executor/`: the `Executor` interface ("hands") with `Local` and `Docker` implementations.
- `internal/session/`: the events a session produces (`Event`, one type per kind of event), and the event log they are kept in (`ReadLog`, `LogPath`). The log is the session's source of truth; `Agent.Restore` rebuilds a conversation from it.

## Commands
Dev tasks live in `Taskfile.yml` (`task` lists them). Before finishing a change, run `task check` and make sure it passes; CI runs the same.
- Check: `task check` (gofmt check, `go vet`, `go test -race -short`; the tests use a fake Anthropic API)
- All tests: `task test-all` (adds the Docker executor test; it is skipped if Docker is down)
- Format: `task fmt`
- Build: `task build` (writes `bin/ultrared`)
- Run: `task run` (needs `ANTHROPIC_API_KEY`; tools run in the sandbox container on `.tmp/workspace`; session logs go to `.tmp/sessions/`). `task resume ID=<id>` continues a session
- WASM experiment: separate module in `experiments/wasm/`; `task wasm:check` builds it

Without Task, the plain Go commands work too: `go vet ./...`, `go test ./...`, `go run ./cmd/ultrared -dir <workspace>`.

Scratch output (workspaces, session logs) belongs in `.tmp/`, never in the source tree.

## Conventions
- Standard library first. Current dependencies are `anthropic-sdk-go` and `invopop/jsonschema`. Ask before adding more, including dev tools pinned in `go.mod`.
- All side effects go through the `Executor` interface (`internal/executor/`). Tools must never call `os`/`exec` directly.
- `internal/agent` must not read stdin or print. It reports everything as `session.Event`s through `Agent.Emit`, and approvals are `approval_requested` events answered with `Agent.Resolve`, so that every client (CLI now, desktop and mobile later) can drive it. A new kind of thing the loop reports needs a new event type in `internal/session/`, not a new callback.
- Tool errors are returned as `error` and become `is_error` tool results. Never `panic` in a tool.
- New tools need a tests-first case in `internal/tools/tools_test.go` or `internal/agent/agent_test.go`.
- Keep `CONTEXT.md` updated when a TODO is done or a design decision changes.

## Pull request reviews
Every push to a non-draft pull request gets an automated Codex review, posted as a PR comment (`.github/workflows/codex-review.yml`). The comment opens with a verdict: 🔴 Changes recommended, 🟡 Changes to consider, or 🟢 Approval recommended. When you work on a PR, read those comments, but verify each finding before acting on it; the reviewer can be wrong.

## Reviewing pull requests
When you are the reviewer, report real problems only, most severe first:
- Bugs, races and goroutine or context leaks, error handling that loses or hides errors, security problems. Tools run model-chosen commands, so pay attention to paths escaping the workspace and to sandbox (Docker flags) and approval bypasses.
- Breaks of the conventions above: side effects outside the `Executor`, `os`/`exec` in tools, terminal I/O in `internal/agent`, `panic` in a tool, a new tool without a test, an unapproved dependency.
- Docs that the change makes wrong: `README.md`, `CONTEXT.md` (a finished TODO or a changed decision not recorded), this file.

Skip formatting (CI runs gofmt and `go vet`), style and naming.

Give each finding a severity:
- `must_fix`: merging as is would ship a bug, a security hole or a convention break listed above. Any `must_fix` finding makes the verdict "Changes recommended".
- `consider`: a likely problem or a real risk that the author may reasonably accept, such as a missing edge-case test or a doc gap. Only `consider` findings make the verdict "Changes to consider".

No findings means "Approval recommended". When unsure whether something is a problem at all, leave it out rather than marking it `consider`.
