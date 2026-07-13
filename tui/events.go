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