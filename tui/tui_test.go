package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// driveMessages pumps msgs through Model.Update and returns the
// updated model. Each msg must implement tea.Msg (anything does).
func driveMessages(m *Model, msgs ...tea.Msg) *Model {
	for _, msg := range msgs {
		updated, _ := m.Update(msg)
		m, _ = updated.(*Model)
	}
	return m
}

func TestViewRendersRepoChangeDoneAndFooter(t *testing.T) {
	m := NewModel()
	m.width = 120
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "task-1"},
		ChangeDoneMsg{Path: repo, Change: "task-1"},
	)
	view := m.View().Content
	if !strings.Contains(view, "proj") {
		t.Fatalf("view missing repo basename:\n%s", view)
	}
	if !strings.Contains(view, "task-1") {
		t.Fatalf("view missing change name:\n%s", view)
	}
	if !strings.Contains(view, "done") {
		t.Fatalf("view missing done phase:\n%s", view)
	}
	// The footer's last row is the help bar (sourced from the typed
	// keymap). It must mention the quit binding.
	lines := strings.Split(view, "\n")
	if last := lines[len(lines)-1]; !strings.Contains(last, "quit") {
		t.Fatalf("view missing help bar on the footer's last row:\n%s", view)
	}
	if !strings.Contains(view, "1 done") {
		t.Fatalf("view missing live count:\n%s", view)
	}
}

func TestViewHandlesNoSpecRepo(t *testing.T) {
	m := NewModel()
	m.width = 120
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: false},
	)
	view := m.View().Content
	if !strings.Contains(view, "idle") {
		t.Fatalf("view missing idle phase for repo without openspec:\n%s", view)
	}
	// Layout: summary, header, row, footer. The row sits at index 2.
	lines := strings.Split(view, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines (summary + header + row), got %d:\n%s", len(lines), view)
	}
	row := lines[2]
	if !strings.Contains(row, "—") {
		t.Fatalf("no-spec row missing em-dash placeholder:\n%s", row)
	}
	if strings.Contains(row, "retry") || strings.Contains(row, "/0") {
		t.Fatalf("no-spec row should not show retry counters:\n%s", row)
	}
}

func TestViewTruncatesLongNames(t *testing.T) {
	m := NewModel()
	m.width = 120
	long := strings.Repeat("a", 50)
	repo := "/tmp/" + long
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "task-1"},
	)
	view := m.View().Content
	// The repo column is 24 wide; a 50-char name must be truncated to
	// fit, ending with the ellipsis.
	lines := strings.Split(view, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines (summary + header + row), got %d:\n%s", len(lines), view)
	}
	row := lines[2]
	if !strings.Contains(row, "…") {
		t.Fatalf("long name not truncated with ellipsis:\n%s", row)
	}
	// Make sure the untruncated name does NOT appear in the row.
	if strings.Contains(row, long) {
		t.Fatalf("full long name leaked into row:\n%s", row)
	}
}

func TestUpdateIgnoresUnknownEvents(t *testing.T) {
	m := NewModel()
	m.width = 120
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "task-1"},
	)
	beforeView := m.View().Content
	beforeRows := map[string]*RepoRow{}
	for k, v := range m.rows {
		beforeRows[k] = v
	}
	// unknownEv is a tea.Msg the tui package doesn't handle. Update
	// should return the model unchanged.
	type unknownEv struct{}
	updated, cmd := m.Update(unknownEv{})
	got, ok := updated.(*Model)
	if !ok {
		t.Fatalf("Update returned non-Model: %T", updated)
	}
	if cmd != nil {
		t.Fatalf("Update returned a non-nil Cmd for unknown event: %v", cmd)
	}
	if len(got.rows) != len(beforeRows) {
		t.Fatalf("row map changed: before %d, after %d", len(beforeRows), len(got.rows))
	}
	for k := range beforeRows {
		if _, ok := got.rows[k]; !ok {
			t.Fatalf("row %q disappeared after unknown event", k)
		}
	}
	if got.View().Content != beforeView {
		t.Fatalf("View changed for unknown event")
	}
}

func TestUpdateQuitsOnQ(t *testing.T) {
	m := NewModel()
	m.width = 120
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatalf("expected tea.Quit on q key, got nil cmd")
	}
	if updated == nil {
		t.Fatalf("expected non-nil model on q key")
	}
}

// TestUpdateQuitsOnCtrlC pins the ctrl+c half of the Quit binding so
// the typed keymap matches the historical quit key and not only q.
func TestUpdateQuitsOnCtrlC(t *testing.T) {
	m := NewModel()
	m.width = 120
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatalf("expected tea.Quit on ctrl+c, got nil cmd")
	}
	_ = updated
}

// TestUpdateHelpToggleFlipsShowAll pins the ? binding: it flips the
// help bar's ShowAll state and returns no command (the help package
// is otherwise stateless). The toggle starts false and alternates.
func TestUpdateHelpToggleFlipsShowAll(t *testing.T) {
	m := NewModel()
	m.width = 120
	if m.help.ShowAll {
		t.Fatal("help.ShowAll should start false (the short view)")
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "?"})
	if cmd != nil {
		t.Fatalf("? toggle should return no cmd, got %v", cmd)
	}
	m, _ = updated.(*Model)
	if !m.help.ShowAll {
		t.Fatal("pressing ? should flip ShowAll to true")
	}
	updated, cmd = m.Update(tea.KeyPressMsg{Text: "?"})
	if cmd != nil {
		t.Fatalf("second ? toggle should return no cmd, got %v", cmd)
	}
	m, _ = updated.(*Model)
	if m.help.ShowAll {
		t.Fatal("pressing ? again should flip ShowAll back to false")
	}
}

