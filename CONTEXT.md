# CONTEXT: project state and handover notes

*This file started as a snapshot of the research and coding session on 12 Nov 2026, so the work can be resumed by you or by an AI assistant. Last updated 25 Nov 2026. Update it as you go.*

## Goal
Learn how AI coding harnesses (Claude Code, Codex, OpenCode, Pi, …) work by building one in Go. The long-term target architecture is **client–server**:
- thin clients on several devices
- a Go server that runs agent loops
- tool execution in per-session containers

## What has been done
1. **Research** (docs/01–04). Key takeaways:
   - Every harness is a ReAct loop plus seven subsystems: loop, LLM integration, tools, context, safety, orchestration, extensibility. A ~100-line harness is competitive on benchmarks. Most production code goes to safety, UX and platform features (doc 02).
   - Results depend mostly on the model. The harness mostly affects **cost and context efficiency**: Databricks measured >2× cost differences with the same model, at equal quality (doc 01).
   - No production harness uses an agent framework or embedding search. They use plain loops plus ripgrep, glob and Markdown context files.
   - **Architecture decision:** separate the *session* (durable event log), the *brain* (stateless loop) and the *hands* (sandbox), following Anthropic's Managed Agents post (doc 03).
   - **WASM decision:** not the main executor, because it can't run real toolchains. Candidate for "code mode" and plugins via wazero (doc 04).
2. **Code**, evolved from the original `main.go`. Fixes:
   - tools never executed
   - `ReadFileDefinition` was unregistered
   - the schema lacked `Required`
   - `panic` on bad input
   - `MaxTokens` 1024 was too small
   
   Additions:
   - inner tool loop
   - bash, write and edit tools
   - **`Executor` interface** with Local and Docker implementations
   - approvals, JSONL log, AGENTS.md loading
   - Ctrl-C cancels the running turn only (`interrupt.go`); at the prompt it prints a hint, and ctrl-d quits so cleanup runs
   - tests
   
   As of 20 Nov 2026, `go test ./...` passes on macOS too, including the Docker executor test.

   The code lives in the module `github.com/juharin/ultrared`: the CLI in `cmd/ultrared/`, and the loop, tools and executors in `internal/agent/`, `internal/tools/` and `internal/executor/`.
3. **Experiment.** `experiments/wasm`: wazero ran a Go-built WASI guest with a read-only mount. Instantiation took ~10 ms; the host filesystem was invisible to the guest.
4. **Published** to `github.com/juharin/ultrared` as `origin`, with `main` as the default branch. The upstream workshop repo (`ghuntley/how-to-build-a-coding-agent`) is not configured as a remote. The local `harness` branch points at the same commit as `main` and was never pushed; delete it or keep it, it holds nothing extra.

## Key design decisions (and why)
| Decision | Why | Revisit if… |
|---|---|---|
| Go | Concurrency plus context cancellation, single binary, the container/gVisor ecosystem, the user's preference. Speed is *not* the reason: the LLM dominates latency | — |
| Anthropic SDK directly, no framework | The field consensus; easier to debug | You want multi-provider support. Then add a small provider interface; consider Crush's `fantasy` library for ideas |
| Exact-unique `edit_file` | The Claude Code/Pi contract for frontier models | You use weaker or open models. Then add a fuzzy fallback (doc 02, §3 Step 2) |
| `Executor` seam ("hands") | Lets execution move to container, remote or local runner without touching the loop | — |
| Docker hardened (cap-drop, no-new-privs, no network, limits, host UID) | Good enough for a single user on their own repos | Untrusted or multi-tenant code. Then use gVisor or Firecracker/Kata (Linux) |
| Loop outside the container | Anthropic's lessons: debuggability, lazy provisioning, credentials kept out of the sandbox | — |
| Target clients: a desktop app, maybe mobile; the CLI is the first client | The user's goal (20 Nov 2026). A phone can't run the loop or Docker, so the server must be reachable over the network, with auth from day one. The desktop app either starts the server locally or connects to a remote one | — |
| Clients talk to the loop only through typed, serializable events (with session id and sequence number), and approvals are asynchronous events | Desktop and mobile clients will be in other languages. Events give replay, resume and multi-device for free. `internal/agent` never does terminal I/O | — |
| A session's state is its append-only event log, one JSONL file per session; resume rebuilds the conversation from it, with no separate conversation store | One source of truth for rendering, replay and resume (doc 03: the session as a durable event log). JSONL is easy to read, diff and recover: a torn last line is dropped, and cut from the file before the session continues | Many sessions on a server, or queries across sessions. Then move the same events into SQLite |
| Wire contract in `api/` (OpenAPI + event JSON schema), written once the server exists; clients generate code from it | Language-neutral; the OpenCode precedent (doc 02) | You prefer typed RPC (e.g. Connect/protobuf) |
| Task (`Taskfile.yml`) as the task runner, installed with Homebrew locally and `go-task/setup-task` in CI; not pinned in `go.mod` | Reads `.env` natively, includes sub-Taskfiles (for `apps/` later), skips up-to-date work, lists tasks with descriptions, works on Windows. Keeping it out of `go.mod` keeps the module's dependencies at two | The extra install becomes a problem for contributors |
| Scratch workspaces and logs in `.tmp/`, not `tmp/` | Go tooling and the gofmt check skip dot-directories, so agent-written Go code never becomes part of this module | — |
| CI on the latest stable Go, not the `go.mod` minimum | Matches local development; govulncheck would flag stdlib issues in an old patch release | You need to prove compatibility with older Go versions |
| Layout: `cmd/` + `internal/` packages along the brain / hands / client boundaries, one binary with subcommands later (`ultrared chat`, `ultrared serve`) | `internal/` keeps APIs free to change; the server becomes a second client of the same packages. UI clients go in `apps/` in the same repo, so protocol and client changes land together | The UI needs its own release cadence |

