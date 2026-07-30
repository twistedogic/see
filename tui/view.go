package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

// bubblyGridStyles produces a Bubbles table style set with no cell
// or header padding and a Selected style that matches the cell style
// so the unfocused table does not introduce visible selection state.
// The header uses default styling (no padding); bold is preserved so
// existing tests that check for header content remain stable.
func bubblyGridStyles() table.Styles {
	zero := lipgloss.NewStyle()
	cell := lipgloss.NewStyle().Padding(0, 0)
	return table.Styles{
		Cell:     cell,
		Header:   cell,
		Selected: zero,
	}
}

var phaseStyles = struct {
	idle    lipgloss.Style
	working lipgloss.Style
	done    lipgloss.Style
	failed  lipgloss.Style
}{
	idle:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	working: lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
	done:    lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
	failed:  lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
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

// renderContent builds the status grid as a string. It is the
// rendering seam that View() wraps in a tea.View, and the seam that
// tests inspect without depending on terminal-control behavior.
func (m *Model) renderContent() string {
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
	if errWidth < 0 {
		errWidth = 0
	}
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

	tbl := m.buildTable(visible, showAge, showErr, errWidth)

	parts := []string{summary, tbl.View()}
	if len(visible) == 0 {
		parts = append(parts, "(no repos scanned yet)")
	}
	if m.infraErr != "" {
		parts = append(parts, "! "+m.infraErr)
	}
	parts = append(parts, "[q] quit")
	return strings.Join(parts, "\n")
}

// buildTable constructs an unfocused bubbles table fed by the
// already-prioritized and height-fitted repository rows. The table
// remains unfocused (focus=false) so the bubbles navigation keys
// stay inert; cell and header padding are zero so the fixed-column
// width arithmetic above remains valid. Selected style is
// visually identical to Cell so cursor state never leaks into the
// rendered output.
func (m *Model) buildTable(visible []*RepoRow, showAge, showErr bool, errWidth int) table.Model {
	cols := []table.Column{
		{Title: "REPO", Width: wRepo},
		{Title: "CHANGE", Width: wChange},
		{Title: "PHASE", Width: wPhase},
		{Title: "RETRY", Width: wRetry},
	}
	if showAge {
		cols = append(cols, table.Column{Title: "AGE", Width: wAge})
	}
	if showErr {
		cols = append(cols, table.Column{Title: "ERROR", Width: errWidth})
	}

	rows := make([]table.Row, 0, len(visible))
	for _, r := range visible {
		rows = append(rows, m.buildCells(r, showAge, showErr, errWidth))
	}

	// +1 for the header line that bubbles' View() prepends.
	height := len(visible) + 1
	if height < 2 {
		height = 2 // table needs at least height=1 (header alone)
	}
	tbl := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithHeight(height),
		table.WithWidth(m.width),
		table.WithFocused(false),
		table.WithStyles(bubblyGridStyles()),
	)
	return tbl
}

// buildCells returns the visible cells for one repository row at
// the responsive column configuration currently in effect. The PHASE
// cell carries its Lip Gloss color so the glyph renders colored in
// the bubbles table; the rest of the cells are plain strings.
// Each cell is truncated to its column width to keep the row on
// one physical line even when the source value runs long.
func (m *Model) buildCells(r *RepoRow, showAge, showErr bool, errWidth int) table.Row {
	phaseStr := phaseCellValue(r)
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

	// Column widths are anchored to the styles above; bubbles truncates
	// cells to col.Width via ansi.Truncate, so the source value can be
	// passed at its natural length. Repos without an openspec/ get an
	// em-dash change column so the grid stays readable without a
	// dedicated phase.
	change := r.Change
	if r.Workflow != "" {
		change = r.Workflow + ": " + change
	}
	if !r.HasChange {
		change = "—"
	}

	cells := []string{name, change, phaseStr, retry}
	if showAge {
		cells = append(cells, age)
	}
	if showErr {
		// Collapse whitespace so a multi-line git/exec error cannot
		// wrap the row; bubbles truncates the cell to errWidth.
		errCell := "—"
		if r.LastErr != "" {
			errCell = oneLine(r.LastErr)
		}
		cells = append(cells, errCell)
	}
	return cells
}

func phaseCellValue(r *RepoRow) string {
	var glyph, label string
	switch r.Phase {
	case PhaseIdle:
		glyph, label = "○", "idle"
	case PhaseWorking:
		glyph, label = "●", "working"
	case PhaseDone:
		glyph, label = "✓", "done"
	case PhaseFailed:
		glyph, label = "✗", "failed"
	default:
		glyph, label = "?", "?"
	}
	var style lipgloss.Style
	switch r.Phase {
	case PhaseIdle:
		style = phaseStyles.idle
	case PhaseWorking:
		style = phaseStyles.working
	case PhaseDone:
		style = phaseStyles.done
	case PhaseFailed:
		style = phaseStyles.failed
	}
	return style.Render(glyph + " " + label)
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

// View returns the Bubble Tea v2 view: the rendered content with the
// alternate-screen buffer declared on the returned view so Bubble
// Tea's lifecycle takes care of entry and exit.
func (m *Model) View() tea.View {
	v := tea.NewView(m.renderContent())
	v.AltScreen = true
	return v
}
