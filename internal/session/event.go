// Package session defines the events a session produces.
//
// Events are the only way the agent loop talks to its clients: the CLI now,
// and later a server streaming them to desktop and mobile apps. So they are
// plain data that serializes to JSON, numbered in order, and complete enough
// to render, log and (later) replay a session from.
package session

import (
	"encoding/json"
	"time"
)

type Type string

const (
	UserMessage       Type = "user_message"       // Text: what the user sent
	AssistantText     Type = "assistant_text"     // Text: a text block from the model
	ToolCall          Type = "tool_call"          // ToolUseID, Tool, Input: the model asked to run a tool
	ApprovalRequested Type = "approval_requested" // ToolUseID, Tool, Input: the loop waits until a client answers
	ApprovalResolved  Type = "approval_resolved"  // ToolUseID, Approved
	ToolResult        Type = "tool_result"        // ToolUseID, Tool, Output, IsError
	TurnDone          Type = "turn_done"          // Error: set if the turn failed or was interrupted
)

// Event is one thing that happened in a session. Which fields are set depends
// on Type (see the constants above); the rest are left empty.
type Event struct {
	Session string    `json:"session"`
	Seq     int64     `json:"seq"` // 1, 2, 3, … within a session; a reconnecting client resumes after the last seq it saw
	Time    time.Time `json:"time"`
	Type    Type      `json:"type"`
	// Step is the model response an event belongs to: 1, 2, 3, … across the
	// session. Set on assistant_text, tool_call, approval_* and tool_result;
	// it tells one response with two tool calls apart from two responses.
	Step int64 `json:"step,omitempty"`

	Text      string          `json:"text,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Output    string          `json:"output,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Approved  *bool           `json:"approved,omitempty"` // a pointer, so that "denied" (false) is still sent
	Error     string          `json:"error,omitempty"`
}