// Regression: AGE is time.Since(StartedAt), recomputed at render
// time, so the model must drive periodic re-renders while a row is
// in PhaseWorking — otherwise the column is frozen at whatever
// View() produced when the last event arrived. Init() returns the
// first tick; a tickMsg reschedules the next.
func TestTickDrivesRedrawDuringWork(t *testing.T) {
	m := NewModel()
	m.width = 120
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "c1"},
	)
	if m.Init() == nil {
		t.Fatalf("Init must return a tick cmd to start the redraw loop")
	}
	_, cmd := m.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatalf("tickMsg must reschedule the next tick (got nil cmd)")
	}
}

func TestPhaseString(t *testing.T) {
	cases := []struct {
		p    Phase
		want string
	}{
		{PhaseIdle, "idle"},
		{PhaseWorking, "working"},
		{PhaseDone, "done"},
		{PhaseFailed, "failed"},
	}
	for _, c := range cases {
		if got := c.p.String(); got != c.want {
			t.Fatalf("Phase(%d).String() = %q, want %q", c.p, got, c.want)
		}
	}
}

// TestViewRendersMeasureFailed: a MeasureFailedMsg drives the
// row to PhaseFailed with the failure message in the error
// column, identical to ChangeFailedMsg / CheckFailedMsg. The
// grid has one failed state; the message type stays distinct at
// the event layer so consumers can branch on the cause.
func TestViewRendersMeasureFailed(t *testing.T) {
	m := NewModel()
	m.width = 120
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "task-1"},
		MeasureFailedMsg{Path: repo, Change: "task-1", Err: "no improvement", Baseline: "0.73", Candidate: "0.71"},
	)
	view := m.View().Content
	if !strings.Contains(view, "failed") {
		t.Fatalf("view missing failed phase for MeasureFailed:\n%s", view)
	}
	if !strings.Contains(view, "no improvement") {
		t.Fatalf("view missing measure failure message:\n%s", view)
	}
}

func TestRowOrderingStableAcrossEvents(t *testing.T) {
	m := NewModel()
	m.width = 120
	// Send RepoSeen for two repos in order: first "alpha", then "beta".
	// Then a ChangeStarted for "beta" first, then "alpha". The View
	// should still render alpha before beta because scan order is
	// the order RepoSeen arrived.
	m = driveMessages(m,
		RepoSeenMsg{Path: "/wd/alpha", HasChange: true},
		RepoSeenMsg{Path: "/wd/beta", HasChange: true},
		ChangeStartedMsg{Path: "/wd/beta", Change: "c1"},
		ChangeStartedMsg{Path: "/wd/alpha", Change: "c2"},
	)
	view := m.View().Content
	alphaIdx := strings.Index(view, "alpha")
	betaIdx := strings.Index(view, "beta")
	if alphaIdx < 0 || betaIdx < 0 {
		t.Fatalf("missing repo names in view:\n%s", view)
	}
	if alphaIdx > betaIdx {
		t.Fatalf("alpha should render before beta; got alpha=%d beta=%d in:\n%s", alphaIdx, betaIdx, view)
	}
}

func TestViewFooterCountsByPhase(t *testing.T) {
	m := NewModel()
	m.width = 120
	m = driveMessages(m,
		RepoSeenMsg{Path: "/wd/done-repo", HasChange: true},
		ChangeDoneMsg{Path: "/wd/done-repo", Change: "c1"},
		RepoSeenMsg{Path: "/wd/working-repo", HasChange: true},
		ChangeStartedMsg{Path: "/wd/working-repo", Change: "c2"},
		RepoSeenMsg{Path: "/wd/idle-repo", HasChange: true},
		RepoSeenMsg{Path: "/wd/failed-repo", HasChange: true},
		ChangeFailedMsg{Path: "/wd/failed-repo", Change: "c3", Err: "boom"},
		RepoSeenMsg{Path: "/wd/nospec-repo", HasChange: false},
	)
	view := m.View().Content
	// The repo without openspec/ also renders at PhaseIdle, so the
	// footer carries "2 idle" (one real idle + the no-spec row).
	for _, want := range []string{"1 done", "1 working", "2 idle", "1 failed"} {
		if !strings.Contains(view, want) {
			t.Fatalf("footer missing %q:\n%s", want, view)
		}
	}
}

func TestViewHidesErrColumnBelow100Cols(t *testing.T) {
	m := NewModel()
	m.width = 90 // between 80 and 100: AGE shows, ERROR hides
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeFailedMsg{Path: repo, Change: "c1", Err: "boom"},
	)
	view := m.View().Content
	lines := strings.Split(view, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected summary+header+row, got %d lines:\n%s", len(lines), view)
	}
	header := lines[1]
	if !strings.Contains(header, "AGE") {
		t.Fatalf("header missing AGE column at width 90:\n%s", view)
	}
	if strings.Contains(header, "ERR") {
		t.Fatalf("ERROR column should be hidden at width 90:\n%s", view)
	}
	// Below the threshold the failure reason must not render at all.
	row := lines[2]
	if strings.Contains(row, "boom") {
		t.Fatalf("failure reason should not render below the ERROR width threshold:\n%s", row)
	}
}

