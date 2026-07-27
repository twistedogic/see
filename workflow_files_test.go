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

// TestParseWorkflowFileDisableRoundTrip proves frontmatter disable
// threads onto the produced WorkflowConfig, and that an absent
// disable decodes to false. The strict decoder accepts the fifth key;
// a sixth key continues to be rejected (covered by the unknown-key test).
func TestParseWorkflowFileDisableRoundTrip(t *testing.T) {
	t.Run("disable true", func(t *testing.T) {
		path := writeWorkflowFile(t, "parked.md", strings.Join([]string{
			"---",
			"condition: \"echo add-dark-mode\"",
			"commit: \"see: apply {change}\"",
			"disable: true",
			"---",
			"Apply the change.",
		}, "\n"))
		wf, err := parseWorkflowFile(path)
		if err != nil {
			t.Fatalf("parseWorkflowFile: %v", err)
		}
		if !wf.Disable {
			t.Fatalf("Disable = false, want true")
		}
	})
	t.Run("absent disable", func(t *testing.T) {
		path := writeWorkflowFile(t, "live.md", strings.Join([]string{
			"---",
			"condition: \"echo add-dark-mode\"",
			"commit: \"see: apply {change}\"",
			"---",
			"Apply the change.",
		}, "\n"))
		wf, err := parseWorkflowFile(path)
		if err != nil {
			t.Fatalf("parseWorkflowFile: %v", err)
		}
		if wf.Disable {
			t.Fatalf("Disable = true, want false for absent field")
		}
	})
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

// makeWorkflowFile drops a minimal-valid workflow markdown file at
// <dir>/<name> and returns nothing; the test reads the directory
// through loadWorkflowFiles so the helper stays focused on laying
// out the input.
func makeWorkflowFile(t *testing.T, dir, name string) {
	t.Helper()
	body := strings.Join([]string{
		"---",
		"condition: \"echo add-dark-mode\"",
		"commit: \"see: apply {change}\"",
		"---",
		"Apply the change.",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestLoadWorkflowFilesAlphabeticalSort(t *testing.T) {
	dir := t.TempDir()
	makeWorkflowFile(t, dir, "02-deps.md")
	makeWorkflowFile(t, dir, "01-openspec.md")
	wfs, err := loadWorkflowFiles(dir)
	if err != nil {
		t.Fatalf("loadWorkflowFiles: %v", err)
	}
	if len(wfs) != 2 {
		t.Fatalf("len = %d, want 2", len(wfs))
	}
	if wfs[0].Name != "01-openspec" {
		t.Fatalf("wfs[0].Name = %q, want 01-openspec", wfs[0].Name)
	}
	if wfs[1].Name != "02-deps" {
		t.Fatalf("wfs[1].Name = %q, want 02-deps", wfs[1].Name)
	}
}

func TestLoadWorkflowFilesSkipsHidden(t *testing.T) {
	dir := t.TempDir()
	makeWorkflowFile(t, dir, "openspec.md")
	// A hidden file would not parse anyway (it has no frontmatter
	// delimiter), but the discovery filter must exclude it before
	// the parser ever sees it.
	if err := os.WriteFile(filepath.Join(dir, ".disabled.md"),
		[]byte("not a workflow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wfs, err := loadWorkflowFiles(dir)
	if err != nil {
		t.Fatalf("loadWorkflowFiles: %v", err)
	}
	if len(wfs) != 1 {
		t.Fatalf("len = %d, want 1", len(wfs))
	}
	if wfs[0].Name != "openspec" {
		t.Fatalf("wfs[0].Name = %q, want openspec", wfs[0].Name)
	}
}

func TestLoadWorkflowFilesIgnoresNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	makeWorkflowFile(t, dir, "openspec.md")
	if err := os.WriteFile(filepath.Join(dir, "README.txt"),
		[]byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wfs, err := loadWorkflowFiles(dir)
	if err != nil {
		t.Fatalf("loadWorkflowFiles: %v", err)
	}
	if len(wfs) != 1 {
		t.Fatalf("len = %d, want 1", len(wfs))
	}
	if wfs[0].Name != "openspec" {
		t.Fatalf("wfs[0].Name = %q, want openspec", wfs[0].Name)
	}
}

func TestLoadWorkflowFilesMissingDirNoOp(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	wfs, err := loadWorkflowFiles(missing)
	if err != nil {
		t.Fatalf("loadWorkflowFiles: want nil err for missing dir, got %v", err)
	}
	if wfs != nil {
		t.Fatalf("wfs = %v, want nil slice", wfs)
	}
}

func TestLoadWorkflowFilesSkipsSubdirectory(t *testing.T) {
	dir := t.TempDir()
	makeWorkflowFile(t, dir, "openspec.md")
	// A nested file with the right extension must not be picked up
	// because subdirectories are not traversed.
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	makeWorkflowFile(t, filepath.Join(dir, "nested"), "hidden.md")
	wfs, err := loadWorkflowFiles(dir)
	if err != nil {
		t.Fatalf("loadWorkflowFiles: %v", err)
	}
	if len(wfs) != 1 {
		t.Fatalf("len = %d, want 1 (nested file must be ignored)", len(wfs))
	}
	if wfs[0].Name != "openspec" {
		t.Fatalf("wfs[0].Name = %q, want openspec", wfs[0].Name)
	}
}
