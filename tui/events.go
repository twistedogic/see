package tui

import "time"

// Message types for the TUI's Update loop. Defined in the tui package
// (not imported from main) so the tui package has no dependency on
// main's Event types. main.go provides an Observer adapter that
// translates each Event into one of these messages and sends it to
// the bubbletea Program via Program.Send.

type RepoSeenMsg struct {
	Path      string
	HasChange bool
}

type ActivityMsg struct {
	Text string
}

// PiActivityMsg is kept as a descriptive alias for callers that want
// to distinguish this presentation message from other TUI updates.
type PiActivityMsg = ActivityMsg

type ChangeStartedMsg struct {
	Path     string
	Workflow string
	Change   string
}

type RetryAttemptMsg struct {
	Path     string
	Workflow string
	Change   string
	N, Max   int
	Err      string
}

type ChangeDoneMsg struct {
	Path     string
	Workflow string
	Change   string
	// Baseline and Candidate are populated when the workflow's
	// measure gate resolved and a successful landing carried both
	// metric strings. They are empty when no measure gate ran.
	Baseline  string
	Candidate string
}

type ChangeFailedMsg struct {
	Path     string
	Workflow string
	Change   string
	Err      string
}

// CheckFailedMsg is the TUI mirror of main's CheckFailed event: the
// final attempt for a repository failed at the workflow check gate.
// It renders like ChangeFailedMsg (the row goes to PhaseFailed with
// the failure message in the error column) but is distinguished at
// the event layer so consumers that treat "check failed" differently
// from "agent failed" can branch on the message type.
type CheckFailedMsg struct {
	Path     string
	Workflow string
	Change   string
	Command  string
	ExitCode int
	Err      string
}

// MeasureFailedMsg is the TUI mirror of main's MeasureFailed event:
// the final attempt failed at the workflow measure gate (baseline
// capture failure, candidate capture failure, or non-improvement).
// It renders identically to CheckFailedMsg and ChangeFailedMsg —
// the row goes to PhaseFailed with the failure message in the
// error column — but is distinguished at the event layer so
// consumers that surface "no improvement" differently from "check
// failed" or "agent failed" can branch on the message type.
type MeasureFailedMsg struct {
	Path      string
	Workflow  string
	Change    string
	Command   string
	ExitCode  int
	Baseline  string
	Candidate string
	Err       string
}

// WarningMsg reports a per-repo cleanup or pre-run check step that
// failed. The TUI renders a ⚠ glyph next to the repo's row and
// counts the row in the footer's warning counter until the next
// ChangeStartedMsg for the same repo clears it.
type WarningMsg struct {
	Path     string
	Workflow string
	Change   string
	Msg      string
}

// InfraErrorMsg reports a process-level failure surfaced by runTUI
// (Watcher.Watch returned an error, or bubbletea Program.Run
// returned an error). The TUI renders it as a banner between the
// grid body and the footer; latest event wins.
type InfraErrorMsg struct {
	Where string
	Err   string
}

// tickMsg fires once a second to drive a re-render. AGE is
// recomputed at render time (time.Since(StartedAt)) so without a
// periodic tick the column freezes at whatever View() produced
// when the last event arrived — which is wrong for any row that
// has been in PhaseWorking longer than the gap between events.
type tickMsg time.Time

type marqueeTickMsg struct{}
