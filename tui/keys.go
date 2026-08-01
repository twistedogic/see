package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

// keymap is the single source of truth for the TUI footer's key
// bindings. The matcher's source of truth is each binding's Keys
// (key.Matches reads them); the label's source of truth is each
// binding's Help (the help bar renders it). The two live on the same
// field, so there is no parallel string literal that can drift.
//
// Adding a binding is a one-field change to this struct plus, at most,
// a matching key.Matches case in Model.Update — the help bar picks it
// up from ShortHelp/FullHelp with no footer-rendering change.
type keymap struct {
	Quit key.Binding
	Help key.Binding
}

// Compile-time assertion that keymap satisfies help.KeyMap so the
// help package's View/ShortHelpView/FullHelpView accept it without a
// runtime type assertion.
var _ help.KeyMap = keymap{}

func defaultKeymap() keymap {
	return keymap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
	}
}

// ShortHelp returns the one-line help sequence, in render order.
func (k keymap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit, k.Help}
}

// FullHelp stacks both bindings in a single column so the toggled
// full view is two physical lines tall (the height budget grows by
// one line when ShowAll flips on). The short and full renderings
// reuse the same keymap; the full view is not a separate screen.
func (k keymap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Quit, k.Help}}
}
