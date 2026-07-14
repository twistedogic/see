package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/twistedogic/see/tui"
)

type runMode int

const (
	modeUnknown runMode = iota
	modeLog
	modeTUI
)

func selectRunMode(mode string, isTTY bool) (runMode, string) {
	switch mode {
	case "log":
		return modeLog, ""
	case "tui":
		if !isTTY {
			return modeUnknown, "see: --mode=tui requires a TTY; rerun with --mode=log"
		}
		return modeTUI, ""
	default:
		return modeUnknown, fmt.Sprintf("see: unknown --mode=%q (want: tui, log)", mode)
	}
}

// Agent.Run returns the log path (empty only when capture failed
// before the agent was invoked) so the caller can surface it via
// the LogPath event. After the silent-tui change, a non-empty
// logPath is the rule rather than the exception — the log directory
// is validated up front by ensureLogDir.
type Agent interface {
	Run(ctx context.Context, path, change, prompt string) (string, error)
}

type PiAgent struct {
	Binary string
	LogDir string
}

// pathFor builds the per-invocation JSONL filename. Pure: no I/O,
// no environment lookup, no fallback. The caller is responsible for
// ensuring LogDir exists.
func pathFor(repo, change string) string {
	return fmt.Sprintf("%s--%s--%s--%d.jsonl",
		filepath.Base(repo),
		change,
		time.Now().UTC().Format("20060102T150405"),
		os.Getpid(),
	)
}

func (p PiAgent) Run(ctx context.Context, path, change, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, p.Binary, "--mode", "json", "--no-session", prompt)
	cmd.Dir = path
	logPath := filepath.Join(p.LogDir, pathFor(path, change))
	f, err := os.Create(logPath)
	if err != nil {
		return "", fmt.Errorf("see: log file create failed: %w", err)
	}
	defer f.Close()
	cmd.Stdout = f
	cmd.Stderr = f
	return logPath, cmd.Run()
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

type LogPath struct {
	Path   string
	Change string
}

func (LogPath) isEvent() {}

// Warning reports a per-repo cleanup or pre-run check step that
// failed but is not itself the reason Watcher.work returns an error.
// The Msg field carries the human-readable detail; the JSONL is the
// source of truth for the message text.
type Warning struct {
	Path   string
	Change string
	Msg    string
}

func (Warning) isEvent() {}

// InfraError reports a process-level failure: the watcher goroutine
// returned an error (Where == "watcher") or the bubbletea program
// returned an error from Run (Where == "tui"). It is emitted before
// the process exits with a non-zero status.
type InfraError struct {
	Where string
	Err   string
}

func (InfraError) isEvent() {}

type Observer interface{ Observe(Event) }

type Watcher struct {
	agent Agent

	observer Observer

	RetyCount int
	// ponytail: Once mirrors RetyCount — zero-default knob on the watcher,
	// read inside Watch. Once=true makes Watch run a single pass and return.
	Once bool
}

func NewWatcher(binary string, n int) Watcher {
	return Watcher{agent: PiAgent{Binary: binary}, RetyCount: n}
}

