package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juharin/ultrared/internal/executor"
)

func TestEditFileErrors(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a a"), 0o644)
	ex := &executor.Local{Root: dir}
	in, _ := json.Marshal(EditFileInput{Path: "f.txt", OldStr: "a", NewStr: "b"})
	if _, err := EditFile.Function(context.Background(), ex, in); err == nil ||
		!strings.Contains(err.Error(), "2 times") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
	in, _ = json.Marshal(EditFileInput{Path: "f.txt", OldStr: "zzz", NewStr: "b"})
	if _, err := EditFile.Function(context.Background(), ex, in); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}
