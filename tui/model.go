package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type Phase int

const (
	PhaseIdle Phase = iota
	PhaseWorking
	PhaseDone
	PhaseFailed
	PhaseNoSpec
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
	case PhaseNoSpec:
		return "no-spec"
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
	case PhaseNoSpec:
		return "○"
	}
	return "?"
}

type RepoRow struct {
	Name        string
	Change      string
	Phase       Phase
	RetryN      int
	RetryMax    int
	StartedAt   time.Time
	LastErr     string
	HasOpenspec bool
	LogPath     string
	Warning     bool
}

type Model struct {
	rows     map[string]*RepoRow // keyed by repo path
	order    []string            // scan order, for stable rendering
	width    int
	height   int
	infraErr string
}

func NewModel() *Model {
	return &Model{rows: map[string]*RepoRow{}, width: 120, height: 24}
}

func (m *Model) ensureRow(path string) *RepoRow {
	if r, ok := m.rows[path]; ok {
		return r
	}
	r := &RepoRow{Name: basename(path), Phase: PhaseIdle, HasOpenspec: true}
	m.rows[path] = r
	m.order = append(m.order, path)
	return r
}

func basename(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case RepoSeenMsg:
		r := m.ensureRow(msg.Path)
		r.HasOpenspec = msg.HasOpenspec
		if !msg.HasOpenspec {
			r.Phase = PhaseNoSpec
			r.Change = "—"
		}
	case ChangeStartedMsg:
		r := m.ensureRow(msg.Path)
		r.Phase = PhaseWorking
		r.Change = msg.Change
		r.StartedAt = time.Now()
		r.LastErr = ""
		r.RetryN = 0
		r.RetryMax = 0
		r.Warning = false
	case RetryAttemptMsg:
		r := m.ensureRow(msg.Path)
		r.Phase = PhaseWorking
		r.Change = msg.Change
		r.RetryN = msg.N
		r.RetryMax = msg.Max
		r.LastErr = msg.Err
	case ChangeDoneMsg:
		r := m.ensureRow(msg.Path)
		r.Phase = PhaseDone
		r.Change = msg.Change
		r.LastErr = ""
		r.RetryN = 0
		r.RetryMax = 0
	case ChangeFailedMsg:
		r := m.ensureRow(msg.Path)
		r.Phase = PhaseFailed
		r.Change = msg.Change
		r.LastErr = msg.Err
	case LogPathMsg:
		r := m.ensureRow(msg.Path)
		r.LogPath = msg.Path
		if r.Change == "" {
			r.Change = msg.Change
		}
	case WarningMsg:
		r := m.ensureRow(msg.Path)
		r.Warning = true
	case InfraErrorMsg:
		m.infraErr = msg.Err
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
