package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeWorkflowFile drops body at <dir>/<name> and returns the
// absolute path. The dir is created on demand so the helper mirrors
// how an operator would lay out a workflow directory.
func writeWorkflowFile(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write workflow file: %v", err)
	}
	return path
}

func TestParseWorkflowFileHappyPath(t *testing.T) {
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"---",
		"name: openspec",
		"condition: \"echo add-dark-mode\"",
		"commit: \"see: apply {change}\"",
		"model: \"openai/gpt-5-mini\"",
		"---",
		"",
		"Apply the OpenSpec change {change}.",
	}, "\n"))
	wf, err := parseWorkflowFile(path)
	if err != nil {
		t.Fatalf("parseWorkflowFile: %v", err)
	}
	if wf.Prompt != "Apply the OpenSpec change {change}." {
		t.Fatalf("Prompt = %q", wf.Prompt)
	}
	if wf.Condition != "echo add-dark-mode" {
		t.Fatalf("Condition = %q", wf.Condition)
	}
	if wf.Commit != "see: apply {change}" {
		t.Fatalf("Commit = %q", wf.Commit)
	}
	if wf.Model != "openai/gpt-5-mini" {
		t.Fatalf("Model = %q", wf.Model)
	}
}

func TestParseWorkflowFileIgnoresFrontmatterName(t *testing.T) {
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"---",
		"name: apply-openspec",
		"condition: \"echo add-dark-mode\"",
		"commit: \"see: apply {change}\"",
		"---",
		"Apply the change.",
	}, "\n"))
	wf, err := parseWorkflowFile(path)
	if err != nil {
		t.Fatalf("parseWorkflowFile: %v", err)
	}
	// parseWorkflowFile leaves Name empty; the caller (loadWorkflowFiles)
	// derives Name from the filename.
	if wf.Name != "" {
		t.Fatalf("Name = %q, want empty (caller derives Name from filename)", wf.Name)
	}
	if wf.Prompt != "Apply the change." {
		t.Fatalf("Prompt = %q", wf.Prompt)
	}
}

func TestParseWorkflowFileMissingOpeningDelimiter(t *testing.T) {
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"condition: \"echo add-dark-mode\"",
		"commit: \"see: apply {change}\"",
		"---",
		"Apply the change.",
	}, "\n"))
	_, err := parseWorkflowFile(path)
	if err == nil || !strings.Contains(err.Error(), "missing opening '---'") {
		t.Fatalf("err = %v, want missing opening '---'", err)
	}
}

func TestParseWorkflowFileMissingClosingDelimiter(t *testing.T) {
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"---",
		"condition: \"echo add-dark-mode\"",
		"commit: \"see: apply {change}\"",
		"Apply the change.",
	}, "\n"))
	_, err := parseWorkflowFile(path)
	if err == nil || !strings.Contains(err.Error(), "missing closing '---'") {
		t.Fatalf("err = %v, want missing closing '---'", err)
	}
}

func TestParseWorkflowFileUnknownFrontmatterKey(t *testing.T) {
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"---",
		"condition: \"echo add-dark-mode\"",
		"commit: \"see: apply {change}\"",
		"description: \"this is wrong\"",
		"---",
		"Apply the change.",
	}, "\n"))
	_, err := parseWorkflowFile(path)
	if err == nil {
		t.Fatalf("parseWorkflowFile: want error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %q, want it to name %s", err, path)
	}
	if !strings.Contains(err.Error(), "description") {
		t.Fatalf("err = %q, want it to name the unknown key 'description'", err)
	}
}

func TestParseWorkflowFileBlankCondition(t *testing.T) {
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"---",
		"condition: \"  \"",
		"commit: \"see: apply {change}\"",
		"---",
		"Apply the change.",
	}, "\n"))
	_, err := parseWorkflowFile(path)
	if err == nil || !strings.Contains(err.Error(), "condition is required") {
		t.Fatalf("err = %v, want 'condition is required'", err)
	}
}

func TestParseWorkflowFileMissingCondition(t *testing.T) {
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"---",
		"commit: \"see: apply {change}\"",
		"---",
		"Apply the change.",
	}, "\n"))
	_, err := parseWorkflowFile(path)
	if err == nil || !strings.Contains(err.Error(), "condition is required") {
		t.Fatalf("err = %v, want 'condition is required'", err)
	}
}

