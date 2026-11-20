package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/invopop/jsonschema"
)

// ToolDefinition is your original type, with two changes: the function gets a
// context and the Executor, and errors are returned instead of panicking.
// A tool error goes back to the model as an is_error tool_result so it can
// correct itself.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema anthropic.ToolInputSchemaParam
	Function    func(ctx context.Context, ex Executor, input json.RawMessage) (string, error)
}

func Tools() []ToolDefinition {
	return []ToolDefinition{BashDefinition, ReadFileDefinition, WriteFileDefinition, EditFileDefinition}
}

const maxOutput = 30_000 // chars; keeps one big output from eating the context

func clip(s string) string {
	if len(s) <= maxOutput {
		return s
	}
	h := maxOutput / 2
	return s[:h] + "\n...[truncated]...\n" + s[len(s)-h:]
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("invalid input: %w", err)
	}
	return v, nil
}

// --- bash ---------------------------------------------------------------------

type BashInput struct {
	Command string `json:"command" jsonschema_description:"Shell command to run in the workspace root."`
}

var BashDefinition = ToolDefinition{
	Name:        "bash",
	Description: "Run a shell command in the workspace. Use it for ls, rg, tests, builds and git. Output is truncated if very long.",
	InputSchema: GenerateSchema[BashInput](),
	Function: func(ctx context.Context, ex Executor, raw json.RawMessage) (string, error) {
		in, err := decode[BashInput](raw)
		if err != nil {
			return "", err
		}
		out, code, err := ex.Run(ctx, in.Command)
		if err != nil {
			return "", err
		}
		return clip(fmt.Sprintf("exit=%d\n%s", code, out)), nil
	},
}

// --- read_file ----------------------------------------------------------------

type ReadFileInput struct {
	Path string `json:"path" jsonschema_description:"Relative path of a file in the workspace."`
}

var ReadFileDefinition = ToolDefinition{
	Name:        "read_file",
	Description: "Read a file and return its contents with line numbers. Do not use on directories.",
	InputSchema: GenerateSchema[ReadFileInput](),
	Function: func(ctx context.Context, ex Executor, raw json.RawMessage) (string, error) {
		in, err := decode[ReadFileInput](raw)
		if err != nil {
			return "", err
		}
		b, err := ex.ReadFile(ctx, in.Path)
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		for i, line := range strings.Split(string(b), "\n") {
			fmt.Fprintf(&sb, "%5d| %s\n", i+1, line)
		}
		return clip(sb.String()), nil
	},
}

// --- write_file ---------------------------------------------------------------

type WriteFileInput struct {
	Path    string `json:"path" jsonschema_description:"Relative path of the file to create or overwrite."`
	Content string `json:"content" jsonschema_description:"Full new contents of the file."`
}

var WriteFileDefinition = ToolDefinition{
	Name:        "write_file",
	Description: "Create a new file or completely overwrite an existing one. Prefer edit_file for changes to existing files.",
	InputSchema: GenerateSchema[WriteFileInput](),
	Function: func(ctx context.Context, ex Executor, raw json.RawMessage) (string, error) {
		in, err := decode[WriteFileInput](raw)
		if err != nil {
			return "", err
		}
		if err := ex.WriteFile(ctx, in.Path, []byte(in.Content)); err != nil {
			return "", err
		}
		return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
	},
}

// --- edit_file ----------------------------------------------------------------
// Exact unique-substring replacement (the Claude Code / Pi contract). The
// logic runs in the harness; only the read and the write cross the Executor.

type EditFileInput struct {
	Path   string `json:"path" jsonschema_description:"Relative path of the file to edit."`
	OldStr string `json:"old_str" jsonschema_description:"Exact text to replace. Must occur exactly once in the file."`
	NewStr string `json:"new_str" jsonschema_description:"Replacement text."`
}

var EditFileDefinition = ToolDefinition{
	Name:        "edit_file",
	Description: "Replace one exact, unique occurrence of old_str with new_str in a file. Include enough surrounding lines to make old_str unique.",
	InputSchema: GenerateSchema[EditFileInput](),
	Function: func(ctx context.Context, ex Executor, raw json.RawMessage) (string, error) {
		in, err := decode[EditFileInput](raw)
		if err != nil {
			return "", err
		}
		if in.OldStr == "" {
			return "", fmt.Errorf("old_str is empty; use write_file to create files")
		}
		b, err := ex.ReadFile(ctx, in.Path)
		if err != nil {
			return "", err
		}
		text := string(b)
		switch n := strings.Count(text, in.OldStr); n {
		case 1:
		case 0:
			return "", fmt.Errorf("old_str not found in %s; re-read the file and copy the text exactly", in.Path)
		default:
			return "", fmt.Errorf("old_str occurs %d times in %s; add surrounding context so it is unique", n, in.Path)
		}
		if err := ex.WriteFile(ctx, in.Path, []byte(strings.Replace(text, in.OldStr, in.NewStr, 1))); err != nil {
			return "", err
		}
		return "OK", nil
	},
}

// GenerateSchema is your original helper, now also filling in Required
// (without it the model may omit arguments).
func GenerateSchema[T any]() anthropic.ToolInputSchemaParam {
	r := jsonschema.Reflector{AllowAdditionalProperties: false, DoNotReference: true}
	var v T
	s := r.Reflect(v)
	return anthropic.ToolInputSchemaParam{Properties: s.Properties, Required: s.Required}
}
