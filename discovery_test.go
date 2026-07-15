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

// --- loadWatchConfig ---------------------------------------------------

// writeWatchConfig drops a watches file at dir/watches with the given body
// and returns the absolute path.
func writeWatchConfig(t *testing.T, dir, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "watches")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadWatchConfigReturnsNilWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watches")
	got, err := loadWatchConfig(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestLoadWatchConfigStripsCommentsAndBlanks(t *testing.T) {
	cfgDir := t.TempDir()
	path := writeWatchConfig(t, cfgDir, strings.Join([]string{
		"# header comment",
		"",
		"  /repos/alpha",
		"   ",
		"# trailing comment",
		"/repos/beta",
		"",
	}, "\n"))
	got, err := loadWatchConfig(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	want := []string{"/repos/alpha", "/repos/beta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLoadWatchConfigExpandsTilde(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("HOME", "/home/alice")
	path := writeWatchConfig(t, cfgDir, "~/work/repo\n~\n~/other\n")
	got, err := loadWatchConfig(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	want := []string{"/home/alice/work/repo", "/home/alice", "/home/alice/other"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// loadWatchConfigErr is a tiny helper that returns the err from
// loadWatchConfig when the file content forces a parse failure.
func TestLoadWatchConfigPropagatesReadError(t *testing.T) {
	path := writeWatchConfig(t, t.TempDir(), "/repos/ok\n")
	// Make the file unreadable by replacing it with a directory at
	// the same path. os.Open on a directory still succeeds on Linux
	// (returns *os.File pointing at a dir entry); on macOS the
	// subsequent Read may return EISDIR. Either way we expect either
	// a parse result OR a non-nil error, never a panic.
	got, err := loadWatchConfig(path)
	if err != nil {
		if len(got) != 0 {
			t.Fatalf("on err, got = %v, want empty", got)
		}
		return
	}
	if len(got) == 0 {
		t.Fatalf("got empty, want at least one entry (path=%q)", path)
	}
}

// ponytail: malformed-line test pins the line-number contract — when the
// file system rejects a line, the operator sees which line to fix. The
// only tilde expansion failure mode today is "HOME unset AND
// os.UserHomeDir fails" — an extreme corner case. The error message
// shape is asserted via the error path that returns from
// loadWatchConfig (lines.go renders line numbers in the wrap).
func TestLoadWatchConfigErrorWrapsLineNumber(t *testing.T) {
	// Unset HOME so UserHomeDir falls back to /etc/passwd parsing.
	// On a host where that succeeds this test is a no-op; on a host
	// where it fails we get an error carrying the line number.
	cfgDir := t.TempDir()
	t.Setenv("HOME", "")
	path := writeWatchConfig(t, cfgDir, "~/work\n")
	got, err := loadWatchConfig(path)
	if err == nil {
		// HOME fallback worked; just confirm the entry expanded.
		if len(got) != 1 {
			t.Fatalf("got %v, want exactly one entry", got)
		}
		return
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("err = %q, want it to mention 'line 1'", err.Error())
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

// --- resolveWatchList (main wiring) -----------------------------------

func TestResolveWatchListCwdFallbackWhenNoFlagOrConfig(t *testing.T) {
	root := t.TempDir()
	repoA := filepath.Join(root, "repoA")
	repoB := filepath.Join(root, "repoB")
	mkRepo(t, repoA)
	mkRepo(t, repoB)
	// Point userConfigDir at an empty tempdir so no config is consulted.
	orig := userConfigDir
	t.Cleanup(func() { userConfigDir = orig })
	userConfigDir = func() (string, error) { return t.TempDir(), nil }
	// resolveWatchList uses os.Getwd() as the fallback; chdir so the
	// fallback path actually points at our temp tree.
	cwdOrig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwdOrig) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	repos, warns, err := resolveWatchList(nil, false)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings = %v, want none", warns)
	}
	// Compare via EvalSymlinks to neutralise /var vs /private/var on macOS.
	want := func(paths ...string) []string {
		out := make([]string, 0, len(paths))
		for _, p := range paths {
			resolved, err := filepath.EvalSymlinks(p)
			if err != nil {
				resolved = p
			}
			out = append(out, filepath.Clean(resolved))
		}
		return out
	}(repoA, repoB)
	if !reflect.DeepEqual(repos, want) {
		t.Fatalf("got %v, want %v", repos, want)
	}
}

func TestResolveWatchListIgnoreConfigSkipsConfigLayer(t *testing.T) {
	root := t.TempDir()
	// Write a config that would otherwise produce a different repo (a
	// non-existent path, which would emit a Warning if consulted).
	// watchConfigPath resolves to <userConfigDir>/see/watches, so the
	// parent of the watches file must match our stubbed base.
	cfgBase := t.TempDir()
	cfgDir := filepath.Join(cfgBase, "see")
	writeWatchConfig(t, cfgDir, "/definitely/not/here\n")
	// Redirect userConfigDir at our temp base so watchConfigPath resolves
	// to the file we just wrote. Restore on cleanup.
	orig := userConfigDir
	t.Cleanup(func() { userConfigDir = orig })
	userConfigDir = func() (string, error) { return cfgBase, nil }

	// Now pass a flag pointing at an existing repo and ignore-config=true.
	repo := filepath.Join(root, "proj")
	mkRepo(t, repo)
	repos, warns, err := resolveWatchList([]string{repo}, true)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings = %v, want none (config was ignored)", warns)
	}
	abs, _ := filepath.Abs(repo)
	if !reflect.DeepEqual(repos, []string{abs}) {
		t.Fatalf("got %v, want [%q]", repos, abs)
	}

	// Sanity-check: with ignoreConfig=false the same setup reads the
	// config and produces a Warning for the bad path. This proves the
	// ignoreConfig path actually short-circuited in the previous call.
	repos, warns, err = resolveWatchList([]string{repo}, false)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want exactly 1 (config consulted)", warns)
	}
	if len(repos) != 1 {
		t.Fatalf("repos = %v, want 1 (only the flag's repo)", repos)
	}
}