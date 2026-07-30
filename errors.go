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
