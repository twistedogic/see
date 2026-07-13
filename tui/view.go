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
	colErr    = lipgloss.NewStyle().Align(lipgloss.Left)

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
	// Column visibility by terminal width: at >=100 cols show all six,
	// at <100 drop ERR, at <80 drop AGE then ERR (REPO and PHASE only).
	showAge := m.width >= 80
	showErr := m.width >= 100

	header := m.renderHeader(showAge, showErr)
	rows := make([]string, 0, len(m.order))
	for _, p := range m.order {
		rows = append(rows, m.renderRow(m.rows[p], showAge, showErr))
	}

	body := strings.Join(rows, "\n")
	footer := m.renderFooter()

	if body == "" {
		body = "(no repos scanned yet)"
	}
	return strings.Join([]string{header, body, footer}, "\n")
}

func (m *Model) renderHeader(showAge, showErr bool) string {
	parts := []string{
		colRepo.Render("REPO"),
		colChange.Render("CHANGE"),
		colPhase.Render("PHASE"),
		colRetry.Render("RETRY"),
	}
	if showAge {
		parts = append(parts, colAge.Render("AGE"))
	}
	if showErr {
		parts = append(parts, colErr.Render("ERR"))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m *Model) renderRow(r *RepoRow, showAge, showErr bool) string {
	phaseStr := phaseString(r)
	retry := "—"
	if r.RetryMax > 0 {
		retry = fmt.Sprintf("%d/%d", r.RetryN, r.RetryMax)
	}
	age := "—"
	if !r.StartedAt.IsZero() {
		age = time.Since(r.StartedAt).Round(time.Second).String()
	}
	errStr := r.LastErr
	if errStr == "" {
		errStr = ""
	}

	// Column widths are anchored to the styles above; truncate change
	// and err by the column width to avoid overflow on narrow terminals.
	change := truncate(r.Change, 30)
	errCol := truncate(errStr, 40)

	parts := []string{
		colRepo.Render(truncate(r.Name, 24)),
		colChange.Render(change),
		colPhase.Render(phaseStr),
		colRetry.Render(retry),
	}
	if showAge {
		parts = append(parts, colAge.Render(age))
	}
	if showErr {
		parts = append(parts, colErr.Render(errCol))
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
	var done, working, idle, failed, nospec int
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
	summary := strings.Join(parts, " · ")
	if summary == "" {
		summary = "no repos"
	}
	return summary + "    [q] quit"
}
