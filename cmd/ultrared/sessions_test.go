package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/juharin/ultrared/internal/session"
)

// A crash in the middle of writing an event leaves a torn last line. Resuming
// drops it, and must also cut it from the file: otherwise the next event is
// appended to the partial line, and the session can never be resumed again.
func TestResumeAfterTornWrite(t *testing.T) {
	dir := t.TempDir()
	torn := `{"session":"s1","seq":1,"type":"user_message","text":"hi"}
{"session":"s1","seq":2,"ty`
	if err := os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(torn), 0o644); err != nil {
		t.Fatal(err)
	}

	f, events, err := openSessionLog(dir, "s1", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("first resume: got %d events, want 1", len(events))
	}
	if err := json.NewEncoder(f).Encode(session.Event{Session: "s1", Seq: 2, Type: session.TurnDone}); err != nil {
		t.Fatal(err)
	}
	f.Close()

	f, events, err = openSessionLog(dir, "s1", true)
	if err != nil {
		t.Fatalf("second resume: %v", err)
	}
	f.Close()
	if len(events) != 2 || events[1].Type != session.TurnDone {
		t.Fatalf("second resume: got %+v", events)
	}
}
