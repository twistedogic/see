package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-runewidth"
)

func TestTickerLifecycleAndLatestActivity(t *testing.T) {
	m := NewModel()
	m.width = 80
	footer := func() string {
		return strings.Split(m.renderContent(), "\n")[len(strings.Split(m.renderContent(), "\n"))-1]
	}
	if got := footer(); !strings.Contains(got, "pi › waiting") || !strings.Contains(got, "[q] quit") {
		t.Fatalf("initial footer = %q, want waiting and quit chrome", got)
	}
	m = driveMessages(m, ChangeStartedMsg{Path: "/repo", Change: "task"})
	if got := footer(); !strings.Contains(got, "pi › starting") {
		t.Fatalf("started footer = %q, want starting", got)
	}
	m = driveMessages(m, ActivityMsg{Text: "first activity"}, ActivityMsg{Text: "latest activity"})
	if got := footer(); !strings.Contains(got, "latest activity") || strings.Contains(got, "first activity") {
		t.Fatalf("latest activity did not replace prior text: %q", got)
	}
}

func TestTickerFooterIsOneLineAndKeepsQuitHintOnNarrowWidths(t *testing.T) {
	for _, width := range []int{1, 7, 8, 12, 30, 120} {
		m := NewModel()
		m.width = width
		m = driveMessages(m, ActivityMsg{Text: "a very long activity that must not wrap"})
		footer := m.renderFooter()
		if strings.ContainsAny(footer, "\r\n") {
			t.Fatalf("width %d footer contains a line break: %q", width, footer)
		}
		if width >= runewidth.StringWidth("[q] quit") && !strings.Contains(footer, "[q] quit") {
			t.Fatalf("width %d footer lost quit hint: %q", width, footer)
		}
	}
}

func TestTickerMarqueeOnlyRunsForOverflowAndMovesByDisplayCell(t *testing.T) {
	m := NewModel()
	m.width = 20
	short := ActivityMsg{Text: "ok"}
	if _, cmd := m.Update(short); cmd != nil {
		t.Fatal("fitting activity scheduled a marquee")
	}

	long := ActivityMsg{Text: "界abcdefgh"}
	_, cmd := m.Update(long)
	if cmd == nil {
		t.Fatal("overflowing activity did not schedule a marquee")
	}
	before := m.renderFooter()
	_, cmd = m.Update(marqueeTickMsg{})
	if cmd == nil {
		t.Fatal("overflowing marquee did not rearm its tick")
	}
	if m.tickerOffset != 1 {
		t.Fatalf("ticker offset = %d, want one display-cell step", m.tickerOffset)
	}
	if after := m.renderFooter(); after == before {
		t.Fatalf("marquee did not change footer:\n%s", after)
	}
	if !strings.Contains(before, "pi ›") || !strings.Contains(before, "[q] quit") || !strings.Contains(m.renderFooter(), "pi ›") || !strings.Contains(m.renderFooter(), "[q] quit") {
		t.Fatalf("marquee moved fixed chrome: before=%q after=%q", before, m.renderFooter())
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

func TestTickerLoopContainsVisibleGap(t *testing.T) {
	m := NewModel()
	m.width = 15
	m.activity = "abcdef"
	m.tickerOffset = runewidth.StringWidth(m.activity)
	footer := m.renderFooter()
	if !strings.Contains(footer, "pi › ") || !strings.Contains(footer, "[q] quit") {
		t.Fatalf("footer missing chrome: %q", footer)
	}
	prefixEnd := strings.Index(footer, "pi › ") + len("pi › ")
	quitStart := strings.Index(footer, "[q] quit")
	if prefixEnd >= quitStart {
		t.Fatalf("footer %q has no middle window", footer)
	}
	middle := strings.TrimRight(footer[prefixEnd:quitStart], " ")
	if middle != "" {
		t.Fatalf("ticker loop at activity width should show its gap, got %q in footer %q", middle, footer)
	}
}
