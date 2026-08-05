package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// invocFileName builds the canonical per-invocation filename for a
// stem and a fixed-width UTC timestamp, mirroring logFilename so
// tests don't depend on the wall clock.
func invocFileName(stem, ts string) string {
	return fmt.Sprintf("%s--%s--1.jsonl", stem, ts)
}

// writeInvocFiles creates one JSONL file per (stem, ts) pair inside
// dir, returning the basenames in the same order.
func writeInvocFiles(t *testing.T, dir, stem string, timestamps []string) []string {
	t.Helper()
	var names []string
	for _, ts := range timestamps {
		name := invocFileName(stem, ts)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		names = append(names, name)
	}
	return names
}

// listNames returns the basenames in dir that exist on disk, sorted
// ascending so the caller can compare directly.
func listNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

// listStem returns the basenames in dir whose name has the exact
// prefix stem+"--" and the .jsonl suffix, sorted ascending. Used to
// inspect the per-stem population after PiAgent.Run.
func listStem(t *testing.T, dir, stem string) []string {
	t.Helper()
	prefix := stem + "--"
	suffix := ".jsonl"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, prefix) && strings.HasSuffix(n, suffix) {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names
}

// TestRotateLogsTrimsToKeepCount: a stem with 7 files is reduced to
// the 5 newest; the oldest 2 are deleted; a stem with ≤5 is
// untouched. (tasks.md 3.1)
func TestRotateLogsTrimsToKeepCount(t *testing.T) {
	dir := t.TempDir()
	stem := "myproj--add-dark-mode"
	writeInvocFiles(t, dir, stem, []string{
		"20240101T000000", "20240101T000100", "20240101T000200",
		"20240101T000300", "20240101T000400", "20240101T000500",
		"20240101T000600",
	})
	rotateLogs(dir, stem, maxInvocLogsPerStem)
	got := listNames(t, dir)
	want := []string{
		invocFileName(stem, "20240101T000200"),
		invocFileName(stem, "20240101T000300"),
		invocFileName(stem, "20240101T000400"),
		invocFileName(stem, "20240101T000500"),
		invocFileName(stem, "20240101T000600"),
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("after rotate got %v, want %v", got, want)
	}
}

// TestRotateLogsNoopForUnderLimit: a stem with ≤ keep files is left
// untouched. (tasks.md 3.1)
func TestRotateLogsNoopForUnderLimit(t *testing.T) {
	dir := t.TempDir()
	stem := "myproj--add"
	written := writeInvocFiles(t, dir, stem, []string{
		"20240101T000000", "20240101T000100", "20240101T000200",
	})
	rotateLogs(dir, stem, maxInvocLogsPerStem)
	got := listNames(t, dir)
	if strings.Join(got, "\n") != strings.Join(written, "\n") {
		t.Fatalf("under-limit stem touched: got %v, want %v", got, written)
	}
}

