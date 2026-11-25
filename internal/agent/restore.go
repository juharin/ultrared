package agent

import (
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/juharin/ultrared/internal/session"
)

// Restore rebuilds the conversation from a session's events, so the session
// can continue where it stopped: later events keep the session id and carry
// on with the next seq and step. Call it before the first Send.
//
// The events hold everything the history needs: user text, the model's text
// and tool calls (grouped into messages by their step), and tool results. A
// tool call without a recorded result (the session stopped while the tool ran
// or waited for approval) gets an error result, because the API rejects a
// tool_use that has no tool_result.
func (a *Agent) Restore(events []session.Event) error {
	if len(a.conversation) > 0 {
		return errors.New("restore: the agent already has a conversation")
	}
	if len(events) == 0 {
		return errors.New("restore: no events")
	}
	id := events[0].Session
	if a.SessionID != "" && a.SessionID != id {
		return fmt.Errorf("restore: events are from session %q, the agent is %q", id, a.SessionID)
	}

	var (
		conv    []anthropic.MessageParam
		lastSeq int64
		maxStep int64

		// The assistant message being rebuilt, and its tool calls.
		step    int64 = -1
		blocks  []anthropic.ContentBlockParamUnion
		calls   []string
		results = map[string]anthropic.ContentBlockParamUnion{}
	)
	flush := func() {
		if len(blocks) == 0 {
			return
		}
		conv = append(conv, anthropic.NewAssistantMessage(blocks...))
		if len(calls) > 0 {
			rs := make([]anthropic.ContentBlockParamUnion, 0, len(calls))
			for _, c := range calls {
				r, ok := results[c]
				if !ok {
					r = anthropic.NewToolResultBlock(c, "ERROR: no result was recorded; the session stopped before this tool finished", true)
				}
				rs = append(rs, r)
			}
			conv = append(conv, anthropic.NewUserMessage(rs...))
		}
		step, blocks, calls = -1, nil, nil
		results = map[string]anthropic.ContentBlockParamUnion{}
	}

	for i, e := range events {
		if e.Session != id {
			return fmt.Errorf("restore: event %d is from session %q, not %q", i+1, e.Session, id)
		}
		if e.Seq <= lastSeq {
			return fmt.Errorf("restore: event %d has seq %d, after seq %d", i+1, e.Seq, lastSeq)
		}
		lastSeq = e.Seq
		maxStep = max(maxStep, e.Step)

		switch e.Type {
		case session.UserMessage:
			flush()
			conv = append(conv, anthropic.NewUserMessage(anthropic.NewTextBlock(e.Text)))
		case session.AssistantText, session.ToolCall:
			if e.Step != step {
				flush()
				step = e.Step
			}
			if e.Type == session.AssistantText {
				blocks = append(blocks, anthropic.NewTextBlock(e.Text))
			} else {
				blocks = append(blocks, anthropic.NewToolUseBlock(e.ToolUseID, e.Input, e.Tool))
				calls = append(calls, e.ToolUseID)
			}
		case session.ToolResult:
			results[e.ToolUseID] = anthropic.NewToolResultBlock(e.ToolUseID, e.Output, e.IsError)
		}
		// approval_* and turn_done don't change the history.
	}
	flush()

	a.mu.Lock()
	a.seq = lastSeq
	a.mu.Unlock()
	a.step = maxStep
	a.SessionID = id
	a.conversation = conv
	return nil
}
