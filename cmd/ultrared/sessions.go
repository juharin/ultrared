package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/juharin/ultrared/internal/session"
)

// openSessionLog prepares the event log of session id in dir, which the CLI
// appends every event to. When resuming, it first reads the events already
// there; the log must exist then. A torn last line, left by a crash while an
// event was being written, is cut off so new events start on a fresh line.
func openSessionLog(dir, id string, resuming bool) (*os.File, []session.Event, error) {
	path, err := session.LogPath(dir, id)
	if err != nil {
		return nil, nil, err
	}
	if !resuming {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, err
		}
		return f, nil, nil
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("resume: %w", err)
	}
	events, size, err := session.ReadLog(f)
	if err == nil {
		err = f.Truncate(size) // O_APPEND writes then go to the new end
	}
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("resume %s: %w", path, err)
	}
	return f, events, nil
}

// recap describes a resumed session in one line: how many turns it had and
// the model's last words.
func recap(events []session.Event) string {
	turns, last := 0, ""
	for _, e := range events {
		switch e.Type {
		case session.UserMessage:
			turns++
		case session.AssistantText:
			last = e.Text
		}
	}
	s := fmt.Sprintf("%d earlier turn(s)", turns)
	if last != "" {
		s += ". Last reply: " + truncate(last, 200)
	}
	return s
}

// newSessionID returns a short random id, e.g. "3f9a1c0b2d4e".
func newSessionID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