// TestRotateLogsPrefixSelectivity: stem "myproj--add" must not delete
// files for stem "myproj--add-dark-mode"; each group rotates
// independently. (tasks.md 3.2)
func TestRotateLogsPrefixSelectivity(t *testing.T) {
	dir := t.TempDir()
	short := "myproj--add"
	long := "myproj--add-dark-mode"
	writeInvocFiles(t, dir, short, []string{
		"20240101T000000", "20240101T000100", "20240101T000200",
		"20240101T000300", "20240101T000400", "20240101T000500",
	})
	writeInvocFiles(t, dir, long, []string{
		"20240101T000000", "20240101T000100", "20240101T000200",
		"20240101T000300", "20240101T000400", "20240101T000500",
	})
	rotateLogs(dir, short, maxInvocLogsPerStem)
	rotateLogs(dir, long, maxInvocLogsPerStem)
	got := listNames(t, dir)
	wantPrefix := []string{
		invocFileName(short, "20240101T000100"),
		invocFileName(short, "20240101T000200"),
		invocFileName(short, "20240101T000300"),
		invocFileName(short, "20240101T000400"),
		invocFileName(short, "20240101T000500"),
	}
	wantLong := []string{
		invocFileName(long, "20240101T000100"),
		invocFileName(long, "20240101T000200"),
		invocFileName(long, "20240101T000300"),
		invocFileName(long, "20240101T000400"),
		invocFileName(long, "20240101T000500"),
	}
	want := append(append([]string{}, wantPrefix...), wantLong...)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestRotateLogsBatchLogExclusion: see--<ts>--<pid>.jsonl files in the
// same directory are never selected or deleted by rotation. (tasks.md 3.3)
func TestRotateLogsBatchLogExclusion(t *testing.T) {
	dir := t.TempDir()
	batch := "see--20240101T000000--1.jsonl"
	if err := os.WriteFile(filepath.Join(dir, batch), []byte("batch"), 0o644); err != nil {
		t.Fatal(err)
	}
	stem := "myproj--add"
	writeInvocFiles(t, dir, stem, []string{
		"20240101T000000", "20240101T000100", "20240101T000200",
		"20240101T000300", "20240101T000400", "20240101T000500",
	})
	rotateLogs(dir, stem, maxInvocLogsPerStem)
	got := listNames(t, dir)
	if !slices.Contains(got, batch) {
		t.Fatalf("batch-level event log %q deleted by rotation; remaining %v", batch, got)
	}
	if len(got) != 1+5 {
		t.Fatalf("after rotate got %d files, want 6 (batch + 5 newest); %v", len(got), got)
	}
}

// TestRotateLogsBestEffortDeletion: an unremovable older file does
// not fail the run; the deletable files are still removed. (tasks.md
// 3.5) We replace one of the oldest files with a non-empty
// directory of the same name so os.Remove returns ENOTEMPTY;
// rotateLogs must swallow that and still remove the other deletable
// file.
func TestRotateLogsBestEffortDeletion(t *testing.T) {
	dir := t.TempDir()
	stem := "myproj--add"
	oldest := invocFileName(stem, "20240101T000000")
	trap := filepath.Join(dir, oldest)
	if err := os.MkdirAll(trap, 0o755); err != nil {
		t.Fatal(err)
	}
	// Non-empty so os.Remove fails with ENOTEMPTY on the trap,
	// while the other oldest entries remain regular files that
	// os.Remove can delete.
	if err := os.WriteFile(filepath.Join(trap, "marker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeInvocFiles(t, dir, stem, []string{
		"20240101T000100", "20240101T000200", "20240101T000300",
		"20240101T000400", "20240101T000500", "20240101T000600",
	})
	rotateLogs(dir, stem, maxInvocLogsPerStem)
	got := listNames(t, dir)
	want := []string{
		oldest, // trap survives the swallow
		invocFileName(stem, "20240101T000200"),
		invocFileName(stem, "20240101T000300"),
		invocFileName(stem, "20240101T000400"),
		invocFileName(stem, "20240101T000500"),
		invocFileName(stem, "20240101T000600"),
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %v, want %v", got, want)
	}
	// The trap must still be a directory with its marker intact:
	// the swallow didn't remove it via some other code path.
	info, err := os.Stat(filepath.Join(trap, "marker"))
	if err != nil {
		t.Fatalf("trap marker missing after rotation: %v", err)
	}
	if info.Size() != 1 {
		t.Fatalf("trap marker size = %d, want 1", info.Size())
	}
}

// TestPiAgentRunRotatesAfterSuccess: after a successful agent run the
// stem is bounded to maxInvocLogsPerStem; the just-written file is
// retained and is closed before rotation; the run's returned
// (logPath, err) is unchanged by rotation. (tasks.md 3.6)
func TestPiAgentRunRotatesAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "myproj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	change := "add"
	// Pre-populate the stem with 6 older files so the new run forces
	// rotation: 7 total in the stem after Run, the oldest must be
	// deleted to bring the count back to the cap.
	for i := 0; i < 6; i++ {
		name := invocFileName("myproj--add", fmt.Sprintf("20240101T00000%d", i))
		if err := os.WriteFile(filepath.Join(logDir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	logPath, err := (PiAgent{binary: script, logDir: logDir}).Run(context.Background(), repo, change, "prompt", "", nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if logPath == "" {
		t.Fatal("logPath empty on successful run")
	}
	stem := invocStem(repo, change)
	matches := listStem(t, logDir, stem)
	if len(matches) != maxInvocLogsPerStem {
		t.Fatalf("after Run, stem has %d files, want %d (cap): %v", len(matches), maxInvocLogsPerStem, matches)
	}
	if !slices.Contains(matches, filepath.Base(logPath)) {
		t.Fatalf("just-written file %q missing from retained set %v", filepath.Base(logPath), matches)
	}
}

// TestPiAgentRunRotatesAfterFailure: the same cap applies when the
// agent run fails; the run's returned error is unchanged. (tasks.md
// 3.6)
func TestPiAgentRunRotatesAfterFailure(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "myproj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	change := "add"
	for i := 0; i < 6; i++ {
		name := invocFileName("myproj--add", fmt.Sprintf("20240101T00000%d", i))
		if err := os.WriteFile(filepath.Join(logDir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	logPath, err := (PiAgent{binary: script, logDir: logDir}).Run(context.Background(), repo, change, "prompt", "", nil)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("Run() error = %v, want exit code 7", err)
	}
	if logPath == "" {
		t.Fatal("logPath empty after failure")
	}
	stem := invocStem(repo, change)
	matches := listStem(t, logDir, stem)
	if len(matches) != maxInvocLogsPerStem {
		t.Fatalf("after failed Run, stem has %d files, want %d (cap): %v", len(matches), maxInvocLogsPerStem, matches)
	}
}

// TestPiAgentRunRotatesWithActivityParser: rotation runs on the
// activity-parser code path too, not only the nil-activity one.
// (tasks.md 2.2)
func TestPiAgentRunRotatesWithActivityParser(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "myproj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	change := "add"
	for i := 0; i < 6; i++ {
		name := invocFileName("myproj--add", fmt.Sprintf("20240101T00000%d", i))
		if err := os.WriteFile(filepath.Join(logDir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Any non-nil ActivityCallback forces the parser path. The
	// body is irrelevant — static code structure picks the parser
	// branch whenever activity != nil, and rotation runs on every
	// return path the same way.
	activity := func(string) {}
	if _, err := (PiAgent{binary: script, logDir: logDir}).Run(context.Background(), repo, change, "prompt", "", activity); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	matches := listStem(t, logDir, invocStem(repo, change))
	if len(matches) != maxInvocLogsPerStem {
		t.Fatalf("after activity-parser Run, stem has %d files, want %d: %v", len(matches), maxInvocLogsPerStem, matches)
	}
}

// TestPiAgentRunSkipsRotationOnFileCreateFailure: when the per-run
// file cannot be created, no rotation runs (no file was written, so
// nothing tipped the stem over the cap). (tasks.md 2.3)
func TestPiAgentRunSkipsRotationOnFileCreateFailure(t *testing.T) {
	dir := t.TempDir()
	// Rogue: a regular file where the logDir would need to be a
	// directory. MkdirAll fails, so os.Create fails, Run returns
	// the error path before rotation.
	rogue := filepath.Join(dir, "file")
	if err := os.WriteFile(rogue, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(rogue, "logs")
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "myproj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath, err := (PiAgent{binary: script, logDir: logDir}).Run(context.Background(), repo, "add", "prompt", "", nil)
	if err == nil {
		t.Fatal("Run returned nil error, want non-nil on file-create failure")
	}
	if logPath != "" {
		t.Fatalf("logPath = %q, want empty on capture failure", logPath)
	}
}

// TestPiAgentRunRotationBothModes: compatibility-mode
// <repo>--<change> and custom-mode <repo>--<digest> stems rotate
// independently. (tasks.md 3.7) We seed both stems with overflow
// files and call Run twice with different `change` arguments; each
// stem must be bounded independently.
func TestPiAgentRunRotationBothModes(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "myproj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	compatChange := "add"
	digest := customChangeDigest("add-dark-mode")
	// Seed 6 files for each stem.
	for i := 0; i < 6; i++ {
		ts := fmt.Sprintf("20240101T00000%d", i)
		_ = os.WriteFile(filepath.Join(logDir, invocFileName("myproj--"+compatChange, ts)), []byte(ts), 0o644)
		_ = os.WriteFile(filepath.Join(logDir, invocFileName("myproj--"+digest, ts)), []byte(ts), 0o644)
	}
	if _, err := (PiAgent{binary: script, logDir: logDir}).Run(context.Background(), repo, compatChange, "prompt", "", nil); err != nil {
		t.Fatalf("compat Run: %v", err)
	}
	if _, err := (PiAgent{binary: script, logDir: logDir}).Run(context.Background(), repo, digest, "prompt", "", nil); err != nil {
		t.Fatalf("custom Run: %v", err)
	}
	if got := listStem(t, logDir, invocStem(repo, compatChange)); len(got) != maxInvocLogsPerStem {
		t.Fatalf("compat stem has %d files, want %d: %v", len(got), maxInvocLogsPerStem, got)
	}
	if got := listStem(t, logDir, invocStem(repo, digest)); len(got) != maxInvocLogsPerStem {
		t.Fatalf("custom stem has %d files, want %d: %v", len(got), maxInvocLogsPerStem, got)
	}
}
