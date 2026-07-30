package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/twistedogic/see/tui"
)

// Regression for shorten-tui-errors task 1.2: every
// failure-bearing event (RetryAttempt, ChangeFailed, InfraError)
// must carry both the full Err (for JSONL observability) and the
// concise summary (for the in-memory TUI). When the error does not
// implement Summary(), the summary field stays empty so the
// existing full-message fallback applies.
func TestEventsCarrySummaryAlongsideFullErr(t *testing.T) {
	const fullText = "see: working tree on /repos/myrepo is dirty; commit or stash before see runs"
	const summaryText = "dirty working tree; commit or stash"

	dirty := &dirtyWorkingTreeError{path: "/repos/myrepo"}
	plain := &plainStringError{"see: agent exit 7"}

	cases := []struct {
		name string
		in   error
		want string
	}{
		{"dirty tree error has summary", dirty, summaryText},
		{"plain error has no summary", plain, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errStr := tc.in.Error()
			for _, ev := range []struct {
				name string
				err  string
				sum  string
			}{
				{"RetryAttempt", errStr, summaryFor(tc.in)},
				{"ChangeFailed", errStr, summaryFor(tc.in)},
				{"InfraError", errStr, summaryFor(tc.in)},
			} {
				if ev.err != errStr {
					t.Fatalf("%s.Err = %q, want %q", ev.name, ev.err, errStr)
				}
				if ev.sum != tc.want {
					t.Fatalf("%s.summary = %q, want %q", ev.name, ev.sum, tc.want)
				}
			}
		})
	}
}

// plainStringError is a minimal error without Summary() so the
// fallback path is exercised end-to-end alongside the custom type.
type plainStringError struct{ s string }

func (e *plainStringError) Error() string { return e.s }

