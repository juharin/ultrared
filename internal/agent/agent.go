// Package agent is the "brain": the loop that calls the model, runs the tools it
// asks for through an executor, and feeds the results back.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/juharin/ultrared/internal/executor"
	"github.com/juharin/ultrared/internal/tools"
)

type Agent struct {
	Client   *anthropic.Client
	Model    anthropic.Model
	Exec     executor.Executor
	Tools    []tools.Definition
	Approve  func(name string, input json.RawMessage) bool // nil = allow everything
	MaxTurns int
	Log      io.Writer // optional JSONL transcript: your debugger and, later, your session store

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
func (a *Agent) Send(ctx context.Context, userText string, emit func(kind, text string)) error {
	a.conversation = append(a.conversation,
		anthropic.NewUserMessage(anthropic.NewTextBlock(userText)))

	for turn := 0; turn < a.MaxTurns; turn++ {
		msg, err := a.infer(ctx)
		if err != nil {
			return err
		}
		a.conversation = append(a.conversation, msg.ToParam())
		a.record("assistant", msg)

		var results []anthropic.ContentBlockParamUnion
		for _, block := range msg.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				emit("text", b.Text)
			case anthropic.ToolUseBlock:
				emit("tool", fmt.Sprintf("%s %s", b.Name, truncate(string(b.Input), 200)))
				out, isErr := a.runTool(ctx, b.Name, b.Input)
				emit("result", truncate(out, 200))
				results = append(results, anthropic.NewToolResultBlock(b.ID, out, isErr))
			}
		}
		if len(results) == 0 { // stop rule: model is done
			return nil
		}
		a.conversation = append(a.conversation, anthropic.NewUserMessage(results...))
		a.record("tool_results", results)
	}
	return fmt.Errorf("stopped after %d turns", a.MaxTurns)
}

func (a *Agent) runTool(ctx context.Context, name string, input json.RawMessage) (string, bool) {
	for _, t := range a.Tools {
		if t.Name != name {
			continue
		}
		if a.Approve != nil && !a.Approve(name, input) {
			return "denied by user", true
		}
		out, err := t.Function(ctx, a.Exec, input)
		if err != nil {
			return "ERROR: " + err.Error(), true
		}
		return out, false
	}
	return "ERROR: unknown tool " + name, true
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

func (a *Agent) record(kind string, v any) {
	if a.Log == nil {
		return
	}
	line, _ := json.Marshal(map[string]any{"kind": kind, "data": v})
	a.Log.Write(append(line, '\n'))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
