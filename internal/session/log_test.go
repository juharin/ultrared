package session

import (
	"strings"
	"testing"
)

func TestReadLog(t *testing.T) {
	log := `{"session":"s","seq":1,"type":"user_message","text":"hi"}
{"session":"s","seq":2,"type":"turn_done"}
`
	events, err := ReadLog(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Text != "hi" || events[1].Type != TurnDone {
		t.Fatalf("got %+v", events)
	}
}

// A crash while writing leaves a last line without its newline. That event
// never fully happened, so it is dropped instead of failing the whole log.
func TestReadLogDropsTornLastLine(t *testing.T) {
	log := `{"session":"s","seq":1,"type":"user_message","text":"hi"}
{"session":"s","seq":2,"ty`
	events, err := ReadLog(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
}

func TestReadLogRejectsBadLine(t *testing.T) {
	log := `{"session":"s","seq":1,"type":"user_message"}
not json
{"session":"s","seq":2,"type":"turn_done"}
`
	if _, err := ReadLog(strings.NewReader(log)); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v, want an error naming line 2", err)
	}
}

func TestLogPath(t *testing.T) {
	p, err := LogPath("/x/sessions", "3f9a1c0b2d4e")
	if err != nil || p != "/x/sessions/3f9a1c0b2d4e.jsonl" {
		t.Fatalf("LogPath = %q, %v", p, err)
	}
	for _, bad := range []string{"", "../etc/passwd", "a/b", "a.b", "A B"} {
		if _, err := LogPath("/x", bad); err == nil {
			t.Errorf("LogPath accepted session id %q", bad)
		}
	}
}
