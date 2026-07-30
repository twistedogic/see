package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Column widths. The fixed columns sum to
// wRepo+wChange+wPhase+wRetry (plus wAge when the terminal is
// >= 80 cols). The ERROR column is flex: it absorbs the remaining
// width, so fixedSum + errWidth == terminal width and a row can
// never wrap past one physical line.
const (
	wRepo       = 24
	wChange     = 30
	wPhase      = 10
	wRetry      = 8
	wAge        = 8
	errMinWidth = 20 // ERROR shows only when remaining width >= this
)

var (
	colRepo   = lipgloss.NewStyle().Width(wRepo).Align(lipgloss.Left)
	colChange = lipgloss.NewStyle().Width(wChange).Align(lipgloss.Left)
	colPhase  = lipgloss.NewStyle().Width(wPhase).Align(lipgloss.Left)
	colRetry  = lipgloss.NewStyle().Width(wRetry).Align(lipgloss.Left)
	colAge    = lipgloss.NewStyle().Width(wAge).Align(lipgloss.Right)

	glyphIdle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○ idle")
	glyphWorking = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("● working")
	glyphDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓ done")
	glyphFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗ failed")
)

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n-1]) + "…"
}

// oneLine collapses every run of whitespace (including the carriage
// returns and line feeds that git/exec errors are full of) to a
// single space, so an error never wraps its row past one physical
// line. The full text remains in the batch JSONL.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// viewportCap is the upper bound on rendered repository entries.
// Short terminals may render fewer; we never render more.
const viewportCap = 10

// prioritizedRows returns at most viewportCap rows, ranked by
// attention priority then by recent activity then by stable
// discovery order. All retained rows remain in m.rows regardless
// of the result.
func (m *Model) prioritizedRows() []*RepoRow {
	all := make([]*RepoRow, 0, len(m.order))
	for _, p := range m.order {
		all = append(all, m.rows[p])
	}
	sort.SliceStable(all, func(i, j int) bool {
		return rowLess(all[i], all[j])
	})
	if len(all) > viewportCap {
		all = all[:viewportCap]
	}
	return all
}

// fitToHeight shrinks the prioritized slice to the number of
// rows that fit in the available height. Every row occupies exactly
// one physical line, so the budget is one line per retained row.
func fitToHeight(rows []*RepoRow, avail int) []*RepoRow {
	if avail <= 0 {
		return nil
	}
	out := make([]*RepoRow, 0, len(rows))
	budget := avail
	for _, r := range rows {
		if budget < 1 {
			break
		}
		out = append(out, r)
		budget--
	}
	return out
}

func rowLess(a, b *RepoRow) bool {
	pa, pb := priorityClass(a), priorityClass(b)
	if pa != pb {
		return pa < pb // lower class index wins
	}
	if a.ActivitySeq != b.ActivitySeq {
		return a.ActivitySeq > b.ActivitySeq // more recent activity first
	}
	return a.DiscoverSeq < b.DiscoverSeq // stable: earlier discovery first
}

func priorityClass(r *RepoRow) int {
	switch {
	case r.Phase == PhaseWorking:
		return 0
	case r.Phase == PhaseFailed:
		return 1
	case r.Warning:
		return 2
	default:
		return 3
	}
}

