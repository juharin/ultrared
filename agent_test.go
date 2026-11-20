package main

// End-to-end test of the agent loop against a fake Anthropic API (httptest).
// No API key or network needed:   go test ./...
// The Docker variant runs only if Docker is available: go test -run Docker ./...

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// fakeAnthropic scripts three model turns: edit_file, then bash, then a final answer.
func fakeAnthropic(t *testing.T) *httptest.Server {
	var mu sync.Mutex
	step := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		default:
			last := string(req.Messages[len(req.Messages)-1])
			if !strings.Contains(last, "x = 2") {
				t.Errorf("bash result not fed back; last message: %s", last)
			}
			content = []map[string]any{{"type": "text", "text": "Done."}}
			stop = "end_turn"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg", "type": "message", "role": "assistant", "model": "fake",
			"content": content, "stop_reason": stop, "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
}

func runScenario(t *testing.T, ex Executor, dir string) {
	srv := fakeAnthropic(t)
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test"))
	a := &Agent{client: &client, model: DefaultModel, exec: ex, tools: Tools(), maxTurns: 10}
	var texts []string
	err := a.Send(context.Background(), "set x to 2", func(kind, s string) {
		if kind == "text" {
			texts = append(texts, s)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if strings.TrimSpace(string(b)) != "x = 2" {
		t.Fatalf("file not edited, got %q", b)
	}
	if len(texts) == 0 || texts[len(texts)-1] != "Done." {
		t.Fatalf("unexpected final text: %v", texts)
	}
}

func TestLoopLocal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x = 1\n"), 0o644)
	runScenario(t, &LocalExecutor{Root: dir}, dir)
}

func TestLoopDocker(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x = 1\n"), 0o644)
	d, err := NewDockerExecutor(context.Background(), "alpine:3.20", dir, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	runScenario(t, d, dir)
}

func TestEditFileErrors(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a a"), 0o644)
	ex := &LocalExecutor{Root: dir}
	in, _ := json.Marshal(EditFileInput{Path: "f.txt", OldStr: "a", NewStr: "b"})
	if _, err := EditFileDefinition.Function(context.Background(), ex, in); err == nil ||
		!strings.Contains(err.Error(), "2 times") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
	in, _ = json.Marshal(EditFileInput{Path: "f.txt", OldStr: "zzz", NewStr: "b"})
	if _, err := EditFileDefinition.Function(context.Background(), ex, in); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}
