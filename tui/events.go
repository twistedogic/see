package tui

// Message types for the TUI's Update loop. Defined in the tui package
// (not imported from main) so the tui package has no dependency on
// main's Event types. main.go provides an Observer adapter that
// translates each Event into one of these messages and sends it to
// the bubbletea Program via Program.Send.

type RepoSeenMsg struct {
	Path        string
	HasOpenspec bool
}

type ChangeStartedMsg struct {
	Path   string
	Change string
}

type RetryAttemptMsg struct {
	Path   string
	Change string
	N, Max int
	Err    string
}

type ChangeDoneMsg struct {
	Path   string
	Change string
}

type ChangeFailedMsg struct {
	Path   string
	Change string
	Err    string
}

type LogPathMsg struct {
	Path   string
	Change string
}

// WarningMsg reports a per-repo cleanup or pre-run check step that
// failed. The TUI renders a ⚠ glyph next to the repo's row and
// counts the row in the footer's warning counter until the next
// ChangeStartedMsg for the same repo clears it.
type WarningMsg struct {
	Path   string
	Change string
	Msg    string
}

// InfraErrorMsg reports a process-level failure surfaced by runTUI
// (Watcher.Watch returned an error, or bubbletea Program.Run
// returned an error). The TUI renders it as a banner between the
// grid body and the footer; latest event wins.
type InfraErrorMsg struct {
	Where string
	Err   string
}
