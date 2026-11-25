# ultrared: a coding-agent harness in Go, built to learn

This started as a small chat loop (the workshop's `main.go`). It is now a working coding agent:
- the model calls tools, the harness runs them, feeds the results back, and repeats
- tools can run on the host, or inside a locked-down Docker container

`docs/` holds the research behind it.

> **Read `CONTEXT.md` next.** It records where the project stands, the decisions made and why, and what to do next.

---

## Quick start (macOS)

Prerequisites: Go ≥ 1.24, Docker Desktop (or OrbStack / colima), and [Task](https://taskfile.dev) (`brew install go-task`).

```sh
task check                         # format check, vet, fast tests against a fake API; no key needed
cp .env.example .env               # then put your ANTHROPIC_API_KEY in .env
task run                           # agent with tools in the sandbox container, on a scratch workspace
task run DIR=$HOME/src/some-repo   # ... on a real repo
task run -- -yolo                  # pass CLI flags after --
```

`task` on its own lists every task. The everyday ones:

| Task | Does |
|---|---|
| `task check` | gofmt check, `go vet`, `go test -race -short`. Run before every push; CI runs it too |
| `task test-all` | All tests, including the Docker executor test |
| `task run` / `task run:host` | Run the agent with tools in the sandbox container / directly on your Mac (no sandbox!) |
| `task sandbox` | Build the sandbox image (`sandbox/Dockerfile`). `task run` does this when needed |
| `task build` | Build `bin/ultrared` |
| `task vuln` | `govulncheck` on code and dependencies |
| `task fmt` | Format all Go code |

**How `task run` works:**
- It loads `.env`, builds the `ultrared-sandbox` image if it's missing or `sandbox/Dockerfile` changed, and runs `go run ./cmd/ultrared -docker ultrared-sandbox`.
- The default workspace is `.tmp/workspace`, and transcripts go to `.tmp/logs/`. `.tmp/` is gitignored, and Go tooling skips it because its name starts with a dot, so agent-written code never mixes with the project's.
- The sandbox image has Go, git and ripgrep. The container has no network unless you pass `-net`, so `go test` in a workspace whose modules aren't downloaded yet needs `task run -- -net`.
- Ctrl-C stops the running turn, as without Task. Pressing it three times quickly makes Task itself exit.

Without Task, the CLI runs directly:

```sh
go run ./cmd/ultrared -dir ~/src/some-repo -docker ultrared-sandbox   # after `task sandbox`, or any image with your toolchain
go install ./cmd/ultrared                                             # install the binary as `ultrared`
```

| Flag | Default | Meaning |
|---|---|---|
| `-dir` | `.` | Workspace the agent works on (bind-mounted at `/workspace` in Docker mode) |
| `-docker IMAGE` | (none) | Run tools in a container from IMAGE. Use an image that has your toolchain |
| `-net` | off | Give the container network access |
| `-runtime` | (none) | Container runtime, e.g. `runsc` (gVisor). **Linux hosts only; not available on macOS** |
| `-yolo` | off | Don't ask before `bash` / `write_file` / `edit_file` |
| `-model` | Sonnet 5.5 | Model id, e.g. `claude-opus-5-5` (see `internal/agent/config.go`) |
| `-log FILE` | (none) | Append a JSONL transcript of every turn |

**macOS notes:**
- Docker Desktop runs containers inside a Linux VM, so bind-mounted file I/O and `docker exec` are slower than on Linux. That is fine for learning.
- File ownership mapping (`--user uid:gid`) is handled by Docker Desktop.
- gVisor needs a Linux host. To try it, use a Linux VM or a cloud box.

## Layout

```
cmd/ultrared/          the CLI: flags, approval prompt, terminal rendering
  interrupt.go         Ctrl-C cancels only the running turn (per-turn context)
internal/agent/        the loop ("brain"): infer → run tools → feed results back → until no tool calls
  config.go            default model
  agent_test.go        end-to-end tests with an httptest fake of the Anthropic API
internal/tools/        bash, read_file, write_file, edit_file (+ GenerateSchema with Required)
internal/executor/     Executor interface ("hands"): Local, Docker
sandbox/Dockerfile     the image tools run in (Go, git, ripgrep)
Taskfile.yml           dev tasks (`task` lists them)
.env.example           settings for `task run`; copy to .env
.github/               CI and Codex review workflows, Dependabot
AGENTS.md              instructions for coding agents working on this repo (incl. this one)
CONTEXT.md             project state, decisions, roadmap: start here when resuming
docs/                  research reports (read in order; see below)
experiments/wasm/      wazero WASI sandbox experiment (separate Go module)
```

## CI

GitHub Actions (`.github/workflows/ci.yml`) runs on every pull request and on pushes to `main`:
- `task check`, `task test-all` (the Docker test runs on GitHub's Ubuntu runners) and `task wasm:check`
- `task vuln`, as a separate job

CI uses the latest stable Go. Dependabot opens weekly grouped updates for Go modules, GitHub Actions and the sandbox base image.

**Codex review** (`.github/workflows/codex-review.yml`) runs on every push to a pull request, and posts its findings as a PR comment. Draft PRs are skipped until they are marked ready for review, which triggers a review:
- It reviews the PR's merge commit with OpenAI Codex in a read-only sandbox without network. What it looks for is in the "Reviewing pull requests" section of `AGENTS.md`, which Codex reads.
- The comment opens with a verdict: 🔴 Changes recommended (at least one `must_fix` finding), 🟡 Changes to consider (only `consider` findings), or 🟢 Approval recommended (none). Codex returns JSON that matches a schema in the workflow, and the workflow derives the verdict from the severities.
- It needs the `OPENAI_API_KEY` repository secret. It skips PRs from forks and runs triggered by Dependabot, since neither can read that secret.
- A newer push cancels a review still running for the same PR.
- It is not a required check, so a failed review does not block merging.

## Learning path: the docs

| # | Doc | What you learn |
|---|---|---|
| 01 | `docs/01-landscape-report.md` | The field: Claude Code, Codex, OpenCode, Pi; how to classify them; where they're heading; benchmark evidence (Terminal-Bench 4.1, Databricks) |
| 02 | `docs/02-how-harnesses-work.md` | Anatomy (7 subsystems), concrete differences between the big four, and a step-by-step build path |
| 03 | `docs/03-architecture-client-server.md` | Local vs. client–server; Anthropic's "brain / hands / session" split; sandbox options; why Go; what changed in your code |
| 04 | `docs/04-wasm-evaluation.md` | Honest evaluation of WASM as the execution sandbox (verdict: not as the main executor; yes for code mode and plugins) |

**Primary sources worth reading in full:**
- Barbaste et al., *Harness Engineering* (source-code study of 11 harnesses), arXiv:2609.00006, https://arxiv.org/html/2609.00006v1. §16 is a practitioner's guide.
- Anthropic Engineering, *Scaling Managed Agents: Decoupling the brain from the hands*, https://www.anthropic.com/engineering/managed-agents
- Mario Zechner, *What I learned building an opinionated and minimal coding agent* (Pi), https://mariozechner.at/posts/2025-11-30-pi-coding-agent/
- Databricks, *Benchmarking Coding Agents on Databricks' Multi-Million Line Codebase*, https://www.databricks.com/blog/benchmarking-coding-agents-databricks-multi-million-line-codebase

**Code to read:**
- mini-swe-agent: the ~100-line floor
- Pi: the minimal production harness
- **Crush** (`charmbracelet/crush`): a production coding agent in Go

## Status
- **Tested:**
  - `go test ./...` against a fake API: loop, tool results, edit errors, Ctrl-C handling, and the Docker executor. Passes on Linux and macOS
  - the WASM experiment
- **Not yet tested:**
  - the real Anthropic API (first thing to try on your Mac)
  - gVisor
