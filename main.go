// A minimal coding-agent harness in Go, grown from your original main.go.
//
//	go run . -dir ./myrepo                     # tools run on the host (no isolation!)
//	go run . -dir ./myrepo -docker golang:1.24 # tools run in a locked-down container
//	go run . -dir ./myrepo -docker golang:1.24 -runtime runsc   # + gVisor
//
// Needs ANTHROPIC_API_KEY. ANTHROPIC_BASE_URL is honoured too.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

func main() {
	dir := flag.String("dir", ".", "workspace directory")
	image := flag.String("docker", "", "run tools in a container from this image (empty = run on host)")
	runtime := flag.String("runtime", "", "container runtime, e.g. runsc for gVisor")
	network := flag.Bool("net", false, "give the container network access")
	yolo := flag.Bool("yolo", false, "do not ask before bash/write/edit")
	model := flag.String("model", string(DefaultModel), "model id")
	logPath := flag.String("log", "", "append a JSONL transcript here")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var ex Executor = &LocalExecutor{Root: *dir}
	if *image != "" {
		d, err := NewDockerExecutor(ctx, *image, *dir, *runtime, *network)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer d.Close()
		ex = d
		fmt.Printf("sandbox container %s (network=%v)\n", d.ID[:12], *network)
	} else {
		fmt.Println("WARNING: tools run directly on this machine. Use -docker for isolation.")
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	readLine := func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		return scanner.Text(), true
	}

	client := anthropic.NewClient()
	agent := &Agent{
		client: &client, model: anthropic.Model(*model), exec: ex,
		tools: Tools(), maxTurns: 60,
	}
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			agent.log = f
			defer f.Close()
		}
	}
	if !*yolo {
		agent.approve = func(name string, input json.RawMessage) bool {
			if name == "read_file" {
				return true
			}
			fmt.Printf("\033[31mallow\033[0m %s %s ? [y/N] ", name, truncate(string(input), 300))
			ans, _ := readLine()
			return strings.TrimSpace(strings.ToLower(ans)) == "y"
		}
	}

	emit := func(kind, text string) {
		switch kind {
		case "text":
			fmt.Printf("\033[33mClaude\033[0m: %s\n", text)
		case "tool":
			fmt.Printf("\033[36m→ %s\033[0m\n", text)
		case "result":
			fmt.Printf("\033[90m  %s\033[0m\n", strings.ReplaceAll(text, "\n", "⏎"))
		}
	}

	fmt.Println("Chat with Claude (ctrl-d to quit)")
	for {
		fmt.Print("\033[34mYou\033[0m: ")
		in, ok := readLine()
		if !ok {
			return
		}
		if strings.TrimSpace(in) == "" {
			continue
		}
		if err := agent.Send(ctx, in, emit); err != nil {
			fmt.Printf("Error: %s\n", err)
		}
	}
}
