# AI Coding Harnesses (Claude Code, Codex, OpenCode, Pi, …): State of the Field, Categories, and Directions

*Research date: 12 November 2026 (revised the same day: Terminal-Bench figures re-sourced from tbench.ai; Databricks benchmark re-sourced to the primary post). Method: targeted web searches. Wherever possible, claims are cited to primary sources: the arXiv source-code study, vendor documentation, and the project's own posts or READMEs. Where only secondary sources exist, the claim is labeled **[secondary]**.*

## Original question
What is the current state of AI coding harnesses such as Claude Code, OpenAI Codex, OpenCode and Pi? Can they be sorted into categories, and in which directions are they developing?

## Sub-questions
1. What is a "harness", and what do current harnesses have in common architecturally?
2. What are the main harnesses today, and how does each one position itself?
3. Can they be classified, and along which axes?
4. In which directions are they developing?
5. Where do sources disagree, and what remains unknown? (See the final section.)

---

## SQ1. What is a harness, and what do they share?

- **Definition.** "An agent is a model plus a harness: the runtime that couples an LLM to the world through a loop, tools, context management, safety controls, orchestration, and extension surfaces." The study divides a harness into seven subsystems: agent loop, LLM integration, tools, context management, safety/permissions, orchestration, and extensibility. Interface layers sit on top: TUI, IDE, SDK and server. — Barbaste et al., *Harness Engineering: Anatomy, Architecture, and Evolution of Coding Agents — A Source-Code Study of Eleven Systems*, arXiv:2609.00006 (submitted 15 July 2026). https://arxiv.org/abs/2609.00006 ; HTML: https://arxiv.org/html/2609.00006v1
- **Corpus.** Claude Code, Codex CLI, Gemini CLI, Mistral Vibe, OpenHands, Aider, Mini-SWE-Agent, Hermes, Pi, OpenCode and OpenClaw. The study also examines Omnigent (Databricks) as a "meta-harness" for comparison. (arXiv:2609.00006)
- **No frameworks, no embeddings.** "Across roughly four million lines of Python, TypeScript, and Rust, no agent runtime imports a general-purpose agentic framework, and none retrieves code with vector embeddings; the field runs on hand-rolled async loops and deterministic retrieval" (ripgrep, tree-sitter, glob, Markdown context files). (arXiv:2609.00006, abstract)
- **Loop complexity does not predict results.** The systems "span three orders of magnitude in code size … yet loop sophistication does not predict benchmark performance". For example, Mini-SWE-Agent, at about 100 lines, reports results in the same range as OpenHands. (arXiv:2609.00006, Observation 1, https://arxiv.org/pdf/2609.00006)
- **Caveats stated by the paper itself:**
  - It reads source code only. It does not run or rank the harnesses.
  - The Claude Code analysis uses a circulated source snapshot from March 2026, not an official release.
  - The paper discloses that it was written with substantial help from Claude.

## SQ2. The main harnesses and how they position themselves

| Harness | Owner / license | Positioning (primary source) |
|---|---|---|
| **Claude Code** | Anthropic, proprietary | Terminal agent that also has IDE, desktop and web surfaces. It runs non-interactively with `claude -p`. The **Claude Agent SDK** embeds "the same engine as the CLI" in TypeScript or Python apps. **Cloud scheduled tasks (routines)** run on Anthropic infrastructure; these are a research preview, can run at most hourly, and start from a fresh clone. **Managed Agents** is "a hosted agent harness that runs the agent loop" in an Anthropic-managed or self-hosted sandbox. Sources: Agent SDK overview https://code.claude.com/docs/en/agent-sdk/overview ; Anthropic Academy, *Scheduled tasks and headless* https://academy.claude.com/courses/claude-code-in-action/routines-and-headless |
| **OpenAI Codex** | OpenAI; the CLI is open source (Rust) | One agent across several surfaces: CLI, IDE extension, ChatGPT desktop app and cloud. Locally, "OS-level mechanisms enforce sandbox policies. Defaults include no network access and write permissions limited to the active workspace". The sandbox uses Seatbelt on macOS and bubblewrap on Linux, plus a native Windows sandbox. Sandbox mode and approval policy are separate controls. `codex exec` handles CI runs. Enterprise features include `requirements.toml`, permission profiles and OpenTelemetry export. Sources: https://developers.openai.com/codex/agent-approvals-security ; https://developers.openai.com/codex/concepts/sandboxing ; https://developers.openai.com/codex/cli/reference ; https://developers.openai.com/codex/config-reference |
| **OpenCode** | Anomaly (formerly SST), MIT | Model-agnostic terminal agent with a client/server architecture. The paper calls this "the corpus's most thoroughgoing client/server architecture" and identifies OpenCode as "the most-starred dedicated coding agent" (arXiv:2609.00006). From OpenCode's own docs:<br>- An agent "combines a system prompt, model preference, permissions, and display details into a named assistant profile", defined in JSON or Markdown. "Subagents run with fresh context in foreground or background child sessions".<br>- Permissions are ordered rules: "If no rule matches, OpenCode uses ask". However, the base policy for every agent begins with `"action": "*", "effect": "allow"`. Because of that, "reads and edits outside the Location and worktree proceed without a prompt" unless you configure otherwise.<br>- The shell tool "has the host user's filesystem, process, and network authority". This means OpenCode has **no OS sandbox**, only permission rules.<br>- MCP tools are reached by default through a **Code Mode** tool, `execute`, that "runs JavaScript … so the agent can call and combine tools".<br>- Configuration also covers skills, plugins, worktrees and organisation-wide `policies`.<br>Sources: https://opencode.ai/docs/agents/ ; https://opencode.ai/docs/permissions/ ; https://opencode.ai/docs/tools/ ; https://opencode.ai/docs/config/ |
| **Pi** | Mario Zechner, now Earendil Inc., MIT | Minimal-core harness. By default it has four tools (read, write, edit, bash) and a very short system prompt. Everything else is added through TypeScript extensions, skills and packages. It runs as an interactive TUI, in print/JSON mode, over RPC, or as an SDK. The README says "No sub-agents… No permission popups. Run in a container… No plan mode… No built-in to-dos." Pi 1.0 (1 October 2026) **added MCP through "Codemode"** and shipped an experimental **Pi Durable** package for long-running agents. Sources: Zechner, *What I learned building an opinionated and minimal coding agent* https://mariozechner.at/posts/2025-11-30-pi-coding-agent/ ; npm README https://www.npmjs.com/package/@mariozechner/pi-coding-agent ; Earendil, *Pi 1.0* https://earendil.com/posts/pi-1-0/ |
| Others mentioned in sources | — | Gemini CLI, which was replaced for consumers by the closed-source Antigravity CLI in June 2026 (arXiv:2609.00006). Also Mistral Vibe, OpenHands, Aider, Mini-SWE-Agent, Hermes and OpenClaw (all in arXiv:2609.00006), plus Cursor, Copilot CLI, Devin, Cline, Qwen Code and Kimi Code **[secondary]**. |

## SQ3. Classification

No single taxonomy fits, because the tools differ along several independent axes. The table below is my synthesis; the sources for each axis are given in the right-hand column.

| Axis | Categories | Examples / evidence |
|---|---|---|
| **1. Execution surface** | (a) terminal CLI; (b) IDE-embedded; (c) cloud/async runner that returns a PR; (d) desktop agent manager | Claude Code, Codex CLI, OpenCode and Pi are terminal-first. Cursor and Cline are IDE-based. Codex cloud, Claude routines, Devin and the Copilot cloud agent are cloud runners. The vendors' own docs confirm that one harness spans several surfaces:<br>- **Codex** applies the same sandbox "across the ChatGPT desktop app, Codex CLI, and IDE extension". It also has a separate cloud mode that "runs in isolated OpenAI-managed containers" (https://developers.openai.com/codex/agent-approvals-security).<br>- **Claude Code** offers a CLI, headless `-p` mode, the SDK, and cloud scheduled tasks "on Anthropic's infrastructure" (Anthropic Academy).<br>So this axis describes *deployment modes* that one product can offer, rather than separate product categories. The axis itself is my own synthesis from these docs. |
| **2. Model coupling** | (a) vendor-native, tied to the vendor's model and subscription; (b) model-agnostic / bring-your-own-key | (a) Claude Code, Codex, Antigravity CLI, Mistral Vibe, Qwen Code. (b) OpenCode, Pi, Aider, Cline. Pi's provider layer is described in Zechner's post. |
| **3. Design philosophy** | (a) "batteries-included" product; (b) minimal core plus extensions; (c) research-minimal | (a) Claude Code and Codex, with hooks, subagents, plan modes, plugins and skills. (b) Pi, which the paper calls "the minimal-core countertrend". (c) Mini-SWE-Agent, "the 100-line research floor". (arXiv:2609.00006) |
| **4. Integration role** | (a) end-user tool; (b) embeddable SDK/library; (c) gateway that delegates coding to another harness; (d) meta-harness orchestrating several harnesses | (b) Claude Agent SDK, Codex SDK, OpenHands SDK, Pi SDK/RPC. (c) OpenClaw, which runs on Pi; Hermes is a hybrid. (d) Omnigent. OpenHands can also host Claude Code, Codex or Gemini CLI as backends over ACP. (arXiv:2609.00006) |
| **5. Safety model** | (a) OS-enforced sandbox on by default; (b) permission rules / approval prompts; (c) none, with isolation left to the user | (a) **Codex.** OpenAI's docs say the default is OS-enforced, with no network access and writes limited to the workspace.<br>(a′) **Claude Code: OS sandbox available, but you have to turn it on.** Anthropic's sandboxing page is titled "Turn it on, set the boundary, and fix what it breaks". It says "The sandbox covers shell commands only. Claude's file tools, MCP servers, and hooks run outside it". Administrators can make it mandatory through managed settings (https://code.claude.com/docs/en/sandboxing ; Anthropic engineering post https://www.anthropic.com/engineering/claude-code-sandboxing). This confirms, from the primary source, the claim that the sandbox is "opt-in". Without it, Claude Code relies on permission rules and modes.<br>(b) **OpenCode.** Ordered permission rules only, and the shell runs with the host user's authority (OpenCode docs).<br>(c) **Pi.** "No permission popups. Run in a container" (Pi README). |

**Practical summary.** The most useful single split is **vendor-native products** (Claude Code, Codex) versus **open, model-agnostic harnesses** (OpenCode, Pi). Within the open group, the next split is **feature-rich** (OpenCode) versus **minimal-core** (Pi).

## SQ4. Directions of development

1. **From tool to platform.** This is the thesis of the paper: "in the first half of 2026 the coding harness completed a turn from tool to platform." The evidence it gives:
   - Harnesses have become importable SDKs, and framework vendors now ship their own harnesses.
   - Marketplaces, tools that lower switching costs, and enterprise governance layers have appeared.
   - The agent can now be addressed as a model behind an OpenAI-compatible endpoint.
   - A meta-harness orchestrates several vendor harnesses.
   
   In the paper's words, "The competitive unit of the field is no longer the agent loop; it is the ecosystem surface around it." (arXiv:2609.00006)
2. **Convergence, then imitation, and shared standards.**
   - Codex adopted Claude Code's hook vocabulary verbatim and ships an importer for Claude Code sessions and settings. OpenHands reads Claude Code's plugin format.
   - SKILL.md skills (9 of 11 systems) slightly lead MCP (8 of 11). ACP ships in 6 systems and now serves as a way for one harness to host another.
   - Behavioral rules are moving "from prompt prose to configuration".
   
   (arXiv:2609.00006)
3. **Minimalists are adopting the standards too, while keeping the core small.** Pi had said "No MCP"; Pi 1.0 adds MCP through Codemode. Long-running, multi-surface agent work was split out into **Pi Durable** "rather than stray from our minimal roots". (Earendil, https://earendil.com/posts/pi-1-0/)
4. **Tool calling by writing code, and loading tools only when needed.** In Pi's Codemode, the model writes a JavaScript script that calls tools inside a sandbox, and only the final output returns to the model's context. Pi 1.0 also defers tool loading (Earendil, https://earendil.com/posts/pi-1-0/). OpenCode independently uses the same pattern. Its `execute` tool "runs JavaScript in Code Mode so the agent can call and combine tools", and MCP tools are reached through it by default (https://opencode.ai/docs/tools/). Two unrelated harnesses converging on this design is primary-source evidence that it is a real trend. I dropped an earlier, secondary-only claim about Codex having a "code mode", because I could not confirm it in OpenAI's docs.
5. **Autonomy: background, scheduled and hosted agents.**
   - Claude Code has cloud scheduled tasks/routines, headless mode, and the Agent SDK. Managed Agents is Anthropic's hosted harness. (Anthropic docs/Academy)
   - Codex has cloud tasks and `codex exec` for CI. (OpenAI docs)
   - The paper names an emerging practice of "loop engineering": outer loops that prompt, verify and re-run agents. (arXiv:2609.00006)
6. **Sandboxing and enterprise governance.** Codex combines an OS-level sandbox, separate approval policies, permission profiles, network proxy allowlists, admin-enforced `requirements.toml` and OTel telemetry. (OpenAI docs: config-reference, permissions)
7. **Consolidation and partial closing of the source.** Google ended consumer Gemini CLI service on 18 June 2026. Its successor, Antigravity CLI, "ships as a closed-source Go binary". The paper also reports SpaceX/xAI acquisition news involving Cursor. Note that the paper states these market events are *not* source-verified. (arXiv:2609.00006)

## SQ5 / Confidence & gaps

**High confidence**
- The architectural anatomy, the "no frameworks / no embeddings" finding, the convergence and imitation trend, and the platform-turn thesis. These come from a peer-style source-code study (arXiv:2609.00006). It is a preprint and not yet peer-reviewed. Its Claude Code analysis uses a March snapshot, and it was written with help from Claude.
- Pi's design philosophy and the Pi 1.0 changes (Zechner's post, the README, Earendil's announcement).
- Codex's sandbox and approval model (OpenAI docs).
- Claude Code's headless mode, SDK, routines and Managed Agents (Anthropic docs/Academy).

**Medium or low confidence, and conflicting sources**
- **Benchmarks: now checked against the official board; it corrects the aggregators.** (Revision of 12 Nov 2026.) I fetched https://www.tbench.ai/leaderboard directly. Three findings:
  - The official board is now **Terminal-Bench 4.1**, not 4.0. Its data were refreshed on 3 Nov 2026 (`created_at` 2026-11-03).
  - Top harness+model rows (accuracy ± 95% CI):
    - Claude Code + Opus 5.5: **65.8% ± 3.1**
    - Claude Code + Sonnet 5.5: 63.9% ± 3.0
    - Codex + GPT-6.1 Sol: **60.9% ± 3.0**
    - Codex + GPT-6 Astra: 60.3% ± 2.8 (best of several runs)
    - Claude Code + Fable 5.1: 57.6% ± 3.3 (best run)
  - The aggregator figures (64.8% vs 58.2%) are therefore **stale or from the 4.0 board**. The current top gap is about 5 points with overlapping-adjacent intervals (62.7–68.9 vs 57.9–63.9). That is a weaker lead than the 7 points reported before. **Status:** primary source, high confidence for the numbers. Medium confidence for the per-row submission dates, because the page's date fields could not be reliably matched to rows.
  - **New evidence relevant to "harness vs. model":** the board contains Claude Code runs with *non-Anthropic* models: GLM-5.3 41.2%, GLM-5.3-Flash 35.5%, Qwen3.8-Max 27.0%. So the harness is held fixed and the model varies, which again shows that the model dominates the score.
  - **OpenCode and Pi do not appear on the current 4.1 board**, so it cannot compare the four harnesses this report focuses on. The Pi results quoted elsewhere refer to the older Terminal-Bench 2.0 board **[secondary: eesel.ai]**.
  - The caution about vendor-reported scores still applies: Anthropic's own 70.6% for Sonnet 5.5 is well above the board's 63.9%.
- **Adoption figures: the differences come from dates, not real disagreement.** OpenCode's star counts are about 171k (June, Firecrawl), about 196k (mid-2026, Tensorlake) and 199,640 (21 August, morphllm, which cites the GitHub API). Pi's are 65.9k (an undated Medium post), about 111.8k (1 October, aicoder.com) and about 115k (14 October, vdf.ai).
  - These are monotonic growth curves, so I take the **latest dated figure as most credible**: about 200k for OpenCode and about 115k for Pi as of August and October respectively. Both have probably grown since.
  - I tried to query the GitHub API directly, but this environment blocked it (HTTP 403), so these figures remain secondary.
  - The ranking itself, with OpenCode as the most-starred dedicated coding agent, is confirmed by the primary arXiv paper.
- **Claude Code's sandbox default: now resolved from the primary source.** Anthropic's docs confirm that the Bash sandbox must be turned on, and that it covers shell commands only (https://code.claude.com/docs/en/sandboxing). This is high confidence.
- **Databricks benchmark: primary source now found.** Gaba, Mathur, Singh, Wendell & Zaharia, *Benchmarking Coding Agents on Databricks' Multi-Million Line Codebase*, Databricks Blog, 8 Jul 2026, modified 20 Oct 2026. https://www.databricks.com/blog/benchmarking-coding-agents-databricks-multi-million-line-codebase
  - **Method.** Tasks come from recent human-written internal PRs, with held-out tests and no LLM judge. Harnesses ran in their out-of-the-box setups. Git history was sealed after the authors found that agents could recover the merged solution from it.
  - **Primary claims.**
    - "The harness a model is called from dramatically impacts cost and quality. In many cases, simple harnesses like Pi performed best on our workloads."
    - With the same model and effort, Claude Code/Codex vs. Pi showed cost per task differing "more than 2x in some cases, while quality remained the same".
    - "Pi sent about 3x less context per turn."
    - GLM 5.2 was "statistically tied with Opus 4.8 on quality" at $1.28 vs. $1.94 per task.
    - Sonnet 5 cost *more* per task than Opus 4.8 ($2.09 vs. $1.94) and scored lower (81% vs. 87%).
  - **Disagreement over what it shows.** Databricks' text says *same quality, lower cost*. Earendil, Pi's owner, reads the figure as Pi + Opus 4.8 xhigh having "the highest overall pass-rate" (https://earendil.com/posts/pi-autoresearch-and-databricks/). Secondary posts give 90% and $1.94 vs. ~$0.95. I could not verify those numbers from the text, because they are only in a chart image. **I treat the Databricks wording as authoritative.** Databricks also cautions that "the lesson here isn't that one harness is always cheaper or that native harnesses are worse".
  - **Conflicts of interest.** Databricks builds Omnigent, a meta-harness whose pitch is easy harness swapping. Earendil sells Pi. The study is a single internal codebase and is not reproducible externally.
  - **Corroboration** (independent but weaker, single-company): Fullscript Engineering reports that Pi resolved more tasks with about 43% fewer tokens, while Claude Code finished about 40% faster (https://builders.fullscript.com/posts/benchmarking-the-agents-that-write-our-code).
- **Claude Code's share of GitHub commits** (Medium, citing SemiAnalysis) is unverified and was excluded from the analysis.

**Coverage gaps**
- I did not read primary docs for Cursor, Copilot CLI, Devin, Antigravity CLI or the open-weight-lab harnesses (Qwen, Kimi, DeepSeek).
- IDE-native and cloud agents are covered less thoroughly than terminal harnesses.
- Fixed-model harness comparisons now exist (Databricks, Fullscript), but only on private codebases. No public, reproducible fixed-model harness benchmark covering Claude Code, Codex, OpenCode and Pi was found. Terminal-Bench 4.1 lacks OpenCode and Pi.
- GitHub star counts are still secondary, because the GitHub API returned HTTP 403 again on 12 Nov. eesel.ai reports earendil-works/pi at about 113k stars, which is consistent with the ~115k figure.
- Several comparison articles come from vendors with a commercial interest, such as Winder.AI and Alongside.
- The field changes weekly: Pi went from 0.99 to 1.2 in about two weeks, and Codex removed `--full-auto`. Specific version details will go stale quickly.