## Known limitations / TODO (rough priority)
- [x] Publish to your own GitHub repo with `main` as the default branch (see "What has been done", item 4).
- [x] Project layout: `cmd/` + `internal/`, module `github.com/juharin/ultrared`.
- [x] **Dev loop:** `Taskfile.yml` (`check`, `test-all`, `build`, `run`, `sandbox`, …), the sandbox image `sandbox/Dockerfile` (Go, git, ripgrep), `.env.example`, and a gitignored `.tmp/` for scratch workspaces and logs. The Docker test skips under `-short`.
- [x] **CI:** `.github/workflows/ci.yml` runs `task check`, `task test-all` (with Docker), `task wasm:check` and `task vuln` on PRs and pushes to `main`. Dependabot covers Go modules, Actions and the sandbox image.
- [x] **Branch protection** on `main` (a GitHub setting, turned on 24 Nov 2026): the "Check and test" CI job must pass before merging. Not required: the vulnerability check (a new advisory would otherwise block unrelated PRs), reviews (single maintainer), an up-to-date branch. Admins can bypass. Force pushes and deleting `main` are blocked.
- [x] **Codex PR review** (25 Nov 2026): `.github/workflows/codex-review.yml` reviews every push to a non-draft PR (and a draft when marked ready) with `openai/codex-action` (read-only, no network) and posts a comment that opens with a verdict (changes recommended / to consider / approval recommended), derived from the severities in Codex's schema-checked JSON output. Review guidance lives in `AGENTS.md`, which Codex loads. Needs the `OPENAI_API_KEY` secret; skips forks and Dependabot. Not a required check.
- [ ] Live-API smoke test: a manually triggered workflow with the API key as a secret, behind a `live` build tag.
- [ ] **Run against the real API on the Mac** and fix whatever breaks. Watch: prompt caching (not enabled), `max_tokens` stop reason, very long outputs.
- [x] **Event seam:** `internal/session` defines typed, numbered events (`user_message`, `assistant_text`, `tool_call`, `approval_requested`, `approval_resolved`, `tool_result`, `turn_done`). The agent emits them through `Agent.Emit`, and approvals are asynchronous: the loop waits until any client calls `Agent.Resolve`, or the turn is cancelled. The CLI renders events, answers approvals from the keyboard, and `-log` writes the events as JSONL.
- [x] **Resume:** each session's events are kept in `<dir>/<id>.jsonl` (`-sessions DIR`; `task run` uses `.tmp/sessions`). `-resume ID` (`task resume ID=…`) reads the log, `Agent.Restore` rebuilds the conversation, and the session continues in the same log with the next seq and step. Events carry a `step` (which model response they belong to) so messages are rebuilt exactly; a tool call without a recorded result gets an error result. Not recorded yet: the workspace and executor a session used, so you pass the same `DIR` yourself.
- [ ] Context management: no compaction yet. Add token counting (`usage` from responses) and threshold compaction (doc 02, Rec. 7).
- [ ] Streaming responses (`client.Messages.NewStreaming`) for a responsive UI.
- [ ] `grep`/`glob` tools (ripgrep), and maybe a `list_files`.
- [ ] Permission rules as data (allow/ask/deny patterns), replacing yes/no prompts.
- [ ] Persistent in-container executor process, to avoid ~85 ms of `docker exec` per call.
- [ ] Server split: HTTP API + SSE event stream + auth; approvals as events; the CLI becomes a client (doc 03 §7). Then write `api/openapi.yaml` and build a first desktop prototype in `apps/desktop/`.
- [ ] Mobile: when the app is in the background, approvals need push notifications (iOS drops live connections).
- [ ] Sandbox lifecycle: clone the repo inside the container, return a branch or diff, TTL reaper, lazy provisioning.
- [ ] Measurement: 10–20 tasks from your own merged PRs; compare pass rate and tokens against Pi with the same model.
- [ ] Optional: code mode via QuickJS-in-wazero (doc 04 §3.1).

## Open questions
- Desktop UI technology: a web UI in Tauri 2 (one codebase for desktop and mobile), Wails (Go, desktop only), or native SwiftUI? Decide once the server API exists.
- Where will the server run? On a Linux box, gVisor becomes available. Mobile needs it reachable over the network; for personal use a private network such as Tailscale is the simplest.
- Single user only, or multi-user later? This decides the isolation tier and auth design.

## Caveats on the research
- Many facts date from mid-to-late 2026 and change weekly: versions, tool counts, benchmark rows.
- The main architectural source is a non-peer-reviewed preprint (arXiv:2609.00006), and its Claude Code analysis uses an unofficial source snapshot.
- Each doc ends with a "Confidence & gaps" section. Read it before relying on a claim.
