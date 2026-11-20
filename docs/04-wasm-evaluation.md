# Should the agent's execution run in WebAssembly? An honest evaluation

*12 Nov 2026. This is the fourth document in the series. It follows `03-architecture-client-server.md`.*

## Bottom line
**As the *main* executor for a coding agent: no.** WASM is safe and starts fast, but it can't run what a coding agent actually needs to run: your project's real toolchain (`go build`, `go test`, `npm`, `cargo`, `git`, and arbitrary shell pipelines). The workarounds either recreate a Linux machine inside WASM, which is slow and experimental, or lock you into one vendor's POSIX extension. Startup speed is also a smaller advantage than it looks, because the LLM dominates latency.

**As a *second, narrower* sandbox inside your harness: yes, this is worth doing and fits your Go design well.** The good uses are:
1. **"Code mode":** the model writes a script that combines tools, and you run it in-process.
2. **Plugins and extensions** that get capabilities only by explicit grant.
3. **Deterministic helper tools** compiled to WASM.

The Go runtime **wazero** (pure Go, no cgo) makes this almost free to add.

---

## 1. What WASM gives you, precisely
- **Isolation by construction.** WebAssembly has no built-in syscalls, filesystem or network. As one sandboxing field guide puts it, WebAssembly "defines no built-in syscall, filesystem, or network API. Interaction with the environment comes through imports and other resources supplied by the host." The same author adds that this "makes a restricted capability policy possible; it does not make every embedding secure by default" (Luis Cardoso, *A field guide to sandboxes for AI*, https://www.luiscardoso.dev/blog/sandboxes-for-ai, secondary). In WASI, files are reachable only through directories you explicitly pre-open.
- **Fast startup, measured here.** I measured this on this machine, which is noisy (cgroup v1, a Docker daemon inside a VM), so read the numbers as orders of magnitude:

| Operation | Time |
|---|---|
| Fresh `docker run --rm alpine true` | ~550 ms |
| `docker exec` into an already-running container | ~85 ms per call |
| wazero: compile a 2.5 MB Go-built WASI module (one-time, cacheable) | ~1 s |
| wazero: instantiate + run that module | **~8–10 ms** (38 ms on the first run) |
| Plain local fork/exec | ~1.3 ms |

  The capability model also held. The guest read the mounted file under `/work`, but `/etc/passwd` failed with "Bad file number", because nothing outside the mount exists for it (`experiments/wasm/`).

## 2. Why it doesn't work as the main executor

### 2.1 Your toolchains don't run in it
A coding agent's main loop is "edit, then run the build or tests, then read the errors". That needs native compilers, interpreters, package managers, git and a shell that can start processes.
- **Plain WASI cannot spawn processes.** Wasmer's own docs say that "Go's wasip1 port is single-threaded WASI, not WASIX. Programs that need sockets or subprocesses in Go need a different design or language" (https://docs.wasmer.io/runtime/wasix/). This means `go build` can't run inside WASI, and neither can your Go harness's `bash` tool.
- **WASIX** (Wasmer) adds fork/exec, sockets and threads, and bash, curl and CPython run under it. However:
  - "Currently, the only runtime that supports WASIX is Wasmer" (https://www.wasix.org/docs/). That is vendor lock-in, and Wasmer's runtime is not pure Go, so you'd need cgo or a subprocess.
  - Every tool must be **recompiled for WASIX**. The WASIX package catalogue is growing, but it is nowhere near "anything apt or npm can install".
  - The standard WASI track is unlikely to adopt fork. A comparison site argues that "fork() conflicts directly with Component Model philosophy" (https://wasmruntime.com/en/blog/wasi-preview2-vs-wasix-2026, secondary).

### 2.2 The "full Linux in WASM" route throws away the speed advantage
**container2wasm** converts an ordinary container image into a `.wasm` file. It works by "compil[ing] a CPU emulator to WebAssembly, boot[ing] a real Linux kernel inside that emulator, and start[ing] the container with runc" (Bochs for x86_64, TinyEMU for RISC-V). Its README calls it "experimental software" and says non-x86_64/RISC-V images are "slow because of additional emulation" (https://github.com/container2wasm/container2wasm).
- The build defaults are 1 vCPU and 128 MB of guest memory (Dockerfile args, as reported by a secondary review).
- Every instruction is emulated, so a `go test ./...` or `npm install` would be far slower than in Docker.
- The result is the safety of WASM plus the semantics of Linux, but with the performance of an emulator. For a coding agent that compiles and tests constantly, this is the wrong trade.

The experimental "Linux kernel compiled natively to WASM" projects avoid emulation. They still require rebuilding all user-space programs, and early users report crashes (secondary coverage). They are research toys today.

### 2.3 Fast startup is a small advantage here
Rough numbers for a coding session:
- **Model latency:** 50–200 model turns at 2–20 s each, so minutes of model time.
- **Container start:** paid **once per session**, and you can pre-warm a pool. This is also why Anthropic provisions sandboxes lazily (previous report).
- **Per-call overhead:** `docker exec` adds about 85 ms per tool call on this box; WASM would cut that to about 10 ms. Over 100 tool calls that saves about 7–8 s per session, perhaps 1–3% of wall time. The real cost per call is usually the work itself (compiling, testing), which WASM doesn't speed up and often slows down.
- **A better fix for exec overhead:** run a tiny persistent executor process *inside* the container, as OpenHands' action-execution server does, and talk to it over a socket. That reduces per-call overhead to near zero without changing sandbox technology.

**Where startup speed does matter:** thousands of short, independent executions, as in serverless or edge platforms and many-tenant code interpreters. That isn't your workload.

### 2.4 "Should be quite safe" is true only with caveats
- **The boundary is the runtime plus your host imports.** Every host function you expose is attack surface. Give the guest a "run shell command" import and you have rebuilt the original problem.
- **Real-world failure.** At DEF CON 34 (August 2026), Cyera researchers presented **seven Pyodide sandbox escapes** across AI-agent runtimes and other products, four with CVEs scored 8.3–9.9. Examples include smolagents CVE-2026-10613 and Grist CVE-2026-24002. The pattern was that products relied on blocklisting Python names while Emscripten and ctypes bridges to the host stayed reachable (Obiguard summary, https://obiguard.ai/news/pyodide-sandbox-escapes-ai-code-execution; **secondary**, and I did not read the talk itself). The lesson is not that WASM is unsafe. It is that **"runs in WASM" is not the same as "is sandboxed"**: the embedding decides.
- **Compared with your current plan.** A hardened container plus gVisor already gives a strong boundary *and* runs your real toolchain. WASM's isolation is excellent, but it is not *needed* here, and it costs you compatibility.

## 3. Where WASM *is* a good fit in your harness

### 3.1 Code mode (the best use)
The field is moving toward letting the model write a short script that calls and combines tools, so that only the final result enters the model's context:
- Codex runs "tool calls as V8-executed code".
- OpenCode's `execute` tool "runs JavaScript in Code Mode".
- Pi 1.0 added Codemode.

These are cited in the earlier reports. The script runs in a sandbox whose *only* capabilities are the tool functions you expose.

In Go you could do this as follows:
- Run **QuickJS compiled to WASM under wazero**, in-process, with millisecond startup and a memory cap.
- Export host functions like `read(path)`, `grep(pattern)` and `edit(...)`, each routed through your existing `Executor` and permission policy.

This is the WASM sweet spot: short, untrusted, compute-only code with an explicit capability list. Note that QuickJS can enforce deadlines via an interrupt handler, while Pyodide cannot easily interrupt a running loop (FoundrySoft tutorial, secondary). This is another reason to choose JS for code mode.

### 3.2 Plugins and extensions
Your harness will eventually want user extensions: custom tools and hooks. Native Go plugins are fragile, and subprocess plugins get full user privileges. WASM plugins under wazero:
- are **deny-by-default**: a plugin that needs the network must be granted it;
- are portable (one `.wasm` works on every OS);
- can be written in any language.

Microsoft's **Wassette** applies the same idea to MCP tools: a Wasmtime runtime running WebAssembly components, with a deny-by-default permission system (Zylos summary, secondary).

### 3.3 Deterministic helper tools
Pure-computation tools can run as WASM modules with read-only mounts:
- tree-sitter parsing and symbol extraction
- formatters
- diff or patch application
- linters that don't spawn processes

This is safe even on the *local* executor path.

### 3.4 Client-side execution (optional)
Your web client could run small previews or visualizations in the browser's WASM sandbox, as in NVIDIA's Pyodide pattern (https://developer.nvidia.com/blog/sandboxing-agentic-ai-workflows-with-webassembly/). This is a niche use and keeps such code off your server.

## 4. Recommended layering

```
 server (Go)
  ├─ agent loop
  ├─ wazero (in-process, ms startup)        ← code-mode scripts, plugins, pure tools
  │     capabilities = explicit host funcs → policy → Executor
  └─ Executor → container per session      ← bash, builds, tests, git (real toolchains)
                (hardened Docker → gVisor)
```

The two sandboxes do different jobs:
- **WASM** contains *code the model writes to orchestrate tools*.
- **The container** contains *the project's own programs*.

## 5. If you want to try it anyway (learning experiments, in order)
1. **Already done:** `experiments/wasm/` contains a Go WASI guest under a wazero host, a read-only mount, and proof that `/etc/passwd` is unreachable. See `experiments/wasm/README.md` to run it.
2. **Add a `WasmExecutor`** that implements only `ReadFile`/`WriteFile` against a mounted directory, and have `Run` return "unsupported". This makes the limits concrete: watch the agent fail the moment it wants to run tests.
3. **Build code mode:** QuickJS-WASM in wazero, exposing 3–4 host functions, offered to the model as one `execute` tool. Compare token use with and without it on the same tasks.
4. **Optional:** convert your dev image with container2wasm and time `go test` against Docker, to see the emulation cost for yourself.

## Confidence & gaps
- **High:** WASI's lack of subprocesses (Wasmer's own docs), WASIX being Wasmer-only (WASIX docs), and container2wasm's emulator design and experimental status (its README).
- **High for this machine only:** the timings in §1. They came from one run on a noisy VM. Redo them on your hardware before relying on them.
- **Medium/secondary:** the Pyodide CVE details (from a news summary), Wassette details, and QuickJS/Pyodide interruption behaviour. I didn't verify these against primary sources. The security lesson is well established regardless.
- **Not found:** any production coding agent that uses WASM as its *main* execution environment for builds and tests. The WASM uses I found are all code interpreters, plugins, or browser-side execution. This absence fits the evaluation above, but it isn't proof.
