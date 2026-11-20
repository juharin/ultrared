# How Coding Harnesses Work: Architectures, Differences, and How to Build One

*12 Nov 2026. This is a companion to `01-landscape-report.md`.*

**Main source.** Barbaste et al., *Harness Engineering: Anatomy, Architecture, and Evolution of Coding Agents*, arXiv:2609.00006 (15 Jul 2026), https://arxiv.org/html/2609.00006v1. I read it in full. It is a source-code study and does not benchmark the systems.

**Caveats on the main source:**
- It is a preprint, so it has not been peer-reviewed.
- Its Claude Code analysis is based on a circulated source snapshot, not an official release.
- It was written with substantial help from Claude.

Section numbers below (§) refer to the paper.

---

## 1. How every harness works

Every harness runs the same loop:

```
messages = [system_prompt + project context, user_task]
loop:
    reply = LLM(messages, tool_schemas)
    if reply has no tool calls: done
    for each tool call: (check permission) -> execute -> append result to messages
    if context too big: compact
```

All eleven systems studied are variants of ReAct: "All eleven systems implement variations of the ReAct pattern, but the implementations differ radically in complexity, concurrency, and state management" (§6.1).

The paper splits a harness into **seven subsystems** around this loop, plus an interface layer (TUI, IDE, SDK, server). For each subsystem it gives the smallest and largest version it found (Table 1):

| Subsystem | What it does | Minimal form | Maximal form |
|---|---|---|---|
| Agent loop | Alternates inference and action; decides when to stop | Mini-SWE-Agent: a `while` loop over one bash tool | OpenHands: event-sourced log with parallel action batches |
| LLM integration | Builds the prompt; speaks provider APIs; handles caching and thinking | One LiteLLM call and one template | Hermes: 29 provider profiles. Codex: model catalog delivered from the server |
| Tools | What the agent can do, above all file editing | bash only | Claude Code: 43 typed tools with deferred loading. Codex: tool calls run as code in V8 |
| Memory & context | Rations the context window; persists knowledge | An unbounded message list | Codex: cross-session memory maintained by the agent |
| Safety | Decides what runs, what asks first, and what is forbidden | Step and cost limits | Codex: policy rules, an LLM reviewer and an OS sandbox |
| Orchestration | Sub-agents and links to other agents | None (Aider) | Claude Code: recursive sub-agents |
| Extensibility | Hooks, skills, plugins, MCP | Python protocols | Pi: everything is an extension. Codex: plugin marketplace |

**Key finding: the floor is low.** Mini-SWE-Agent implements all seven subsystems in about 100 lines. The paper says it "reports results on SWE-bench Verified in the same range as systems three orders of magnitude larger" (§2.4). These are self-reported figures. In the paper's view, what separates the floor from production systems "is not task completion but everything else": safety, recovery, cost management and extensibility.

**What no harness uses.** None of the systems uses an agent framework (LangChain and similar), and none retrieves code with vector embeddings. Instead they use ripgrep, glob, tree-sitter and Markdown context files.

## 2. Major differences between Claude Code, Codex, OpenCode and Pi

Most of the real differences sit in five places: tool design, file editing, context and compaction, safety, and extension and process architecture.