func (m *Model) View() string {
	// Column visibility by terminal width: at >=80 cols show AGE,
	// below that REPO/CHANGE/PHASE/RETRY only.
	showAge := m.width >= 80
	// ERROR is a flex column: the terminal width minus the active
	// fixed columns. It shows only when that remainder is at least
	// errMinWidth, so a row can never exceed one physical line.
	fixedSum := wRepo + wChange + wPhase + wRetry
	if showAge {
		fixedSum += wAge
	}
	errWidth := m.width - fixedSum
	showErr := errWidth >= errMinWidth

	// Fixed lines: summary, header, footer, optional infrastructure
	// error. Rows fill whatever remains, capped at viewportCap.
	fixedLines := 3
	if m.infraErr != "" {
		fixedLines++
	}
	avail := m.height - fixedLines
	if m.height == 0 {
		// Pre-WindowSizeMsg: render up to the cap so the first
		// paint shows real content.
		avail = viewportCap
	}
	visible := fitToHeight(m.prioritizedRows(), avail)
	summary := m.renderSummary(len(visible))

	header := m.renderHeader(showAge, showErr, errWidth)
	body := make([]string, 0, len(visible))
	for _, r := range visible {
		body = append(body, m.renderRow(r, showAge, showErr, errWidth))
	}

	parts := []string{summary, header}
	if len(body) == 0 {
		parts = append(parts, "(no repos scanned yet)")
	} else {
		parts = append(parts, strings.Join(body, "\n"))
	}
	if m.infraErr != "" {
		parts = append(parts, "! "+m.infraErr)
	}
	parts = append(parts, "[q] quit")
	return strings.Join(parts, "\n")
}

func (m *Model) renderHeader(showAge, showErr bool, errWidth int) string {
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
		parts = append(parts, lipgloss.NewStyle().Width(errWidth).Align(lipgloss.Left).Render("ERROR"))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m *Model) renderRow(r *RepoRow, showAge, showErr bool, errWidth int) string {
	phaseStr := phaseString(r)
	retry := "—"
	if r.RetryMax > 0 {
		retry = fmt.Sprintf("%d/%d", r.RetryN, r.RetryMax)
	}
	age := "—"
	if !r.StartedAt.IsZero() {
		age = time.Since(r.StartedAt).Round(time.Second).String()
	}

	// The ⚠ glyph lives in the REPO cell so a fast scan lights up the
	// row even on terminals too narrow for the ERROR column; the
	// failure reason renders inline in ERROR when wide enough and
	// always lands in the batch JSONL.
	name := r.Name
	if r.Warning {
		name = name + " ⚠"
	}

	// Column widths are anchored to the styles above; truncate change
	// by the column width to avoid overflow on narrow terminals.
	// Repos without an openspec/ get an em-dash change column so the
	// grid stays readable without a dedicated phase.
	change := r.Change
	if r.Workflow != "" {
		change = r.Workflow + ": " + change
	}
	if !r.HasChange {
		change = "—"
	}
	change = truncate(change, wChange)

	parts := []string{
		colRepo.Render(truncate(name, wRepo)),
		colChange.Render(change),
		colPhase.Render(phaseStr),
		colRetry.Render(retry),
	}
	if showAge {
		parts = append(parts, colAge.Render(age))
	}
	if showErr {
		// Collapse whitespace so a multi-line git/exec error cannot
		// wrap the row; the flex width keeps the cell within the line.
		err := "—"
		if r.LastErr != "" {
			err = truncate(oneLine(r.LastErr), errWidth)
		}
		parts = append(parts, lipgloss.NewStyle().Width(errWidth).Align(lipgloss.Left).Render(err))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
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
	}
	return "?"
}

func (m *Model) renderSummary(visible int) string {
	var total, done, working, idle, failed, warnings int
	for _, p := range m.order {
		r := m.rows[p]
		total++
		switch r.Phase {
		case PhaseDone:
			done++
		case PhaseWorking:
			working++
		case PhaseIdle:
			idle++
		case PhaseFailed:
			failed++
		}
		if r.Warning {
			warnings++
		}
	}
	// At narrow widths, hide zero-count buckets and the per-phase
	// breakdown to keep the summary on one line. The total and
	// visible counters are always shown.
	compact := m.width > 0 && m.width < 80
	parts := []string{fmt.Sprintf("%d total", total)}
	add := func(label string, n int) {
		if compact && n == 0 {
			return
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, label))
	}
	add("working", working)
	add("done", done)
	add("idle", idle)
	add("failed", failed)
	add("warning", warnings)
	parts = append(parts, fmt.Sprintf("%d/%d visible", visible, total))
	return strings.Join(parts, "  ")
}
