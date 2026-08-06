package main

import (
	"errors"
	"fmt"
)

// errorSummary is the optional interface an error implements to
// opt into a concise Terminal User Interface (TUI) summary.
// errors.As walks the wrapping chain so ordinary fmt.Errorf("%w")
// wrapping does not erase it.
type errorSummary interface {
	Summary() string
}

// summaryFor returns the concise summary text for err, or "" when
// no error in the chain implements errorSummary. The empty signal
// lets the TUI adapter fall back to the exported Err instead of
// duplicating err.Error() in the unexported event field.
func summaryFor(err error) string {
	var s errorSummary
	if errors.As(err, &s) {
		return s.Summary()
	}
	return ""
}

// dirtyWorkingTreeError reports a dirty working tree at the moment
// of the run. Error() preserves the existing path-bearing diagnostic;
// Summary() prefixes cause + action so it stays identifying at the
// narrowest ERROR tier.
type dirtyWorkingTreeError struct {
	path string
}

func (e *dirtyWorkingTreeError) Error() string {
	return fmt.Sprintf("see: working tree on %s is dirty; commit or stash before see runs", e.path)
}

func (e *dirtyWorkingTreeError) Summary() string {
	return "dirty working tree; commit or stash"
}

// checkFailedError reports that a workflow's check gate exited
// nonzero after a successful agent run. The rendered command, exit
// code, and captured stderr are carried so retries can summarize
// the failure and the terminal CheckFailed event has everything it
// needs without re-running the shell. Error() prefixes the rendered
// command and surfaces stderr; Summary() exposes the concise
// "check failed" tier so the TUI grid and RetryAttempt events stay
// short. The sentinel implements errorSummary so errors.As walks
// any rollback wrapper cleanly.
type checkFailedError struct {
	command  string
	exitCode int
	stderr   string
}

func (e *checkFailedError) Error() string {
	return fmt.Sprintf("see: check failed: %s (exit %d): %s", e.command, e.exitCode, e.stderr)
}

func (e *checkFailedError) Summary() string {
	return "check failed"
}

// measureFailedError reports that a workflow's measure gate failed
// at either the baseline (before the agent ran) or the candidate
// (after the agent succeeded). It is the new sibling of
// checkFailedError: the rendered command, exit code, the baseline
// and candidate values when available, and the captured stderr are
// carried so retries can summarize the failure and the terminal
// MeasureFailed event has everything it needs without re-running
// the shell. Error() distinguishes "no improvement" (both
// baseline and candidate populated, candidate not strictly greater)
// from "command errored" (baseline empty; candidate may carry the
// unparseable value when the candidate phase caught it).
// Summary() exposes the concise "no improvement" / "measure failed"
// tier so the TUI grid and RetryAttempt events stay short. The
// sentinel implements errorSummary so errors.As walks any rollback
// wrapper cleanly.
type measureFailedError struct {
	command   string
	exitCode  int
	baseline  string
	candidate string
	stderr    string
}

func (e *measureFailedError) Error() string {
	switch {
	case e.baseline != "" && e.candidate != "":
		return fmt.Sprintf("see: measure failed: candidate %s does not exceed baseline %s (%s)", e.candidate, e.baseline, e.command)
	case e.candidate != "":
		return fmt.Sprintf("see: measure failed: %s output %q is not a number", e.command, e.candidate)
	case e.stderr != "":
		return fmt.Sprintf("see: measure failed: %s (exit %d): %s", e.command, e.exitCode, e.stderr)
	default:
		return fmt.Sprintf("see: measure failed: %s (exit %d)", e.command, e.exitCode)
	}
}

func (e *measureFailedError) Summary() string {
	if e.baseline != "" && e.candidate != "" {
		return "no improvement"
	}
	return "measure failed"
}
