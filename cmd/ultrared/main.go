// A minimal coding-agent harness in Go, grown from your original main.go.
//
//	go run ./cmd/ultrared -dir ./myrepo                             # tools run on the host (no isolation!)
//	go run ./cmd/ultrared -dir ./myrepo -docker ultrared-sandbox    # tools run in a locked-down container
//	go run ./cmd/ultrared -dir ./myrepo -docker ultrared-sandbox -runtime runsc   # + gVisor
//	go run ./cmd/ultrared -dir ./myrepo -log events.jsonl           # also write the session's events
//
// Day to day, `task run` does this for you (see Taskfile.yml).
//
// Needs ANTHROPIC_API_KEY. ANTHROPIC_BASE_URL is honoured too.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/juharin/ultrared/internal/agent"
	"github.com/juharin/ultrared/internal/executor"
	"github.com/juharin/ultrared/internal/session"
	"github.com/juharin/ultrared/internal/tools"
)

func main() {
	dir := flag.String("dir", ".", "workspace directory")
	image := flag.String("docker", "", "run tools in a container from this image (empty = run on host)")
	runtime := flag.String("runtime", "", "container runtime, e.g. runsc for gVisor")
	network := flag.Bool("net", false, "give the container network access")
	yolo := flag.Bool("yolo", false, "do not ask before bash/write/edit")
	model := flag.String("model", string(agent.DefaultModel), "model id")
	logPath := flag.String("log", "", "append the session's events here, one JSON object per line")
	flag.Parse()

	ctx := context.Background()

	// The event log: one JSON event per line, the same events any client sees.
	// Opened before the container starts, so a bad path can't leak a container.
	var eventLog *json.Encoder
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, "event log:", err)
			os.Exit(1)
		}
		defer f.Close()
		eventLog = json.NewEncoder(f)
	}

	var ex executor.Executor = &executor.Local{Root: *dir}
	if *image != "" {
		d, err := executor.NewDocker(ctx, *image, *dir, *runtime, *network)
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
	ag := &agent.Agent{
		Client: &client, Model: anthropic.Model(*model), Exec: ex,
		Tools: tools.All(), MaxTurns: 60, SessionID: newSessionID(),
	}
	if !*yolo {
		ag.NeedsApproval = func(name string, _ json.RawMessage) bool { return name != "read_file" }
	}

	// The CLI is a client of the agent's event stream: it logs and renders
	// every event, and answers approval requests from the keyboard.
	ag.Emit = func(e session.Event) {
		if eventLog != nil {
			if err := eventLog.Encode(e); err != nil {
				fmt.Fprintln(os.Stderr, "event log:", err)
			}
		}
		render(e)
		if e.Type == session.ApprovalRequested {
			// The tool_call line just above shows the input.
			fmt.Printf("  \033[31mallow\033[0m %s? [y/N] ", e.Tool)
			ans, _ := readLine()
			ag.Resolve(e.ToolUseID, strings.TrimSpace(strings.ToLower(ans)) == "y")
		}
	}

	// Ctrl-C stops the running turn. At the prompt it only prints a hint, so the
	// deferred cleanup (e.g. removing the container) still runs on ctrl-d.
	var intr interrupter
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	defer signal.Stop(sigs)
	go func() {
		for range sigs {
			if !intr.interrupt() {
				fmt.Print("\n(ctrl-d to quit)\n\033[34mYou\033[0m: ")
			}
		}
	}()

	fmt.Printf("Session %s. Chat with Claude (ctrl-c stops a turn, ctrl-d quits)\n", ag.SessionID)
	for {
		fmt.Print("\033[34mYou\033[0m: ")
		in, ok := readLine()
		if !ok {
			return
		}
		if strings.TrimSpace(in) == "" {
			continue
		}
		turnCtx, done := intr.turn(ctx)
		err := ag.Send(turnCtx, in)
		done()
		if err != nil {
			fmt.Printf("Error: %s\n", err)
		}
	}
}

// render prints the events a person wants to see. user_message is what they
// just typed, and approval_resolved is their own answer, so both are skipped;
// turn_done errors are printed by the prompt loop.
func render(e session.Event) {
	switch e.Type {
	case session.AssistantText:
		fmt.Printf("\033[33mClaude\033[0m: %s\n", e.Text)
	case session.ToolCall:
		fmt.Printf("\033[36m→ %s %s\033[0m\n", e.Tool, truncate(string(e.Input), 300))
	case session.ToolResult:
		fmt.Printf("\033[90m  %s\033[0m\n", strings.ReplaceAll(truncate(e.Output, 200), "\n", "⏎"))
	}
}

// newSessionID returns a short random id, e.g. "3f9a1c0b2d4e".
func newSessionID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