func TestViewHidesAgeColumnBelow80Cols(t *testing.T) {
	m := NewModel()
	m.width = 60
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "c1"},
	)
	view := m.View().Content
	header := strings.Split(view, "\n")[0]
	if strings.Contains(header, "AGE") {
		t.Fatalf("AGE should be hidden at width 60:\n%s", view)
	}
	if strings.Contains(header, "ERR") {
		t.Fatalf("ERR should be hidden at width 60:\n%s", view)
	}
}

func TestViewRendersErrColumnForFailedRow(t *testing.T) {
	m := NewModel()
	m.width = 120 // >= 100: ERROR column shows
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "c1"},
		RetryAttemptMsg{Path: repo, Change: "c1", N: 1, Max: 3, Err: "transient boom"},
		ChangeFailedMsg{Path: repo, Change: "c1", Err: "fatal: merge conflict"},
	)
	view := m.View().Content
	lines := strings.Split(view, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected summary+header+row, got %d lines:\n%s", len(lines), view)
	}
	header := lines[1]
	if !strings.Contains(header, "ERROR") {
		t.Fatalf("header missing ERROR column at width 120:\n%s", view)
	}
	// ERROR is the final column, after AGE.
	if ageIdx, errIdx := strings.Index(header, "AGE"), strings.Index(header, "ERROR"); !(ageIdx >= 0 && errIdx > ageIdx) {
		t.Fatalf("ERROR column must follow AGE in the header:\n%s", header)
	}
	// The failed row carries the final error in its ERROR cell.
	row := lines[2]
	if !strings.Contains(row, "fatal: merge conflict") {
		t.Fatalf("failed row missing final error in ERROR cell:\n%s", row)
	}
}

func TestViewErrColumnCollapsesMultilineToOneLine(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 24
	repo := "/tmp/proj"
	multiline := "fatal: conflict\nsecond line\r\nthird"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeFailedMsg{Path: repo, Change: "c1", Err: multiline},
	)
	view := m.View().Content
	// Every retained row is exactly one physical line: summary +
	// header + one-line-per-row + footer (activity, separator, help
	// = 3 rows in the short help view).
	lines := strings.Split(view, "\n")
	if want := 5 + len(m.rows); len(lines) != want {
		t.Fatalf("multi-line error wrapped the row: got %d lines, want %d:\n%s", len(lines), want, view)
	}
	row := lines[2]
	if strings.Contains(row, "\n") || strings.Contains(row, "\r") {
		t.Fatalf("ERROR cell leaked a newline/CR into the row:\n%q", row)
	}
	// The whitespace runs are collapsed to single spaces.
	if !strings.Contains(row, "fatal: conflict second line") {
		t.Fatalf("collapsed error text missing from row:\n%s", row)
	}
}

// Regression for remove-tui-log-path: the per-invocation agent log
// path is never rendered in the grid, so every row occupies exactly
// one physical line regardless of terminal width, and no .jsonl
// fragment ever reaches the frame. fitToHeight must keep the rendered
// row count within the one-line-per-row height budget.
func TestViewOmitsLogPathAndKeepsRowOneLine(t *testing.T) {
	m := NewModel()
	m.width = 60
	m.height = 24
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "add-dark-mode"},
		ChangeDoneMsg{Path: repo, Change: "add-dark-mode"},
	)
	view := m.View().Content
	if strings.Contains(view, ".jsonl") {
		t.Fatalf("view must not contain any jsonl path fragment:\n%s", view)
	}
	// Each retained row is exactly one physical line, so the view is
	// summary + header + one-line-per-row + footer (activity,
	// separator, help = 3 rows in the short help view).
	lines := strings.Split(view, "\n")
	if want := 5 + len(m.rows); len(lines) != want {
		t.Fatalf("every row must be one line; got %d lines, want %d (5 fixed + %d rows):\n%s", len(lines), want, len(m.rows), view)
	}
	// fitToHeight never returns more rows than the budget allows at
	// one line each, and never more than the input slice.
	for _, avail := range []int{0, 1, 2, 3, 5, 100} {
		prio := m.prioritizedRows()
		got := fitToHeight(prio, avail)
		if len(got) > len(prio) {
			t.Fatalf("fitToHeight(avail=%d) returned %d rows, more than %d input", avail, len(got), len(prio))
		}
		if len(got) > avail {
			t.Fatalf("fitToHeight(avail=%d) returned %d rows, exceeding the one-line budget", avail, len(got))
		}
	}
}

func TestViewOmitsLogPathWhenUnset(t *testing.T) {
	m := NewModel()
	m.width = 120
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "task-1"},
		ChangeDoneMsg{Path: repo, Change: "task-1"},
	)
	view := m.View().Content
	// Default shape is summary + header + 1 row + footer (activity,
	// separator, help) = 6 lines. No extra row bearing a path
	// (which would be a 7th line).
	lines := strings.Split(view, "\n")
	if len(lines) != 6 {
		t.Fatalf("expected exactly 6 lines (summary + header + row + 3-row footer), got %d:\n%s", len(lines), view)
	}
	if strings.Contains(view, ".jsonl") {
		t.Fatalf("view leaked a log path when none was set:\n%s", view)
	}
}

func TestViewRendersWorkflowNameWithChange(t *testing.T) {
	m := NewModel()
	m.width = 120
	m = driveMessages(m,
		RepoSeenMsg{Path: "/tmp/proj", HasChange: true},
		ChangeStartedMsg{Path: "/tmp/proj", Workflow: "dependencies", Change: "package-update"},
	)
	if view := m.View().Content; !strings.Contains(view, "dependencies: package-update") {
		t.Fatalf("view missing workflow and change:\n%s", view)
	}
}

