// Package agent is the "brain": the loop that calls the model, runs the tools it
// asks for through an executor, and feeds the results back.
//
// The loop never talks to a user directly. It reports everything as
// session.Events through Emit, and when a tool needs approval it emits an
// approval_requested event and waits until some client calls Resolve. That
// keeps it usable from any client: the CLI now, a server and apps later.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/juharin/ultrared/internal/executor"
	"github.com/juharin/ultrared/internal/session"
	"github.com/juharin/ultrared/internal/tools"
)

type Agent struct {
	Client    *anthropic.Client
	Model     anthropic.Model
	Exec      executor.Executor
	Tools     []tools.Definition
	MaxTurns  int
	SessionID string // stamped on every event

	// Emit receives every event, in order, on the goroutine running Send.
	// It may call Resolve. nil drops events.
	Emit func(session.Event)
	// NeedsApproval decides whether a tool call must be approved before it
	// runs. nil means nothing needs approval.
	NeedsApproval func(tool string, input json.RawMessage) bool

	mu      sync.Mutex
	seq     int64
	pending map[string]chan bool // tool_use_id -> answer, while waiting for approval

	conversation []anthropic.MessageParam
}

const systemPrompt = `You are a coding agent working in a repository at the workspace root.
Inspect before you edit (read_file, bash with ls/rg). Make small, exact edits with edit_file.
Verify your work by running the project's tests or build. When finished, reply with a short
summary and no tool calls.`

// Send runs one user turn: it calls the model, executes tools, feeds results
// back, and repeats until the model answers without tool calls.
// Your original Run() did one inference per user message and dropped
// tool_use blocks, so tools never ran. This inner loop is the missing piece.
// The turn always ends with a turn_done event, carrying the error if any.
func (a *Agent) Send(ctx context.Context, userText string) (err error) {
	a.emit(session.Event{Type: session.UserMessage, Text: userText})
	defer func() {
		done := session.Event{Type: session.TurnDone}
		if err != nil {
			done.Error = err.Error()
		}
		a.emit(done)
	}()

	a.conversation = append(a.conversation,
		anthropic.NewUserMessage(anthropic.NewTextBlock(userText)))

	for turn := 0; turn < a.MaxTurns; turn++ {
		msg, err := a.infer(ctx)
		if err != nil {
			return err
		}
		a.conversation = append(a.conversation, msg.ToParam())

		var results []anthropic.ContentBlockParamUnion
		for _, block := range msg.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				a.emit(session.Event{Type: session.AssistantText, Text: b.Text})
			case anthropic.ToolUseBlock:
				a.emit(session.Event{Type: session.ToolCall, ToolUseID: b.ID, Tool: b.Name, Input: b.Input})
				out, isErr := a.runTool(ctx, b.ID, b.Name, b.Input)
				a.emit(session.Event{Type: session.ToolResult, ToolUseID: b.ID, Tool: b.Name, Output: out, IsError: isErr})
				results = append(results, anthropic.NewToolResultBlock(b.ID, out, isErr))
			}
		}
		if len(results) == 0 { // stop rule: model is done
			return nil
		}
		a.conversation = append(a.conversation, anthropic.NewUserMessage(results...))
	}
	return fmt.Errorf("stopped after %d turns", a.MaxTurns)
}

func (a *Agent) runTool(ctx context.Context, id, name string, input json.RawMessage) (string, bool) {
	for _, t := range a.Tools {
		if t.Name != name {
			continue
		}
		if a.NeedsApproval != nil && a.NeedsApproval(name, input) {
			if ok, why := a.awaitApproval(ctx, id, name, input); !ok {
				return why, true
			}
		}
		out, err := t.Function(ctx, a.Exec, input)
		if err != nil {
			return "ERROR: " + err.Error(), true
		}
		return out, false
	}
	return "ERROR: unknown tool " + name, true
}

// awaitApproval emits approval_requested and blocks until a client calls
// Resolve or the turn is cancelled. It reports whether the tool may run and,
// if not, why.
func (a *Agent) awaitApproval(ctx context.Context, id, name string, input json.RawMessage) (bool, string) {
	answer := make(chan bool, 1) // buffered: Resolve never blocks, even when called from inside Emit
	a.mu.Lock()
	if a.pending == nil {
		a.pending = make(map[string]chan bool)
	}
	a.pending[id] = answer
	a.mu.Unlock()

	a.emit(session.Event{Type: session.ApprovalRequested, ToolUseID: id, Tool: name, Input: input})

	allow, why := false, "denied by user"
	select {
	case allow = <-answer:
	case <-ctx.Done():
		a.mu.Lock()
		delete(a.pending, id)
		a.mu.Unlock()
		why = "interrupted while waiting for approval"
	}
	a.emit(session.Event{Type: session.ApprovalResolved, ToolUseID: id, Approved: &allow})
	return allow, why
}

// Resolve answers a pending approval request. Any client may call it, from
// any goroutine, including from inside Emit. The first answer wins; it is an
// error if nothing is waiting for that tool_use_id.
func (a *Agent) Resolve(toolUseID string, allow bool) error {
	a.mu.Lock()
	answer, ok := a.pending[toolUseID]
	delete(a.pending, toolUseID)
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("no pending approval for %s", toolUseID)
	}
	answer <- allow
	return nil
}

// emit numbers and stamps an event and hands it to Emit. The lock is not held
// while Emit runs, so Emit may call Resolve.
func (a *Agent) emit(e session.Event) {
	a.mu.Lock()
	a.seq++
	e.Seq = a.seq
	a.mu.Unlock()
	e.Session = a.SessionID
	e.Time = time.Now().UTC()
	if a.Emit != nil {
		a.Emit(e)
	}
}

func (a *Agent) infer(ctx context.Context) (*anthropic.Message, error) {
	params := make([]anthropic.ToolUnionParam, 0, len(a.Tools))
	for _, t := range a.Tools {
		params = append(params, anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: t.InputSchema,
		}})
	}
	return a.Client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     a.Model,
		MaxTokens: 16_000, // 1024 is too small: long edits get cut off mid tool call
		System:    []anthropic.TextBlockParam{{Text: systemPrompt + loadAgentsMD(ctx, a.Exec)}},
		Messages:  a.conversation,
		Tools:     params,
	})
}

func loadAgentsMD(ctx context.Context, ex executor.Executor) string {
	b, err := ex.ReadFile(ctx, "AGENTS.md")
	if err != nil || len(b) == 0 {
		return ""
	}
	return "\n\n<project_context file=\"AGENTS.md\">\n" + string(b) + "\n</project_context>"
}
