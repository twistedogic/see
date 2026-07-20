package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAgent records the path each Run was invoked with. logPath, when
// non-nil, overrides the default success-path return so a test can
// exercise the capture-failure path (logPath pointing at "" means
// capture was unavailable; nil preserves the default non-empty path).
// prompts records the 4th Run argument (the rendered prompt) on every
// call so tests can assert what `pi` would receive.
type fakeAgent struct {
	runs    []string
	prompts []string
	err     error
	logPath *string
	onRun   func() error
}

func (f *fakeAgent) Run(_ context.Context, path, _, prompt string) (string, error) {
	f.runs = append(f.runs, path)
	f.prompts = append(f.prompts, prompt)
	if f.onRun != nil {
		if err := f.onRun(); err != nil {
			return "", err
		}
	}
	if f.err != nil {
		return "", f.err
	}
	if f.logPath != nil {
		return *f.logPath, nil
	}
	return "/tmp/fakeAgent.jsonl", nil
}

// recordingObserver captures the event sequence emitted by a Watcher.
type recordingObserver struct{ events []Event }

func (r *recordingObserver) Observe(e Event) { r.events = append(r.events, e) }

// eventTypes returns the type names in order, useful when a test only cares
// about sequence shape and not full event payloads.
func (r *recordingObserver) eventTypes() []string {
	out := make([]string, len(r.events))
	for i, e := range r.events {
		out[i] = fmt.Sprintf("%T", e)
	}
	return out
}

func TestSelectRunMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		isTTY   bool
		want    runMode
		wantErr string
	}{
		{"log with TTY", "log", true, modeLog, ""},
		{"log without TTY", "log", false, modeLog, ""},
		{"tui with TTY", "tui", true, modeTUI, ""},
		{"tui without TTY", "tui", false, modeUnknown, "see: --mode=tui requires a TTY; rerun with --mode=log"},
		{"unknown value with TTY", "foo", true, modeUnknown, `see: unknown --mode="foo" (want: tui, log)`},
		{"empty value with TTY", "", true, modeUnknown, `see: unknown --mode="" (want: tui, log)`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMode, gotErr := selectRunMode(tc.mode, tc.isTTY)
			if gotMode != tc.want {
				t.Fatalf("mode = %v, want %v", gotMode, tc.want)
			}
			gotMsg := ""
			if gotErr != nil {
				gotMsg = "see: " + gotErr.Error()
			}
			if gotMsg != tc.wantErr {
				t.Fatalf("err = %q, want %q", gotMsg, tc.wantErr)
			}
		})
	}
}