// Regression for add-custom-workflows task 5.3: the TUI CHANGE
// column must render the normalized custom condition value
// (e.g. "add-dark-mode"), not its SHA-256 digest. The watcher
// hashes the change for branch identity but ships the human value
// on every event; the TUI must not regress to displaying the
// digest even if a future change touches the model.
func TestViewRendersCustomChangeNameNotDigest(t *testing.T) {
	m := NewModel()
	m.width = 120
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "add-dark-mode"},
	)
	view := m.View().Content
	if !strings.Contains(view, "add-dark-mode") {
		t.Fatalf("view missing custom change value:\n%s", view)
	}
	// Sanity: the digest is 64 hex characters; ensure no 64-hex
	// substring appears in the change column. The repo column
	// holds "proj" plus the header labels — a digest collision
	// there is impossible — so a flat substring scan is enough.
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "add-dark-mode") {
			continue
		}
		for i := 0; i+64 <= len(line); i++ {
			chunk := line[i : i+64]
			allHex := true
			for _, r := range chunk {
				if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
					allHex = false
					break
				}
			}
			if allHex {
				t.Fatalf("TUI leaked a digest substring %q in row:\n%s", chunk, view)
			}
		}
	}
}

// Regression for add-custom-workflows task 5.3: a custom resolver
// that exits 1 (no change) must render the same idle row as the
// OpenSpec fallback. The CHANGE column shows the em-dash
// placeholder; PHASE is idle; the footer counts the row in the
// idle bucket.
func TestViewRendersCustomIdleRowWithEmDash(t *testing.T) {
	m := NewModel()
	m.width = 120
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: false},
	)
	view := m.View().Content
	lines := strings.Split(view, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines (summary + header + row), got %d:\n%s", len(lines), view)
	}
	row := lines[2]
	if !strings.Contains(row, "—") {
		t.Fatalf("custom-idle row missing em-dash placeholder:\n%s", row)
	}
	if !strings.Contains(view, "idle") {
		t.Fatalf("view missing idle phase for custom-idle row:\n%s", view)
	}
	if !strings.Contains(view, "1 idle") {
		t.Fatalf("view missing idle counter:\n%s", view)
	}
}

// Regression for add-custom-workflows task 5.3: phase transitions
// and warning semantics must survive the HasOpenspec→HasChange
// rename. A Warning followed by ChangeStarted must still toggle the
// ⚠ glyph off; PHASE must still update from done to working; the
// row's HasChange rename must not leak into the rendered view.
func TestViewPhaseAndWarningUnchangedAcrossRename(t *testing.T) {
	m := NewModel()
	m.width = 120
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "task-1"},
		ChangeDoneMsg{Path: repo, Change: "task-1"},
		WarningMsg{Path: repo, Change: "task-1", Msg: "rollback hiccup"},
		ChangeStartedMsg{Path: repo, Change: "task-1"},
	)
	view := m.View().Content
	if !strings.Contains(view, "working") {
		t.Fatalf("view missing working phase after re-start:\n%s", view)
	}
	if strings.Contains(view, "⚠") {
		t.Fatalf("view still shows warning glyph after ChangeStarted cleared it:\n%s", view)
	}
	// The summary line is always present and includes the literal
	// word "warning" as its label, so check for the counter (1+).
	if strings.Contains(view, "1 warning") {
		t.Fatalf("view still counts a warning after ChangeStarted cleared it:\n%s", view)
	}
	if strings.Contains(view, "HasChange") || strings.Contains(view, "HasOpenspec") {
		t.Fatalf("view leaked a struct field name:\n%s", view)
	}
}

// --- rework-tui-layout regression coverage -----------------------------

// Summary counts every retained repository, even those not in the
// visible viewport. The visible count is reported independently.
func TestSummaryCountsAllRetainedRepos(t *testing.T) {
	m := NewModel()
	m.width = 120
	// 12 repos; only 10 fit. Two will be dropped from the viewport.
	msgs := []tea.Msg{}
	for i := 0; i < 12; i++ {
		path := "/wd/r" + string(rune('a'+i))
		msgs = append(msgs,
			RepoSeenMsg{Path: path, HasChange: true},
			ChangeDoneMsg{Path: path, Change: "c"},
		)
	}
	m = driveMessages(m, msgs...)
	view := m.View().Content
	if !strings.Contains(view, "12 total") {
		t.Fatalf("summary should report 12 total, view:\n%s", view)
	}
	if !strings.Contains(view, "10/12") {
		t.Fatalf("summary should report visible count 10/12, view:\n%s", view)
	}
}

// The viewport must never render more than ten repository entries,
// regardless of how many repos the model retains.
func TestViewportCappedAtTenEntries(t *testing.T) {
	m := NewModel()
	m.width = 120
	// 15 repos, all with at least one meaningful event so the
	// selector has the full set to pick from.
	for i := 0; i < 15; i++ {
		path := "/wd/r" + string(rune('a'+i))
		m = driveMessages(m,
			RepoSeenMsg{Path: path, HasChange: true},
			ChangeStartedMsg{Path: path, Change: "c"},
		)
	}
	view := m.View().Content
	// Count the number of repo rows. A row is a line that begins
	// with a path basename. We rely on the "REPO" header being the
	// first non-summary line and look for repo basenames r..r..
	rows := 0
	for _, line := range strings.Split(view, "\n") {
		if len(line) >= 2 && line[0] == 'r' && line[1] >= 'a' && line[1] <= 'o' {
			rows++
		}
	}
	if rows != 10 {
		t.Fatalf("expected exactly 10 visible rows, got %d:\n%s", rows, view)
	}
	// Model must still retain all 15 rows for the summary.
	if len(m.rows) != 15 {
		t.Fatalf("model should retain all 15 rows, got %d", len(m.rows))
	}
}

