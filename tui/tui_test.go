package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	view := m.View()
	if !strings.Contains(view, "proj") {
		t.Fatalf("view missing repo basename:\n%s", view)
	}
	if !strings.Contains(view, "task-1") {
		t.Fatalf("view missing change name:\n%s", view)
	}
	if !strings.Contains(view, "done") {
		t.Fatalf("view missing done phase:\n%s", view)
	}
	if !strings.Contains(view, "[q] quit") {
		t.Fatalf("view missing footer hint:\n%s", view)
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
	view := m.View()
	if !strings.Contains(view, "idle") {
		t.Fatalf("view missing idle phase for repo without openspec:\n%s", view)
	}
	// The change column should show the em-dash placeholder, not
	// the repo path or anything else.
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines (header + row), got %d:\n%s", len(lines), view)
	}
	row := lines[1]
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
	view := m.View()
	// The repo column is 24 wide; a 50-char name must be truncated to
	// fit, ending with the ellipsis.
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d:\n%s", len(lines), view)
	}
	row := lines[1]
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
	beforeView := m.View()
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
	if got.View() != beforeView {
		t.Fatalf("View changed for unknown event")
	}
}

func TestUpdateQuitsOnQ(t *testing.T) {
	m := NewModel()
	m.width = 120
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("expected tea.Quit on q key, got nil cmd")
	}
	if updated == nil {
		t.Fatalf("expected non-nil model on q key")
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"abcdef", 3, "ab…"},
		{"abc", 1, "…"},
		{"abc", 0, ""},
		{"", 5, ""},
	}
	for _, c := range cases {
		got := truncate(c.in, c.n)
		if got != c.want {
			t.Fatalf("truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
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
	view := m.View()
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
	view := m.View()
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
	m.width = 90 // between 80 and 100: AGE shows, ERR hides
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeFailedMsg{Path: repo, Change: "c1", Err: "boom"},
	)
	view := m.View()
	if !strings.Contains(view, "AGE") {
		t.Fatalf("header missing AGE column at width 90:\n%s", view)
	}
	if strings.Contains(view, " ERR ") || strings.HasSuffix(strings.Split(view, "\n")[0], "ERR") {
		// best-effort: header line shouldn't end with ERR at width 90
	}
	// Stronger check: the ERR header label must not be present.
	if strings.Contains(strings.Split(view, "\n")[0], "ERR") {
		t.Fatalf("ERR column should be hidden at width 90:\n%s", view)
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
	view := m.View()
	header := strings.Split(view, "\n")[0]
	if strings.Contains(header, "AGE") {
		t.Fatalf("AGE should be hidden at width 60:\n%s", view)
	}
	if strings.Contains(header, "ERR") {
		t.Fatalf("ERR should be hidden at width 60:\n%s", view)
	}
}

func TestViewRendersLogPathWhenSet(t *testing.T) {
	m := NewModel()
	m.width = 120
	repo := "/tmp/proj"
	lp := "/tmp/see/task-1--20260714T153022--12345.jsonl"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "task-1"},
		ChangeDoneMsg{Path: repo, Change: "task-1"},
		LogPathMsg{Path: lp, Change: "task-1"},
	)
	view := m.View()
	if !strings.Contains(view, lp) {
		t.Fatalf("view missing log path %q:\n%s", lp, view)
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
	view := m.View()
	// Default shape is header + 1 row + footer = 3 lines. No extra row
	// bearing a path (which would be a 4th line).
	lines := strings.Split(view, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected exactly 3 lines (header + row + footer), got %d:\n%s", len(lines), view)
	}
	if strings.Contains(view, ".jsonl") {
		t.Fatalf("view leaked a log path when none was set:\n%s", view)
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
	view := m.View()
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
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d:\n%s", len(lines), view)
	}
	row := lines[1]
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
	view := m.View()
	if !strings.Contains(view, "working") {
		t.Fatalf("view missing working phase after re-start:\n%s", view)
	}
	if strings.Contains(view, "⚠") {
		t.Fatalf("view still shows warning glyph after ChangeStarted cleared it:\n%s", view)
	}
	if strings.Contains(view, "warning") {
		t.Fatalf("view footer still counts a warning after ChangeStarted cleared it:\n%s", view)
	}
	if strings.Contains(view, "HasChange") || strings.Contains(view, "HasOpenspec") {
		t.Fatalf("view leaked a struct field name:\n%s", view)
	}
}
