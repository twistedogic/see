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
	// Guarantee HEAD is on a real branch named main; modern git auto-creates an
	// unborn main on the first commit, older git may land on master.
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
	// After the merge-back, main's tip is the merge commit; the apply commit
	// survives as the merge's second parent and is reachable only via --all.
	out, err := exec.Command("git", "-C", repo, "log", "--all", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	gl := strings.TrimSpace(string(out))
	if !strings.Contains(gl, "see: apply openspec change task-1") {
		t.Fatalf("expected apply commit in log --all, got:\n%s", gl)
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

// Regression: when the agent errors mid-run, the original branch must be left
// untouched and the see/<change> branch must be deleted.
func TestWorkRollsBackBranchOnAgentFailure(t *testing.T) {
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
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	preSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	agentErr := errors.New("agent boom")
	agent := &fakeAgent{
		err: agentErr,
		onRun: func() error {
			// Make a tracked commit on see/<change> so the rollback has to
			// wipe something real. Returns nil so fakeAgent.Run propagates f.err.
			if err := os.WriteFile(filepath.Join(repo, "halfway.txt"), []byte("x"), 0o644); err != nil {
				return err
			}
			add := exec.Command("git", "-C", repo, "add", "halfway.txt")
			if err := add.Run(); err != nil {
				return err
			}
			return exec.Command("git", "-C", repo, "commit", "-q", "-m", "halfway").Run()
		},
	}
	w := Watcher{agent: agent, RetyCount: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = w.work(ctx, repo)
	if !errors.Is(err, agentErr) {
		t.Fatalf("expected agent error %v, got %v", agentErr, err)
	}
	postSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if preSHA != postSHA {
		t.Fatalf("expected HEAD at %s after rollback, got %s", preSHA, postSHA)
	}
	if _, err := os.Stat(filepath.Join(repo, "halfway.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected halfway.txt rolled back, stat err = %v", err)
	}
	branches, err := exec.Command("git", "-C", repo, "branch", "--list", "see/task-1").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(branches)) != "" {
		t.Fatalf("expected see/task-1 to be deleted, got:\n%s", branches)
	}
	branch, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(branch)); got != "main" {
		t.Fatalf("working tree on %q, want main", got)
	}
}

// Regression: a drifted see/<change> branch from a prior partial run must be
// reset to the captured SHA before the agent runs, so old commits don't
// leak into the merge-back.
func TestWorkReusesExistingBranchAndResetsToOriginalSHA(t *testing.T) {
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
	if err := exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", "refs/heads/main").Run(); err != nil {
		run("switch", "-c", "main")
	}
	originalSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "task-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-create see/task-1 with a sentinel commit past the original SHA.
	run("switch", "-c", "see/task-1")
	if err := os.WriteFile(filepath.Join(repo, "sentinel.txt"), []byte("drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "sentinel")
	run("switch", "main")
	agent := &fakeAgent{
		onRun: func() error {
			if err := os.Rename(
				filepath.Join(repo, "openspec", "changes", "task-1"),
				filepath.Join(repo, "openspec", "changes", "archive", "task-1"),
			); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(repo, "applied.txt"), []byte("done"), 0o644)
		},
	}
	w := Watcher{agent: agent, RetyCount: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.work(ctx, repo); err != nil {
		t.Fatal(err)
	}
	// Sentinel wiped by reset --hard before the agent ran.
	if _, err := os.Stat(filepath.Join(repo, "sentinel.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected sentinel.txt gone after reset, stat err = %v", err)
	}
	// Merge commit on main must not carry the sentinel file.
	out, err := exec.Command("git", "-C", repo, "log", "--oneline", "main").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	mergeLine := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	mergeSHA := strings.Fields(mergeLine)[0]
	statOut, err := exec.Command("git", "-C", repo, "show", "--stat", mergeSHA).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(statOut), "sentinel.txt") {
		t.Fatalf("merge commit %s contains sentinel.txt:\n%s", mergeSHA, statOut)
	}
	branches, err := exec.Command("git", "-C", repo, "branch", "--list", "see/task-1").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(branches)) != "" {
		t.Fatalf("expected see/task-1 to be deleted, got:\n%s", branches)
	}
	showMain, err := exec.Command("git", "-C", repo, "log", "--pretty=%H %P", "main", "-1").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(strings.TrimSpace(string(showMain)))
	if len(fields) < 3 {
		t.Fatalf("expected merge commit with 2 parents, got %q", showMain)
	}
	if fields[1] != originalSHA {
		t.Fatalf("merge commit's first parent = %s, want %s", fields[1], originalSHA)
	}
}

// Regression: Watcher.work must refuse to run on a detached HEAD before
// mutating repo state. v1 contract is "bail with an error"; the user must
// switch to a real branch first.
func TestWorkRejectsDetachedHead(t *testing.T) {
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
	detachedSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	run("checkout", detachedSHA)
	branch, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(branch)); got != "HEAD" {
		t.Fatalf("expected HEAD detached, got branch %q", got)
	}
	agent := &fakeAgent{err: nil}
	w := Watcher{agent: agent, RetyCount: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = w.work(ctx, repo)
	if err == nil {
		t.Fatal("expected error from work on detached HEAD, got nil")
	}
	listOut, err := exec.Command("git", "-C", repo, "branch", "--list").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(listOut), "see/") {
		t.Fatalf("expected no see/ branches, got:\n%s", listOut)
	}
	// Bail happens before any commit or branch creation, so HEAD SHA must
	// still match the fixture's pre-detach SHA.
	postSHA, err := GetCurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if postSHA != detachedSHA {
		t.Fatalf("expected HEAD at %s after bail, got %s", detachedSHA, postSHA)
	}
}