// warn emits a Warning event to the observer when one is wired.
// Centralised so cleanup-step call sites read as `w.warn(...)` and
// stay silent in log mode without an observer (mirrors the
// observer-nil pattern used elsewhere in Watcher.work).
func (w Watcher) warn(path, change, msg string) {
	if w.observer != nil {
		w.observer.Observe(Warning{Path: path, Change: change, Msg: msg})
	}
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
		w.warn(path, "", "detached HEAD; switch to a branch first")
		return fmt.Errorf("detached HEAD on %s", path)
	}
	changes := ListActiveOpenSpecChanges(path)
	if len(changes) == 0 {
		return nil
	}
	change := changes[0]
	branch := "see/" + change
	if w.observer != nil {
		w.observer.Observe(ChangeStarted{Path: path, Change: change})
	}
	if err := ensureBranch(path, current, branch); err != nil {
		return err
	}
	logPath, runErr := w.agent.Run(ctx, path, change, applyPrompt(change))
	if logPath != "" && w.observer != nil {
		w.observer.Observe(LogPath{Path: logPath, Change: change})
	}
	if runErr != nil {
		// ponytail: rollback runs every cleanup step regardless of the previous failure so a partial undo doesn't strand the branch.
		if out, rerr := exec.Command("git", "-C", path, "switch", ref).CombinedOutput(); rerr != nil {
			w.warn(path, change, fmt.Sprintf("switch back to %q failed: %v\n%s", ref, rerr, out))
		}
		if out, rerr := exec.Command("git", "-C", path, "reset", "--hard", current).CombinedOutput(); rerr != nil {
			w.warn(path, change, fmt.Sprintf("reset --hard %s failed: %v\n%s", current, rerr, out))
		}
		if out, rerr := exec.Command("git", "-C", path, "branch", "-D", branch).CombinedOutput(); rerr != nil {
			w.warn(path, change, fmt.Sprintf("branch -D %s failed: %v\n%s", branch, rerr, out))
		}
		return runErr
	}
	done := !slices.Contains(ListActiveOpenSpecChanges(path), change)
	if !done {
		return nil
	}
	// ponytail: same inline commit pattern as before — runs even when archive or commit fails so partial progress isn't lost.
	add := exec.Command("git", "-C", path, "add", "-A")
	if err := add.Run(); err != nil {
		w.warn(path, change, fmt.Sprintf("git add failed: %v", err))
	}
	msg := fmt.Sprintf("see: apply openspec change %s", change)
	if err := exec.Command("git", "-C", path, "commit", "-m", msg).Run(); err != nil {
		w.warn(path, change, fmt.Sprintf("git commit failed: %v", err))
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
		w.warn(path, change, fmt.Sprintf("merge --no-ff %s failed: %v\n%s", branch, err, out))
		if aout, aerr := exec.Command("git", "-C", path, "merge", "--abort").CombinedOutput(); aerr != nil {
			w.warn(path, change, fmt.Sprintf("merge --abort failed: %v\n%s", aerr, aout))
		}
		if rout, rerr := exec.Command("git", "-C", path, "reset", "--hard", current).CombinedOutput(); rerr != nil {
			w.warn(path, change, fmt.Sprintf("reset --hard %s failed: %v\n%s", current, rerr, rout))
		}
		if dout, derr := exec.Command("git", "-C", path, "branch", "-D", branch).CombinedOutput(); derr != nil {
			w.warn(path, change, fmt.Sprintf("branch -D %s failed: %v\n%s", branch, derr, dout))
		}
		return fmt.Errorf("merge %s: %w", branch, err)
	}
	if out, err := exec.Command("git", "-C", path, "branch", "-d", branch).CombinedOutput(); err != nil {
		w.warn(path, change, fmt.Sprintf("branch -d %s failed: %v\n%s", branch, err, out))
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
	if w.Once {
		if err := w.runOnce(ctx, wd); err != nil {
			return err
		}
		return nil
	}
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
	var (
		pi       = flag.String("pi", "pi", "path to the pi binary")
		retry    = flag.Int("retry", 3, "retries per repo on failure")
		modeFlag = flag.String("mode", "tui", "output mode (default \"tui\"); one of: tui, log")
		once     = flag.Bool("once", false, "run one scan and exit")
	)
	flag.Parse()
	w := NewWatcher(*pi, *retry)
	w.Once = *once
	path, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
		os.Exit(1)
	}

	mode, msg := selectRunMode(*modeFlag, term.IsTerminal(int(os.Stdout.Fd())))
	if mode == modeUnknown {
		fmt.Fprintln(os.Stderr, msg)
		flag.Usage()
		os.Exit(2)
	}

	logDir, err := ensureLogDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
		os.Exit(2)
	}
	events, err := openEventLogger(logDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
		os.Exit(2)
	}
	defer events.Close()

	w.agent = PiAgent{Binary: *pi, LogDir: logDir}
	if mode == modeTUI {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := runTUI(ctx, &w, events, path); err != nil {
			os.Exit(1)
		}
		return
	}
	// mode == modeLog
	w.observer = events
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	if err := w.Watch(ctx, path); err != nil {
		os.Exit(1)
	}
}

// runTUI runs the watcher with an eventLogger wired as the
// Watcher.observer; the eventLogger fans events out to the bubbletea
// ChanObserver in addition to the batch-level JSONL. The bubbletea
// program owns signal handling; when it exits we cancel the
// watcher's context so the tight poll loop returns.
func runTUI(ctx context.Context, w *Watcher, events *eventLogger, wd string) error {
	prog, obs := tui.New()
	events.Attach(tuiObserver{obs: obs})
	w.observer = events
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	watchErr := make(chan error, 1)
	go func() { watchErr <- w.Watch(ctx, wd) }()
	// ponytail: watcher-exit → prog.Quit() wiring for --once. In loop mode the
	// watcher only exits after cancel() runs (which fires after prog.Run()
	// returns), so Quit() on an already-exited program is a no-op.
	go func() {
		if err := <-watchErr; err != nil {
			events.Observe(InfraError{Where: "watcher", Err: err.Error()})
		}
		prog.Quit()
	}()
	_, runErr := prog.Run()
	// Program.Run() returned; cancel to break the watcher out of its
	// poll loop if it's still running, then wait for it.
	cancel()
	if werr := <-watchErr; werr != nil && runErr == nil {
		runErr = werr
	}
	if runErr != nil {
		events.Observe(InfraError{Where: "tui", Err: runErr.Error()})
	}
	return runErr
}

// tuiObserver implements main.Observer by translating each Event into
// a typed method call on tui.ChanObserver, which sends a bubbletea
// message to the running Program. The type-switch lives here (not in
// the tui package) so the tui package has no dependency on main's
// Event types.
type tuiObserver struct{ obs *tui.ChanObserver }

func (o tuiObserver) Observe(e Event) {
	switch e := e.(type) {
	case RepoSeen:
		o.obs.RepoSeen(e.Path, e.HasOpenspec)
	case ChangeStarted:
		o.obs.ChangeStarted(e.Path, e.Change)
	case RetryAttempt:
		o.obs.RetryAttempt(e.Path, e.Change, e.N, e.Max, e.Err)
	case ChangeDone:
		o.obs.ChangeDone(e.Path, e.Change)
	case ChangeFailed:
		o.obs.ChangeFailed(e.Path, e.Change, e.Err)
	case LogPath:
		o.obs.LogPath(e.Path, e.Change)
	case Warning:
		o.obs.Warning(e.Path, e.Change, e.Msg)
	case InfraError:
		o.obs.InfraError(e.Where, e.Err)
	}
}
