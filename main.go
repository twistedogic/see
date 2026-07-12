package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
)

type Agent interface {
	Run(ctx context.Context, path, prompt string) error
}

type PiAgent struct {
	Binary         string
	RedirectOutput bool
}

func (p PiAgent) Run(ctx context.Context, path, prompt string) error {
	cmd := exec.CommandContext(ctx, p.Binary, "--mode", "json", "--no-session", prompt)
	cmd.Dir = path
	if p.RedirectOutput {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

func applyPrompt(change string) string {
	return fmt.Sprintf(
		"Apply the openspec change %q: read its proposal and tasks, implement them, run the tests, verify, then archive the change. Sync specs if needed.",
		change,
	)
}

func ListActiveOpenSpecChanges(cwd string) []string {
	entries, err := os.ReadDir(filepath.Join(cwd, "openspec", "changes"))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "archive" {
			names = append(names, e.Name())
		}
	}
	return names
}

func GetCurrentCommit(cwd string) (string, error) {
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// originalRef returns the short symbolic-ref for HEAD on the repo at path,
// or empty string when HEAD is detached. `git symbolic-ref --short HEAD`
// exits non-zero with "fatal: not a symbolic ref" when HEAD points directly
// at a commit; that's an expected state, not a watch error.
func originalRef(path string) (string, error) {
	out, err := exec.Command("git", "-C", path, "symbolic-ref", "--short", "HEAD").CombinedOutput()
	if err != nil && strings.Contains(string(out), "not a symbolic ref") {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ensureBranch creates or reuses branch name in the repo at path, then pins
// its tip to sha via reset --hard. Idempotent: a leftover branch from a
// prior partial run is reused, and the reset wipes whatever state it had.
func ensureBranch(path, sha, name string) error {
	list, err := exec.Command("git", "-C", path, "branch", "--list", name).CombinedOutput()
	if err != nil {
		return err
	}
	args := []string{"switch", name}
	if !strings.Contains(string(list), name) {
		args = []string{"switch", "-c", name}
	}
	if out, err := exec.Command("git", append([]string{"-C", path}, args...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("git %v on %s: %v\n%s", args, path, err, out)
	}
	if out, err := exec.Command("git", "-C", path, "reset", "--hard", sha).CombinedOutput(); err != nil {
		return fmt.Errorf("git reset --hard %s on %s: %v\n%s", sha, path, err, out)
	}
	return nil
}

// ponytail: previously took (bool, error); the (false, nil) state could
// clobber a prior error and report silent success. Contract collapsed to
// func() error so the bug class is unreachable.
func retryN(n int, f func() error) error {
	var err error
	for range n {
		if err = f(); err == nil {
			return nil
		}
	}
	return err
}

// Event is a sealed interface; only the five concrete types below may
// implement it. The watcher emits one of these at each phase boundary
// and the tui package type-switches in Update.
type Event interface{ isEvent() }

type RepoSeen struct {
	Path        string
	HasOpenspec bool
}

func (RepoSeen) isEvent() {}

type ChangeStarted struct {
	Path   string
	Change string
}

func (ChangeStarted) isEvent() {}

type RetryAttempt struct {
	Path   string
	Change string
	N, Max int
	Err    string
}

func (RetryAttempt) isEvent() {}

type ChangeDone struct {
	Path   string
	Change string
}

func (ChangeDone) isEvent() {}

type ChangeFailed struct {
	Path   string
	Change string
	Err    string
}

func (ChangeFailed) isEvent() {}

type Observer interface{ Observe(Event) }

type Watcher struct {
	agent Agent

	observer Observer

	RetyCount int
}

func NewWatcher(binary string, n int) Watcher {
	return Watcher{agent: PiAgent{Binary: binary}, RetyCount: n}
}

func (w Watcher) work(ctx context.Context, path string) error {
	current, err := GetCurrentCommit(path)
	if err != nil {
		return err
	}
	ref, err := originalRef(path)
	if err != nil {
		return err
	}
	if ref == "" {
		log.Printf("detached HEAD on %s; switch to a branch first", path)
		return fmt.Errorf("detached HEAD on %s", path)
	}
	changes := ListActiveOpenSpecChanges(path)
	if len(changes) == 0 {
		log.Printf("no active change on %s", path)
		return nil
	}
	change := changes[0]
	log.Printf("working %q on %s", change, path)
	branch := "see/" + change
	if w.observer != nil {
		w.observer.Observe(ChangeStarted{Path: path, Change: change})
	}
	if err := ensureBranch(path, current, branch); err != nil {
		return err
	}
	if err := w.agent.Run(ctx, path, applyPrompt(change)); err != nil {
		log.Printf("failed %q on %s: %v", change, path, err)
		// ponytail: rollback runs every cleanup step regardless of the previous failure so a partial undo doesn't strand the branch.
		if out, rerr := exec.Command("git", "-C", path, "switch", ref).CombinedOutput(); rerr != nil {
			log.Printf("switch back to %q failed: %v\n%s", ref, rerr, out)
		}
		if out, rerr := exec.Command("git", "-C", path, "reset", "--hard", current).CombinedOutput(); rerr != nil {
			log.Printf("reset --hard %s failed: %v\n%s", current, rerr, out)
		}
		if out, rerr := exec.Command("git", "-C", path, "branch", "-D", branch).CombinedOutput(); rerr != nil {
			log.Printf("branch -D %s failed: %v\n%s", branch, rerr, out)
		}
		return err
	}
	done := !slices.Contains(ListActiveOpenSpecChanges(path), change)
	if !done {
		return nil
	}
	log.Printf("completed %q on %s", change, path)
	// ponytail: same inline commit pattern as before — runs even when archive or commit fails so partial progress isn't lost.
	add := exec.Command("git", "-C", path, "add", "-A")
	if err := add.Run(); err != nil {
		log.Printf("git add failed %q on %s: %v", change, path, err)
	}
	msg := fmt.Sprintf("see: apply openspec change %s", change)
	if err := exec.Command("git", "-C", path, "commit", "-m", msg).Run(); err != nil {
		log.Printf("git commit failed %q on %s: %v", change, path, err)
	}
	if w.observer != nil {
		w.observer.Observe(ChangeDone{Path: path, Change: change})
	}
	// ponytail: merge --no-ff so the watcher's involvement shows up as a graph node even on a single-commit see/<change>.
	if out, err := exec.Command("git", "-C", path, "switch", ref).CombinedOutput(); err != nil {
		return fmt.Errorf("switch to %s: %w\n%s", ref, err, out)
	}
	mergeMsg := fmt.Sprintf("see: merge openspec change %s", change)
	if out, err := exec.Command("git", "-C", path, "merge", "--no-ff", branch, "-m", mergeMsg).CombinedOutput(); err != nil {
		log.Printf("merge --no-ff %s failed: %v\n%s", branch, err, out)
		if aout, aerr := exec.Command("git", "-C", path, "merge", "--abort").CombinedOutput(); aerr != nil {
			log.Printf("merge --abort failed: %v\n%s", aerr, aout)
		}
		if rout, rerr := exec.Command("git", "-C", path, "reset", "--hard", current).CombinedOutput(); rerr != nil {
			log.Printf("reset --hard %s failed: %v\n%s", current, rerr, rout)
		}
		if dout, derr := exec.Command("git", "-C", path, "branch", "-D", branch).CombinedOutput(); derr != nil {
			log.Printf("branch -D %s failed: %v\n%s", branch, derr, dout)
		}
		return fmt.Errorf("merge %s: %w", branch, err)
	}
	if out, err := exec.Command("git", "-C", path, "branch", "-d", branch).CombinedOutput(); err != nil {
		log.Printf("branch -d %s failed: %v\n%s", branch, err, out)
	}
	return nil
}

func (w Watcher) runOnce(ctx context.Context, wd string) error {
	entries, err := os.ReadDir(wd)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil
		default:
			if !entry.IsDir() {
				continue
			}
			repo := filepath.Join(wd, entry.Name())
			if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
				continue
			}
			if _, err := GetCurrentCommit(repo); err != nil {
				log.Printf("skipping %s: no commits", repo)
				continue
			}
			if w.observer != nil {
				w.observer.Observe(RepoSeen{Path: repo, HasOpenspec: repoHasOpenspec(repo)})
			}
			var prevErr error
			attempt := 0
			var lastChange string
			err := retryN(w.RetyCount, func() error {
				attempt++
				var changeName string
				if cs := ListActiveOpenSpecChanges(repo); len(cs) > 0 {
					changeName = cs[0]
					lastChange = changeName
				}
				if prevErr != nil && w.observer != nil {
					w.observer.Observe(RetryAttempt{
						Path: repo, Change: changeName, N: attempt, Max: w.RetyCount,
						Err: prevErr.Error(),
					})
				}
				workErr := w.work(ctx, repo)
				prevErr = workErr
				return workErr
			})
			if err != nil {
				if w.observer != nil {
					w.observer.Observe(ChangeFailed{Path: repo, Change: lastChange, Err: err.Error()})
				}
				return fmt.Errorf("%s: %w", repo, err)
			}
		}
	}
	return nil
}

// ponytail: lives here so the observer wiring in runOnce doesn't pull
// repoHasOpenspec through an extra hop. Same logic as ListActiveOpenSpecChanges
// minus the name list — boolean only so the RepoSeen payload stays small.
func repoHasOpenspec(path string) bool {
	entries, err := os.ReadDir(filepath.Join(path, "openspec", "changes"))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != "archive" {
			return true
		}
	}
	return false
}

func (w Watcher) Watch(ctx context.Context, wd string) error {
	// ponytail: tight poll loop, add a backoff sleep when watching large trees.
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if err := w.runOnce(ctx, wd); err != nil {
				return err
			}
		}
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	var (
		pi    = flag.String("pi", "pi", "path to the pi binary")
		retry = flag.Int("retry", 3, "retries per repo on failure")
	)
	flag.Parse()
	w := NewWatcher(*pi, *retry)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	path, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if err := w.Watch(ctx, path); err != nil {
		log.Fatal(err)
	}
}