func TestListActiveOpenSpecChanges(t *testing.T) {
	dir := t.TempDir()
	changes := filepath.Join(dir, "openspec", "changes")
	for _, name := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(changes, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// file in changes/ must be ignored.
	if err := os.WriteFile(filepath.Join(changes, "readme.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ListActiveOpenSpecChanges(dir)
	want := map[string]bool{"alpha": true, "beta": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, name := range got {
		if !want[name] {
			t.Fatalf("unexpected change %q in %v", name, got)
		}
	}
}

func TestRenderTemplateSubstitutesChangeInPromptAndCommit(t *testing.T) {
	for template, want := range map[string]string{
		"Apply {change}":                 "Apply add-foo",
		"see: apply {change} ({change})": "see: apply add-foo (add-foo)",
		"Apply {change} in {repo}":       "Apply add-foo in {repo}",
	} {
		if got := renderTemplate(template, "add-foo"); got != want {
			t.Errorf("renderTemplate(%q) = %q, want %q", template, got, want)
		}
	}
	if got, want := renderTemplate("Apply {change}", ""), "Apply "; got != want {
		t.Errorf("renderTemplate() = %q, want %q", got, want)
	}
}

func TestDefaultTemplateMentionsChange(t *testing.T) {
	if !strings.Contains(renderTemplate(defaultPromptTemplate, "add-foo"), "add-foo") {
		t.Fatalf("default template should mention change name after substitution; got %q",
			renderTemplate(defaultPromptTemplate, "add-foo"))
	}
}

func TestWatcherRendersUserPrompt(t *testing.T) {
	dir := bootstrapPromptRepo(t, "add-foo")
	agent := &fakeAgent{}
	w := Watcher{agent: agent, RetryCount: 1}
	w.SetPromptTemplate("Apply {change} now")
	if err := w.work(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if len(agent.prompts) != 1 {
		t.Fatalf("agent Run called %d times, want 1", len(agent.prompts))
	}
	if got, want := agent.prompts[0], "Apply add-foo now"; got != want {
		t.Fatalf("rendered prompt = %q, want %q", got, want)
	}
}

func TestWatcherFallsBackToEmbeddedDefault(t *testing.T) {
	dir := bootstrapPromptRepo(t, "add-foo")
	agent := &fakeAgent{}
	w := Watcher{agent: agent, RetryCount: 1}
	w.SetPromptTemplate("   ")
	if got, want := w.PromptTemplate, defaultPromptTemplate; got != want {
		t.Fatalf("PromptTemplate after whitespace setter = %q, want embedded default", got)
	}
	if err := w.work(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if len(agent.prompts) != 1 {
		t.Fatalf("agent Run called %d times, want 1", len(agent.prompts))
	}
	if got, want := agent.prompts[0], renderTemplate(defaultPromptTemplate, "add-foo"); got != want {
		t.Fatalf("rendered prompt = %q, want %q (default substituted with add-foo)", got, want)
	}
}

// bootstrapPromptRepo creates a temp git repository with a single
// active openspec change and HEAD on main. The prompt-template tests
// only need a working git tree + one active change to drive
// Watcher.work down to the Agent.Run call.
func bootstrapPromptRepo(t *testing.T, change string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := exec.Command("git", "-C", dir, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	if err := os.MkdirAll(filepath.Join(dir, "openspec", "changes", change), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPiAgentCapturesOutputToLogFile(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf stdout-line\nprintf stderr-line >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	logPath, err := (PiAgent{binary: script, logDir: logDir}).Run(context.Background(), dir, "task-1", "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(logPath, logDir) {
		t.Fatalf("logPath = %q, want prefix %q", logPath, logDir)
	}
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "stdout-line") || !strings.Contains(got, "stderr-line") {
		t.Fatalf("log content = %q, want agent stdout and stderr", got)
	}
}

func TestPiAgentRespectsSeeLogDir(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "custom")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf hello\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath, err := (PiAgent{binary: script, logDir: logDir}).Run(context.Background(), dir, "task-1", "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(logPath, logDir+string(filepath.Separator)) {
		t.Fatalf("logPath = %q, want prefix %q", logPath, logDir+string(filepath.Separator))
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("log file not created at %q: %v", logPath, err)
	}
}

func TestPiAgentPreservesExitCodeByDefault(t *testing.T) {
	script := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := (PiAgent{binary: script, logDir: t.TempDir()}).Run(context.Background(), t.TempDir(), "task-1", "prompt")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("Run() error = %v, want exit code 7", err)
	}
}

// Regression: PiAgent.Run must surface a per-run file creation failure
// as a non-nil error and an empty logPath, without invoking the agent.
// The agent must not run with stderr/stdout unredirected.
func TestPiAgentRunSurfacesFileCreateError(t *testing.T) {
	dir := t.TempDir()
	// SEE_LOG_DIR points at a path under a regular file -> MkdirAll
	// fails, so any os.Create at that location fails with ENOTDIR.
	// EnsureLogDir is intentionally NOT called: this pins the
	// PiAgent-level contract that capture failure propagates without
	// any silent fallback to running the agent unredirected.
	rogue := filepath.Join(dir, "file")
	if err := os.WriteFile(rogue, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(rogue, "logs")

	// Agent that records whether it was invoked. A flag file the
	// script would write proves whether Run let it through.
	marker := filepath.Join(dir, "agent-ran")
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	logPath, err := (PiAgent{binary: script, logDir: logDir}).Run(context.Background(), dir, "task-1", "prompt")
	if err == nil {
		t.Fatal("Run returned nil error, want non-nil on capture failure")
	}
	if logPath != "" {
		t.Fatalf("logPath = %q, want empty on capture failure", logPath)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("agent was invoked despite capture failure")
	}
}

func TestEnsureLogDirFailsWhenPathBlocked(t *testing.T) {
	dir := t.TempDir()
	// SEE_LOG_DIR points at a path under a regular file -> MkdirAll
	// of the leaf directory fails. ensureLogDir returns a wrapped
	// error; main() turns that into exit-2 with a stderr line.
	rogue := filepath.Join(dir, "file")
	if err := os.WriteFile(rogue, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEE_LOG_DIR", filepath.Join(rogue, "logs"))

	_, err := ensureLogDir()
	if err == nil {
		t.Fatal("ensureLogDir returned nil, want error")
	}
	if !strings.Contains(err.Error(), "log dir") {
		t.Fatalf("error = %v, want wrapped 'log dir' marker", err)
	}
}

func TestEventLoggerWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	events, err := openEventLogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()

	events.Observe(RepoSeen{Path: "/some/repo", HasOpenspec: true})
	events.Observe(ChangeStarted{Path: "/some/repo", Change: "task-1"})
	events.Observe(Warning{Path: "/some/repo", Change: "task-1", Msg: "rollback hiccup"})
	events.Observe(InfraError{Where: "watcher", Err: "boom"})

	files, err := filepath.Glob(filepath.Join(dir, "see--*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1: %v", len(files), files)
	}
	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), body)
	}
	// Each line is valid JSON; spot-check the first and last to
	// confirm fields round-trip through the default JSON encoder.
	var first, last struct {
		Event map[string]any `json:"event"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line[0] is not valid JSON: %v", err)
	}
	if first.Event["Path"] != "/some/repo" {
		t.Fatalf("line[0] event.Path = %v, want /some/repo", first.Event["Path"])
	}
	if first.Event["HasOpenspec"] != true {
		t.Fatalf("line[0] event.HasOpenspec = %v, want true", first.Event["HasOpenspec"])
	}
	if err := json.Unmarshal([]byte(lines[3]), &last); err != nil {
		t.Fatalf("line[3] is not valid JSON: %v", err)
	}
	if last.Event["Where"] != "watcher" || last.Event["Err"] != "boom" {
		t.Fatalf("line[3] event = %v, want InfraError watcher/boom", last.Event)
	}
}

// Contract: an eventLogger can mirror encoded events to a side
// sink (typically os.Stdout in --mode=log when stdout is not a
// TTY, so `see --mode=log | jq` works). The mirror receives valid
// JSONL in the same order as the file sink — one line per event.
// Used to fail in the same shape as TestEventLoggerWritesJSONL:
// the mirror was missing entirely and events only landed on disk.
func TestEventLoggerMirrorsEncodedEvents(t *testing.T) {
	dir := t.TempDir()
	events, err := openEventLogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()

	var buf bytes.Buffer
	events.SetMirror(&buf)

	events.Observe(RepoSeen{Path: "/some/repo", HasOpenspec: true})
	events.Observe(ChangeStarted{Path: "/some/repo", Change: "task-1"})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("mirror got %d lines, want 2:\n%s", len(lines), buf.String())
	}
	var first, second struct {
		Event map[string]any `json:"event"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("mirror line[0] is not valid JSON: %v", err)
	}
	if first.Event["Path"] != "/some/repo" || first.Event["HasOpenspec"] != true {
		t.Fatalf("mirror line[0] event = %v, want RepoSeen /some/repo", first.Event)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("mirror line[1] is not valid JSON: %v", err)
	}
	if second.Event["Change"] != "task-1" {
		t.Fatalf("mirror line[1] event = %v, want ChangeStarted task-1", second.Event)
	}
}

// Contract: each JSONL line carries the observed-at timestamp under
// field `ts` (RFC3339Nano in UTC) wrapping an `event` field with the
// underlying event payload. Without the envelope, downstream
// consumers have no time-correlation information beyond the file
// name's UTC second — once two events share the same second they
// lose ordering. The wrapper is the only line format; mixing wrapped
// and unwrapped lines is invalid.
func TestEventLoggerStampsObservedAtOnEachEntry(t *testing.T) {
	dir := t.TempDir()
	events, err := openEventLogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()

	before := time.Now().UTC().Add(-time.Second)
	events.Observe(RepoSeen{Path: "/some/repo", HasOpenspec: true})
	events.Observe(ChangeStarted{Path: "/some/repo", Change: "task-1"})
	after := time.Now().UTC().Add(time.Second)

	files, err := filepath.Glob(filepath.Join(dir, "see--*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), body)
	}
	for i, line := range lines {
		var entry struct {
			Ts    string          `json:"ts"`
			Event json.RawMessage `json:"event"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line[%d] is not valid JSON: %v\n%s", i, err, line)
		}
		if entry.Ts == "" {
			t.Fatalf("line[%d] missing ts field:\n%s", i, line)
		}
		got, err := time.Parse(time.RFC3339Nano, entry.Ts)
		if err != nil {
			t.Fatalf("line[%d] ts = %q is not RFC3339Nano: %v", i, entry.Ts, err)
		}
		// Tolerance: clock moves slightly between sample and the
		// observed-at capture; ±1s window keeps the test honest
		// without being flaky under load.
		if got.Before(before) || got.After(after) {
			t.Fatalf("line[%d] ts = %v outside [%v, %v]", i, got, before, after)
		}
		if len(entry.Event) == 0 {
			t.Fatalf("line[%d] event field empty:\n%s", i, line)
		}
	}
}

func TestEventLoggerFansOutToSecondary(t *testing.T) {
	dir := t.TempDir()
	events, err := openEventLogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()

	obs := &recordingObserver{}
	events.Attach(obs)

	events.Observe(ChangeStarted{Path: "/some/repo", Change: "task-1"})
	events.Observe(Warning{Path: "/some/repo", Change: "task-1", Msg: "test"})

	if got := len(obs.events); got != 2 {
		t.Fatalf("secondary observer got %d events, want 2", got)
	}
	if _, ok := obs.events[0].(ChangeStarted); !ok {
		t.Fatalf("event[0] = %T, want ChangeStarted", obs.events[0])
	}
	if _, ok := obs.events[1].(Warning); !ok {
		t.Fatalf("event[1] = %T, want Warning", obs.events[1])
	}
}

// Contract: when every attempt errors, Watcher.runOnce's inlined retry
// loop returns the LAST error, not nil and not the first error. This
// pins the post-refactor contract that the silent-failure scenario (a
// later attempt returning nil while a prior attempt errored) is
// unreachable.
func TestRunOnceRetryLoopReturnsLastErrorWhenAllAttemptsFail(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "p")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	// An active change is the trigger for work() to invoke the agent.
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "dummy"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, b, c := errors.New("a"), errors.New("b"), errors.New("c")
	calls := 0
	agent := &fakeAgent{onRun: func() error {
		calls++
		switch calls {
		case 1:
			return a
		case 2:
			return b
		default:
			return c
		}
	}}
	w := Watcher{agent: agent, RetryCount: 3}
	err := w.runOnce(context.Background(), []string{repo})
	if !errors.Is(err, c) {
		t.Fatalf("runOnce err = %v, want %v", err, c)
	}
	if calls != 3 {
		t.Fatalf("agent Run called %d times, want 3", calls)
	}
}

// Regression: after work completes without error, the worktree must be committed
// so the change survives a watcher restart. The agent's edits would otherwise
// sit unstaged in the working tree.
func TestWorkCommitsOnSuccess(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	// Guarantee HEAD is on a real branch named main; modern git auto-creates an
	// unborn main on the first commit, older git may land on master.
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	agent := &fakeAgent{
		onRun: func() error {
			if err := os.WriteFile(filepath.Join(repo, "applied.txt"), []byte("done"), 0o644); err != nil {
				return err
			}
			return os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			)
		},
	}
	w := Watcher{agent: agent, RetryCount: 1}
	ctx := t.Context()
	if err := w.work(ctx, repo); err != nil {
		t.Fatal(err)
	}
	// After the catch-up commit, see/<change>'s tip carries the apply commit.
	// The apply commit is the only `see`-owned commit on the workspace branch;
	// the user's starting branch (main) is unchanged.
	out, err := exec.Command("git", "-C", repo, "log", "--all", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	gl := strings.TrimSpace(string(out))
	if !strings.Contains(gl, "see: apply openspec change task-1") {
		t.Fatalf("expected apply commit in log --all, got:\n%s", gl)
	}
}

// Regression: runOnce on a successful change must emit RepoSeen,
// ChangeStarted, and ChangeDone in that order. This pins the observer
// seam added by the add-tui-grid change so future refactors cannot
// silently drop event emission.
func TestRunOnceEmitsEventSequenceOnSuccess(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	agent := &fakeAgent{
		onRun: func() error {
			// Write a file so git add/commit in work() have something
			// to stage; otherwise the trailing commit fails and emits
			// a Warning event that disrupts the pinned sequence.
			if err := os.WriteFile(filepath.Join(repo, "applied.txt"), []byte("done"), 0o644); err != nil {
				return err
			}
			return os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			)
		},
	}
	obs := &recordingObserver{}
	w := Watcher{agent: agent, RetryCount: 1, observer: obs}
	ctx := t.Context()
	if err := w.runOnce(ctx, []string{repo}); err != nil {
		t.Fatal(err)
	}
	got := obs.eventTypes()
	wantPrefix := []string{"main.RepoSeen", "main.ChangeStarted", "main.LogPath", "main.ChangeDone"}
	if len(got) < len(wantPrefix) {
		t.Fatalf("got %d events, want at least %d: %v", len(got), len(wantPrefix), got)
	}
	for i, name := range wantPrefix {
		if got[i] != name {
			t.Fatalf("event[%d] = %s, want %s (full sequence: %v)", i, got[i], name, got)
		}
	}
	// Confirm the RepoSeen payload reports HasOpenspec=true.
	rs, ok := obs.events[0].(RepoSeen)
	if !ok {
		t.Fatalf("first event is not RepoSeen: %T", obs.events[0])
	}
	if !rs.HasOpenspec {
		t.Fatalf("RepoSeen.HasOpenspec = false, want true")
	}
	if rs.Path != repo {
		t.Fatalf("RepoSeen.Path = %q, want %q", rs.Path, repo)
	}
	cs, ok := obs.events[1].(ChangeStarted)
	if !ok {
		t.Fatalf("second event is not ChangeStarted: %T", obs.events[1])
	}
	if cs.Change != "task-1" || cs.Path != repo {
		t.Fatalf("ChangeStarted = %+v, want {Path: %q, Change: task-1}", cs, repo)
	}
	cd, ok := obs.events[3].(ChangeDone)
	if !ok {
		t.Fatalf("fourth event is not ChangeDone: %T", obs.events[3])
	}
	if cd.Change != "task-1" || cd.Path != repo {
		t.Fatalf("ChangeDone = %+v, want {Path: %q, Change: task-1}", cd, repo)
	}
}

// Regression: agent runs must not pollute the original branch directly. They
// run on a dedicated see/<change> branch; on success the user's starting
// branch is left untouched and HEAD stays on the workspace branch. Pins
// the post-remove-merge-step contract.
func TestWorkIsolatesAgentRunOnBranch(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	// Guarantee HEAD is on a real branch named main. Modern git auto-creates
	// one on first commit; only seed when the init/config didn't.
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	originalSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	agent := &fakeAgent{
		onRun: func() error {
			if err := os.WriteFile(filepath.Join(repo, "applied.txt"), []byte("done"), 0o644); err != nil {
				return err
			}
			return os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			)
		},
	}
	w := Watcher{agent: agent, RetryCount: 1}
	ctx := t.Context()
	if err := w.work(ctx, repo); err != nil {
		t.Fatal(err)
	}

	// (a) main is unchanged: still at the pre-run SHA, with no merge commit.
	postMainSHA, err := GetCurrentCommit(repo)
	if postMainSHA != originalSHA {
		// HEAD may not be on main anymore; follow main explicitly.
		mainSHA, err := exec.Command("git", "-C", repo, "rev-parse", "main").CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(mainSHA)) != originalSHA {
			t.Fatalf("main moved: pre=%s post=%s", originalSHA, strings.TrimSpace(string(mainSHA)))
		}
	}
	// Avoid "declared and not used" if the inline branch fires.
	_ = postMainSHA
	// (a-extra) The captured sha pre-run must still resolve to main's tip.
	mainSHAOut, err := exec.Command("git", "-C", repo, "rev-parse", "main").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(mainSHAOut)) != originalSHA {
		t.Fatalf("main advanced; pre=%s post=%s", originalSHA, strings.TrimSpace(string(mainSHAOut)))
	}
	logMain, err := exec.Command("git", "-C", repo, "log", "--oneline", "main").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logMain), "see: merge openspec change task-1") {
		t.Fatalf("expected NO merge commit on main, got:\n%s", logMain)
	}

	// (b) see/<change> exists at run end.
	branches, err := exec.Command("git", "-C", repo, "branch", "--list", "see/task-1").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(branches)) == "" {
		t.Fatalf("expected see/task-1 to exist, got:\n%s", branches)
	}

	// (c) working tree on see/<change>, not on main.
	branch, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(branch)); got != "see/task-1" {
		t.Fatalf("working tree on %q, want see/task-1", got)
	}

	// (d) agent's apply commit reachable from see/task-1 but NOT from main.
	logAll, err := exec.Command("git", "-C", repo, "log", "--all", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logAll), "see: apply openspec change task-1") {
		t.Fatalf("expected apply commit in log --all, got:\n%s", logAll)
	}
	reachable, err := exec.Command("git", "-C", repo, "merge-base", "--is-ancestor", "HEAD", "main").CombinedOutput()
	if err == nil {
		t.Fatalf("apply commit reachable from main, expected NOT reachable:\n%s", reachable)
	}
}

// Regression: runOnce must pass the repo subdir path to work, not the parent wd.
func TestRunOncePassesRepoPathToAgent(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	// non-repo sibling must be skipped.
	if err := os.Mkdir(filepath.Join(root, "not-a-repo"), 0o755); err != nil {
		t.Fatal(err)
	}

	agent := &fakeAgent{err: nil}
	obs := &recordingObserver{}
	w := Watcher{agent: agent, RetryCount: 1, observer: obs}
	ctx := t.Context()
	if err := w.runOnce(ctx, []string{repo}); err != nil {
		t.Fatal(err)
	}
	if len(agent.runs) != 1 || agent.runs[0] != repo {
		t.Fatalf("agent.Run called with %v, want exactly [%q]", agent.runs, repo)
	}
	// runOnce must emit exactly one RepoSeen for the proj repo; the
	// non-repo sibling emits none. The proj repo also picks up the
	// active change so ChangeStarted fires after RepoSeen.
	got := obs.eventTypes()
	if len(got) < 1 || got[0] != "main.RepoSeen" {
		t.Fatalf("first event must be RepoSeen, got: %v", got)
	}
	repoSeenCount := 0
	for _, name := range got {
		if name == "main.RepoSeen" {
			repoSeenCount++
		}
	}
	if repoSeenCount != 1 {
		t.Fatalf("RepoSeen count = %d, want 1 (full sequence: %v)", repoSeenCount, got)
	}
}

// Regression: when the agent errors mid-run, the original branch must be left
// untouched and the see/<change> branch must be deleted.
func TestWorkRollsBackBranchOnAgentFailure(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	preSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	agentErr := errors.New("agent boom")
	agent := &fakeAgent{
		err: agentErr,
		onRun: func() error {
			// Make a tracked commit on see/<change> so the rollback has to
			// wipe something real. Returns nil so fakeAgent.Run propagates f.err.
			if err := os.WriteFile(filepath.Join(repo, "halfway.txt"), []byte("x"), 0o644); err != nil {
				return err
			}
			add := exec.Command("git", "-C", repo, "add", "halfway.txt")
			if err := add.Run(); err != nil {
				return err
			}
			return exec.Command("git", "-C", repo, "commit", "-q", "-m", "halfway").Run()
		},
	}
	w := Watcher{agent: agent, RetryCount: 1}
	ctx := t.Context()
	err = w.work(ctx, repo)
	if !errors.Is(err, agentErr) {
		t.Fatalf("expected agent error %v, got %v", agentErr, err)
	}
	postSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if preSHA != postSHA {
		t.Fatalf("expected HEAD at %s after rollback, got %s", preSHA, postSHA)
	}
	if _, err := os.Stat(filepath.Join(repo, "halfway.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected halfway.txt rolled back, stat err = %v", err)
	}
	branches, err := exec.Command("git", "-C", repo, "branch", "--list", "see/task-1").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(branches)) != "" {
		t.Fatalf("expected see/task-1 to be deleted, got:\n%s", branches)
	}
	branch, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(branch)); got != "main" {
		t.Fatalf("working tree on %q, want main", got)
	}
}

// Regression: a drifted see/<change> branch from a prior partial run must be
// reset to the captured SHA before the agent runs, so old commits don't
// leak into the run. After a successful run the workspace branch is left
// in place on the user's working tree.
func TestWorkReusesExistingBranchAndResetsToOriginalSHA(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	originalSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-create see/task-1 with a sentinel commit past the original SHA.
	run("switch", "-c", "see/task-1")
	if err := os.WriteFile(filepath.Join(repo, "sentinel.txt"), []byte("drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "sentinel")
	run("switch", "main")
	agent := &fakeAgent{
		onRun: func() error {
			if err := os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(repo, "applied.txt"), []byte("done"), 0o644)
		},
	}
	w := Watcher{agent: agent, RetryCount: 1}
	ctx := t.Context()
	if err := w.work(ctx, repo); err != nil {
		t.Fatal(err)
	}
	// Sentinel wiped by reset --hard before the agent ran.
	if _, err := os.Stat(filepath.Join(repo, "sentinel.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected sentinel.txt gone after reset, stat err = %v", err)
	}
	// Main is unchanged: tip equals the pre-run SHA.
	mainSHAOut, err := exec.Command("git", "-C", repo, "rev-parse", "main").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(mainSHAOut)) != originalSHA {
		t.Fatalf("main advanced: pre=%s post=%s", originalSHA, strings.TrimSpace(string(mainSHAOut)))
	}
	// No merge commit on main (the success path no longer merges).
	logMain, err := exec.Command("git", "-C", repo, "log", "--oneline", "main").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logMain), "see: merge openspec change task-1") {
		t.Fatalf("expected no merge commit on main, got:\n%s", logMain)
	}
	// The workspace branch is left in place at run end.
	branches, err := exec.Command("git", "-C", repo, "branch", "--list", "see/task-1").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(branches)) == "" {
		t.Fatalf("expected see/task-1 to exist at run end, got:\n%s", branches)
	}
	// workspace branch tip is the catch-up commit whose only parent is the
	// pre-run SHA. (Single-commit agent run, no merge.) The catch-up
	// commit's parent must be the captured pre-run SHA.
	showWS, err := exec.Command("git", "-C", repo, "log", "--pretty=%H %P", "see/task-1", "-1").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(strings.TrimSpace(string(showWS)))
	if len(fields) < 2 {
		t.Fatalf("expected workspace tip with 1 parent, got %q", showWS)
	}
	if fields[1] != originalSHA {
		t.Fatalf("workspace tip's parent = %s, want %s", fields[1], originalSHA)
	}
}

// Regression: Watcher.work must refuse to run on a detached HEAD before
// mutating repo state. v1 contract is "bail with an error"; the user must
// switch to a real branch first.
func TestWorkRejectsDetachedHead(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	detachedSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	run("checkout", detachedSHA)
	branch, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(branch)); got != "HEAD" {
		t.Fatalf("expected HEAD detached, got branch %q", got)
	}
	agent := &fakeAgent{err: nil}
	w := Watcher{agent: agent, RetryCount: 1}
	ctx := t.Context()
	err = w.work(ctx, repo)
	if err == nil {
		t.Fatal("expected error from work on detached HEAD, got nil")
	}
	listOut, err := exec.Command("git", "-C", repo, "branch", "--list").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(listOut), "see/") {
		t.Fatalf("expected no see/ branches, got:\n%s", listOut)
	}
	// Bail happens before any commit or branch creation, so HEAD SHA must
	// still match the fixture's pre-detach SHA.
	postSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if postSHA != detachedSHA {
		t.Fatalf("expected HEAD at %s after bail, got %s", detachedSHA, postSHA)
	}
}

// Regression: RetryAttempt events must fire between failed attempts,
// not on the first attempt. Pin the ordering for the agent-fails-twice-
// then-succeeds scenario.
func TestObserverReceivesRetrySequence(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	var calls int
	agent := &fakeAgent{
		onRun: func() error {
			calls++
			if calls < 3 {
				return errors.New("flake")
			}
			// Write a file so the trailing git commit has something to
			// stage; otherwise the retry-success path emits a Warning
			// event that disrupts the pinned sequence.
			if err := os.WriteFile(filepath.Join(repo, "applied.txt"), []byte("done"), 0o644); err != nil {
				return err
			}
			return os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			)
		},
	}
	obs := &recordingObserver{}
	w := Watcher{agent: agent, RetryCount: 3, observer: obs}
	ctx := t.Context()
	if err := w.runOnce(ctx, []string{repo}); err != nil {
		t.Fatal(err)
	}
	got := obs.eventTypes()
	want := []string{
		"main.RepoSeen",
		"main.ChangeStarted",
		"main.RetryAttempt",
		"main.ChangeStarted",
		"main.RetryAttempt",
		"main.ChangeStarted",
		"main.LogPath",
		"main.ChangeDone",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("event[%d] = %s, want %s (full sequence: %v)", i, got[i], name, got)
		}
	}
	// RetryAttempt payload sanity-check: N=2 on first retry, N=3 on second.
	if ra, ok := obs.events[2].(RetryAttempt); !ok || ra.N != 2 || ra.Max != 3 {
		t.Fatalf("event[2] = %+v, want RetryAttempt{N:2, Max:3}", obs.events[2])
	}
	if ra, ok := obs.events[4].(RetryAttempt); !ok || ra.N != 3 || ra.Max != 3 {
		t.Fatalf("event[4] = %+v, want RetryAttempt{N:3, Max:3}", obs.events[4])
	}
}

// Regression: after retryN exhausts attempts, ChangeFailed must fire
// exactly once with the final error.
func TestObserverReceivesChangeFailedAfterRetriesExhausted(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	agentErr := errors.New("always fails")
	agent := &fakeAgent{err: agentErr}
	obs := &recordingObserver{}
	w := Watcher{agent: agent, RetryCount: 2, observer: obs}
	ctx := t.Context()
	if err := w.runOnce(ctx, []string{repo}); err == nil {
		t.Fatal("expected runOnce to return error after retries exhausted")
	}
	got := obs.eventTypes()
	want := []string{
		"main.RepoSeen",
		"main.ChangeStarted",
		"main.RetryAttempt",
		"main.ChangeStarted",
		"main.ChangeFailed",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("event[%d] = %s, want %s (full sequence: %v)", i, got[i], name, got)
		}
	}
	cf, ok := obs.events[len(got)-1].(ChangeFailed)
	if !ok {
		t.Fatalf("last event is not ChangeFailed: %T", obs.events[len(got)-1])
	}
	if cf.Change != "task-1" {
		t.Fatalf("ChangeFailed.Change = %q, want task-1", cf.Change)
	}
	if !strings.Contains(cf.Err, "always fails") {
		t.Fatalf("ChangeFailed.Err = %q, want it to contain %q", cf.Err, "always fails")
	}
}

// Regression: a git repo with no openspec/ must still emit RepoSeen
// (with HasOpenspec=false) and nothing else.
func TestRepoSeenFiresForRepoWithoutOpenspec(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	obs := &recordingObserver{}
	agent := &fakeAgent{err: nil}
	w := Watcher{agent: agent, RetryCount: 1, observer: obs}
	ctx := t.Context()
	if err := w.runOnce(ctx, []string{repo}); err != nil {
		t.Fatal(err)
	}
	got := obs.eventTypes()
	if len(got) != 1 || got[0] != "main.RepoSeen" {
		t.Fatalf("events = %v, want exactly [RepoSeen]", got)
	}
	rs := obs.events[0].(RepoSeen)
	if rs.HasOpenspec {
		t.Fatalf("RepoSeen.HasOpenspec = true, want false")
	}
	if rs.Path != repo {
		t.Fatalf("RepoSeen.Path = %q, want %q", rs.Path, repo)
	}
	// And the agent must not have been invoked for a no-spec repo.
	if len(agent.runs) != 0 {
		t.Fatalf("agent.Run called %d times for no-spec repo, want 0", len(agent.runs))
	}
}

// Regression: a Watcher constructed without an observer must run the
// same code path as today without panicking. Existing tests cover this
// transitively; this is the explicit assertion.
func TestNilObserverIsSafe(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	agent := &fakeAgent{
		onRun: func() error {
			return os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			)
		},
	}
	// No observer set; the watcher should still run end-to-end.
	w := Watcher{agent: agent, RetryCount: 1}
	ctx := t.Context()
	if err := w.runOnce(ctx, []string{repo}); err != nil {
		t.Fatal(err)
	}
	if len(agent.runs) != 1 {
		t.Fatalf("agent.Run called %d times, want 1", len(agent.runs))
	}
}

// Regression: Watcher.work must emit LogPath after a successful agent
// run with capture enabled. Pins the surfacing of the per-run log
// path so TUI consumers can render it.
func TestWorkEmitsLogPathOnSuccessfulCapture(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	mkRepoWithChange(t, repo, "task-1")
	want := "/var/folders/x/see/task-1--20260714T153022--99999.jsonl"
	agent := &fakeAgent{
		logPath: &want,
		onRun: func() error {
			return os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			)
		},
	}
	obs := &recordingObserver{}
	w := Watcher{agent: agent, RetryCount: 1, observer: obs}
	ctx := t.Context()
	if err := w.work(ctx, repo); err != nil {
		t.Fatal(err)
	}
	var lp *LogPath
	for _, e := range obs.events {
		if p, ok := e.(LogPath); ok {
			lp = &p
		}
	}
	if lp == nil {
		t.Fatalf("no LogPath event emitted; events: %v", obs.eventTypes())
	}
	if lp.Path != want {
		t.Fatalf("LogPath.Path = %q, want %q", lp.Path, want)
	}
	if lp.Change != "task-1" {
		t.Fatalf("LogPath.Change = %q, want task-1", lp.Change)
	}
}

// Regression: when the agent signals capture-failure (empty logPath),
// work must NOT emit LogPath. Capture is observability, not correctness,
// and a missing file means there is no path to surface.
func TestWorkDoesNotEmitLogPathOnCaptureFailure(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	mkRepoWithChange(t, repo, "task-1")
	empty := ""
	agent := &fakeAgent{
		logPath: &empty,
		onRun: func() error {
			return os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			)
		},
	}
	obs := &recordingObserver{}
	w := Watcher{agent: agent, RetryCount: 1, observer: obs}
	ctx := t.Context()
	if err := w.work(ctx, repo); err != nil {
		t.Fatal(err)
	}
	for _, e := range obs.events {
		if _, ok := e.(LogPath); ok {
			t.Fatalf("LogPath emitted despite capture failure; events: %v", obs.eventTypes())
		}
	}
}

// Regression: with Watcher.Once=true, Watch returns after a single pass,
// even when the context is still live. Agent is invoked exactly once.
func TestWatchReturnsAfterOnePassWhenOnce(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	mkRepoWithChange(t, repo, "task-1")
	agent := &fakeAgent{
		onRun: func() error {
			return os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			)
		},
	}
	w := Watcher{agent: agent, RetryCount: 1, Once: true}
	ctx := t.Context()
	if err := w.Watch(ctx, []string{repo}); err != nil {
		t.Fatalf("Watch returned %v, want nil", err)
	}
	if len(agent.runs) != 1 {
		t.Fatalf("agent.Run called %d times, want 1", len(agent.runs))
	}
}

// Regression: with Watcher.Once=false (default), Watch keeps looping
// until the context is cancelled. Agent runs at least once and ctx
// cancellation propagates a clean nil return.
func TestWatchLoopsUntilCtxCancelWhenNotOnce(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	mkRepoWithChange(t, repo, "task-1")
	agent := &fakeAgent{err: nil} // no onRun → change stays active → loop continues
	w := Watcher{agent: agent, RetryCount: 1, Once: false}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	if err := w.Watch(ctx, []string{repo}); err != nil {
		t.Fatalf("Watch returned %v, want nil after cancel", err)
	}
	if len(agent.runs) < 1 {
		t.Fatalf("agent.Run called %d times, want >= 1", len(agent.runs))
	}
}

// Regression: with Watcher.Once=false, a non-nil runOnce error must
// surface immediately from Watch without invoking runOnce again.
func TestWatchStopsOnFirstPassError(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	mkRepoWithChange(t, repo, "task-1")
	boom := errors.New("boom")
	agent := &fakeAgent{err: boom}
	w := Watcher{agent: agent, RetryCount: 1, Once: false}
	ctx := t.Context()
	err := w.Watch(ctx, []string{repo})
	if err == nil {
		t.Fatal("Watch returned nil, want error")
	}
	if len(agent.runs) != 1 {
		t.Fatalf("agent.Run called %d times, want 1", len(agent.runs))
	}
}

// newIntervalTestRepo bootstraps a one-commit git repo on branch main
// with one active change that the agent archive on Run, so each
// pass-through of the watcher is immediately ready to run again.
func newIntervalTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	mkRepoWithChange(t, repo, "task-1")
	return repo
}

// runClock records the wall-clock offset (relative to the first
// observation) of each agent invocation so the test can assert the
// spacing between consecutive passes.
type runClock struct {
	mu      sync.Mutex
	first   time.Time
	offsets []time.Duration
}

func (r *runClock) record(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.first.IsZero() {
		r.first = t
	}
	r.offsets = append(r.offsets, t.Sub(r.first))
}

func (r *runClock) snapshot() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]time.Duration, len(r.offsets))
	copy(out, r.offsets)
	return out
}

// Regression: with Watcher.Once=false and a configured PollInterval,
// the first pass is immediate and the next pass does not start until
// at least the interval has elapsed since the previous pass.
func TestWatchDelaysNextPassByPollInterval(t *testing.T) {
	repo := newIntervalTestRepo(t)
	clock := &runClock{}
	agent := &fakeAgent{
		onRun: func() error {
			clock.record(time.Now())
			return nil
		},
	}
	interval := 80 * time.Millisecond
	w := Watcher{agent: agent, RetryCount: 1, Once: false, PollInterval: interval}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		// Give the loop room for two passes plus the interval.
		time.Sleep(3 * interval)
		cancel()
	}()
	if err := w.Watch(ctx, []string{repo}); err != nil {
		t.Fatalf("Watch returned %v, want nil", err)
	}
	offsets := clock.snapshot()
	if len(offsets) < 2 {
		t.Fatalf("agent.Run called %d times, want >= 2; offsets=%v", len(offsets), offsets)
	}
	if offsets[0] > 20*time.Millisecond {
		t.Fatalf("first pass took %v, want < 20ms (immediate)", offsets[0])
	}
	if offsets[1] < interval {
		t.Fatalf("second pass at %v, want >= %v (interval)", offsets[1], interval)
	}
}

// Regression: cancellation during a long configured interval wakes
// Watch promptly without starting another pass.
func TestWatchCancellationInterruptsPollInterval(t *testing.T) {
	repo := newIntervalTestRepo(t)
	agent := &fakeAgent{err: nil}
	// Long interval so the test only completes when ctx cancels.
	w := Watcher{agent: agent, RetryCount: 1, Once: false, PollInterval: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	go func() {
		// Wait for the first pass to complete (one Run), then cancel.
		for len(agent.runs) == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()
	if err := w.Watch(ctx, []string{repo}); err != nil {
		t.Fatalf("Watch returned %v, want nil", err)
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("Watch took %v to return after cancel, want < 2s", elapsed)
	}
	if len(agent.runs) != 1 {
		t.Fatalf("agent.Run called %d times, want exactly 1 (cancel before next pass)", len(agent.runs))
	}
}

// Regression: PollInterval=0 preserves the immediate-polling behavior
// — the next pass runs as soon as the previous one completes.
func TestWatchZeroIntervalPollsImmediately(t *testing.T) {
	repo := newIntervalTestRepo(t)
	clock := &runClock{}
	agent := &fakeAgent{
		onRun: func() error {
			clock.record(time.Now())
			return nil
		},
	}
	w := Watcher{agent: agent, RetryCount: 1, Once: false, PollInterval: 0}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for len(agent.runs) < 3 {
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()
	if err := w.Watch(ctx, []string{repo}); err != nil {
		t.Fatalf("Watch returned %v, want nil", err)
	}
	offsets := clock.snapshot()
	if len(offsets) < 3 {
		t.Fatalf("agent.Run called %d times, want >= 3 (immediate polling); offsets=%v", len(offsets), offsets)
	}
	if offsets[2] > 200*time.Millisecond {
		t.Fatalf("third pass at %v, want < 200ms (zero interval should not wait)", offsets[2])
	}
}

// Regression: NewWatcher carries the 5-minute default interval so
// production callers do not busy-poll. Direct Watcher literals still
// observe the zero-value zero-delay default — tests stay explicit.
func TestNewWatcherDefaultPollInterval(t *testing.T) {
	w := NewWatcher("pi", "/tmp/logs", 3, false)
	if got, want := w.PollInterval, 5*time.Minute; got != want {
		t.Fatalf("NewWatcher PollInterval = %v, want %v (5-minute default)", got, want)
	}
}

// Regression: a literal Watcher{} has PollInterval=0 so direct
// construction is explicit and matches the pre-change tight-poll loop.
func TestWatcherLiteralZeroPollInterval(t *testing.T) {
	var w Watcher
	if w.PollInterval != 0 {
		t.Fatalf("Watcher{}.PollInterval = %v, want 0", w.PollInterval)
	}
}

// mkRepoWithChange bootstraps a one-commit git repo on branch main with
// the given active change and an archive/ sibling. Shared by the
// Watcher.Watch tests that need a real repo for the watcher to find.
func mkRepoWithChange(t *testing.T, repo, change string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", change), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// Regression for add-custom-workflows task 1.3: a Watcher with no
// custom condition must keep the legacy OpenSpec contract end-to-end:
// active-change discovery, the `see/<change>` branch name, archival
// as completion, and the default OpenSpec-format commit message.
// Pins the compatibility-mode surface so adding the custom resolver
// in tasks 2/3 cannot silently regress it. Subtests give each scenario
// a fresh repo so the success path's branch state cannot leak into
// the failure path.
func TestCompatibilityModeRetainsOpenSpecContract(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := filepath.Join(t.TempDir(), "proj")
		mkRepoWithChange(t, repo, "task-1")
		preSHA, err := GetCurrentCommit(repo)
		if err != nil {
			t.Fatal(err)
		}

		agent := &fakeAgent{
			onRun: func() error {
				if err := os.WriteFile(filepath.Join(repo, "applied.txt"), []byte("done"), 0o644); err != nil {
					return err
				}
				return os.Rename(
					filepath.Join(repo, "openspec", "changes", "task-1"),
					filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
				)
			},
		}
		w := Watcher{agent: agent, RetryCount: 1}
		if err := w.work(t.Context(), repo); err != nil {
			t.Fatalf("compatibility-mode success: %v", err)
		}

		// Discovery + branch naming: the watcher created `see/task-1`
		// (the OpenSpec name) and nothing digest-shaped.
		branchList, err := exec.Command("git", "-C", repo, "branch", "--list", "see/task-1").CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(branchList)) == "" {
			t.Fatalf("expected see/task-1 to exist (compatibility branch name); got:\n%s", branchList)
		}
		digestList, err := exec.Command("git", "-C", repo, "branch", "--list", "see/[0-9a-f][0-9a-f][0-9a-f]*").CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(digestList)) != "" {
			t.Fatalf("compatibility mode must not create digest-named branches; got:\n%s", digestList)
		}

		// Default commit message: the OpenSpec subject format, no
		// unresolved {change} tokens.
		logOut, err := exec.Command("git", "-C", repo, "log", "--all", "--format=%s").CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		logLines := strings.Split(strings.TrimSpace(string(logOut)), "\n")
		wantSubject := "see: apply openspec change task-1"
		found := false
		for _, line := range logLines {
			if line == wantSubject {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected default commit subject %q in %v", wantSubject, logLines)
		}
		for _, line := range logLines {
			if strings.Contains(line, "{change}") {
				t.Fatalf("commit subject leaks unresolved {change} token: %q", line)
			}
		}

		// Archive completion: nothing active remains under
		// openspec/changes/ except the archive directory.
		active, err := os.ReadDir(filepath.Join(repo, "openspec", "changes"))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range active {
			if e.IsDir() && e.Name() != "archive" {
				t.Fatalf("expected no active change remaining, found %q", e.Name())
			}
		}

		// main is preserved: the apply commit lives on see/task-1,
		// never on main.
		mainSHA, err := exec.Command("git", "-C", repo, "rev-parse", "main").Output()
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(mainSHA)) != preSHA {
			t.Fatalf("main advanced during compatibility run: pre=%s post=%s", preSHA, strings.TrimSpace(string(mainSHA)))
		}
	})

	t.Run("failure_rolls_back", func(t *testing.T) {
		repo := filepath.Join(t.TempDir(), "proj")
		mkRepoWithChange(t, repo, "task-1")
		preSHA, err := GetCurrentCommit(repo)
		if err != nil {
			t.Fatal(err)
		}

		agentErr := errors.New("agent boom")
		agent := &fakeAgent{
			err: agentErr,
			onRun: func() error {
				// Commit on the see lane so rollback has real state
				// to undo; the agent error is returned after.
				if err := os.WriteFile(filepath.Join(repo, "halfway.txt"), []byte("x"), 0o644); err != nil {
					return err
				}
				add := exec.Command("git", "-C", repo, "add", "halfway.txt")
				if err := add.Run(); err != nil {
					return err
				}
				return exec.Command("git", "-C", repo, "commit", "-q", "-m", "halfway").Run()
			},
		}
		w := Watcher{agent: agent, RetryCount: 1}
		err = w.work(t.Context(), repo)
		if !errors.Is(err, agentErr) {
			t.Fatalf("compatibility-mode failure: err = %v, want %v", err, agentErr)
		}

		// Rollback contract: HEAD restored to pre-run tip, the see
		// lane deleted, the working tree back on main, and the agent's
		// untracked commit artifact gone. (TestWorkRollsBackBranchOnAgentFailure
		// covers this in isolation; this subtest pins that the
		// compatibility contract is the one in force.)
		postSHA, err := GetCurrentCommit(repo)
		if err != nil {
			t.Fatal(err)
		}
		if postSHA != preSHA {
			t.Fatalf("HEAD after rollback = %s, want %s (pre-run snapshot)", postSHA, preSHA)
		}
		mainSHA, err := exec.Command("git", "-C", repo, "rev-parse", "main").Output()
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(mainSHA)) != preSHA {
			t.Fatalf("main advanced during failed compatibility run: pre=%s post=%s", preSHA, strings.TrimSpace(string(mainSHA)))
		}
		if _, err := os.Stat(filepath.Join(repo, "halfway.txt")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected halfway.txt removed by rollback; stat err = %v", err)
		}
		branches, err := exec.Command("git", "-C", repo, "branch", "--list", "see/task-1").CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(branches)) != "" {
			t.Fatalf("expected see/task-1 deleted on failure; got:\n%s", branches)
		}
		branch, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(string(branch)); got != "main" {
			t.Fatalf("working tree on %q, want main (compatibility rollback)", got)
		}
	})
}

func platformCondition(unix, windows string) string {
	if runtime.GOOS == "windows" {
		return windows
	}
	return unix
}

func TestResolveCustomConditionUsesPlatformShellAndNormalizesOutput(t *testing.T) {
	got, err := resolveCustomCondition(t.Context(), t.TempDir(), platformCondition(
		`printf 'add-dark-mode\r\n'`,
		`echo add-dark-mode`,
	))
	if err != nil {
		t.Fatalf("resolveCustomCondition: %v", err)
	}
	if got != "add-dark-mode" {
		t.Fatalf("normalized change = %q, want %q", got, "add-dark-mode")
	}
}

func TestResolveCustomConditionExitOneReportsIdle(t *testing.T) {
	got, err := resolveCustomCondition(t.Context(), t.TempDir(), platformCondition(
		`exit 1`,
		`exit /b 1`,
	))
	if err != nil {
		t.Fatalf("exit 1 returned error: %v", err)
	}
	if got != "" {
		t.Fatalf("idle change = %q, want empty", got)
	}
}

func TestResolveCustomConditionFailureIncludesStderr(t *testing.T) {
	_, err := resolveCustomCondition(t.Context(), t.TempDir(), platformCondition(
		`printf 'syntax error' >&2; exit 2`,
		`echo syntax error 1>&2 & exit /b 2`,
	))
	if err == nil {
		t.Fatal("resolveCustomCondition returned nil error for exit 2")
	}
	if !strings.Contains(err.Error(), "syntax error") {
		t.Fatalf("error = %q, want captured stderr", err)
	}
}

func TestResolveCustomConditionCancellationStopsShell(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dir := t.TempDir()
	result := make(chan error, 1)
	go func() {
		_, err := resolveCustomCondition(ctx, dir, platformCondition(
			`touch condition-started; sleep 30`,
			`echo started > condition-started & ping -n 30 127.0.0.1 >NUL`,
		))
		result <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "condition-started")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("condition did not start before cancellation test deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resolveCustomCondition did not stop after cancellation")
	}
}

func TestResolveCustomConditionRejectsInvalidSuccessfulOutput(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr string
	}{
		{
			name:    "empty",
			command: platformCondition(`printf ''`, `type nul`),
			wantErr: "empty",
		},
		{
			name:    "whitespace-only",
			command: platformCondition(`printf ' \t'`, `echo    `),
			wantErr: "empty",
		},
		{
			name:    "multiline",
			command: platformCondition(`printf 'first\nsecond\n'`, `echo first & echo second`),
			wantErr: "single-line",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveCustomCondition(t.Context(), t.TempDir(), tc.command)
			if err == nil {
				t.Fatal("resolveCustomCondition returned nil error for invalid output")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestCustomChangeDigestDefinesFullSHA256BranchIdentity(t *testing.T) {
	const want = "243ec3b4ff67401f58a9534d5661dc6bcf486321807fe4bc73db785264a7c1db"
	got := customChangeDigest("add-dark-mode")
	if got != want {
		t.Fatalf("customChangeDigest = %q, want full SHA-256 digest %q", got, want)
	}
	if branch := "see/" + got; branch != "see/"+want {
		t.Fatalf("custom branch = %q, want %q", branch, "see/"+want)
	}
}

func TestCustomChangeDigestIsStableAndDistinct(t *testing.T) {
	first := customChangeDigest("add-dark-mode")
	if repeated := customChangeDigest("add-dark-mode"); repeated != first {
		t.Fatalf("repeated digest = %q, want stable %q", repeated, first)
	}
	if changed := customChangeDigest("fix-cache"); changed == first {
		t.Fatalf("different changes produced the same digest %q", first)
	}
}

func TestCustomAgentLogFilenameUsesDigest(t *testing.T) {
	const digest = "982587f309f9eb3d4ba019c9ee283fa89351bc6fc8905c80c9924dc38d00a93a"
	const change = "../unsafe change"
	got := pathFor("/repos/myproj", customChangeDigest(change))
	if wantPrefix := "myproj--" + digest + "--"; !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("custom log filename = %q, want prefix %q", got, wantPrefix)
	}
	if strings.Contains(got, change) {
		t.Fatalf("custom log filename contains raw change: %q", got)
	}
	if filepath.Dir(got) != "." {
		t.Fatalf("custom log filename escaped its log directory: %q", got)
	}
}

// mkCleanCustomRepo bootstraps a one-commit git repo on branch main
// with no openspec/changes/. Used by tests that exercise the custom
// workflow path; the condition-driven resolver replaces the OpenSpec
// change directory in custom mode, so the repo must not depend on
// openspec/ to drive work discovery.
func mkCleanCustomRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	return repo
}

// branchTip returns the SHA the given branch points to, or fails the
// test if the branch does not exist.
func branchTip(t *testing.T, repo, branch string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-parse", branch).CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse %s: %v\n%s", branch, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Custom-mode branch lifecycle: the lane `see/<digest>` is created
// at the captured commit on the first run, preserved across runs
// when already checked out, and refused when the lane exists on a
// different branch. A dirty working tree blocks all three. These
// tests pin the four behaviors the watcher must satisfy before
// invoking the agent.

// TestCustomLaneRejectsDirtyWorkingTree: custom mode must refuse a
// dirty tree before any branching. The agent never runs, the lane is
// never created, and the operator's edits remain unchanged.
func TestCustomLaneRejectsDirtyWorkingTree(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, err := ensureCustomLane(repo, "add-foo")
	if err == nil {
		t.Fatal("ensureCustomLane accepted dirty tree; want error")
	}
	if created {
		t.Fatal("created = true on dirty tree, want false")
	}
	if !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("err = %v, want 'dirty' in message", err)
	}
	listOut, err := exec.Command("git", "-C", repo, "branch", "--list", "see/*").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(listOut)) != "" {
		t.Fatalf("expected no see/ branches after dirty rejection; got:\n%s", listOut)
	}
	if _, err := os.Stat(filepath.Join(repo, "untracked.txt")); err != nil {
		t.Fatalf("operator's untracked file should be preserved; stat err = %v", err)
	}
}

// TestCustomLaneFirstRunCreatesBranch: when see/<digest> does not
// exist, the watcher creates it at the captured current commit and
// checks it out before the agent runs. `created` reports true.
func TestCustomLaneFirstRunCreatesBranch(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	preSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	digest := customChangeDigest("add-foo")
	branch := "see/" + digest
	created, err := ensureCustomLane(repo, "add-foo")
	if err != nil {
		t.Fatalf("ensureCustomLane: %v", err)
	}
	if !created {
		t.Fatal("created = false on first run, want true")
	}
	if tip := branchTip(t, repo, branch); tip != preSHA {
		t.Fatalf("branch tip = %s, want %s (captured current commit)", tip, preSHA)
	}
	branchOut, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(branchOut)); got != branch {
		t.Fatalf("HEAD on %q, want %q (lane created and checked out)", got, branch)
	}
}

// TestCustomLaneResumesWithoutReset: when see/<digest> exists and is
// already checked out with prior successful commits, the watcher
// runs the agent from the lane tip without resetting or deleting
// prior commits. `created` reports false.
func TestCustomLaneResumesWithoutReset(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	digest := customChangeDigest("add-foo")
	branch := "see/" + digest
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("switch", "-c", branch)
	if err := os.WriteFile(filepath.Join(repo, "sentinel.txt"), []byte("drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "sentinel")
	preTip := branchTip(t, repo, branch)
	if preTip == branchTip(t, repo, "main") {
		t.Fatal("sentinel commit did not advance the lane past main")
	}
	created, err := ensureCustomLane(repo, "add-foo")
	if err != nil {
		t.Fatalf("ensureCustomLane: %v", err)
	}
	if created {
		t.Fatal("created = true on resume, want false")
	}
	if tip := branchTip(t, repo, branch); tip != preTip {
		t.Fatalf("lane tip moved on resume: pre=%s post=%s", preTip, tip)
	}
	// Prior commits reachable from the lane.
	logOut, err := exec.Command("git", "-C", repo, "log", "--oneline", branch).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logOut), "sentinel") {
		t.Fatalf("lane history dropped sentinel commit on resume:\n%s", logOut)
	}
	branchOut, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(branchOut)); got != branch {
		t.Fatalf("HEAD on %q, want %q (lane preserved checked-out)", got, branch)
	}
}

// TestCustomLaneRefusesWhenLaneNotCheckedOut: when see/<digest>
// exists but the operator has checked out a different branch, the
// watcher must refuse without switching or mutating either branch.
// `created` reports false and the agent never runs.
func TestCustomLaneRefusesWhenLaneNotCheckedOut(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	digest := customChangeDigest("add-foo")
	branch := "see/" + digest
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("switch", "-c", branch)
	if err := os.WriteFile(filepath.Join(repo, "sentinel.txt"), []byte("drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "sentinel")
	preLaneTip := branchTip(t, repo, branch)
	preMainTip := branchTip(t, repo, "main")
	run("switch", "main")

	created, err := ensureCustomLane(repo, "add-foo")
	if err == nil {
		t.Fatal("ensureCustomLane accepted lane on different branch; want error")
	}
	if created {
		t.Fatal("created = true on refusal, want false")
	}
	if !strings.Contains(err.Error(), branch) {
		t.Fatalf("err = %v, want lane name %q in message", err, branch)
	}
	if tip := branchTip(t, repo, branch); tip != preLaneTip {
		t.Fatalf("lane tip moved on refusal: pre=%s post=%s", preLaneTip, tip)
	}
	if tip := branchTip(t, repo, "main"); tip != preMainTip {
		t.Fatalf("main moved on refusal: pre=%s post=%s", preMainTip, tip)
	}
	branchOut, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(branchOut)); got != "main" {
		t.Fatalf("HEAD on %q, want main (refusal must not switch)", got)
	}
}

// TestCustomLaneAcceptsIgnoredOnlyChanges: files matched by .gitignore
// are not dirtiness for the custom lane check. A repo with only
// ignored files beyond the commit still qualifies as clean and the
// lane can be created or resumed.
func TestCustomLaneAcceptsIgnoredOnlyChanges(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", "-A")
	run("commit", "-q", "-m", "ignore ignored")
	if err := os.MkdirAll(filepath.Join(repo, "ignored"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "ignored", "cache.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, err := ensureCustomLane(repo, "add-foo")
	if err != nil {
		t.Fatalf("ensureCustomLane rejected ignored-only tree: %v", err)
	}
	if !created {
		t.Fatal("created = false on first run, want true")
	}
}

func TestRollbackCustomLane(t *testing.T) {
	const change = "add-foo"
	branch := "see/" + customChangeDigest(change)

	t.Run("existing lane restores its tip and untracked state", func(t *testing.T) {
		repo := mkCleanCustomRepo(t)
		run := func(args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = repo
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", "-A")
		run("commit", "-q", "-m", "ignore ignored")
		run("switch", "-c", branch)
		for _, name := range []string{"prior-b", "prior-c"} {
			if err := os.WriteFile(filepath.Join(repo, name+".txt"), []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
			run("add", "-A")
			run("commit", "-q", "-m", name)
		}
		preAttemptTip := branchTip(t, repo, branch)

		if err := os.WriteFile(filepath.Join(repo, "failed-commit.txt"), []byte("failed"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", "-A")
		run("commit", "-q", "-m", "failed attempt")
		if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("failed"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(repo, "ignored"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "ignored", "cache"), []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}

		agentErr := errors.New("agent failed")
		err := (Watcher{}).rollbackCustomLane(repo, change, branch, preAttemptTip, false, agentErr)
		if !errors.Is(err, agentErr) {
			t.Fatalf("rollback error = %v, want original agent error %v", err, agentErr)
		}
		if tip := branchTip(t, repo, branch); tip != preAttemptTip {
			t.Fatalf("lane tip after rollback = %s, want %s", tip, preAttemptTip)
		}
		if got := currentBranch(t, repo); got != branch {
			t.Fatalf("branch after rollback = %q, want %q", got, branch)
		}
		for _, name := range []string{"failed-commit.txt", "untracked.txt"} {
			if _, err := os.Stat(filepath.Join(repo, name)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s survived rollback; stat err = %v", name, err)
			}
		}
		if _, err := os.Stat(filepath.Join(repo, "ignored", "cache")); err != nil {
			t.Fatalf("ignored file did not survive rollback: %v", err)
		}
		logOut, err := exec.Command("git", "-C", repo, "log", "--oneline", branch).CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(logOut), "prior-b") || !strings.Contains(string(logOut), "prior-c") || strings.Contains(string(logOut), "failed attempt") {
			t.Fatalf("lane history after rollback is wrong:\n%s", logOut)
		}
	})

	t.Run("new lane restores source branch and is deleted", func(t *testing.T) {
		repo := mkCleanCustomRepo(t)
		run := func(args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = repo
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", "-A")
		run("commit", "-q", "-m", "ignore ignored")
		preAttemptTip := branchTip(t, repo, "main")
		created, err := ensureCustomLane(repo, change)
		if err != nil || !created {
			t.Fatalf("ensureCustomLane = (%v, %v), want (true, nil)", created, err)
		}
		if err := os.WriteFile(filepath.Join(repo, "failed-commit.txt"), []byte("failed"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", "-A")
		run("commit", "-q", "-m", "failed attempt")
		if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("failed"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(repo, "ignored"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "ignored", "cache"), []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}

		agentErr := errors.New("agent failed")
		err = (Watcher{}).rollbackCustomLane(repo, change, "main", preAttemptTip, true, agentErr)
		if !errors.Is(err, agentErr) {
			t.Fatalf("rollback error = %v, want original agent error %v", err, agentErr)
		}
		if got := currentBranch(t, repo); got != "main" {
			t.Fatalf("branch after rollback = %q, want main", got)
		}
		if tip := branchTip(t, repo, "main"); tip != preAttemptTip {
			t.Fatalf("main tip after rollback = %s, want %s", tip, preAttemptTip)
		}
		if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch).Run(); err == nil {
			t.Fatalf("new lane %q survived rollback", branch)
		}
		for _, name := range []string{"failed-commit.txt", "untracked.txt"} {
			if _, err := os.Stat(filepath.Join(repo, name)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s survived rollback; stat err = %v", name, err)
			}
		}
		if _, err := os.Stat(filepath.Join(repo, "ignored", "cache")); err != nil {
			t.Fatalf("ignored file did not survive rollback: %v", err)
		}
	})
}

func TestRollbackCustomLaneWarnsForEveryCleanupFailure(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	agentErr := errors.New("agent failed")
	obs := &recordingObserver{}
	err := (Watcher{observer: obs}).rollbackCustomLane(repo, "add-foo", "main", "deadbeef", true, agentErr)
	if !errors.Is(err, agentErr) {
		t.Fatalf("rollback error = %v, want original agent error %v", err, agentErr)
	}
	if len(obs.events) != 4 {
		t.Fatalf("warnings = %v, want one for each of switch, reset, clean, and branch deletion", obs.eventTypes())
	}
	for i, event := range obs.events {
		if _, ok := event.(Warning); !ok {
			t.Fatalf("event[%d] = %T, want Warning", i, event)
		}
	}
}

func currentBranch(t *testing.T, repo string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "symbolic-ref", "--short", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("current branch: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeSequenceCondition(t *testing.T, outputs ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sequence condition test uses a POSIX shell script")
	}
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	script := filepath.Join(dir, "condition")
	body := "#!/bin/sh\n"
	body += "n=$(cat \"" + state + "\" 2>/dev/null || printf 0)\n"
	body += "case \"$n\" in\n"
	for i, output := range outputs {
		if output == "<idle>" {
			body += fmt.Sprintf("%d) exit 1;;\n", i)
			continue
		}
		body += fmt.Sprintf("%d) printf '%s';;\n", i, strings.ReplaceAll(output, "'", "'\\\"'\\\"'"))
	}
	fallback := outputs[len(outputs)-1]
	if fallback == "<idle>" {
		body += "*) exit 1;;\n"
	} else {
		body += fmt.Sprintf("*) printf '%s';;\n", strings.ReplaceAll(fallback, "'", "'\\\"'\\\"'"))
	}
	body += "esac\n"
	body += "printf '%s' $((n + 1)) > \"" + state + "\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestCustomConditionIsLevelTriggeredAcrossPollingPasses(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	condition := writeSequenceCondition(t, "same-change\\n")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var runs int
	agent := &fakeAgent{onRun: func() error {
		runs++
		if runs == 2 {
			cancel()
		}
		return nil
	}}
	w := Watcher{agent: agent, Condition: condition, RetryCount: 1, PollInterval: 0}
	if err := w.Watch(ctx, []string{repo}); err != nil {
		t.Fatalf("Watch returned %v, want nil after cancellation", err)
	}
	if runs != 2 {
		t.Fatalf("agent runs = %d, want 2 for a condition that stays true", runs)
	}
	if got := currentBranch(t, repo); got != "see/"+customChangeDigest("same-change") {
		t.Fatalf("current branch = %q, want persistent custom lane", got)
	}
}

func TestCustomConditionExitOneLeavesRepoIdle(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	condition := platformCondition("exit 1", "exit /b 1")
	agent := &fakeAgent{}
	obs := &recordingObserver{}
	w := Watcher{agent: agent, Condition: condition, RetryCount: 1, Once: true, observer: obs}
	if err := w.Watch(t.Context(), []string{repo}); err != nil {
		t.Fatalf("Watch returned %v, want nil for idle condition", err)
	}
	if len(agent.runs) != 0 {
		t.Fatalf("agent runs = %d, want 0 for condition exit 1", len(agent.runs))
	}
	if len(obs.events) != 1 {
		t.Fatalf("events = %v, want exactly RepoSeen", obs.eventTypes())
	}
	rs, ok := obs.events[0].(RepoSeen)
	if !ok || rs.HasOpenspec {
		t.Fatalf("event = %+v, want RepoSeen with no change", obs.events[0])
	}
}

func TestCustomConditionChangeSelectsDifferentLane(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	condition := writeSequenceCondition(t, "first\\n", "second\\n")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var runs int
	agent := &fakeAgent{onRun: func() error {
		runs++
		if runs == 2 {
			cancel()
		}
		return nil
	}}
	w := Watcher{agent: agent, Condition: condition, RetryCount: 1, PollInterval: 0}
	if err := w.Watch(ctx, []string{repo}); err != nil {
		t.Fatalf("Watch returned %v, want nil after cancellation", err)
	}
	if runs != 2 {
		t.Fatalf("agent runs = %d, want 2 after condition changed", runs)
	}
	for _, change := range []string{"first", "second"} {
		branch := "see/" + customChangeDigest(change)
		if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch).Run(); err != nil {
			t.Fatalf("missing custom lane %q", branch)
		}
	}
	if got := currentBranch(t, repo); got != "see/"+customChangeDigest("second") {
		t.Fatalf("current branch = %q, want second custom lane", got)
	}
}

func TestCustomRetryReResolvesCondition(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	condition := writeSequenceCondition(t, "first\\n", "second\\n")
	var runs int
	agent := &fakeAgent{onRun: func() error {
		runs++
		if runs == 1 {
			return errors.New("retry me")
		}
		return nil
	}}
	w := Watcher{agent: agent, Condition: condition, RetryCount: 2, Once: true}
	if err := w.Watch(t.Context(), []string{repo}); err != nil {
		t.Fatalf("Watch returned %v, want retry success", err)
	}
	if runs != 2 {
		t.Fatalf("agent runs = %d, want 2", runs)
	}
	if got := currentBranch(t, repo); got != "see/"+customChangeDigest("second") {
		t.Fatalf("current branch = %q, want lane for re-resolved change", got)
	}
}

func TestCustomRetryConditionExitOneBecomesIdle(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	condition := writeSequenceCondition(t, "first\\n", "<idle>")
	var runs int
	agent := &fakeAgent{onRun: func() error {
		runs++
		return errors.New("retry into idle")
	}}
	w := Watcher{agent: agent, Condition: condition, RetryCount: 2, Once: true}
	if err := w.Watch(t.Context(), []string{repo}); err != nil {
		t.Fatalf("Watch returned %v, want nil when retry becomes idle", err)
	}
	if runs != 1 {
		t.Fatalf("agent runs = %d, want only the first attempt", runs)
	}
}

// Custom-mode catch-up commit: after a successful agent run, leftover
// changes receive a commit with the rendered custom commit message.
// Commits made by the agent stay intact; a successful run that does
// not change anything is a warning-free no-op. The custom lane
// stays checked out in every case so the next polling pass resumes
// the same persistent branch.

func TestCustomCatchUpCommitRendersCommitTemplate(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	condition := platformCondition(`printf 'add-foo'`, `echo add-foo`)
	agent := &fakeAgent{
		onRun: func() error {
			return os.WriteFile(filepath.Join(repo, "leftover.txt"), []byte("agent work"), 0o644)
		},
	}
	w := Watcher{
		agent:          agent,
		Condition:      condition,
		CommitTemplate: "see: complete {change}",
		RetryCount:     1,
		Once:           true,
	}
	if err := w.Watch(t.Context(), []string{repo}); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", repo, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "see: complete add-foo") {
		t.Fatalf("expected rendered commit message in log:\n%s", out)
	}
	// Lane remains checked out; the next polling pass resumes here.
	if got := currentBranch(t, repo); got != "see/"+customChangeDigest("add-foo") {
		t.Fatalf("current branch = %q, want custom lane checked out", got)
	}
}

func TestCustomCatchUpCommitPreservesAgentCommits(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	condition := platformCondition(`printf 'add-foo'`, `echo add-foo`)
	agent := &fakeAgent{
		onRun: func() error {
			if err := os.WriteFile(filepath.Join(repo, "agent.txt"), []byte("agent work"), 0o644); err != nil {
				return err
			}
			run("add", "-A")
			run("commit", "-q", "-m", "agent commit")
			return nil
		},
	}
	obs := &recordingObserver{}
	w := Watcher{
		agent:          agent,
		Condition:      condition,
		CommitTemplate: "see: complete {change}",
		RetryCount:     1,
		Once:           true,
		observer:       obs,
	}
	if err := w.Watch(t.Context(), []string{repo}); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", repo, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	log := string(out)
	if !strings.Contains(log, "agent commit") {
		t.Fatalf("agent commit missing from log:\n%s", log)
	}
	if strings.Contains(log, "see: complete add-foo") {
		t.Fatalf("catch-up commit ran despite agent committing everything:\n%s", log)
	}
	// No no-changes warning for the empty staged diff.
	for _, e := range obs.events {
		if _, ok := e.(Warning); ok {
			t.Fatalf("unexpected warning event when agent committed everything: %+v", e)
		}
	}
}

func TestCustomCatchUpCommitIsWarningFreeNoOpWhenUnchanged(t *testing.T) {
	repo := mkCleanCustomRepo(t)
	condition := platformCondition(`printf 'add-foo'`, `echo add-foo`)
	// agent has no onRun and no error → success with zero changes
	agent := &fakeAgent{}
	obs := &recordingObserver{}
	w := Watcher{
		agent:          agent,
		Condition:      condition,
		CommitTemplate: "see: complete {change}",
		RetryCount:     1,
		Once:           true,
		observer:       obs,
	}
	if err := w.Watch(t.Context(), []string{repo}); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", repo, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "see: complete add-foo") {
		t.Fatalf("expected no commit for unchanged run, got:\n%s", out)
	}
	for _, e := range obs.events {
		if _, ok := e.(Warning); ok {
			t.Fatalf("unexpected warning event for unchanged run: %+v", e)
		}
	}
}