| | **Claude Code** | **Codex CLI** | **OpenCode** | **Pi** |
|---|---|---|---|---|
| Language and size (§5, Table 4) | TypeScript, large | Rust, very large (~1.1M lines) | TypeScript, large; about three-fifths of the code is clients (TUI, web, desktop, SDK) | TypeScript, medium |
| Models | Anthropic | OpenAI | Many providers; one of nine base prompts is chosen by model family | Many (35 providers) |
| Tools | 43 typed tools. Each declares its own validation, permission check, concurrency safety and UI rendering. Most are *deferred*: hidden until the model looks them up with `ToolSearchTool` (§8.2–8.3) | 25–30 tools as Rust trait objects; deferred tools are found with a BM25 `tool_search`; "code mode" | 17 tools; the tool set changes per model (GPT models get Codex's `apply_patch` instead of edit/write); MCP is reached through one `execute` code tool | 7 tools by default, but read/write/edit/bash is the documented core. Each tool has a pluggable `Operations` interface, so SSH or container backends can move execution elsewhere |
| File editing (§8.4, Table 7) | Exact string replacement; the old string must be unique | Custom patch format (`*** Begin Patch`, diff hunks) | Per model: patch format for GPT, otherwise search/replace with a 9-stage fuzzy fallback (Levenshtein 0.65) | Exact replacement with several edits per call; only Unicode and whitespace are normalized |
| Context and compaction (§9) | Compacts at a ~13K-token buffer before the limit, then re-reads files | Server-side compaction plus a cross-session memory pipeline | Incremental "anchored" summaries; estimates size as chars/4 with no tokenizer | Session is an **append-only JSONL tree** you can fork and rewind; summaries are merged incrementally; a hook can replace compaction |
| Safety (§10) | Three layers: static hook rules → LLM permission classifier → user dialog. OS sandbox is opt-in | Four layers: Starlark exec-policy rules → hooks → "Guardian" LLM reviewer → native sandbox on three OSes (Seatbelt, bubblewrap, Windows) | Ordered permission rules (last match wins), with syntax-aware parsing of commands. No sandbox | None by design ("run it in a container") |
| Process architecture | CLI, with SDK and cloud variants | CLI/TUI plus an app-server | **Client/server**: the agent is a server, and the TUI, web and desktop apps are clients | Small core; interactive mode, JSON/print mode, RPC and SDK; everything else is an extension |

**Convergence.** The paper documents three kinds of copying between harnesses (§10.2, §14.5):
- Codex's hook events are "near-verbatim Claude Code's" (`PreToolUse`, `PostToolUse`, `Stop`, …).
- Both flagships now use an LLM to review risky actions.
- Deferred tool loading spread from Claude Code to Codex, Hermes and OpenCode.

**Correction to the earlier report.** The earlier report dropped the claim that Codex has a "code mode", because I couldn't find it in OpenAI's docs. The source study confirms it: tool calls are executed as code in V8, and the tool runtime is shared with code mode (§8.2). The claim now rests on the paper, not on OpenAI's docs.

**Prompt philosophy.**
- Pi's whole system prompt comes from a builder of about 170 lines and is 30–40 lines long. "Almost all behavioral policy is deliberately absent, delegated to user-supplied context files and extensions" (§7.2).
- OpenCode picks one of nine prompts by matching the model id.
- Codex receives its prompts per model from the server.

**Does this matter for results?** It matters mostly for **cost, not quality.** In Databricks' benchmark, the same model in Claude Code/Codex vs. Pi showed costs differing "more than 2x in some cases, while quality remained the same", because "Pi sent about 3x less context per turn" (https://www.databricks.com/blog/benchmarking-coding-agents-databricks-multi-million-line-codebase). On the public Terminal-Bench 4.1 board, the model drives most of the score; see the main report.

## 3. Building one yourself: where to start

### Step 0: Read two small codebases before writing anything
1. **Mini-SWE-Agent** (https://github.com/SWE-agent/mini-swe-agent). This is the ~100-line "floor": the whole idea in one sitting.
2. **Pi** (`@mariozechner/pi-coding-agent`; GitHub org `earendil-works`), together with Zechner's design post: https://mariozechner.at/posts/2025-11-30-pi-coding-agent/. Pi is the smallest *production* harness, and its post explains why each feature was left out.

Then use the arXiv paper as a reference manual. §16, "A Practitioner's Guide", gives 18 recommendations and includes a 90-line Python scaffold.

### Step 1: Run the provided starter (Go, in the repo root)
> *Updated during packaging. The original Python starter was replaced by the Go harness in this repo, because the project uses Go.*

The Go harness in the repo root contains the elements this guide describes:
- the loop with stop rules
- four tools: `bash`, `read_file`, `write_file`, and `edit_file` (exact, unique match)
- AGENTS.md loading
- approval prompts
- a JSONL transcript
- an `Executor` interface with local and Docker back-ends

`go test ./...` runs it end to end against a fake API. See the repo `README.md`.

**Use `-docker` when working on real code.** The local executor has no sandbox.

### Step 2: Extend it in this order, following the paper's §16 recommendations
Each step teaches one subsystem. Add a feature when you hit the failure it fixes, not before (Rec. 3).

1. **Watch it fail.** Log every request and response to a JSONL file. This is your debugger, and later your session store.
2. **File editing** (Rec. 5). Count how often edits fail. Add better error messages first: say whether the string was not found or matched more than once. Try a fuzzy fallback only if you use weaker or open models. Avoid line-number-based edits.
3. **Search tools.** Add `grep` (ripgrep) and `glob` once bash+find becomes clumsy. Don't build embedding search (Rec. 8, 16).
4. **Context** (Rec. 6–7). Use real token counts. Compact at a fixed buffer below the limit. Keep the most recent messages word for word, and merge each new summary into the previous one. Then try Pi's idea: store the session as a tree so you can fork and rewind.
5. **Safety** (Rec. 9–11). Add allow/ask/deny rules stored as data, not code, then modes (plan, default, auto). Keep a floor of forbidden commands even in auto mode. For real isolation, wrap bash in bubblewrap on Linux or `sandbox-exec` on macOS, as Codex does.
6. **Streaming, cancellation, limits.** Stream tokens, handle Ctrl-C, and add cost and turn caps. Add a cheap "same call three times" loop detector, like OpenCode's (Rec. 18).
7. **Extensibility.** Add hooks (before/after a tool runs), then skills (`SKILL.md` folders listed by name and description, with the body loaded on demand), and only then MCP (Rec. 14).
8. **Sub-agents** come last. Stay single-agent until you can point to parallel exploration that clearly helps. Multi-agent setups use ~15× more tokens (Rec. 12).
9. **Optional platform features.** An ACP server lets editors and other harnesses drive yours (Rec. 13). You can also split your harness into client and server, as OpenCode does.

### Step 3: Measure, or you're only guessing
- Build 10–20 tasks from your own repositories' merged PRs and grade them with the held-out tests. This is the Databricks method, scaled down.
- Record pass rate **and** tokens per task.
- Seal the git history so the agent can't look up the answer, as Databricks did.
- Compare your harness against Pi or Mini-SWE-Agent using the same model.

### Design choices to make consciously

| Choice | Options | Trade-off |
|---|---|---|
| Language | Go (chosen for this project; see doc 03 §5), Python (fastest to learn), or TypeScript (Pi, OpenCode, Claude Code ecosystem) | This project uses Go. Read the TypeScript harnesses for ideas, and Crush for idiomatic Go |
| Provider layer | Raw HTTP to one API, or a multi-provider shim | Start with one; per-provider quirks (caching, thinking) are where the complexity grows (Rec. 2) |
| Frameworks | Don't use one (Rec. 15) | If you want batteries included, the harness SDKs (Claude Agent SDK, Codex SDK, OpenHands SDK) are the modern "framework layer" |

## 4. Confidence & gaps
- **High confidence:** the architecture descriptions. They are from a single source-code study, but it is detailed and cites file names. The specifics will go stale as the codebases change: snapshots are from April and July 2026.
- **Not independently verified:**
  - Claude Code internals come from an unofficial source snapshot.
  - Tool counts (43, 25–30, 17, 7) are the paper's numbers as of July 2026.
- **The starter code** was tested only against a mock server, not a real model.
- **Not covered:** IDE-native agents (Cursor, Copilot) and cloud agents, which are architecturally different because they run remotely and return a PR.
