package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Regression for migrate-tui-to-bubbles-v2 task 1.2: the View()
// method returns a tea.View whose AltScreen field is set so the
// alternate screen buffer is entered and released by Bubble Tea v2
// lifecycle handling. Reading .Content is the rendering seam that
// isolates content rendering from terminal control behavior.
func TestViewDeclaresAlternateScreen(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 24
	v := m.View()
	if !v.AltScreen {
		t.Fatalf("View must declare AltScreen=true (got %+v)", v)
	}
}

// Regression for migrate-tui-to-bubbles-v2 task 1.2: the table
// navigation keys (arrow up/down, j/k, pgup/pgdown) must NOT
// change the visible projection. q and ctrl+c remain the only
// quit keys; everything else is inert. The view before any
// navigation key must equal the view after a navigation key.
func TestTableNavigationKeysAreInert(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 24
	repo := "/tmp/proj"
	m = driveMessages(m,
		RepoSeenMsg{Path: repo, HasChange: true},
		ChangeStartedMsg{Path: repo, Change: "task-1"},
	)
	before := m.renderContent()

	navKeys := []tea.KeyPressMsg{
		{Text: "j"},
		{Text: "k"},
		{Code: tea.KeyDown},
		{Code: tea.KeyUp},
		{Code: tea.KeyPgDown},
		{Code: tea.KeyPgUp},
	}
	for _, k := range navKeys {
		updated, cmd := m.Update(k)
		if cmd != nil {
			t.Fatalf("Update returned cmd for inert navigation key %q: %v", k.Text, cmd)
		}
		if updated == nil {
			t.Fatalf("Update returned nil model for inert navigation key %q", k.Text)
		}
		m, _ = updated.(*Model)
	}

	after := m.renderContent()
	if before != after {
		t.Fatalf("navigation key changed the visible projection:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// Regression for migrate-tui-to-bubbles-v2 task 3.1: the bubble
// table is rendered with zero cell padding so existing
// fixed-column widths (REPO=24, CHANGE=30, PHASE=10, RETRY=8)
// remain accurate. The terminal-width invariant
// wRepo+wChange+wPhase+wRetry+... == terminal width (so a row
// can never wrap past one physical line) must hold under the
// bubbles rendering.
func TestBubblesTableProducesOneLineRows(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 24
	m = driveMessages(m,
		RepoSeenMsg{Path: "/tmp/proj", HasChange: true},
		ChangeStartedMsg{Path: "/tmp/proj", Change: "task-1"},
		ChangeFailedMsg{Path: "/tmp/proj", Change: "task-1", Err: "boom\nsecond line"},
	)
	content := m.renderContent()
	lines := strings.Split(content, "\n")
	// Layout: summary + header + rows + footer (+ optional error).
	// Header must not introduce extra blank lines or box-drawing
	// border artefacts; the bubbles table styles we pass suppress
	// borders so cells remain one physical line.
	header := lines[1]
	if strings.Count(header, "│") > 0 || strings.Contains(header, "─") {
		t.Fatalf("header should not contain border artefacts:\n%s", header)
	}
	// The failed row must occupy exactly one physical line (no
	// line wrapping from padding).
	row := lines[2]
	if lipgloss.Width(row) > m.width {
		t.Fatalf("row %d exceeds terminal width %d:\n%s", lipgloss.Width(row), m.width, row)
	}
}

// Regression for migrate-tui-to-bubbles-v2 task 3.1: the table
// remains non-interactive at the model level. The model's Update
// must not depend on or be coupled to the bubbles table's
// keymap; only the model's quit handling responds.
func TestViewDoesNotConsumeUpDownKeys(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 24
	m = driveMessages(m, RepoSeenMsg{Path: "/tmp/a", HasChange: true})
	before := m.renderContent()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("arrow-down produced a cmd (table must not be focused): %v", cmd)
	}
	if updated == nil {
		t.Fatalf("Update returned nil model for arrow-down")
	}
	m, _ = updated.(*Model)
	if m.renderContent() != before {
		t.Fatalf("arrow-down mutated the visible projection")
	}
}
