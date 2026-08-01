package tui

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// TestKeymapPinsQuitAndHelpBindings pins the typed keymap that is the
// single source of truth for the footer's key bindings. The matcher's
// source of truth is Keys; the label's source of truth is Help; the
// two cannot drift because they live in the same field.
func TestKeymapPinsQuitAndHelpBindings(t *testing.T) {
	keys := defaultKeymap()

	// Quit matches q and ctrl+c (the historical quit keys) and
	// nothing else.
	if !key.Matches(tea.KeyPressMsg{Text: "q"}, keys.Quit) {
		t.Fatal("keymap.Quit should match a q keypress")
	}
	if !key.Matches(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, keys.Quit) {
		t.Fatal("keymap.Quit should match a ctrl+c keypress")
	}
	if key.Matches(tea.KeyPressMsg{Text: "x"}, keys.Quit) {
		t.Fatal("keymap.Quit should not match an unrelated key")
	}

	// Help toggles the help bar; it matches only the ? key.
	if !key.Matches(tea.KeyPressMsg{Text: "?"}, keys.Help) {
		t.Fatal("keymap.Help should match a ? keypress")
	}
	if key.Matches(tea.KeyPressMsg{Text: "?"}, keys.Quit) {
		t.Fatal("? must not match the Quit binding")
	}

	// The help labels are what the help bar renders.
	if got := keys.Quit.Help(); got.Key != "q" || got.Desc != "quit" {
		t.Fatalf("Quit help = {Key: %q, Desc: %q}, want {q, quit}", got.Key, got.Desc)
	}
	if got := keys.Help.Help(); got.Key != "?" || got.Desc != "help" {
		t.Fatalf("Help help = {Key: %q, Desc: %q}, want {?, help}", got.Key, got.Desc)
	}
}

// TestKeymapShortAndFullHelpOrder pins the help.KeyMap methods so
// the help package's View/ShortHelpView/FullHelpView render the
// bindings correctly. ShortHelp returns the one-line sequence shown
// by default; FullHelp stacks both bindings in one column so the
// toggled full view is two physical lines tall.
func TestKeymapShortAndFullHelpOrder(t *testing.T) {
	keys := defaultKeymap()

	short := keys.ShortHelp()
	if len(short) != 2 {
		t.Fatalf("ShortHelp has %d bindings, want 2", len(short))
	}
	for i, want := range []struct{ key, desc string }{{"q", "quit"}, {"?", "help"}} {
		if got := short[i].Help(); got.Key != want.key || got.Desc != want.desc {
			t.Fatalf("ShortHelp[%d] = %v, want %v", i, got, want)
		}
	}

	full := keys.FullHelp()
	if len(full) != 1 {
		t.Fatalf("FullHelp has %d columns, want 1 (so the full view is two lines)", len(full))
	}
	col := full[0]
	if len(col) != 2 {
		t.Fatalf("FullHelp column has %d bindings, want 2", len(col))
	}
	for i, want := range []struct{ key, desc string }{{"q", "quit"}, {"?", "help"}} {
		if got := col[i].Help(); got.Key != want.key || got.Desc != want.desc {
			t.Fatalf("FullHelp[0][%d] = %v, want %v", i, got, want)
		}
	}
}
