// WASM sandbox experiment host: runs guest.wasm under wazero (pure Go, no cgo)
// with ONE directory mounted read-only at /work, and nothing else.
//
//	cd experiments/wasm
//	GOOS=wasip1 GOARCH=wasm go build -o guest.wasm ./guest
//	echo hello > /tmp/wasmdemo.txt
//	go run ./host /tmp            # guest reads /work/wasmdemo.txt, cannot read /etc/passwd
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./host <dir-to-mount-at-/work>")
		os.Exit(2)
	}
	ctx := context.Background()
	wasm, err := os.ReadFile("guest.wasm")
	if err != nil {
		fmt.Fprintln(os.Stderr, "build the guest first: GOOS=wasip1 GOARCH=wasm go build -o guest.wasm ./guest")
		os.Exit(1)
	}

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	t0 := time.Now()
	mod, err := r.CompileModule(ctx, wasm) // one-time cost; cacheable with wazero.NewCompilationCache
	if err != nil {
		panic(err)
	}
	fmt.Println("compile:", time.Since(t0))

	for i := 0; i < 3; i++ {
		t := time.Now()
		cfg := wazero.NewModuleConfig().
			WithStdout(os.Stdout).WithStderr(os.Stderr).
			WithFSConfig(wazero.NewFSConfig().WithReadOnlyDirMount(os.Args[1], "/work")).
			WithName("") // empty name allows re-instantiation
		if _, err := r.InstantiateModule(ctx, mod, cfg); err != nil {
			fmt.Println("error:", err)
		}
		fmt.Println("instantiate+run:", time.Since(t))
	}
}
