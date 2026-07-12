package main

import (
	"context"
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
	done, err := w.work(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("expected done=true")
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