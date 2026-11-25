package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
)

// The event log of a session is its events, one JSON object per line, in
// <dir>/<session id>.jsonl. It is the session's source of truth: resuming a
// session means reading it back and rebuilding the conversation from it.

var validID = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// LogPath returns where the event log of session id lives in dir. Session ids
// may come from users (`-resume ID`), so anything that could escape dir is
// rejected.
func LogPath(dir, id string) (string, error) {
	if !validID.MatchString(id) {
		return "", fmt.Errorf("invalid session id %q: use lowercase letters, digits and -", id)
	}
	return filepath.Join(dir, id+".jsonl"), nil
}

// ReadLog reads an event log. A last line without a newline is dropped: that
// is what a crash in the middle of writing an event leaves behind. Any other
// line that isn't a valid event is an error.
func ReadLog(r io.Reader) ([]Event, error) {
	br := bufio.NewReader(r)
	var events []Event
	for n := 1; ; n++ {
		line, err := br.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return events, nil // a torn last line, or nothing left
		}
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("event log line %d: %w", n, err)
		}
		events = append(events, e)
	}
}