// Working rows outrank failed rows outrank warning rows outrank
// everything else, regardless of scan order.
func TestViewportPrioritizesWorkingOverFailedOverWarningOverRest(t *testing.T) {
	m := NewModel()
	m.width = 120
	// 4 repos, all in different attention classes.
	m = driveMessages(m,
		RepoSeenMsg{Path: "/wd/idle", HasChange: true},
		RepoSeenMsg{Path: "/wd/warning", HasChange: true},
		WarningMsg{Path: "/wd/warning", Msg: "hmm"},
		RepoSeenMsg{Path: "/wd/failed", HasChange: true},
		ChangeFailedMsg{Path: "/wd/failed", Change: "c", Err: "boom"},
		RepoSeenMsg{Path: "/wd/working", HasChange: true},
		ChangeStartedMsg{Path: "/wd/working", Change: "c"},
	)
	view := m.View().Content
	// Layout: summary (line 0), header (line 1), rows (line 2+),
	// footer (last line). Repo basenames live on the row lines.
	lines := strings.Split(view, "\n")
	if len(lines) < 6 {
		t.Fatalf("expected summary + header + 4 rows + footer, got %d:\n%s", len(lines), view)
	}
	rowLines := lines[2 : len(lines)-1]
	// Each row is rendered with a colored phase glyph. Find rows
	// by their phase glyphs so we don't conflate the summary's
	// "1 working" label with the row.
	wIdx := -1
	fIdx := -1
	rowOrder := []string{}
	for i, line := range rowLines {
		switch {
		case strings.Contains(line, "●"):
			rowOrder = append(rowOrder, "working")
		case strings.Contains(line, "✗"):
			rowOrder = append(rowOrder, "failed")
		case strings.Contains(line, "⚠"):
			rowOrder = append(rowOrder, "warning")
		case strings.Contains(line, "○"):
			rowOrder = append(rowOrder, "idle")
		}
		_ = i
	}
	if len(rowOrder) != 4 {
		t.Fatalf("expected 4 rows, got %d (rows: %v):\n%s", len(rowOrder), rowOrder, view)
	}
	want := []string{"working", "failed", "warning", "idle"}
	for i, name := range want {
		if rowOrder[i] != name {
			t.Fatalf("row %d should be %q, got %q (order: %v):\n%s", i, name, rowOrder[i], rowOrder, view)
		}
	}
	_ = wIdx
	_ = fIdx
}

// Within a priority class, the most recent meaningful activity
// comes first; stable discovery order breaks ties.
func TestViewportActivityRecencyWithinClass(t *testing.T) {
	m := NewModel()
	m.width = 120
	// Two working rows: alpha started first, beta started second.
	// Beta should appear first (more recent activity).
	m = driveMessages(m,
		RepoSeenMsg{Path: "/wd/alpha", HasChange: true},
		ChangeStartedMsg{Path: "/wd/alpha", Change: "c"},
		RepoSeenMsg{Path: "/wd/beta", HasChange: true},
		ChangeStartedMsg{Path: "/wd/beta", Change: "c"},
	)
	view := m.View().Content
	alphaIdx := strings.Index(view, "alpha")
	betaIdx := strings.Index(view, "beta")
	if alphaIdx < 0 || betaIdx < 0 {
		t.Fatalf("expected both repos in view:\n%s", view)
	}
	if alphaIdx < betaIdx {
		t.Fatalf("beta (more recent activity) should appear before alpha:\n%s", view)
	}
}

// Repeated RepoSeen events for an existing row must not refresh
// its activity order. A new meaningful event on another row
// should push the heartbeat-only row down.
func TestRepeatedRepoSeenDoesNotChangeActivityOrder(t *testing.T) {
	m := NewModel()
	m.width = 120
	// alpha gets ChangeStarted, then 5 RepoSeen heartbeats.
	// beta then gets ChangeStarted and should rank above alpha.
	m = driveMessages(m,
		RepoSeenMsg{Path: "/wd/alpha", HasChange: true},
		ChangeStartedMsg{Path: "/wd/alpha", Change: "c"},
		RepoSeenMsg{Path: "/wd/alpha", HasChange: true},
		RepoSeenMsg{Path: "/wd/alpha", HasChange: true},
		RepoSeenMsg{Path: "/wd/alpha", HasChange: true},
		RepoSeenMsg{Path: "/wd/alpha", HasChange: true},
		RepoSeenMsg{Path: "/wd/alpha", HasChange: true},
		RepoSeenMsg{Path: "/wd/beta", HasChange: true},
		ChangeStartedMsg{Path: "/wd/beta", Change: "c"},
	)
	view := m.View().Content
	alphaIdx := strings.Index(view, "alpha")
	betaIdx := strings.Index(view, "beta")
	if alphaIdx < 0 || betaIdx < 0 {
		t.Fatalf("expected both repos in view:\n%s", view)
	}
	if alphaIdx < betaIdx {
		t.Fatalf("beta's lifecycle event should rank above alpha's heartbeats:\n%s", view)
	}
}

