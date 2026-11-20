# CONTEXT: project state and handover notes

*This file started as a snapshot of the research and coding session on 12 Nov 2026, so the work can be resumed by you or by an AI assistant. Last updated 20 Nov 2026. Update it as you go.*

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

## Known limitations / TODO (rough priority)
- [x] Publish to your own GitHub repo with `main` as the default branch (see "What has been done", item 4).
- [ ] **Run against the real API on the Mac** and fix whatever breaks. Watch: prompt caching (not enabled), `max_tokens` stop reason, very long outputs.
- [ ] Context management: no compaction yet. Add token counting (`usage` from responses) and threshold compaction (doc 02, Rec. 7).
- [ ] Streaming responses (`client.Messages.NewStreaming`) for a responsive UI.
- [ ] Rebuild the conversation from the JSONL/SQLite event log, then add `resume <session>`. This is the first step toward client–server.
- [ ] `grep`/`glob` tools (ripgrep), and maybe a `list_files`.
- [ ] Permission rules as data (allow/ask/deny patterns), replacing yes/no prompts.
- [ ] Persistent in-container executor process, to avoid ~85 ms of `docker exec` per call.
- [ ] Server split: HTTP API + SSE event stream; approvals as events; TUI becomes a client (doc 03 §7).
- [ ] Sandbox lifecycle: clone the repo inside the container, return a branch or diff, TTL reaper, lazy provisioning.
- [ ] Measurement: 10–20 tasks from your own merged PRs; compare pass rate and tokens against Pi with the same model.
- [ ] Optional: code mode via QuickJS-in-wazero (doc 04 §3.1).

## Open questions
- Which clients first: terminal (Bubble Tea) or web?
- Where will the server run? On a Linux box, gVisor becomes available.
- Single user only, or multi-user later? This decides the isolation tier and auth design.

## Caveats on the research
- Many facts date from mid-to-late 2026 and change weekly: versions, tool counts, benchmark rows.
- The main architectural source is a non-peer-reviewed preprint (arXiv:2609.00006), and its Claude Code analysis uses an unofficial source snapshot.
- Each doc ends with a "Confidence & gaps" section. Read it before relying on a claim.
