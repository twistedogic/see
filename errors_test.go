package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Regression for shorten-tui-errors task 1.1: the custom
// dirty-working-tree error must expose both representations —
// the full path-bearing diagnostic on Error() and the concise
// "dirty working tree; commit or stash" summary on Summary() —
// and the summary helper must find Summary() through a %w wrapper.
func TestDirtyWorkingTreeErrorExposesBothDiagnostics(t *testing.T) {
	const path = "/repos/myrepo"
	err := &dirtyWorkingTreeError{path: path}

	wantFull := "see: working tree on /repos/myrepo is dirty; commit or stash before see runs"
	if got := err.Error(); got != wantFull {
		t.Fatalf("Error() = %q, want %q", got, wantFull)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("Error() = %q, want path %q included", err.Error(), path)
	}

	const wantSummary = "dirty working tree; commit or stash"
	if got := err.Summary(); got != wantSummary {
		t.Fatalf("Summary() = %q, want %q", got, wantSummary)
	}
}

// Regression for shorten-tui-errors task 1.1: the summary helper
// must reach Summary() through an ordinary fmt.Errorf("%w") wrapper,
// so callers can add context without losing the concise text. The
// wrapper's own Error() must still surface the full path-bearing
// diagnostic via the inner error's Error() method.
func TestSummaryForFindsSummaryThroughWWrap(t *testing.T) {
	const path = "/repos/myrepo"
	inner := &dirtyWorkingTreeError{path: path}
	wrapped := fmt.Errorf("see: could not start watcher: %w", inner)

	const wantSummary = "dirty working tree; commit or stash"
	if got := summaryFor(wrapped); got != wantSummary {
		t.Fatalf("summaryFor(wrapped) = %q, want %q", got, wantSummary)
	}
	if !strings.Contains(wrapped.Error(), path) {
		t.Fatalf("wrapped.Error() = %q, want path %q surfaced from inner Error()", wrapped.Error(), path)
	}
	if !strings.Contains(wrapped.Error(), "could not start watcher") {
		t.Fatalf("wrapped.Error() = %q, want wrapper context preserved", wrapped.Error())
	}
}

// Contract: summaryFor is a no-op for ordinary errors that do not
// implement the summary interface. The empty string is the "no
// concise summary" signal; the Terminal User Interface (TUI)
// adapter falls back to the exported Err when it sees "".
func TestSummaryForReturnsEmptyForOrdinaryErrors(t *testing.T) {
	for _, in := range []error{
		errors.New("plain error"),
		fmt.Errorf("wrapped: %w", errors.New("inner")),
	} {
		if got := summaryFor(in); got != "" {
			t.Fatalf("summaryFor(%v) = %q, want empty", in, got)
		}
	}
}

// Contract: errors.As on a pointer-receiver Summary() must work so
// event construction sites can pass the *dirtyWorkingTreeError
// directly. (TestSummaryForFindsSummaryThroughWWrap already covers
// the wrapping-chain case; this pins the pointer-receiver detail.)
func TestDirtyWorkingTreeErrorSatisfiesErrorSummaryViaPointer(t *testing.T) {
	err := &dirtyWorkingTreeError{path: "/r"}
	var s errorSummary
	if !errors.As(err, &s) {
		t.Fatal("errors.As did not find errorSummary on *dirtyWorkingTreeError")
	}
	if got, want := s.Summary(), "dirty working tree; commit or stash"; got != want {
		t.Fatalf("Summary() = %q, want %q", got, want)
	}
}
