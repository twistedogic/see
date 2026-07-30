package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// ChanObserver owns a bubbletea Program and forwards any tea.Msg to it
// via Send. main.go's adapter (tuiObserver) builds a `*Msg` literal
// directly and sends it through Send without per-event-type methods;
// the type-switch lives in main so this package has no dependency on
// main's Event types.
type ChanObserver struct {
	p *tea.Program
}

// New constructs a Program and an observer adapter. The caller wires
// the observer to a Watcher (as Watcher.observer), then calls
// prog.Run() to start the UI. prog.Run() blocks until the user quits
// or the program is killed.
//
// In Bubble Tea v2, alternate-screen mode is declared on the model's
// returned tea.View (see Model.View), so no WithAltScreen option is
// needed here.
func New(ctx context.Context) (*tea.Program, *ChanObserver) {
	m := NewModel()
	p := tea.NewProgram(m, tea.WithContext(ctx))
	obs := &ChanObserver{p: p}
	return p, obs
}

// Program exposes the underlying bubbletea Program so main.go can wait
// for it to exit (Done channel) and so the observer can Send messages.
func (c *ChanObserver) Program() *tea.Program { return c.p }

// Send forwards a tea.Msg to the program.
func (c *ChanObserver) Send(msg tea.Msg) {
	c.p.Send(msg)
}