// A new repository observed only via RepoSeen must be selectable
// (so the user sees newly-discovered repos) and stable across
// heartbeats.
func TestNewRepoReceivesDiscoveryOrder(t *testing.T) {
	m := NewModel()
	m.width = 120
	// 11 rows total, but the viewport caps at 10. The latest
	// RepoSeen (after the cap is met) must still be considered for
	// future selection.
	m = driveMessages(m,
		RepoSeenMsg{Path: "/wd/r0", HasChange: true},
		RepoSeenMsg{Path: "/wd/r1", HasChange: true},
		RepoSeenMsg{Path: "/wd/r2", HasChange: true},
		RepoSeenMsg{Path: "/wd/r3", HasChange: true},
		RepoSeenMsg{Path: "/wd/r4", HasChange: true},
		RepoSeenMsg{Path: "/wd/r5", HasChange: true},
		RepoSeenMsg{Path: "/wd/r6", HasChange: true},
		RepoSeenMsg{Path: "/wd/r7", HasChange: true},
		RepoSeenMsg{Path: "/wd/r8", HasChange: true},
		RepoSeenMsg{Path: "/wd/r9", HasChange: true},
		// This eleventh row is invisible initially but should be
		// tracked for the global count.
		RepoSeenMsg{Path: "/wd/r10", HasChange: true},
		// Repeated heartbeats for existing rows must not push r10
		// out of the model.
		RepoSeenMsg{Path: "/wd/r0", HasChange: true},
		RepoSeenMsg{Path: "/wd/r1", HasChange: true},
	)
	view := m.View().Content
	if !strings.Contains(view, "11 total") {
		t.Fatalf("summary should report 11 total:\n%s", view)
	}
	if len(m.rows) != 11 {
		t.Fatalf("model must retain all 11 rows, got %d", len(m.rows))
	}
}

// The footer must no longer carry the phase/warning summary that
// previously duplicated the top section. Only the quit hint and
// any infrastructure error remain.
func TestFooterNoLongerShowsPhaseCounts(t *testing.T) {
	m := NewModel()
	m.width = 120
	m = driveMessages(m,
		RepoSeenMsg{Path: "/wd/done", HasChange: true},
		ChangeDoneMsg{Path: "/wd/done", Change: "c"},
		RepoSeenMsg{Path: "/wd/working", HasChange: true},
		ChangeStartedMsg{Path: "/wd/working", Change: "c"},
		RepoSeenMsg{Path: "/wd/failed", HasChange: true},
		ChangeFailedMsg{Path: "/wd/failed", Change: "c", Err: "boom"},
	)
	view := m.View().Content
	lines := strings.Split(view, "\n")
	last := lines[len(lines)-1]
	// The footer's last row is the help bar (no phase counts). It
	// must mention the quit binding; the top summary owns the counts.
	if !strings.Contains(last, "quit") {
		t.Fatalf("last line should be the help bar mentioning quit:\n%s", view)
	}
	// No "N done", "N working", "N failed", "N idle", "N warning"
	// should appear in the last line.
	for _, banned := range []string{" done", " working", " failed", " idle", " warning"} {
		if strings.Contains(last, banned) {
			t.Fatalf("footer should not contain %q; got %q", banned, last)
		}
	}
}

// A short terminal must render only the number of complete
// repository entries that fit. The ten-row ceiling is still
// honored when the height allows.
func TestShortTerminalCapsVisibleRows(t *testing.T) {
	m := NewModel()
	m.width = 120
	// Height 12: summary (1) + header (1) + 3-row footer (activity,
	// separator, help) = 5 non-row lines, leaving 7 for rows. Add 12
	// repos; the cap (10) and the height budget (7) must both hold.
	m.height = 12
	for i := 0; i < 12; i++ {
		path := "/wd/r" + string(rune('a'+i))
		m = driveMessages(m,
			RepoSeenMsg{Path: path, HasChange: true},
			ChangeStartedMsg{Path: path, Change: "c"},
		)
	}
	view := m.View().Content
	// Count working-phase rows: a row line is the one carrying a
	// "●" glyph.
	count := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "●") {
			count++
		}
	}
	if count > 10 {
		t.Fatalf("short terminal must not show more than 10 rows, got %d:\n%s", count, view)
	}
	// And the model still retains all 12.
	if len(m.rows) != 12 {
		t.Fatalf("model must retain all 12 rows, got %d", len(m.rows))
	}
}

func TestRowPhaseTransitionRefreshesActivity(t *testing.T) {
	m := NewModel()
	m.width = 120
	// Two rows. alpha gets ChangeStarted first, then ChangeDone.
	// beta then gets ChangeStarted; beta should rank above alpha
	// (more recent activity), and alpha should be in the "done"
	// class which is lower priority than working.
	m = driveMessages(m,
		RepoSeenMsg{Path: "/wd/alpha", HasChange: true},
		ChangeStartedMsg{Path: "/wd/alpha", Change: "c"},
		ChangeDoneMsg{Path: "/wd/alpha", Change: "c"},
		RepoSeenMsg{Path: "/wd/beta", HasChange: true},
		ChangeStartedMsg{Path: "/wd/beta", Change: "c"},
	)
	view := m.View().Content
	betaIdx := strings.Index(view, "beta")
	alphaIdx := strings.Index(view, "alpha")
	if betaIdx < 0 || alphaIdx < 0 {
		t.Fatalf("expected both rows in view:\n%s", view)
	}
	if alphaIdx < betaIdx {
		t.Fatalf("beta (working) should outrank alpha (done) regardless of scan order:\n%s", view)
	}
}

