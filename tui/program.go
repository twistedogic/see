package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// ChanObserver owns a bubbletea Program and exposes typed methods for
// each event kind. main.go's adapter (tuiObserver) calls those methods
// from its Observer.Observe(Event). The type-switch lives in main,
// not here, so this package has no dependency on main's Event types.
type ChanObserver struct {
	p *tea.Program
}

// New constructs a Program and an observer adapter. The caller wires
// the observer to a Watcher (as Watcher.observer), then calls
// prog.Run() to start the UI. prog.Run() blocks until the user quits
// or the program is killed.
func New() (*tea.Program, *ChanObserver) {
	m := NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	obs := &ChanObserver{p: p}
	return p, obs
}

// Program exposes the underlying bubbletea Program so main.go can wait
// for it to exit (Done channel) and so the observer can Send messages.
func (c *ChanObserver) Program() *tea.Program { return c.p }

// push sends a tea.Msg to the running program, swallowing send errors
// that occur after the program has already exited (the watcher may
// emit a final event after the user quits).
func (c *ChanObserver) push(msg tea.Msg) {
	defer func() { _ = recover() }()
	c.p.Send(msg)
}

func (c *ChanObserver) RepoSeen(path string, hasOpenspec bool) {
	c.push(RepoSeenMsg{Path: path, HasOpenspec: hasOpenspec})
}

func (c *ChanObserver) ChangeStarted(path, change string) {
	c.push(ChangeStartedMsg{Path: path, Change: change})
}

func (c *ChanObserver) RetryAttempt(path, change string, n, max int, errMsg string) {
	c.push(RetryAttemptMsg{Path: path, Change: change, N: n, Max: max, Err: errMsg})
}

func (c *ChanObserver) ChangeDone(path, change string) {
	c.push(ChangeDoneMsg{Path: path, Change: change})
}

func (c *ChanObserver) ChangeFailed(path, change, errMsg string) {
	c.push(ChangeFailedMsg{Path: path, Change: change, Err: errMsg})
}

func (c *ChanObserver) LogPath(path, change string) {
	c.push(LogPathMsg{Path: path, Change: change})
}
