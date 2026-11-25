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
// tool call without a recorded result gets an error result, because the API
// rejects a tool_use that has no tool_result. If the session stopped while
// the call waited for approval, the tool never ran. Otherwise it may have run
// and done its side effects, with only its result lost, so the model is told
// to check before running it again.
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
		waiting = map[string]bool{} // calls with an unanswered approval request
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
					r = anthropic.NewToolResultBlock(c, missingResult(waiting[c]), true)
				}
				rs = append(rs, r)
			}
			conv = append(conv, anthropic.NewUserMessage(rs...))
		}
		step, blocks, calls = -1, nil, nil
		results = map[string]anthropic.ContentBlockParamUnion{}
		waiting = map[string]bool{}
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
		case session.ApprovalRequested:
			waiting[e.ToolUseID] = true
		case session.ApprovalResolved:
			delete(waiting, e.ToolUseID)
		case session.ToolResult:
			results[e.ToolUseID] = anthropic.NewToolResultBlock(e.ToolUseID, e.Output, e.IsError)
		}
		// turn_done doesn't change the history.
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

// missingResult is the tool_result for a call whose result was never recorded.
func missingResult(waitingForApproval bool) string {
	if waitingForApproval {
		return "ERROR: the session stopped while this tool call waited for approval, so the tool did not run"
	}
	return "ERROR: the session stopped before this tool's result was recorded. The tool may have run, " +
		"fully or in part; check the current state before running it again"
}
