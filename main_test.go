package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAgent records the path each Run was invoked with.
type fakeAgent struct {
	runs  []string
	err   error
	onRun func() error
}

func (f *fakeAgent) Run(_ context.Context, path, _ string) error {
	f.runs = append(f.runs, path)
	if f.onRun != nil {
		if err := f.onRun(); err != nil {
			return err
		}
	}
	return f.err
}



func TestListActiveOpenSpecChanges(t *testing.T) {
	dir := t.TempDir()
	changes := filepath.Join(dir, "openspec", "changes")
	for _, name := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(changes, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// file in changes/ must be ignored.
	if err := os.WriteFile(filepath.Join(changes, "readme.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ListActiveOpenSpecChanges(dir)
	want := map[string]bool{"alpha": true, "beta": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, name := range got {
		if !want[name] {
			t.Fatalf("unexpected change %q in %v", name, got)
		}
	}
}

func TestApplyPromptMentionsChange(t *testing.T) {
	p := applyPrompt("add-foo")
	if !strings.Contains(p, "add-foo") {
		t.Fatalf("prompt should mention change name: %q", p)
	}
}

// Contract: when every attempt errors, retryN returns the LAST error,
// not nil and not the first error. This pins the post-refactor
// contract that the silent-failure scenario (a later attempt returning
// nil while a prior attempt errored) is unreachable.
func TestRetryNReturnsLastErrorWhenAllAttemptsFail(t *testing.T) {
	a := errors.New("a")
	b := errors.New("b")
	c := errors.New("c")
	var calls int
	got := retryN(3, func() error {
		calls++
		switch calls {
		case 1:
			return a
		case 2:
			return b
		default:
			return c
		}
	})
	if got != c {
		t.Fatalf("expected last error (%v), got %v", c, got)
	}
}

// Regression: after work completes without error, the worktree must be committed
// so the change survives a watcher restart. The agent's edits would otherwise
// sit unstaged in the working tree.
func TestWorkCommitsOnSuccess(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	agent := &fakeAgent{
		onRun: func() error {
			if err := os.WriteFile(filepath.Join(repo, "applied.txt"), []byte("done"), 0o644); err != nil {
				return err
			}
			return os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			)
		},
	}
	w := Watcher{agent: agent, RetyCount: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.work(ctx, repo); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", repo, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	gl := strings.TrimSpace(string(out))
	if !strings.Contains(gl, "see: apply openspec change task-1") {
		t.Fatalf("expected commit with change name in log, got:\n%s", gl)
	}
}

// Regression: agent runs must not pollute the original branch directly. They
// run on a dedicated see/<change> branch and the original branch receives a
// --no-ff merge commit afterwards. Pins the post-refactor contract.
func TestWorkIsolatesAgentRunOnBranch(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	// Guarantee HEAD is on a real branch named main. Modern git auto-creates
	// one on first commit; only seed when the init/config didn't.
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	agent := &fakeAgent{
		onRun: func() error {
			if err := os.WriteFile(filepath.Join(repo, "applied.txt"), []byte("done"), 0o644); err != nil {
				return err
			}
			return os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			)
		},
	}
	w := Watcher{agent: agent, RetyCount: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.work(ctx, repo); err != nil {
		t.Fatal(err)
	}

	// (a) HEAD on main has a merge commit with the expected subject.
	logMain, err := exec.Command("git", "-C", repo, "log", "--oneline", "main").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logMain), "see: merge openspec change task-1") {
		t.Fatalf("expected merge commit on main, got:\n%s", logMain)
	}

	// (b) see/<change> deleted after merge-back.
	branches, err := exec.Command("git", "-C", repo, "branch", "--list", "see/task-1").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(branches)) != "" {
		t.Fatalf("expected see/task-1 to be deleted, got:\n%s", branches)
	}

	// (c) working tree on main.
	branch, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(branch)); got != "main" {
		t.Fatalf("working tree on %q, want main", got)
	}

	// (d) agent's apply commit reachable from main (the merge commit's second
	// parent kept it alive after see/<change> was deleted).
	logAll, err := exec.Command("git", "-C", repo, "log", "--all", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logAll), "see: apply openspec change task-1") {
		t.Fatalf("expected apply commit in log --all, got:\n%s", logAll)
	}
}

// Regression: runOnce must pass the repo subdir path to work, not the parent wd.
func TestRunOncePassesRepoPathToAgent(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-q", "-m", "init")
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	// non-repo sibling must be skipped.
	if err := os.Mkdir(filepath.Join(root, "not-a-repo"), 0o755); err != nil {
		t.Fatal(err)
	}

	agent := &fakeAgent{err: nil}
	w := Watcher{agent: agent, RetyCount: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.runOnce(ctx, root); err != nil {
		t.Fatal(err)
	}
	if len(agent.runs) != 1 || agent.runs[0] != repo {
		t.Fatalf("agent.Run called with %v, want exactly [%q]", agent.runs, repo)
	}
}