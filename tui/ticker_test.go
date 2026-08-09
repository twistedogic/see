package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-runewidth"
)

// footerRows splits the three-row footer into its physical lines:
// [activity, separator, help]. Used by the footer-shape tests below.
// In the short help view the footer is exactly three lines.
func footerRows(m *Model) []string {
	return strings.Split(m.renderFooter(), "\n")
}

// footerHelpBlock returns the help portion of renderFooter's output:
// everything after the activity and separator rows. The short view
// is one line; the full (toggled) view is two, so the block may
// itself contain a newline.
func footerHelpBlock(footer string) string {
	parts := strings.SplitN(footer, "\n", 3)
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

// footerActivity returns the activity row (the first of the three
// footer rows).
func footerActivity(footer string) string {
	parts := strings.SplitN(footer, "\n", 3)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func TestTickerLifecycleAndLatestActivity(t *testing.T) {
	m := NewModel()
	m.width = 80
	rows := footerRows(m)
	if len(rows) != 3 {
		t.Fatalf("footer has %d rows, want 3: %q", len(rows), m.renderFooter())
	}
	if !strings.Contains(rows[0], "pi › waiting") {
		t.Fatalf("activity row = %q, want waiting", rows[0])
	}
	if !strings.Contains(rows[2], "quit") {
		t.Fatalf("help row = %q, want the quit binding", rows[2])
	}
	m = driveMessages(m, ChangeStartedMsg{Path: "/repo", Change: "task"})
	if !strings.Contains(footerRows(m)[0], "pi › starting") {
		t.Fatalf("started activity row = %q, want starting", footerRows(m)[0])
	}
	m = driveMessages(m, ActivityMsg{Text: "first activity"}, ActivityMsg{Text: "latest activity"})
	if row := footerRows(m)[0]; !strings.Contains(row, "latest activity") || strings.Contains(row, "first activity") {
		t.Fatalf("latest activity did not replace prior text: %q", row)
	}
}

func TestTickerFooterIsThreeLinesAndHelpTruncatesOnNarrowWidths(t *testing.T) {
	for _, width := range []int{1, 7, 8, 12, 30, 120} {
		m := NewModel()
		// Drive width through WindowSizeMsg so the help bar's
		// SetWidth is called and its built-in truncation engages.
		updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m = updated.(*Model)
		m = driveMessages(m, ActivityMsg{Text: "a very long activity that must not wrap"})
		rows := footerRows(m)
		if len(rows) != 3 {
			t.Fatalf("width %d footer has %d rows, want 3: %q", width, len(rows), m.renderFooter())
		}
		for i, row := range rows {
			if strings.ContainsAny(row, "\r") {
				t.Fatalf("width %d footer row %d contains a carriage return: %q", width, i, row)
			}
		}
		// Each footer row is exactly one physical line: the join
		// above already split on "\n", so each row must itself be a
		// single line (no embedded newlines beyond the splits).
		if strings.Count(m.renderFooter(), "\n") != 2 {
			t.Fatalf("width %d footer is not exactly three lines: %q", width, m.renderFooter())
		}
	}
}

func TestTickerMarqueeOnlyRunsForOverflowAndMovesByDisplayCell(t *testing.T) {
	m := NewModel()
	m.width = 12 // middleWidth = 7: "ok" fits, "界abcdefgh" overflows
	short := ActivityMsg{Text: "ok"}
	if _, cmd := m.Update(short); cmd != nil {
		t.Fatal("fitting activity scheduled a marquee")
	}

	long := ActivityMsg{Text: "界abcdefgh"}
	_, cmd := m.Update(long)
	if cmd == nil {
		t.Fatal("overflowing activity did not schedule a marquee")
	}
	rowsBefore := footerRows(m)
	_, cmd = m.Update(marqueeTickMsg{})
	if cmd == nil {
		t.Fatal("overflowing marquee did not rearm its tick")
	}
	if m.tickerOffset != 1 {
		t.Fatalf("ticker offset = %d, want one display-cell step", m.tickerOffset)
	}
	rowsAfter := footerRows(m)
	if rowsAfter[0] == rowsBefore[0] {
		t.Fatalf("marquee did not move the activity row:\n%s", rowsAfter[0])
	}
	// The marquee moves only the activity row. The separator and the
	// help row are static and must be byte-identical across a tick.
	if rowsAfter[1] != rowsBefore[1] {
		t.Fatalf("marquee moved the separator row:\nbefore=%q\nafter=%q", rowsBefore[1], rowsAfter[1])
	}
	if rowsAfter[2] != rowsBefore[2] {
		t.Fatalf("marquee moved the help row:\nbefore=%q\nafter=%q", rowsBefore[2], rowsAfter[2])
	}
}

func TestTickerResetOnNewActivityAndResize(t *testing.T) {
	m := NewModel()
	m.width = 20
	m = driveMessages(m, ActivityMsg{Text: "long overflowing activity"})
	_, _ = m.Update(marqueeTickMsg{})
	if m.tickerOffset == 0 {
		t.Fatal("precondition: marquee offset did not advance")
	}
	m = driveMessages(m, ActivityMsg{Text: "new long overflowing activity"})
	if m.tickerOffset != 0 {
		t.Fatalf("new activity offset = %d, want zero", m.tickerOffset)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 21, Height: 20})
	m = updated.(*Model)
	if m.tickerOffset != 0 {
		t.Fatalf("resize offset = %d, want zero", m.tickerOffset)
	}
}

// TestTickerLoopContainsVisibleGap verifies the marquee loop embeds
// tickerGap between repetitions, so a scrolling activity never runs
// its tail into its head. At offset == activity width, the window
// starts inside the gap.
func TestTickerLoopContainsVisibleGap(t *testing.T) {
	m := NewModel()
	m.width = 15                 // middleWidth = 10
	m.activity = "abcdefghijklm" // 13 wide → overflows the 10-wide window
	m.tickerOffset = runewidth.StringWidth(m.activity)
	rows := footerRows(m)
	activity := rows[0]
	if !strings.HasPrefix(activity, tickerPrefix) {
		t.Fatalf("activity row missing prefix: %q", activity)
	}
	body := strings.TrimPrefix(activity, tickerPrefix)
	// At the activity-width offset the window begins inside the gap,
	// so the body starts with tickerGap (the visible separation).
	if !strings.HasPrefix(body, tickerGap) {
		t.Fatalf("window at activity-width offset should begin in the gap, got %q", body)
	}
}
