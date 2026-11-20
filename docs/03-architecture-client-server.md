# Where should the harness loop run? Local vs. client–server with containerized execution

*12 Nov 2026. This is the third document in the series, after `01-landscape-report.md` and `02-how-harnesses-work.md`. Language context: Go, building on the user's existing `main.go`.*

## Question
1. Do existing harnesses mainly run locally on the user's device?
2. Would it make sense to build a client–server design, with thin clients on several devices and a server that runs the agent loops in containers, to get both sandboxing and ease of use?

## Short answer
1. **Mostly yes, but every major vendor now also ships a remote mode.** The four harnesses studied are local-first. However:
   - Codex has a cloud mode in which each task runs in its own container or VM.
   - Claude Code has cloud routines and the hosted Managed Agents product.
   - OpenHands runs its agent as a server, with a Docker sandbox for each conversation.
   - OpenCode is already client/server, but the server runs locally.
2. **Yes, it makes sense. It is the direction the field is moving, so the design is not novel.** The one refinement I'd strongly recommend comes from Anthropic's published experience. **Don't put the loop *inside* the container. Separate three things:**
   - the **session log** (durable),
   - the **loop/"brain"** (stateless, on the server),
   - the **execution sandbox/"hands"** (disposable containers, reached through a narrow `execute` interface).
   
   Your clients then talk only to the server.

---

## 1. Today's landscape: where the loop runs and where tools run

| System | Where the loop runs | Where tools execute | Clients | Source |
|---|---|---|---|---|
| Claude Code (CLI) | User's machine | User's machine; OS sandbox for bash is opt-in | TUI, IDE, desktop | Anthropic docs (see main report) |
| Claude Managed Agents | Anthropic servers, stateless | Containers provisioned on demand, or your own sandboxes | API, console | Anthropic engineering post (below) |
| Codex CLI | User's machine | User's machine inside an OS sandbox (Seatbelt/bubblewrap), on by default | TUI, IDE, app | OpenAI docs (see main report) |
| Codex Cloud | OpenAI | A separate workspace per task: container/VM, offline by default during the agent phase | Web, mobile, desktop | OpenAI docs (below) |
| OpenCode | A *local* server process | Same host, no sandbox | TUI, web, desktop and SDK as clients | arXiv:2609.00006 §5 |
| OpenHands | Agent server | Docker, Apptainer, remote or cloud "workspaces" | Web "Canvas", CLI, SDK | OpenHands README/docs (below) |
| Pi | Local | Local by default. A per-tool `Operations` interface lets SSH, container or micro-VM extensions move execution elsewhere | TUI, RPC, SDK | arXiv:2609.00006 §8.2 |
| Crush (Go) | Local | Local; permission prompts and Claude-Code-compatible hooks | TUI, `crush run` | Crush AGENTS.md / README (below) |

