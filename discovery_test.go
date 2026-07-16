package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// mkRepo is a tiny helper that creates a directory with a .git/ child
// so classifyTarget / resolveTargets can recognize it as a repo.
func mkRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// --- expandTilde -------------------------------------------------------

func TestExpandTilde(t *testing.T) {
	t.Setenv("HOME", "/home/alice")
	tests := []struct {
		in, want string
	}{
		{"~/work", "/home/alice/work"},
		{"~", "/home/alice"},
		{"/abs/path", "/abs/path"},
		{"relative/path", "relative/path"},
		{"~other", "~other"}, // bare ~user is not expanded; treated as literal
		{"", ""},
	}
	for _, tc := range tests {
		got, err := expandTilde(tc.in)
		if err != nil {
			t.Fatalf("expandTilde(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("expandTilde(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- resolveTargets ----------------------------------------------------

func TestResolveTargetsLiteralRepo(t *testing.T) {
	dir := t.TempDir()
	mkRepo(t, dir)
	got, warns, err := resolveTargets([]string{dir})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings = %v, want none", warns)
	}
	abs, _ := filepath.Abs(dir)
	if !reflect.DeepEqual(got, []string{abs}) {
		t.Fatalf("got %v, want [%q]", got, abs)
	}
}

func TestResolveTargetsParentOfRepos(t *testing.T) {
	root := t.TempDir()
	repoA := filepath.Join(root, "repoA")
	repoB := filepath.Join(root, "repoB")
	mkRepo(t, repoA)
	mkRepo(t, repoB)
	// Add a non-repo sibling — must be ignored.
	if err := os.Mkdir(filepath.Join(root, "not-a-repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, warns, err := resolveTargets([]string{root})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings = %v, want none (non-repo sibling should be silent)", warns)
	}
	absA, _ := filepath.Abs(repoA)
	absB, _ := filepath.Abs(repoB)
	want := []string{absA, absB}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolveTargetsGlobExpansion(t *testing.T) {
	root := t.TempDir()
	repoA := filepath.Join(root, "repoA")
	repoB := filepath.Join(root, "repoB")
	mkRepo(t, repoA)
	mkRepo(t, repoB)
	pattern := filepath.Join(root, "repo?")
	got, warns, err := resolveTargets([]string{pattern})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings = %v, want none", warns)
	}
	absA, _ := filepath.Abs(repoA)
	absB, _ := filepath.Abs(repoB)
	want := []string{absA, absB}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolveTargetsGlobNoMatchEmitsWarning(t *testing.T) {
	root := t.TempDir()
	pattern := filepath.Join(root, "no-such-thing-*")
	got, warns, err := resolveTargets([]string{pattern})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("repos = %v, want empty", got)
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want exactly 1", warns)
	}
	if warns[0].Path != pattern {
		t.Fatalf("warning path = %q, want %q", warns[0].Path, pattern)
	}
}

func TestResolveTargetsRejectsDoubleStar(t *testing.T) {
	_, _, err := resolveTargets([]string{"/some/path/**"})
	if err == nil {
		t.Fatal("err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "**") {
		t.Fatalf("err = %v, want it to mention **", err)
	}
}

func TestResolveTargetsMissingPathEmitsWarning(t *testing.T) {
	got, warns, err := resolveTargets([]string{"/nonexistent/path/here"})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("repos = %v, want empty", got)
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want exactly 1", warns)
	}
}

func TestResolveTargetsFilePathEmitsWarning(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "just-a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, warns, err := resolveTargets([]string{file})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("repos = %v, want empty", got)
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want exactly 1", warns)
	}
}

func TestResolveTargetsParentWithNoReposEmitsWarning(t *testing.T) {
	root := t.TempDir()
	// Empty directory — exists, is a directory, no .git children.
	got, warns, err := resolveTargets([]string{root})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("repos = %v, want empty", got)
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want exactly 1", warns)
	}
}

// --- dedupeAndSort -----------------------------------------------------

func TestDedupeAndSortCollapsesOverlappingSources(t *testing.T) {
	// Two paths that resolve to the same absolute target.
	a := "/tmp/x"
	b := "/tmp/x/."
	out := dedupeAndSort([]string{a, b, a})
	if len(out) != 1 {
		t.Fatalf("got %d entries, want 1: %v", len(out), out)
	}
}

func TestDedupeAndSortProducesAscendingOrder(t *testing.T) {
	got := dedupeAndSort([]string{"/repos/zeta", "/repos/alpha", "/repos/mu"})
	want := []string{"/repos/alpha", "/repos/mu", "/repos/zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDedupeAndSortEmptyInputReturnsNil(t *testing.T) {
	if got := dedupeAndSort(nil); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
	if got := dedupeAndSort([]string{}); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}
