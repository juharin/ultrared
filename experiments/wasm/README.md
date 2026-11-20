# WASM sandbox experiment (wazero)

This is a separate Go module, so the main harness doesn't depend on wazero. It needs Go ≥ 1.25: wazero's `go.mod` requires it, and `GOTOOLCHAIN=auto`, the default, downloads it automatically.

```sh
cd experiments/wasm
GOOS=wasip1 GOARCH=wasm go build -o guest.wasm ./guest   # build the guest for WASI
echo hello > /tmp/wasmdemo.txt
go run ./host /tmp                                       # mount /tmp read-only at /work
```

Expected output:
- the guest reads `/work/wasmdemo.txt`
- reading `/etc/passwd` fails, because the host filesystem doesn't exist for the guest
- compile takes about 1 s (one-time) and instantiate+run takes about 10 ms

Read `docs/04-wasm-evaluation.md` for what this does and doesn't prove, and for next experiments (`WasmExecutor`, code mode with QuickJS).