func TestWarningRowStillVisibleWhenIdle(t *testing.T) {
	m := NewModel()
	m.width = 120
	// alpha is idle, beta is idle but with a warning. beta should
	// rank above alpha (warning > remaining).
	m = driveMessages(m,
		RepoSeenMsg{Path: "/wd/alpha", HasChange: true},
		RepoSeenMsg{Path: "/wd/beta", HasChange: true},
		WarningMsg{Path: "/wd/beta", Msg: "hmm"},
	)
	view := m.View().Content
	betaIdx := strings.Index(view, "beta")
	alphaIdx := strings.Index(view, "alpha")
	if betaIdx < 0 || alphaIdx < 0 {
		t.Fatalf("expected both rows in view:\n%s", view)
	}
	if alphaIdx < betaIdx {
		t.Fatalf("warning row beta should outrank idle row alpha:\n%s", view)
	}
}

func TestSummaryEmptyRepoState(t *testing.T) {
	m := NewModel()
	m.width = 120
	view := m.View().Content
	if !strings.Contains(view, "0 total") {
		t.Fatalf("empty model summary should report 0 total:\n%s", view)
	}
	if !strings.Contains(view, "0/0 visible") {
		t.Fatalf("empty model summary should report 0/0 visible:\n%s", view)
	}
}

func TestSummaryVisibleCountWith10PlusRepos(t *testing.T) {
	m := NewModel()
	m.width = 120
	for i := 0; i < 47; i++ {
		path := "/wd/r" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		m = driveMessages(m,
			RepoSeenMsg{Path: path, HasChange: true},
			ChangeStartedMsg{Path: path, Change: "c"},
		)
	}
	view := m.View().Content
	if !strings.Contains(view, "47 total") {
		t.Fatalf("summary should report 47 total, view:\n%s", view)
	}
	if !strings.Contains(view, "10/47 visible") {
		t.Fatalf("summary should report 10/47 visible, view:\n%s", view)
	}
}

// Regression for show-age-only-while-working: AGE is a live work
// timer. It must display elapsed time only while the row's phase
// is PhaseWorking; idle, done, and failed phases always render the
// em-dash placeholder, even when StartedAt is retained from a
// prior attempt. tickMsg keeps ticking during the working phase so
// the displayed duration advances once per second.
func TestAGEOnlyShownWhileWorking(t *testing.T) {
	repo := "/tmp/proj"
	m := NewModel()
	m.width = 120
	ageCell := func() string {
		cells := m.buildCells(m.rows[repo], true, false, 0)
		return cells[4]
	}
	startedAt := func() bool { return !m.rows[repo].StartedAt.IsZero() }

	// Fresh idle row: no StartedAt yet, AGE is em-dash.
	m = driveMessages(m, RepoSeenMsg{Path: repo, HasChange: true})
	if got := ageCell(); got != "—" {
		t.Fatalf("idle row before work: AGE = %q, want %q", got, "—")
	}

	// Working: StartedAt is set by ChangeStarted; AGE shows duration.
	m = driveMessages(m, ChangeStartedMsg{Path: repo, Change: "c1"})
	if !startedAt() {
		t.Fatalf("ChangeStarted did not set StartedAt")
	}
	if got := ageCell(); got == "—" {
		t.Fatalf("working row: AGE = %q, want a duration string", got)
	}

	// Done: StartedAt is retained by the transition, but AGE must
	// be em-dash because PHASE is no longer working.
	m = driveMessages(m, ChangeDoneMsg{Path: repo, Change: "c1"})
	if !startedAt() {
		t.Fatalf("ChangeDone must not clear StartedAt (model invariant)")
	}
	if got := ageCell(); got != "—" {
		t.Fatalf("done row: AGE = %q, want %q (retained StartedAt must not leak into non-working AGE)", got, "—")
	}

	// Working again → failed: AGE returns to em-dash on failure.
	m = driveMessages(m, ChangeStartedMsg{Path: repo, Change: "c2"})
	if got := ageCell(); got == "—" {
		t.Fatalf("re-started row: AGE = %q, want a duration string", got)
	}
	m = driveMessages(m, ChangeFailedMsg{Path: repo, Change: "c2", Err: "boom"})
	if !startedAt() {
		t.Fatalf("ChangeFailed must not clear StartedAt (model invariant)")
	}
	if got := ageCell(); got != "—" {
		t.Fatalf("failed row: AGE = %q, want %q (StartedAt from the prior attempt must not bleed through)", got, "—")
	}

	// Idle: a later RepoSeen with HasChange=false forces PhaseIdle;
	// StartedAt from prior work remains, AGE stays em-dash.
	m = driveMessages(m, RepoSeenMsg{Path: repo, HasChange: false})
	if !startedAt() {
		t.Fatalf("RepoSeen with HasChange=false must not clear StartedAt (model invariant)")
	}
	if got := ageCell(); got != "—" {
		t.Fatalf("idle row after prior work: AGE = %q, want %q (StartedAt from prior work must not bleed through)", got, "—")
	}
}

