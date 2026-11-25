package agent

// End-to-end tests of the agent loop against a fake Anthropic API (httptest).
// No API key or network needed:   go test ./...
// The Docker variant runs only if Docker is available, and not with -short.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/juharin/ultrared/internal/executor"
	"github.com/juharin/ultrared/internal/session"
	"github.com/juharin/ultrared/internal/tools"
)

// fakeAnthropic scripts three model turns: edit_file on a.txt, then bash
// `cat a.txt`, then a final answer; any later request gets a plain answer.
// wantFile is what the bash result must contain when it comes back: "x = 2"
// if the edit ran, "x = 1" if it didn't.
func fakeAnthropic(t *testing.T, wantFile string) *httptest.Server {
	var mu sync.Mutex
	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Tools []struct {
				Name        string `json:"name"`
				InputSchema struct {
					Required []string `json:"required"`
				} `json:"input_schema"`
			} `json:"tools"`
			Messages []json.RawMessage `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("bad request json: %v", err)
		}
		for _, tool := range req.Tools {
			if len(tool.InputSchema.Required) == 0 {
				t.Errorf("tool %s has no required fields in schema", tool.Name)
			}
		}
		step++
		var content []map[string]any
		stop := "tool_use"
		switch step {
		case 1:
			content = []map[string]any{
				{"type": "text", "text": "Editing."},
				{"type": "tool_use", "id": "t1", "name": "edit_file",
					"input": map[string]string{"path": "a.txt", "old_str": "x = 1", "new_str": "x = 2"}},
			}
		case 2:
			if !strings.Contains(string(req.Messages[len(req.Messages)-1]), `"tool_result"`) {
				t.Errorf("expected tool_result in last message")
			}
			content = []map[string]any{
				{"type": "tool_use", "id": "t2", "name": "bash", "input": map[string]string{"command": "cat a.txt"}},
			}
		case 3:
			last := string(req.Messages[len(req.Messages)-1])
			if !strings.Contains(last, wantFile) {
				t.Errorf("bash result not fed back (want %q); last message: %s", wantFile, last)
			}
			content = []map[string]any{{"type": "text", "text": "Done."}}
			stop = "end_turn"
		default: // later turns, e.g. after a resume: just answer
			content = []map[string]any{{"type": "text", "text": "Still here."}}
			stop = "end_turn"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg", "type": "message", "role": "assistant", "model": "fake",
			"content": content, "stop_reason": stop, "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newAgent(srv *httptest.Server, ex executor.Executor) *Agent {
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	return &Agent{Client: &client, Model: DefaultModel, Exec: ex, Tools: tools.All(), MaxTurns: 10, SessionID: "s1"}
}

// workspace returns a temp dir holding a.txt with "x = 1".
func workspace(t *testing.T) string {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func fileA(t *testing.T, dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func typesOf(events []session.Event) []session.Type {
	var ts []session.Type
	for _, e := range events {
		ts = append(ts, e.Type)
	}
	return ts
}

func find(events []session.Event, typ session.Type, toolUseID string) (session.Event, int) {
	for i, e := range events {
		if e.Type == typ && e.ToolUseID == toolUseID {
			return e, i
		}
	}
	return session.Event{}, -1
}

func runScenario(t *testing.T, ex executor.Executor, dir string) {
	a := newAgent(fakeAnthropic(t, "x = 2"), ex)
	var texts []string
	a.Emit = func(e session.Event) {
		if e.Type == session.AssistantText {
			texts = append(texts, e.Text)
		}
	}
	if err := a.Send(context.Background(), "set x to 2"); err != nil {
		t.Fatal(err)
	}
	if got := fileA(t, dir); got != "x = 2" {
		t.Fatalf("file not edited, got %q", got)
	}
	if len(texts) == 0 || texts[len(texts)-1] != "Done." {
		t.Fatalf("unexpected final text: %v", texts)
	}
}

func TestLoopLocal(t *testing.T) {
	dir := workspace(t)
	runScenario(t, &executor.Local{Root: dir}, dir)
}

func TestLoopDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container; skipped with -short")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
	dir := workspace(t)
	d, err := executor.NewDocker(context.Background(), "alpine:3.20", dir, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	runScenario(t, d, dir)
}

// A turn is a stream of typed, numbered events that a client can render,
// log or replay without knowing anything else about the loop.
func TestEventSequence(t *testing.T) {
	dir := workspace(t)
	a := newAgent(fakeAnthropic(t, "x = 2"), &executor.Local{Root: dir})
	var events []session.Event
	a.Emit = func(e session.Event) { events = append(events, e) }

	if err := a.Send(context.Background(), "set x to 2"); err != nil {
		t.Fatal(err)
	}

	want := []session.Type{
		session.UserMessage,
		session.AssistantText, session.ToolCall, session.ToolResult,
		session.ToolCall, session.ToolResult,
		session.AssistantText,
		session.TurnDone,
	}
	if got := typesOf(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("event types:\n got %v\nwant %v", got, want)
	}
	for i, e := range events {
		if e.Seq != int64(i+1) {
			t.Errorf("event %d has seq %d, want %d", i, e.Seq, i+1)
		}
		if e.Session != "s1" {
			t.Errorf("event %d has session %q, want s1", i, e.Session)
		}
		if e.Time.IsZero() {
			t.Errorf("event %d has no time", i)
		}
	}
	if events[0].Text != "set x to 2" {
		t.Errorf("user_message text = %q", events[0].Text)
	}
	call, _ := find(events, session.ToolCall, "t1")
	if call.Tool != "edit_file" || !strings.Contains(string(call.Input), `"old_str"`) {
		t.Errorf("tool_call t1 = %+v", call)
	}
	res, _ := find(events, session.ToolResult, "t2")
	if res.IsError || !strings.Contains(res.Output, "x = 2") {
		t.Errorf("tool_result t2 = %+v", res)
	}
	if done := events[len(events)-1]; done.Error != "" {
		t.Errorf("turn_done has error %q", done.Error)
	}
	// Events are plain data: they must survive a JSON round trip unchanged.
	b, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	var back []session.Event
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(typesOf(back), want) || back[3].Output != events[3].Output {
		t.Errorf("events changed in a JSON round trip")
	}
}

// approveEdits asks for approval for edit_file only, so the bash step runs freely.
func approveEdits(tool string, _ json.RawMessage) bool { return tool == "edit_file" }

func TestApprovalAllowed(t *testing.T) {
	dir := workspace(t)
	a := newAgent(fakeAnthropic(t, "x = 2"), &executor.Local{Root: dir})
	a.NeedsApproval = approveEdits
	var events []session.Event
	a.Emit = func(e session.Event) {
		events = append(events, e)
		if e.Type == session.ApprovalRequested {
			// A client may answer from inside Emit, as the CLI does.
			if err := a.Resolve(e.ToolUseID, true); err != nil {
				t.Error(err)
			}
		}
	}
	if err := a.Send(context.Background(), "set x to 2"); err != nil {
		t.Fatal(err)
	}
	if got := fileA(t, dir); got != "x = 2" {
		t.Fatalf("file not edited, got %q", got)
	}
	req, iReq := find(events, session.ApprovalRequested, "t1")
	res, iRes := find(events, session.ApprovalResolved, "t1")
	_, iResult := find(events, session.ToolResult, "t1")
	if iReq < 0 || iRes < 0 || !(iReq < iRes && iRes < iResult) {
		t.Fatalf("want approval_requested < approval_resolved < tool_result, got %v", typesOf(events))
	}
	if req.Tool != "edit_file" || len(req.Input) == 0 {
		t.Errorf("approval_requested = %+v", req)
	}
	if res.Approved == nil || !*res.Approved {
		t.Errorf("approval_resolved = %+v, want approved", res)
	}
	if _, i := find(events, session.ApprovalRequested, "t2"); i >= 0 {
		t.Errorf("bash asked for approval, but NeedsApproval said no")
	}
}

func TestApprovalDenied(t *testing.T) {
	dir := workspace(t)
	a := newAgent(fakeAnthropic(t, "x = 1"), &executor.Local{Root: dir})
	a.NeedsApproval = approveEdits
	var events []session.Event
	a.Emit = func(e session.Event) {
		events = append(events, e)
		if e.Type == session.ApprovalRequested {
			a.Resolve(e.ToolUseID, false)
		}
	}
	if err := a.Send(context.Background(), "set x to 2"); err != nil {
		t.Fatal(err)
	}
	if got := fileA(t, dir); got != "x = 1" {
		t.Fatalf("denied edit still ran, got %q", got)
	}
	res, _ := find(events, session.ApprovalResolved, "t1")
	if res.Approved == nil || *res.Approved {
		t.Errorf("approval_resolved = %+v, want denied", res)
	}
	out, _ := find(events, session.ToolResult, "t1")
	if !out.IsError || out.Output != "denied by user" {
		t.Errorf("tool_result t1 = %+v, want is_error 'denied by user'", out)
	}
}

// Any client can answer, from any goroutine, while the loop waits.
func TestApprovalFromAnotherGoroutine(t *testing.T) {
	dir := workspace(t)
	a := newAgent(fakeAnthropic(t, "x = 2"), &executor.Local{Root: dir})
	a.NeedsApproval = approveEdits
	requests := make(chan session.Event, 1)
	a.Emit = func(e session.Event) {
		if e.Type == session.ApprovalRequested {
			requests <- e
		}
	}
	errc := make(chan error, 1)
	go func() {
		e := <-requests
		time.Sleep(20 * time.Millisecond) // the loop is now blocked waiting
		errc <- a.Resolve(e.ToolUseID, true)
	}()
	if err := a.Send(context.Background(), "set x to 2"); err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if got := fileA(t, dir); got != "x = 2" {
		t.Fatalf("file not edited, got %q", got)
	}
}

// Interrupting the turn while an approval is pending denies the tool and
// ends the turn; a late answer finds nothing to resolve.
func TestInterruptWhileWaitingForApproval(t *testing.T) {
	dir := workspace(t)
	a := newAgent(fakeAnthropic(t, "x = 1"), &executor.Local{Root: dir})
	a.NeedsApproval = approveEdits
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var events []session.Event
	a.Emit = func(e session.Event) {
		events = append(events, e)
		if e.Type == session.ApprovalRequested {
			cancel() // Ctrl-C while the prompt is up
		}
	}
	err := a.Send(ctx, "set x to 2")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context.Canceled", err)
	}
	if got := fileA(t, dir); got != "x = 1" {
		t.Fatalf("edit ran after interrupt, got %q", got)
	}
	res, _ := find(events, session.ApprovalResolved, "t1")
	if res.Approved == nil || *res.Approved {
		t.Errorf("approval_resolved = %+v, want denied", res)
	}
	if done := events[len(events)-1]; done.Type != session.TurnDone || done.Error == "" {
		t.Errorf("last event = %+v, want turn_done with an error", done)
	}
	if err := a.Resolve("t1", true); err == nil {
		t.Errorf("late Resolve succeeded; want an error")
	}
}

func TestResolveUnknownRequest(t *testing.T) {
	var a Agent
	if err := a.Resolve("nope", true); err == nil {
		t.Fatal("Resolve of an unknown request succeeded")
	}
}

// --- resume -------------------------------------------------------------------

// conversationJSON is what the API would be sent as history.
func conversationJSON(t *testing.T, a *Agent) string {
	b, err := json.Marshal(a.conversation)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Events record which model response (step) each block came from, so one
// response with two tool calls is not confused with two responses.
func TestEventSteps(t *testing.T) {
	dir := workspace(t)
	a := newAgent(fakeAnthropic(t, "x = 2"), &executor.Local{Root: dir})
	var events []session.Event
	a.Emit = func(e session.Event) { events = append(events, e) }
	if err := a.Send(context.Background(), "set x to 2"); err != nil {
		t.Fatal(err)
	}
	want := map[session.Type][]int64{
		session.UserMessage:   {0},
		session.AssistantText: {1, 3},
		session.ToolCall:      {1, 2},
		session.ToolResult:    {1, 2},
		session.TurnDone:      {0},
	}
	got := map[session.Type][]int64{}
	for _, e := range events {
		got[e.Type] = append(got[e.Type], e.Step)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("steps by event type:\n got %v\nwant %v", got, want)
	}
}

// Restoring from the events gives back exactly the conversation the agent had,
// and the session continues where it stopped: same id, next seq, next step.
func TestRestoreRebuildsConversation(t *testing.T) {
	dir := workspace(t)
	srv := fakeAnthropic(t, "x = 2")
	a := newAgent(srv, &executor.Local{Root: dir})
	var events []session.Event
	a.Emit = func(e session.Event) { events = append(events, e) }
	if err := a.Send(context.Background(), "set x to 2"); err != nil {
		t.Fatal(err)
	}

	b := newAgent(srv, &executor.Local{Root: dir})
	b.SessionID = "" // taken from the events
	if err := b.Restore(events); err != nil {
		t.Fatal(err)
	}
	if got, want := conversationJSON(t, b), conversationJSON(t, a); got != want {
		t.Fatalf("restored conversation differs:\n got %s\nwant %s", got, want)
	}

	var more []session.Event
	b.Emit = func(e session.Event) { more = append(more, e) }
	if err := b.Send(context.Background(), "anything else?"); err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if more[0].Session != "s1" || more[0].Seq != last.Seq+1 {
		t.Errorf("first new event = %+v, want session s1, seq %d", more[0], last.Seq+1)
	}
	if text, _ := find(more, session.AssistantText, ""); text.Step != 4 || text.Text != "Still here." {
		t.Errorf("new reply = %+v, want step 4 'Still here.'", text)
	}
}

// One model response with two tool calls becomes one assistant message with
// two tool_use blocks, followed by one user message with both results.
func TestRestoreGroupsByStep(t *testing.T) {
	in := json.RawMessage(`{"command":"true"}`)
	events := []session.Event{
		{Session: "s", Seq: 1, Type: session.UserMessage, Text: "go"},
		{Session: "s", Seq: 2, Step: 1, Type: session.ToolCall, ToolUseID: "a", Tool: "bash", Input: in},
		{Session: "s", Seq: 3, Step: 1, Type: session.ToolResult, ToolUseID: "a", Tool: "bash", Output: "ok a"},
		{Session: "s", Seq: 4, Step: 1, Type: session.ToolCall, ToolUseID: "b", Tool: "bash", Input: in},
		{Session: "s", Seq: 5, Step: 1, Type: session.ToolResult, ToolUseID: "b", Tool: "bash", Output: "ok b"},
		{Session: "s", Seq: 6, Step: 2, Type: session.AssistantText, Text: "both done"},
		{Session: "s", Seq: 7, Type: session.TurnDone},
	}
	var a Agent
	if err := a.Restore(events); err != nil {
		t.Fatal(err)
	}
	roles := []string{}
	for _, m := range a.conversation {
		roles = append(roles, fmt.Sprintf("%s:%d", m.Role, len(m.Content)))
	}
	if want := []string{"user:1", "assistant:2", "user:2", "assistant:1"}; !reflect.DeepEqual(roles, want) {
		t.Fatalf("messages = %v, want %v", roles, want)
	}
}

// A session that stopped after a tool call but before its result (a crash, or
// waiting for an approval) gets an error result, so the history stays valid:
// the API rejects a tool_use without a matching tool_result.
// A tool call without a recorded result gets an error result, because the API
// rejects a tool_use without one. What it says depends on what the log shows:
// a tool still waiting for approval never ran, but any other tool may have run
// and done its side effects before the session stopped.
func TestRestoreFillsMissingToolResult(t *testing.T) {
	call := session.Event{Session: "s", Seq: 2, Step: 1, Type: session.ToolCall, ToolUseID: "a", Tool: "bash", Input: json.RawMessage(`{}`)}
	approved := true
	cases := []struct {
		name  string
		after []session.Event
		want  string
	}{
		{"waiting for approval", []session.Event{
			{Session: "s", Seq: 3, Step: 1, Type: session.ApprovalRequested, ToolUseID: "a", Tool: "bash"},
		}, "did not run"},
		{"approved, then stopped", []session.Event{
			{Session: "s", Seq: 3, Step: 1, Type: session.ApprovalRequested, ToolUseID: "a", Tool: "bash"},
			{Session: "s", Seq: 4, Step: 1, Type: session.ApprovalResolved, ToolUseID: "a", Tool: "bash", Approved: &approved},
		}, "may have run"},
		{"no approval needed", nil, "may have run"},
	}
	for _, c := range cases {
		events := append([]session.Event{{Session: "s", Seq: 1, Type: session.UserMessage, Text: "go"}, call}, c.after...)
		var a Agent
		if err := a.Restore(events); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(a.conversation) != 3 {
			t.Fatalf("%s: got %d messages, want user, assistant, user(tool_result)", c.name, len(a.conversation))
		}
		last := conversationJSON(t, &Agent{conversation: a.conversation[2:]})
		if !strings.Contains(last, `"tool_use_id":"a"`) || !strings.Contains(last, `"is_error":true`) || !strings.Contains(last, c.want) {
			t.Errorf("%s: missing tool_result not filled in with %q: %s", c.name, c.want, last)
		}
	}
}

func TestRestoreRejectsBadLogs(t *testing.T) {
	ok := session.Event{Session: "s", Seq: 1, Type: session.UserMessage, Text: "hi"}
	cases := map[string][]session.Event{
		"empty":          nil,
		"two sessions":   {ok, {Session: "other", Seq: 2, Type: session.TurnDone}},
		"seq not rising": {ok, {Session: "s", Seq: 1, Type: session.TurnDone}},
	}
	for name, events := range cases {
		var a Agent
		if err := a.Restore(events); err == nil {
			t.Errorf("%s: Restore succeeded, want an error", name)
		}
	}

	var used Agent
	if err := used.Restore([]session.Event{ok}); err != nil {
		t.Fatal(err)
	}
	if err := used.Restore([]session.Event{ok}); err == nil {
		t.Error("Restore into an agent that already has a conversation succeeded")
	}
}