**Primary-source evidence for the remote modes:**
- **Codex Cloud.** OpenAI's docs say it "runs coding tasks in the cloud, so work can continue while your computer is asleep. Use it on the web, on mobile, or in the desktop app". They also say "Each cloud task runs in its own workspace" (https://learn.chatgpt.com/docs/environments/cloud-environments). For the legacy flow: "Internet access is available during the setup script phase to install dependencies. During the agent phase, internet access is off by default", and secrets "are only available to setup scripts. For security reasons, secrets are removed before the agent phase starts" (https://developers.openai.com/codex/cloud/environments). This matches the multi-device scenario you describe.
- **OpenHands Agent Canvas.** The README says it "runs locally on your machine by default, but can connect to multiple 'agent backends', e.g. running agents in Docker containers, on VMs, or within your company infrastructure". It can also drive "third-party agents such as Claude Code, Codex, Gemini CLI, Pi, and OpenCode". The README also warns that the non-Docker mode "runs the agent-server directly on the machine… the agent will have full access to your filesystem!" (https://github.com/OpenHands/openhands).
- **Platform shift.** The source-code study identifies harnesses becoming platforms. OpenCode is described as "an HTTP server publishing an OpenAPI spec and a generated SDK, with every UI—TUI, desktop, web, IDE plugin, CI action—as a client" (arXiv:2609.00006 §14.1).

**Conclusion.** The open harnesses are local-first. The vendors offer both local and cloud modes. Your idea matches what Codex Cloud, Managed Agents and OpenHands already do commercially, and you would be building an open, self-hosted version of it.

## 2. Lessons learned from someone who built this: Anthropic's Managed Agents

Primary source: Anthropic Engineering, *Scaling Managed Agents: Decoupling the brain from the hands*, https://www.anthropic.com/engineering/managed-agents. This is the most useful document I found for your question, because it describes the *first* design (yours) and why they moved away from it.

- **First design: everything in one container** (session, harness and sandbox). It had three problems:
  - **Debugging.** When something went wrong, a harness bug, a dropped packet and a dead container all looked the same. Inspecting a container meant touching user data.
  - **Network coupling.** "the harness assumed that whatever Claude worked on lived in the container with it", so connecting to a customer's VPC required network peering.
  - **Latency.** Every session paid the container start-up cost before the first token, even sessions that never needed a sandbox.
- **The fix:** "decouple what we thought of as the 'brain' (Claude and its harness) from both the 'hands' (sandboxes and tools that perform actions) and the 'session' (the log of session events)."
- **The interface:** "each hand a tool, execute(name, input) → string… The harness doesn't know whether the sandbox is a container, a phone, or a Pokémon emulator."
- **The payoff:** containers are "provisioned by the brain via a tool call… only if they are needed", and "our p50 TTFT dropped roughly 60% and p95 dropped over 90%."
- **Credentials stay out of the sandbox.** Git tokens are wired into the remote during sandbox initialization, so "Git push and pull work from inside the sandbox without the agent ever handling the token itself". MCP OAuth tokens live in a vault and go through a proxy, so "The harness is never made aware of any credentials."
- **The session is an external, append-only log.** The brain reads slices of it (`getEvents()`), and context management happens in the harness. A crashed harness can therefore be restarted from the log.

**What this means for you:** keep the loop on the server, outside the sandbox. Treat containers as disposable "cattle" that the loop calls into, and keep the event log as the single source of truth. This also solves your multi-device requirement for free: each client subscribes to the session's event stream and can attach, detach or resume.

**Counterpoint.** OpenHands V1 puts an *agent server inside* the sandbox, and the source-code study notes that Browser Use moved its whole loop into a micro-VM (secondary source). Loop-in-sandbox is simpler: there is one process and file operations are plain syscalls, as Anthropic itself notes. It is a valid choice for a single-user system. The separation pays off once you want many sessions, fast start, resumability, or credentials kept out of the sandbox.

## 3. Proposed architecture for your project

```
 ┌──────────── clients (thin) ────────────┐
 │ TUI (Bubble Tea) · web · mobile · CLI  │   HTTP + SSE/WebSocket, auth token
 └──────────────────┬─────────────────────┘
                    │  POST /sessions/{id}/messages      GET /sessions/{id}/events?after=N (stream)
 ┌──────────────────▼─────────────────────┐
 │ server (Go)                            │
 │  • session store: append-only events   │  ← SQLite to start (Crush uses SQLite+sqlc)
 │  • runner: one goroutine per active    │  ← stateless loop; rebuilds context from events
 │    session, cancellable via context    │
 │  • LLM provider client(s)              │
 │  • policy: allow/ask/deny; "ask" =     │  ← approval request is just another event;
 │    event pushed to clients             │    any device can answer it
 │  • secrets vault / egress proxy        │
 └──────────────────┬─────────────────────┘
                    │  Executor interface: Run / ReadFile / WriteFile  (≈ execute(name,input))
 ┌──────────────────▼─────────────────────┐
 │ sandboxes: one container per session   │  ← Docker → gVisor (runsc) → Firecracker/Kata
 │  /workspace = git clone of the repo    │    no secrets inside, network off/allowlisted,
 │                                        │    CPU/mem/pids limits, TTL reaper
 └────────────────────────────────────────┘
```

**Design decisions and their trade-offs:**

| Decision | Recommendation | Why / trade-off |
|---|---|---|
| Code location | **Clone the repo inside the sandbox**; return diffs, branches or PRs to the user | This is the Codex Cloud model. The alternative, syncing local files to the server, is much harder. The cost is that you lose "edit in my local IDE while the agent works". Fix that with git (push a branch, then `git pull`) |
| Session state | Append-only event log on the server | Enables multi-device use, resuming, auditing, and recovery after a crash (Managed Agents, OpenHands, Pi's JSONL tree) |
| Streaming to clients | SSE or WebSocket with an event cursor (`after=N`) | A device that reconnects replays from its last seen event |
| Approvals | Model "ask" as an event; the loop blocks until any client answers or a timeout fires | Approving from your phone works naturally. With a strong sandbox, default to *fewer* asks; isolation lets you be permissive |
| Isolation | Docker, hardened, to start; **gVisor (`--runtime=runsc`)** as the low-effort upgrade; Firecracker/Kata if you ever run untrusted multi-tenant code | See §4 |
| Network | Off by default; allow-listed egress through a proxy; full internet only during setup | This is Codex Cloud's two-phase model: setup online, agent offline by default |
| Secrets | Never in the sandbox. Use credential helpers, a proxy that injects tokens, or setup-phase-only secrets | Managed Agents and Codex Cloud both do this. Prompt injection cannot exfiltrate what isn't there |
| Local hands (optional, later) | A small Go "runner" binary on the user's machine that connects *out* to the server and implements the same `Executor` | Lets one brain work on local files or devices when needed. This is the "many hands" pattern |

**Honest downsides of the client–server design:**
- **Ops burden.** You now run a server, a container host, TLS, auth and backups. Running everything locally avoids all of this.
- **Dev-loop friction.** You review via diffs and branches rather than watching files change in your editor. Codex users live with this; some people dislike it.
- **Environment fidelity.** The container must be able to build and test your project: toolchains, services, sometimes a GPU. Codex Cloud needs whole setup and maintenance-script machinery for this.
- **Latency per tool call.** A `docker exec` adds roughly tens of milliseconds per call. This is negligible next to LLM latency, but it adds up over hundreds of `cat`/`rg` calls. A persistent in-container agent process (OpenHands' action-execution server) avoids it.
- **The sandbox is not a complete defense.** It limits the blast radius. It doesn't stop the agent from writing bad code into the repo, or leaking data through allowed network destinations.

## 4. Isolation options (for the sandbox tier)

Sources: a Fly.io explainer (vendor) and Northflank (vendor). Both sell sandboxes, so treat their claims with that in mind. They agree on the basics.
- **Plain containers (runc)** share the host kernel. Fly.io says: "If the threat is a prompt-injected agent deleting the repo, a container with scoped credentials and a locked-down network covers most of it." This is fine for **your own code, single user**.
- **gVisor** "is an application kernel that implements a Linux-like interface, written in a memory-safe language, Go, and running in userspace". It plugs into Docker as `--runtime=runsc`, and Fly.io reports that gVisor lists OpenAI and Anthropic as users for code execution. Expect some I/O overhead. Northflank estimates 10–30% on I/O-heavy work, which is a vendor figure and I have not verified it.
- **Firecracker microVMs** give each sandbox its own kernel enforced by KVM. Northflank cites boot times of about 125 ms and less than 5 MiB of overhead. Firecracker needs bare metal or nested virtualization. Choose it for untrusted, multi-tenant workloads.

(https://fly.io/learn/firecracker-vs-gvisor/ ; https://northflank.com/blog/how-to-sandbox-ai-agents)

**For a learning project with one user and your own repositories:** hardened Docker is enough, with gVisor as a one-flag upgrade. The provided `DockerExecutor` already applies `--cap-drop=ALL`, `no-new-privileges`, `--network=none`, pids/memory/CPU limits and your host UID, and it accepts `-runtime runsc`.

## 5. Language: Go

Go is a good fit for this design, but for the right reasons:
- **Raw speed is not the reason.** A harness spends almost all its time waiting on the LLM, and the source-code study found that "loop sophistication does not predict benchmark performance". Language speed barely affects results.
- **Concurrency.** Go's real advantages here are goroutines plus `context` cancellation for many concurrent sessions and streams.
- **Deployment.** A single static binary is easy to ship for the server and for an optional local runner.
- **Ecosystem.** gVisor and the Docker/containerd/Kubernetes stack are themselves written in Go.
- **Official SDKs exist.** `anthropic-sdk-go` (v1.84 builds fine with your code) and an OpenAI Go SDK.
- **Prior art in Go.** **Crush** (Charm) is a production Go coding agent worth reading. Its layout, per its AGENTS.md (https://github.com/charmbracelet/crush/blob/main/AGENTS.md):
  - a `SessionAgent` per session and a `Coordinator` for named agents
  - SQLite+sqlc persistence
  - a typed pub/sub broker between agent and UI
  - tools as `.go` files with a `.md` description each
  - Go-template system prompts
  - hooks that run before permission checks
  - its own provider-abstraction library, `fantasy`
  
  Its pub/sub-between-agent-and-UI design is essentially the in-process version of your client/server split.
- **Rust** is the other "efficient" option (Codex is ~1.1M lines of Rust). It offers more safety and control at a higher development cost. Given your existing Go code, Go is the pragmatic choice.

## 6. What I changed in your Go code (repo root)

Your `main.go` is a good start: it is the classic chat-loop skeleton. It had these gaps:
1. **Tools never ran.** `Run()` did one inference per user message and printed only text blocks. `tool_use` blocks were dropped, so no `tool_result` was ever sent back, and the API would reject the next request after the first tool call. **Fix:** an inner loop in `Agent.Send` that executes tools and appends `tool_result` blocks until the model stops calling tools.
2. **`ReadFileDefinition` was never registered**, and `tools` was an empty slice.
3. **`GenerateSchema` dropped `Required`**, so the model may omit arguments.
4. **`panic` on bad tool input** would crash the harness. Errors are now returned to the model as `is_error` results so it can correct itself.
5. **`MaxTokens: 1024`** is too small; long edits get cut off mid tool call. It is now 16k.
6. **New: an `Executor` interface with `LocalExecutor` and `DockerExecutor`.** This is the brain/hands seam. The edit logic runs in the harness, and only reads and writes cross the seam. It is the exact boundary you would later turn into a network call for the client–server version.
7. **Added:**
   - `bash`, `write_file` and `edit_file` tools (exact-unique replacement)
   - AGENTS.md loading
   - approval prompts, which `-yolo` turns off
   - a JSONL transcript (`-log`)
   - Ctrl-C cancellation through `context`

**Testing.** `go vet` is clean. I tested end to end against a mock Anthropic API in both modes:
- **Local:** the edit was applied.
- **Docker** (`alpine`, no network): the edit was applied inside the container, files stayed owned by the host user, and the container was removed on exit.

**Not tested:** against the real API, and with gVisor (`runsc` isn't installed here).

## 7. Suggested build path toward the client–server version
1. **Use the CLI as it is** with `-docker` on a real repo, with logging on. Learn the failure modes.
2. **Make the event log the source of truth.** Rebuild `conversation` from the JSONL/SQLite events instead of keeping it only in memory, then add `resume <session>`.
3. **Split the process.** Move `Agent` behind an HTTP server:
   - `POST /sessions`
   - `POST /sessions/{id}/messages`
   - `GET /sessions/{id}/events` (SSE with a cursor)
   - `POST /sessions/{id}/approvals/{reqId}`
   
   Turn the TUI into a client. Approvals become events.
4. **Change the sandbox lifecycle:**
   - clone the repo into the container at start, instead of bind-mounting
   - return results as a branch or diff
   - add a TTL reaper
   - provision lazily, on the first tool call
5. **Harden:** gVisor runtime, an egress proxy with an allowlist, secrets via credential helper or proxy, and per-session resource limits.
6. **Add more clients** (web, mobile) and, optionally, a local runner that implements `Executor` over an outbound connection.
7. **Go further:** multiple sessions in parallel, scheduled or triggered runs (the "loop engineering" direction), and model and provider abstraction.

## Confidence & gaps
- **High:** that vendors ship cloud modes, and the Managed Agents design rationale. Both come from primary vendor docs and engineering posts.
- **Medium:** isolation performance figures. These come from vendor blogs (Fly.io, Northflank), which have commercial interests. The qualitative ordering (container < gVisor < microVM) is uncontroversial.
- **Not verified from primary sources:**
  - the internals of Codex Cloud's isolation (OpenAI's docs say "container" in the legacy docs and "VM" in the current ones)
  - Claude Code on the web
  - Cursor background agents
- **Missing:** I found no primary source measuring user-experience trade-offs (local vs. cloud agents) such as review friction or developer preference. The "honest downsides" in §3 are my own reasoning, not measured findings.