// Regression for show-age-only-while-working: AGE must keep
// advancing while a row stays in PhaseWorking. A working row with
// a pinned StartedAt in the past must render a non-em-dash
// duration string; the existing TestTickDrivesRedrawDuringWork
// proves the periodic tick reschedules.
func TestAGEAdvancesWhileWorking(t *testing.T) {
	repo := "/tmp/proj"
	m := NewModel()
	m.width = 120
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "c1"},
	)
	m.rows[repo].StartedAt = time.Now().Add(-2 * time.Second)
	cells := m.buildCells(m.rows[repo], true, false, 0)
	if cells[4] == "—" {
		t.Fatalf("working row with past StartedAt must show duration, got em-dash")
	}
}

// TestHelpToggleFlipsShortAndFull pins the ? toggle: the short help
// is one physical line; pressing ? flips to the two-line full view;
// pressing ? again restores the short view. The activity and
// separator rows are unchanged across the toggle (only the help row
// changes), and both renderings reuse the same two keymap bindings.
func TestHelpToggleFlipsShortAndFull(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*Model)

	shortActivity := footerActivity(m.renderFooter())
	shortHelp := footerHelpBlock(m.renderFooter())
	if strings.Contains(shortHelp, "\n") {
		t.Fatalf("short help should be one physical line: %q", shortHelp)
	}
	if !strings.Contains(shortHelp, "quit") || !strings.Contains(shortHelp, "help") {
		t.Fatalf("short help missing a binding: %q", shortHelp)
	}

	// Toggle to the full view.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "?"})
	m = updated.(*Model)
	if !m.help.ShowAll {
		t.Fatal("? did not flip ShowAll to true")
	}
	fullActivity := footerActivity(m.renderFooter())
	fullHelp := footerHelpBlock(m.renderFooter())
	if !strings.Contains(fullHelp, "\n") {
		t.Fatalf("full help should be two physical lines: %q", fullHelp)
	}
	if !strings.Contains(fullHelp, "quit") || !strings.Contains(fullHelp, "help") {
		t.Fatalf("full help missing a binding: %q", fullHelp)
	}
	if shortActivity != fullActivity {
		t.Fatal("activity row changed across the help toggle")
	}

	// Toggle back to the short view.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "?"})
	m = updated.(*Model)
	if m.help.ShowAll {
		t.Fatal("? did not flip ShowAll back to false")
	}
	if got := footerHelpBlock(m.renderFooter()); got != shortHelp {
		t.Fatalf("toggling ? back did not restore the short help block:\n got %q\nwant %q", got, shortHelp)
	}
}

// TestHelpBarIsProjectionOfKeymap pins that the footer's help row is
// exactly m.help.View(m.keys): the keymap is the sole source of truth
// for the footer chrome. (The old tickerQuit literal is a removed
// const, so the compiler already rejects any reference to it; the
// runtime equality below is the real source-of-truth contract.)
func TestHelpBarIsProjectionOfKeymap(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*Model)

	footer := m.renderFooter()
	helpBlock := footerHelpBlock(footer)
	if want := m.help.View(m.keys); helpBlock != want {
		t.Fatalf("help row is not m.help.View(m.keys):\n got %q\nwant %q", helpBlock, want)
	}
}

// TestHelpWidthTracksWindowSize pins that the help bar uses the help
// package's built-in truncation: at a narrow width the help row ends
// in an ellipsis; at a wide width both bindings fit without one.
func TestHelpWidthTracksWindowSize(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 5, Height: 24})
	m = updated.(*Model)
	narrow := footerHelpBlock(m.renderFooter())
	if !strings.Contains(narrow, "…") {
		t.Fatalf("narrow help should truncate with an ellipsis: %q", narrow)
	}

	updated, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*Model)
	wide := footerHelpBlock(m.renderFooter())
	if strings.Contains(wide, "…") {
		t.Fatalf("wide help should fit without truncation: %q", wide)
	}
	if !strings.Contains(wide, "quit") || !strings.Contains(wide, "help") {
		t.Fatalf("wide help missing a binding: %q", wide)
	}
}

// TestHelpFullViewShrinksViewportByOneRow pins the spec scenario that
// the full (toggled) help view grows the footer by one line and so
// shrinks the table viewport by one row at a tight height, and that
// toggling back restores it.
func TestHelpFullViewShrinksViewportByOneRow(t *testing.T) {
	m := NewModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	m = updated.(*Model)
	// Eight repos at height 12 with the short help: fixedLines = 5
	// (summary + header + activity + separator + 1-line help), so
	// avail = 7 and seven rows render.
	for i := 0; i < 8; i++ {
		path := "/wd/r" + string(rune('a'+i))
		m = driveMessages(m,
			RepoSeenMsg{Path: path, HasChange: true},
			ChangeStartedMsg{Path: path, Change: "c"},
		)
	}
	countWorking := func() int {
		n := 0
		for _, line := range strings.Split(m.View().Content, "\n") {
			if strings.Contains(line, "●") {
				n++
			}
		}
		return n
	}
	if got := countWorking(); got != 7 {
		t.Fatalf("short help at height 12 should show 7 rows, got %d", got)
	}
	// Toggle to the full help: the footer gains a second help line,
	// fixedLines becomes 6, avail becomes 6.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "?"})
	m = updated.(*Model)
	if got := countWorking(); got != 6 {
		t.Fatalf("full help at height 12 should show 6 rows, got %d", got)
	}
	// Toggle back: the viewport grows again.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "?"})
	m = updated.(*Model)
	if got := countWorking(); got != 7 {
		t.Fatalf("short help restored should show 7 rows, got %d", got)
	}
}
