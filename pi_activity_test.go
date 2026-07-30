package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPiActivityParserSummarizesMeaningfulEvents(t *testing.T) {
	var got []string
	p := newPiActivityParser(func(activity string) { got = append(got, activity) })
	input := strings.Join([]string{
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"ignore"}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Finished\n\n the work"}]}}`,
		`{"type":"tool_execution_start","toolCallId":"call-1","toolName":"bash","args":{"command":"go test ./..."}}`,
		`{"type":"tool_execution_start","toolCallId":"call-2","toolName":"custom","args":{"secret":"do not show"}}`,
		`{"type":"tool_execution_end","toolCallId":"call-1","isError":false,"result":{"content":[{"type":"text","text":"full result must stay hidden"}]} }`,
		`{"type":"tool_execution_end","toolCallId":"call-2","isError":true,"error":"permission denied"}`,
		`{"type":"tool_execution_update","toolCallId":"call-1","partialResult":"ignore progress"}`,
		`{"type":"unknown_event","message":"ignore"}`,
		`diagnostic ` + "\x1b" + `[31mbeep` + "\x07",
	}, "\n")
	if _, err := p.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
	p.Flush()

	want := []string{
		"Finished the work",
		"▶ bash: go test ./...",
		"▶ custom",
		"✓ bash: go test ./...",
		"✗ custom: permission denied",
		"diagnostic beep",
	}
	if len(got) != len(want) {
		t.Fatalf("activities = %q, want %d (%q)", got, len(want), want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("activity[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestPiActivityParserIgnoresOversizedLinesAndBoundsState(t *testing.T) {
	var got []string
	p := newPiActivityParser(func(activity string) { got = append(got, activity) })
	oversized := strings.Repeat("x", maxPiParserLineBytes+100)
	if _, err := p.Write([]byte(oversized)); err != nil {
		t.Fatal(err)
	}
	if len(p.line) > maxPiParserLineBytes {
		t.Fatalf("retained line grew to %d bytes, limit is %d", len(p.line), maxPiParserLineBytes)
	}
	if _, err := p.Write([]byte("\nsmall diagnostic\n")); err != nil {
		t.Fatal(err)
	}
	p.Flush()
	if len(got) != 1 || got[0] != "small diagnostic" {
		t.Fatalf("activities after oversized line = %q, want only following diagnostic", got)
	}
}

func TestPiActivityParserTreatsEmbeddedNewlineAsFieldSeparator(t *testing.T) {
	var got []string
	p := newPiActivityParser(func(activity string) { got = append(got, activity) })
	if _, err := p.Write([]byte("first line\nsecond line\n")); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "first line" || got[1] != "second line" {
		t.Fatalf("activities = %q, want two sanitized lines", got)
	}
}

func TestPiAgentStreamsActivityBeforeExitAndPreservesRawBytes(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"type":"tool_execution_start","toolCallId":"1","toolName":"bash","args":{"command":"echo hi"}}` + "\n"
	rawPath := filepath.Join(dir, "raw.jsonl")
	if err := os.WriteFile(rawPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat '"+rawPath+"'\nsleep 0.2\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	activities := make(chan string, 1)
	result := make(chan error, 1)
	var logPath string
	go func() {
		var err error
		logPath, err = (PiAgent{binary: script, logDir: logDir}).Run(context.Background(), dir, "task", "prompt", "", func(activity string) {
			activities <- activity
		})
		result <- err
	}()

	select {
	case activity := <-activities:
		if !strings.Contains(activity, "bash") || !strings.Contains(activity, "echo hi") {
			t.Fatalf("activity = %q, want streamed tool summary", activity)
		}
	case <-time.After(time.Second):
		t.Fatal("activity arrived after process exit or not at all")
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != raw {
		t.Fatalf("raw log = %q, want exact bytes %q", body, raw)
	}
}

func TestPiAgentRunWithoutActivityKeepsLogModePath(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' diagnostic\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath, err := (PiAgent{binary: script, logDir: logDir}).Run(context.Background(), dir, "task", "prompt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "diagnostic" {
		t.Fatalf("log-mode raw output = %q, want diagnostic bytes", body)
	}
}