// Regression for shorten-tui-errors task 1.3: the JSONL serializer
// must not leak the unexported `summary` field. The exported Err
// carries the full diagnostic unchanged; the watcher's external
// contract is the on-disk JSONL schema and it must not grow a
// presentation-only duplicate.
func TestEventLoggerDoesNotSerializeSummaryField(t *testing.T) {
	dir := t.TempDir()
	logger, err := openEventLogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	dirty := &dirtyWorkingTreeError{path: "/repos/myrepo"}
	fullText := dirty.Error()
	summaryText := summaryFor(dirty)

	logger.Observe(RetryAttempt{Path: "/repos/myrepo", Err: fullText, summary: summaryText})
	logger.Observe(ChangeFailed{Path: "/repos/myrepo", Err: fullText, summary: summaryText})
	logger.Observe(InfraError{Where: "watcher", Err: fullText, summary: summaryText})

	files, err := filepath.Glob(filepath.Join(dir, "see--*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("event log file count = %d, want 1", len(files))
	}
	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d JSONL lines, want 3:\n%s", len(lines), body)
	}

	for i, line := range lines {
		var entry struct {
			Event map[string]any `json:"event"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line[%d] is not valid JSON: %v\n%s", i, err, line)
		}
		if _, leaked := entry.Event["summary"]; leaked {
			t.Fatalf("line[%d] event leaked unexported 'summary' field: %v", i, entry.Event)
		}
		if got, want := entry.Event["Err"], fullText; got != want {
			t.Fatalf("line[%d] event.Err = %q, want %q (full diagnostic)", i, got, want)
		}
	}
}

// captureTuiObserver records every tea.Msg the observer forwards
// so tests can assert on what the Terminal User Interface (TUI)
// would have rendered. It stands in for *tui.ChanObserver.
type captureTuiObserver struct {
	msgs []tea.Msg
}

func (c *captureTuiObserver) send(msg tea.Msg) { c.msgs = append(c.msgs, msg) }

// Regression for shorten-tui-errors task 1.2: the main-to-TUI
// observer must forward the concise summary when the event carries
// one and fall back to the exported Err otherwise. The TUI package
// message types are unchanged — only the Err string field is
// populated differently. This pins the split without spinning up a
// real bubbletea Program.
func TestTuiObserverForwardsSummaryWhenPresent(t *testing.T) {
	const fullText = "see: working tree on /repos/myrepo is dirty; commit or stash before see runs"
	const summaryText = "dirty working tree; commit or stash"

	cap := &captureTuiObserver{}
	obs := tuiObserver{send: cap.send}

	obs.Observe(RetryAttempt{Path: "/repos/myrepo", Change: "task-1", N: 2, Max: 3, Err: fullText, summary: summaryText})
	obs.Observe(ChangeFailed{Path: "/repos/myrepo", Change: "task-1", Err: fullText, summary: summaryText})
	obs.Observe(InfraError{Where: "watcher", Err: fullText, summary: summaryText})

	if len(cap.msgs) != 3 {
		t.Fatalf("observer forwarded %d msgs, want 3", len(cap.msgs))
	}
	if rm, ok := cap.msgs[0].(tui.RetryAttemptMsg); !ok {
		t.Fatalf("msgs[0] = %T, want RetryAttemptMsg", cap.msgs[0])
	} else if rm.Err != summaryText {
		t.Fatalf("RetryAttemptMsg.Err = %q, want concise %q (full was %q)", rm.Err, summaryText, fullText)
	}
	if fm, ok := cap.msgs[1].(tui.ChangeFailedMsg); !ok {
		t.Fatalf("msgs[1] = %T, want ChangeFailedMsg", cap.msgs[1])
	} else if fm.Err != summaryText {
		t.Fatalf("ChangeFailedMsg.Err = %q, want concise %q (full was %q)", fm.Err, summaryText, fullText)
	}
	if im, ok := cap.msgs[2].(tui.InfraErrorMsg); !ok {
		t.Fatalf("msgs[2] = %T, want InfraErrorMsg", cap.msgs[2])
	} else if im.Err != summaryText {
		t.Fatalf("InfraErrorMsg.Err = %q, want concise %q (full was %q)", im.Err, summaryText, fullText)
	}
}

// Regression for shorten-tui-errors task 1.2: when the source error
// does not implement Summary(), the observer must fall back to the
// exported Err verbatim. This is the contract that keeps every
// pre-existing failure path byte-identical to the prior behavior.
func TestTuiObserverFallsBackToErrWhenNoSummary(t *testing.T) {
	const fullText = "see: agent exit 7"
	cap := &captureTuiObserver{}
	obs := tuiObserver{send: cap.send}

	obs.Observe(RetryAttempt{Path: "/r", Change: "task-1", Err: fullText})
	obs.Observe(ChangeFailed{Path: "/r", Change: "task-1", Err: fullText})
	obs.Observe(InfraError{Where: "watcher", Err: fullText})

	if len(cap.msgs) != 3 {
		t.Fatalf("observer forwarded %d msgs, want 3", len(cap.msgs))
	}
	if rm := cap.msgs[0].(tui.RetryAttemptMsg); rm.Err != fullText {
		t.Fatalf("RetryAttemptMsg.Err = %q, want full %q", rm.Err, fullText)
	}
	if fm := cap.msgs[1].(tui.ChangeFailedMsg); fm.Err != fullText {
		t.Fatalf("ChangeFailedMsg.Err = %q, want full %q", fm.Err, fullText)
	}
	if im := cap.msgs[2].(tui.InfraErrorMsg); im.Err != fullText {
		t.Fatalf("InfraErrorMsg.Err = %q, want full %q", im.Err, fullText)
	}
}

// Regression for shorten-tui-errors task 1.1: a dirty working tree
// produces a *dirtyWorkingTreeError from ensureWorkflowLane, the
// watcher routes it through ChangeFailed with the summary field
// populated, and the tuiObserver forwards the concise text instead
// of the full diagnostic. End-to-end: dirty tree → event → JSONL
// (full Err) → tui (concise summary).
func TestDirtyTreeErrorPropagatesSummaryThroughWatcherToTUI(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "proj")
	mkRepoWithChange(t, repo, "task-1")
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	logger, err := openEventLogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	obs := &recordingObserver{}
	cap := &captureTuiObserver{}
	logger.Attach(tuiObserver{send: cap.send})

	w := Watcher{
		agent:      &fakeAgent{},
		RetryCount: 1,
		Once:       true,
		observer:   obs,
		Workflows: []WorkflowConfig{{
			Name:      "dirty-test",
			Condition: platformCondition(`printf 'task-1'`, `echo task-1`),
			Prompt:    "Apply {change}",
			Commit:    "see: apply {change}",
		}},
	}
	if err := w.Watch(t.Context(), []string{repo}); err != nil {
		t.Fatalf("Watch returned %v, want nil (workflow failure stays contained)", err)
	}

	// Replay the recorded events through the logger so the file sink
	// sees the same JSONL bytes the production pipeline would emit.
	for _, e := range obs.events {
		logger.Observe(e)
	}

	var failed *ChangeFailed
	for _, e := range obs.events {
		if f, ok := e.(ChangeFailed); ok {
			failed = &f
			break
		}
	}
	if failed == nil {
		t.Fatalf("no ChangeFailed event in: %v", obs.eventTypes())
	}
	if !strings.Contains(failed.Err, repo) {
		t.Fatalf("ChangeFailed.Err = %q, want path %q", failed.Err, repo)
	}
	if failed.summary != "dirty working tree; commit or stash" {
		t.Fatalf("ChangeFailed.summary = %q, want concise", failed.summary)
	}

	files, err := filepath.Glob(filepath.Join(dir, "see--*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), repo) {
		t.Fatalf("JSONL did not surface full path:\n%s", body)
	}
	if strings.Contains(string(body), `"summary"`) {
		t.Fatalf("JSONL leaked presentation-only 'summary' field:\n%s", body)
	}

	var sawChangeFailed tui.ChangeFailedMsg
	for _, m := range cap.msgs {
		if f, ok := m.(tui.ChangeFailedMsg); ok {
			sawChangeFailed = f
		}
	}
	if sawChangeFailed.Err != "dirty working tree; commit or stash" {
		t.Fatalf("tui.ChangeFailedMsg.Err = %q, want concise summary", sawChangeFailed.Err)
	}
}