func TestParseWorkflowFileBlankCommit(t *testing.T) {
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"---",
		"condition: \"echo add-dark-mode\"",
		"commit: \"\"",
		"---",
		"Apply the change.",
	}, "\n"))
	_, err := parseWorkflowFile(path)
	if err == nil || !strings.Contains(err.Error(), "commit is required") {
		t.Fatalf("err = %v, want 'commit is required'", err)
	}
}

func TestParseWorkflowFileBlankBody(t *testing.T) {
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"---",
		"condition: \"echo add-dark-mode\"",
		"commit: \"see: apply {change}\"",
		"---",
		"   \n\t  ",
	}, "\n"))
	_, err := parseWorkflowFile(path)
	if err == nil || !strings.Contains(err.Error(), "prompt body is required") {
		t.Fatalf("err = %v, want 'prompt body is required'", err)
	}
}

func TestParseWorkflowFileBodyLeadingNewlineTrimmed(t *testing.T) {
	// Operators naturally insert a blank line between the closing
	// '---' and the first content line; the parser trims that one
	// leading newline so the rendered prompt matches the operator's
	// intent.
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"---",
		"condition: \"echo add-dark-mode\"",
		"commit: \"see: apply {change}\"",
		"---",
		"",
		"Apply the change.",
	}, "\n"))
	wf, err := parseWorkflowFile(path)
	if err != nil {
		t.Fatalf("parseWorkflowFile: %v", err)
	}
	if wf.Prompt != "Apply the change." {
		t.Fatalf("Prompt = %q, want single line", wf.Prompt)
	}
}

func TestParseWorkflowFileBodyTrailingWhitespaceTrimmed(t *testing.T) {
	contents := strings.Join([]string{
		"---",
		"condition: \"echo add-dark-mode\"",
		"commit: \"see: apply {change}\"",
		"---",
		"Apply the change.",
		"   ",
		"\t",
		"",
	}, "\n")
	path := writeWorkflowFile(t, "openspec.md", contents)
	wf, err := parseWorkflowFile(path)
	if err != nil {
		t.Fatalf("parseWorkflowFile: %v", err)
	}
	if wf.Prompt != "Apply the change." {
		t.Fatalf("Prompt = %q, want trimmed", wf.Prompt)
	}
}

func TestParseWorkflowFileMultiLineBodyPreserved(t *testing.T) {
	body := strings.Join([]string{
		"Apply the OpenSpec change {change}.",
		"",
		"Steps:",
		"- read the proposal",
		"- implement tasks in order",
		"- run the verification command",
	}, "\n")
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"---",
		"condition: \"echo add-dark-mode\"",
		"commit: \"see: apply {change}\"",
		"---",
		"",
		body,
	}, "\n"))
	wf, err := parseWorkflowFile(path)
	if err != nil {
		t.Fatalf("parseWorkflowFile: %v", err)
	}
	if wf.Prompt != body {
		t.Fatalf("Prompt mismatch:\n got %q\nwant %q", wf.Prompt, body)
	}
}

func TestParseWorkflowFileCRLFLineEndings(t *testing.T) {
	// Files written with Windows line endings must split the same
	// way as Unix ones; the parser trims the trailing \r before
	// matching the delimiter line.
	dir := t.TempDir()
	path := filepath.Join(dir, "openspec.md")
	contents := "---\r\ncondition: \"echo add-dark-mode\"\r\ncommit: \"see: apply {change}\"\r\n---\r\nApply the change.\r\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	wf, err := parseWorkflowFile(path)
	if err != nil {
		t.Fatalf("parseWorkflowFile: %v", err)
	}
	if wf.Prompt != "Apply the change." {
		t.Fatalf("Prompt = %q, want single line", wf.Prompt)
	}
}

func TestParseWorkflowFileBlankModelIsUnset(t *testing.T) {
	// The spec states blank model is treated as unset; the strict
	// decoder accepts the field as-is and the caller (runOneWorkflow)
	// trims whitespace before deciding whether to pass --model.
	path := writeWorkflowFile(t, "openspec.md", strings.Join([]string{
		"---",
		"condition: \"echo add-dark-mode\"",
		"commit: \"see: apply {change}\"",
		"model: \"  \"",
		"---",
		"Apply the change.",
	}, "\n"))
	wf, err := parseWorkflowFile(path)
	if err != nil {
		t.Fatalf("parseWorkflowFile: %v", err)
	}
	if wf.Model != "  " {
		t.Fatalf("Model = %q, want the raw value (whitespace-only handled at call site)", wf.Model)
	}
}
