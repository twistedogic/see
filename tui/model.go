package tui

import (
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type Phase int

const (
	PhaseIdle Phase = iota
	PhaseWorking
	PhaseDone
	PhaseFailed
)

func (p Phase) String() string {
	switch p {
	case PhaseIdle:
		return "idle"
	case PhaseWorking:
		return "working"
	case PhaseDone:
		return "done"
	case PhaseFailed:
		return "failed"
	}
	return "?"
}

func (p Phase) Glyph() string {
	switch p {
	case PhaseIdle:
		return "○"
	case PhaseWorking:
		return "●"
	case PhaseDone:
		return "✓"
	case PhaseFailed:
		return "✗"
	}
	return "?"
}

type RepoRow struct {
	Name        string
	Workflow    string
	Change      string
	Phase       Phase
	RetryN      int
	RetryMax    int
	StartedAt   time.Time
	LastErr     string
	HasChange   bool
	LogPath     string
	Warning     bool
	DiscoverSeq uint64 // assigned on first RepoSeen; stable fallback order
	ActivitySeq uint64 // advanced on meaningful lifecycle events
}

type Model struct {
	rows        map[string]*RepoRow // keyed by repo path
	order       []string            // scan order, for stable rendering
	width       int
	height      int
	infraErr    string
	discoverSeq uint64 // monotonic, assigned to a row on first RepoSeen
	activitySeq uint64 // monotonic, advanced on each meaningful lifecycle event
}

func NewModel() *Model {
	return &Model{rows: map[string]*RepoRow{}, width: 120, height: 24}
}

func (m *Model) ensureRow(path string) *RepoRow {
	if r, ok := m.rows[path]; ok {
		return r
	}
	m.discoverSeq++
	r := &RepoRow{
		Name:        filepath.Base(path),
		Phase:       PhaseIdle,
		HasChange:   true,
		DiscoverSeq: m.discoverSeq,
	}
	m.rows[path] = r
	m.order = append(m.order, path)
	return r
}

// markActivity advances the row's activity sequence. Called for
// every lifecycle event except repeated RepoSeen, so a row that
// only ever receives heartbeats never climbs the activity order.
func (m *Model) markActivity(r *RepoRow) {
	m.activitySeq++
	r.ActivitySeq = m.activitySeq
}

func (m *Model) Init() tea.Cmd {
	// ponytail: 1Hz tick, no-op when nothing is happening. Smarter
	// version (only tick while PhaseWorking is non-empty) when the
	// per-second redraw shows up in profiling.
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case RepoSeenMsg:
		// Repeated RepoSeen for an existing row is a scan heartbeat
		// and must not advance the activity order. ensureRow only
		// assigns a DiscoverSeq on first observation; a heartbeat
		// leaves it untouched.
		r := m.ensureRow(msg.Path)
		r.HasChange = msg.HasChange
		if !msg.HasChange {
			r.Phase = PhaseIdle
			r.Change = "—"
		}
	case ChangeStartedMsg:
		r := m.ensureRow(msg.Path)
		m.markActivity(r)
		r.Phase = PhaseWorking
		r.Workflow = msg.Workflow
		r.Change = msg.Change
		r.StartedAt = time.Now()
		r.LastErr = ""
		r.RetryN = 0
		r.RetryMax = 0
		r.Warning = false
	case RetryAttemptMsg:
		r := m.ensureRow(msg.Path)
		m.markActivity(r)
		r.Phase = PhaseWorking
		r.Workflow = msg.Workflow
		r.Change = msg.Change
		r.RetryN = msg.N
		r.RetryMax = msg.Max
		r.LastErr = msg.Err
	case ChangeDoneMsg:
		r := m.ensureRow(msg.Path)
		m.markActivity(r)
		r.Phase = PhaseDone
		r.Workflow = msg.Workflow
		r.Change = msg.Change
		r.LastErr = ""
		r.RetryN = 0
		r.RetryMax = 0
	case ChangeFailedMsg:
		r := m.ensureRow(msg.Path)
		m.markActivity(r)
		r.Phase = PhaseFailed
		r.Workflow = msg.Workflow
		r.Change = msg.Change
		r.LastErr = msg.Err
	case LogPathMsg:
		r := m.ensureRow(msg.Path)
		r.LogPath = msg.Path
		if msg.Workflow != "" {
			r.Workflow = msg.Workflow
		}
		if r.Change == "" {
			r.Change = msg.Change
		}
	case WarningMsg:
		r := m.ensureRow(msg.Path)
		m.markActivity(r)
		r.Warning = true
		if msg.Workflow != "" {
			r.Workflow = msg.Workflow
		}
	case InfraErrorMsg:
		m.infraErr = msg.Err
	case tickMsg:
		// Re-arm the tick. We don't mutate any row state — View()
		// recomputes AGE from StartedAt, so the tick is purely a
		// redraw trigger.
		return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}
