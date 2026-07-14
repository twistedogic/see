package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	colRepo   = lipgloss.NewStyle().Width(24).Align(lipgloss.Left)
	colChange = lipgloss.NewStyle().Width(30).Align(lipgloss.Left)
	colPhase  = lipgloss.NewStyle().Width(10).Align(lipgloss.Left)
	colRetry  = lipgloss.NewStyle().Width(8).Align(lipgloss.Left)
	colAge    = lipgloss.NewStyle().Width(8).Align(lipgloss.Right)

	glyphIdle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○ idle")
	glyphWorking = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("● working")
	glyphDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓ done")
	glyphFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗ failed")
	glyphNoSpec  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○ no-spec")
)

func truncate(s string, n int) string {
	if n <= 1 {
		if n == 1 {
			return "…"
		}
		return ""
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n-1]) + "…"
}

func (m *Model) View() string {
	// Column visibility by terminal width: at >=80 cols show AGE,
	// below that REPO/CHANGE/PHASE/RETRY only.
	showAge := m.width >= 80

	header := m.renderHeader(showAge)
	rows := make([]string, 0, len(m.order))
	for _, p := range m.order {
		rows = append(rows, m.renderRow(m.rows[p], showAge))
	}

	body := strings.Join(rows, "\n")
	parts := []string{header}
	if body == "" {
		parts = append(parts, "(no repos scanned yet)")
	} else {
		parts = append(parts, body)
	}
	if m.infraErr != "" {
		parts = append(parts, "! "+m.infraErr)
	}
	parts = append(parts, m.renderFooter())
	return strings.Join(parts, "\n")
}

func (m *Model) renderHeader(showAge bool) string {
	parts := []string{
		colRepo.Render("REPO"),
		colChange.Render("CHANGE"),
		colPhase.Render("PHASE"),
		colRetry.Render("RETRY"),
	}
	if showAge {
		parts = append(parts, colAge.Render("AGE"))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m *Model) renderRow(r *RepoRow, showAge bool) string {
	phaseStr := phaseString(r)
	retry := "—"
	if r.RetryMax > 0 {
		retry = fmt.Sprintf("%d/%d", r.RetryN, r.RetryMax)
	}
	age := "—"
	if !r.StartedAt.IsZero() {
		age = time.Since(r.StartedAt).Round(time.Second).String()
	}

	// ponytail: ⚠ glyph lives in the REPO cell so a fast scan lights
	// up the row immediately; the message itself lives in the JSONL
	// (the operator can jq for it). Trailing space keeps the column
	// visually aligned when no warning is set.
	name := r.Name
	if r.Warning {
		name = name + " ⚠"
	}

	// Column widths are anchored to the styles above; truncate change
	// by the column width to avoid overflow on narrow terminals.
	change := truncate(r.Change, 30)

	parts := []string{
		colRepo.Render(truncate(name, 24)),
		colChange.Render(change),
		colPhase.Render(phaseStr),
		colRetry.Render(retry),
	}
	if showAge {
		parts = append(parts, colAge.Render(age))
	}
	row := lipgloss.JoinHorizontal(lipgloss.Left, parts...)
	if r.LogPath != "" {
		row += "\n      " + r.LogPath
	}
	return row
}

func phaseString(r *RepoRow) string {
	switch r.Phase {
	case PhaseIdle:
		return glyphIdle
	case PhaseWorking:
		return glyphWorking
	case PhaseDone:
		return glyphDone
	case PhaseFailed:
		return glyphFailed
	case PhaseNoSpec:
		return glyphNoSpec
	}
	return "?"
}

func (m *Model) renderFooter() string {
	var done, working, idle, failed, nospec, warnings int
	for _, p := range m.order {
		switch m.rows[p].Phase {
		case PhaseDone:
			done++
		case PhaseWorking:
			working++
		case PhaseIdle:
			idle++
		case PhaseFailed:
			failed++
		case PhaseNoSpec:
			nospec++
		}
		if m.rows[p].Warning {
			warnings++
		}
	}
	parts := []string{}
	if done > 0 {
		parts = append(parts, fmt.Sprintf("%d done", done))
	}
	if working > 0 {
		parts = append(parts, fmt.Sprintf("%d working", working))
	}
	if idle > 0 {
		parts = append(parts, fmt.Sprintf("%d idle", idle))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if nospec > 0 {
		parts = append(parts, fmt.Sprintf("%d no-spec", nospec))
	}
	if warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warning", warnings))
	}
	summary := strings.Join(parts, " · ")
	if summary == "" {
		summary = "no repos"
	}
	return summary + "    [q] quit"
}
