package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// workflowFileFrontmatter is the YAML frontmatter a workflow `.md`
// file carries between its two `---` lines. `Name` is decoded so
// the field comment can explain that the filename is authoritative;
// the value itself is never read. The strict decoder rejects
// unknown keys so a typo in the frontmatter (e.g. `condtion`) fails
// startup rather than silently dropping the field, matching the
// same contract the `config.yaml` strict decoder enforces.
type workflowFileFrontmatter struct {
	Name      string `yaml:"name"`      // ignored; filename is authoritative
	Condition string `yaml:"condition"` // required, nonblank
	Commit    string `yaml:"commit"`    // required, nonblank
	Model     string `yaml:"model"`     // optional; blank falls back to agent default
}

// parseWorkflowFile reads path as a workflow file: a YAML
// frontmatter between two `---` lines, followed by a Markdown body
// that becomes the workflow's prompt. The filename is what the
// caller passes as `WorkflowConfig.Name`; this function fills
// `Condition`, `Commit`, `Model`, and `Prompt` from the file
// contents and leaves `Name` empty so the caller cannot accidentally
// use a frontmatter `name:` to override the filename. Errors name
// the file path so the operator can find the offender.
func parseWorkflowFile(path string) (WorkflowConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowConfig{}, fmt.Errorf("workflow file %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	// CR-stripping keeps the delimiter match tolerant of CRLF line
	// endings; the body text keeps its CRs so the trailing-whitespace
	// trim can strip them as part of ` \t\r\n`.
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return WorkflowConfig{}, fmt.Errorf("workflow file %s: missing opening '---' line", path)
	}
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		return WorkflowConfig{}, fmt.Errorf("workflow file %s: missing closing '---' line", path)
	}
	var fm workflowFileFrontmatter
	dec := yaml.NewDecoder(bytes.NewReader([]byte(strings.Join(lines[1:endIdx], "\n"))))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil && err != io.EOF {
		return WorkflowConfig{}, fmt.Errorf("workflow file %s: frontmatter: %w", path, err)
	}
	if strings.TrimSpace(fm.Condition) == "" {
		return WorkflowConfig{}, fmt.Errorf("workflow file %s: condition is required", path)
	}
	if strings.TrimSpace(fm.Commit) == "" {
		return WorkflowConfig{}, fmt.Errorf("workflow file %s: commit is required", path)
	}
	body := strings.TrimPrefix(strings.Join(lines[endIdx+1:], "\n"), "\n")
	body = strings.TrimRight(body, " \t\r\n")
	if strings.TrimSpace(body) == "" {
		return WorkflowConfig{}, fmt.Errorf("workflow file %s: prompt body is required", path)
	}
	return WorkflowConfig{
		Prompt:    body,
		Condition: fm.Condition,
		Commit:    fm.Commit,
		Model:     fm.Model,
	}, nil
}
