package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/twistedogic/see/tui"
)

// defaultPromptTemplate is the per-change prompt `see` hands to `pi`.
// Kept as a checked-in Markdown file so editors reviewing the prompt
// see prose in PRs and the prompt is editable without touching Go.
// `//go:embed` fails the build if prompt.md is missing at the repo
// root alongside main.go, so a deleted default becomes a compile
// error rather than a silent regression.
//
//go:embed prompt.md
var defaultPromptTemplate string

// renderTemplate substitutes every literal token `{change}` in template
// with the active change value. There is no escape syntax and unknown
// tokens remain unchanged.
func renderTemplate(template, change string) string {
	return strings.ReplaceAll(template, "{change}", change)
}

func customChangeDigest(change string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(change)))
}

// workflowIdentityDigest returns the full lowercase SHA-256 digest
// of `name + "\x00" + change`. The null separator prevents two
// pairs like (a, bc) and (ab, c) from colliding. The same name and
// change always produce the same digest across processes and
// polling passes; different names yield different digests even for
// equal change values.
func workflowIdentityDigest(name, change string) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte{0})
	h.Write([]byte(change))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// resolveCustomCondition runs the configured predicate in the platform
// shell. A status of 1 is the normal idle result; successful stdout is
// normalized into the single-line custom change identity.
func resolveCustomCondition(ctx context.Context, path, condition string) (string, error) {
	var shell string
	var args []string
	if runtime.GOOS == "windows" {
		shell, args = "cmd.exe", []string{"/C", condition}
	} else {
		shell, args = "/bin/sh", []string{"-c", condition}
	}

	cmd := exec.CommandContext(ctx, shell, args...)
	configureConditionCommand(cmd)
	cmd.Dir = path
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", nil
		}
		if diagnostic := strings.TrimSpace(stderr.String()); diagnostic != "" {
			return "", fmt.Errorf("see: custom condition failed: %w: %s", err, diagnostic)
		}
		return "", fmt.Errorf("see: custom condition failed: %w", err)
	}

	change := strings.TrimRight(stdout.String(), "\r\n")
	if strings.TrimSpace(change) == "" {
		return "", errors.New("see: custom condition produced an empty change")
	}
	if strings.ContainsAny(change, "\r\n") {
		return "", errors.New("see: custom condition output must be single-line")
	}
	return change, nil
}

type runMode int

const (
	modeUnknown runMode = iota
	modeLog
	modeTUI
)

func selectRunMode(mode string, isTTY bool) (runMode, error) {
	switch mode {
	case "log":
		return modeLog, nil
	case "tui":
		if !isTTY {
			return modeUnknown, errors.New("--mode=tui requires a TTY; rerun with --mode=log")
		}
		return modeTUI, nil
	default:
		return modeUnknown, fmt.Errorf("unknown --mode=%q (want: tui, log)", mode)
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
	binary string
	logDir string
}

// pathFor builds the per-invocation JSONL filename. Pure: no I/O,
// no environment lookup, no fallback. The caller is responsible for
// ensuring LogDir exists.
func pathFor(repo, change string) string {
	return logFilename(filepath.Base(repo) + "--" + change)
}

func (p PiAgent) Run(ctx context.Context, path, change, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, p.binary, "--mode", "json", "--no-session", prompt)
	cmd.Dir = path
	logPath := filepath.Join(p.logDir, pathFor(path, change))
	f, err := os.Create(logPath)
	if err != nil {
		return "", fmt.Errorf("see: log file create failed: %w", err)
	}
	defer f.Close()
	cmd.Stdout = f
	cmd.Stderr = f
	return logPath, cmd.Run()
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
	showRef := exec.Command("git", "-C", path, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	exists := showRef.Run() == nil
	args := []string{"switch", name}
	if !exists {
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

// ensureCustomLane creates or verifies the persistent custom-mode
// lane `see/<digest>` for change on the repo at path. The lane is
// the watcher's workspace while custom mode is active; the watched
// checkout therefore belongs to it.
//
// Behavior:
//   - Dirty working tree: rejected before any branching so a later
//     rollback reset cannot delete operator edits; ignored files do
//     not count toward dirtiness because they would be outside any
//     rollback guarantee.
//   - Lane does not exist: created at the current commit; `created`
//     reports true so the rollback path knows to delete the lane on
//     failure rather than reset it.
//   - Lane exists and HEAD is on it: preserved as-is (no reset);
//     `created` reports false so the rollback path knows to reset to
//     the pre-run tip on failure.
//   - Lane exists but HEAD is on another branch: refused with an
//     actionable error and no mutation, since switching based on a
//     stale condition would mutate the operator's working tree.
//
// The legacy OpenSpec branch path (ensureBranch) is left untouched:
// this helper only governs the custom-mode lane.
func ensureCustomLane(path, change string) (created bool, err error) {
	return ensureWorkflowLane(path, customChangeDigest(change))
}

// ensureWorkflowLane is the workflow-aware variant. The digest is
// supplied by the caller so a workflow can hash its own name plus
// the change while the legacy single-workflow path keeps using
// customChangeDigest for backward compat.
func ensureWorkflowLane(path, digest string) (created bool, err error) {
	branch := "see/" + digest

	// Dirty working tree blocks all three success paths. A clean
	// tree is also the rollback baseline for failed attempts.
	if dirty, derr := hasUntrackedOrModified(path); derr != nil {
		return false, derr
	} else if dirty {
		return false, fmt.Errorf("see: working tree on %s is dirty; commit or stash before see runs", path)
	}

	// Does the lane already exist?
	showRef := exec.Command("git", "-C", path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if showRef.Run() != nil {
		// First run: create the lane at the captured current commit.
		if out, gerr := exec.Command("git", "-C", path, "switch", "-c", branch).CombinedOutput(); gerr != nil {
			return false, fmt.Errorf("see: git switch -c %s on %s: %w\n%s", branch, path, gerr, out)
		}
		return true, nil
	}

	// Lane exists: refuse when the operator has checked out a
	// different branch — switching based on a stale condition would
	// mutate their working tree and could overwrite either branch.
	cur, rerr := originalRef(path)
	if rerr != nil {
		return false, rerr
	}
	if cur != branch {
		return false, fmt.Errorf("see: lane %s exists on branch %q; check it out (or remove/rename the lane) before see runs", branch, cur)
	}
	return false, nil
}

// hasUntrackedOrModified reports whether path has tracked or untracked
// changes; git status excludes ignored files unless explicitly requested.
func hasUntrackedOrModified(path string) (bool, error) {
	out, err := exec.Command("git", "-C", path, "status", "--porcelain").CombinedOutput()
	if err != nil {
		return true, fmt.Errorf("see: git status on %s: %w\n%s", path, err, out)
	}
	return len(out) > 0, nil
}

// rollbackCustomLane restores the clean state captured immediately before a
// custom agent attempt. Existing lanes stay checked out at their prior tip;
// lanes created by this attempt are removed after returning to the source
// branch. git clean intentionally omits -x so ignored files survive.
func (w Watcher) rollbackCustomLane(path, change, ref, commit string, created bool, cause error) error {
	return w.rollbackWorkflowLane(path, change, customChangeDigest(change), ref, commit, created, cause)
}

// rollbackWorkflowLane is the workflow-aware variant: the digest
// identifies the lane to remove, while the human-readable change is
// kept in warning / event messages.
func (w Watcher) rollbackWorkflowLane(path, change, digest, ref, commit string, created bool, cause error) error {
	branch := "see/" + digest
	run := func(label string, args ...string) {
		out, err := exec.Command("git", append([]string{"-C", path}, args...)...).CombinedOutput()
		if err != nil {
			w.warn(path, change, fmt.Sprintf("%s failed: %v\n%s", label, err, out))
		}
	}
	if created {
		run(fmt.Sprintf("switch back to %q", ref), "switch", ref)
	}
	run("reset failed custom attempt", "reset", "--hard", commit)
	run("clean failed custom attempt", "clean", "-fd")
	if created {
		run("delete newly-created lane "+branch, "branch", "-D", branch)
	}
	return cause
}

// catchUpCustomCommit stages all working-tree changes after a
// successful custom agent run and creates a commit with the rendered
// CommitTemplate when the index differs from HEAD. It is a
// successful no-op when the agent committed all work itself or
// made no changes; the empty-staged-diff case skips `git commit`
// entirely so an idempotent run never emits a no-changes Warning.
// `git add` and `git commit` failures are surfaced as Warning
// events so the operator can see why a catch-up commit was missed
// without failing the run. The custom lane is left checked out in
// every case so the next polling pass resumes the same persistent
// branch.
func (w Watcher) catchUpCustomCommit(path, change string) {
	add := exec.Command("git", "-C", path, "add", "-A")
	if out, err := add.CombinedOutput(); err != nil {
		w.warn(path, change, fmt.Sprintf("git add failed: %v\n%s", err, out))
		return
	}
	// ponytail: diff --cached --quiet exits 0 when the index matches
	// HEAD and 1 when it differs. Skipping `git commit` on the
	// empty case keeps an idempotent run warning-free.
	diff := exec.Command("git", "-C", path, "diff", "--cached", "--quiet")
	if err := diff.Run(); err == nil {
		return
	}
	msg := renderTemplate(w.CommitTemplate, change)
	if out, err := exec.Command("git", "-C", path, "commit", "-m", msg).CombinedOutput(); err != nil {
		w.warn(path, change, fmt.Sprintf("git commit failed: %v\n%s", err, out))
	}
}

// runWithRetry invokes workResolved up to w.RetryCount times on repo,
// emitting RetryAttempt events between attempts. Each attempt resolves the
// change again so retries can become idle or select a different lane.
func (w Watcher) runWithRetry(ctx context.Context, repo string) (string, error) {
	lastChange := ""
	var prevErr error
	for attempt := 1; attempt <= w.RetryCount; attempt++ {
		if prevErr != nil && w.observer != nil {
			w.observer.Observe(RetryAttempt{
				Path: repo, Change: lastChange, N: attempt, Max: w.RetryCount,
				Err: prevErr.Error(),
			})
		}

		changeName, err := w.resolveChange(ctx, repo)
		if attempt == 1 && w.observer != nil {
			w.observer.Observe(RepoSeen{Path: repo, HasChange: changeName != ""})
		}
		if err != nil {
			prevErr = err
			continue
		}
		if changeName == "" {
			return "", nil
		}
		lastChange = changeName
		if err := w.workResolved(ctx, repo, changeName); err == nil {
			return lastChange, nil
		} else {
			prevErr = err
		}
	}
	return lastChange, prevErr
}

func (w Watcher) customMode() bool {
	return strings.TrimSpace(w.Condition) != ""
}

func (w Watcher) resolveChange(ctx context.Context, path string) (string, error) {
	if w.customMode() {
		return resolveCustomCondition(ctx, path, w.Condition)
	}
	changes := ListActiveOpenSpecChanges(path)
	if len(changes) == 0 {
		return "", nil
	}
	return changes[0], nil
}

// Event is the watcher→observer interface. Concrete types are below;
// the marker-method pattern is a soft convention, not a hard
// guarantee (Go interfaces are not sealed). The watcher emits one of
// these at each phase boundary and the tui package type-switches on
// the concrete shape.
type Event interface{ isEvent() }

type RepoSeen struct {
	Path      string
	HasChange bool
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
// failed but is not itself the reason the run returns an error.
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

	RetryCount int
	// ponytail: Once mirrors RetryCount — zero-default knob on the watcher,
	// read inside Watch. Once=true makes Watch run a single pass and return.
	Once bool
	// PollInterval is the completion-relative delay Watch waits
	// between successful continuous-mode passes. The first pass is
	// always immediate; a zero value restores the pre-change
	// tight-poll behavior; negative values are rejected at startup
	// before construction. Production callers go through NewWatcher
	// to inherit DefaultPollInterval.
	PollInterval time.Duration
	// PromptTemplate overrides the default prompt body passed to the
	// agent. Empty / whitespace-only is treated as "use the embedded
	// default"; use SetPromptTemplate to apply the normalization
	// rule rather than assigning directly.
	PromptTemplate string
	// Condition selects custom workflow mode when nonblank.
	Condition string
	// CommitTemplate is the catch-up commit message rendered with
	// {change} substitution in custom mode. The startup validator
	// (validateCustomConfig) rejects a blank CommitTemplate whenever
	// Condition is nonblank, so a Watcher in custom mode is expected
	// to carry a nonblank value. Use SetCommitTemplate to apply the
	// trimming rule rather than assigning directly.
	CommitTemplate string
	// Workflows is the ordered multi-workflow configuration. When
	// non-empty, Watcher iterates over each workflow for every
	// repository instead of falling through to the legacy
	// single-workflow path. The combined workflow identity is
	// workflow-scoped (name + change) so different workflows get
	// isolated lanes and log paths even when their normalized
	// change values collide.
	Workflows []WorkflowConfig
	// WorkflowName is set by runOnce when iterating over Workflows
	// so laneDigest / log filenames hash the workflow name into the
	// digest. It is empty in the legacy single-workflow mode and
	// for OpenSpec compatibility, preserving the change-only digest
	// that earlier tests and operators depend on.
	WorkflowName string
}

// DefaultPollInterval is the post-success-pass delay Watch applies in
// continuous mode when constructed via NewWatcher. Exported so tests
// and operators can reference the canonical default without
// hard-coding the literal.
const DefaultPollInterval = 5 * time.Minute

// SetPromptTemplate stores s as the effective prompt template,
// trimming surrounding whitespace and substituting the embedded
// default when the trimmed result is empty.
func (w *Watcher) SetPromptTemplate(s string) {
	if strings.TrimSpace(s) == "" {
		w.PromptTemplate = defaultPromptTemplate
		return
	}
	w.PromptTemplate = s
}

// SetCommitTemplate stores s as the effective custom commit
// message template, trimming surrounding whitespace. A blank
// trimmed value is left as-is; the startup validator guarantees
// nonblank when custom mode is active, and the catch-up helper
// short-circuits on an empty staged diff so a blank template
// cannot surface a "nothing to commit" warning in the common path.
func (w *Watcher) SetCommitTemplate(s string) {
	w.CommitTemplate = strings.TrimSpace(s)
}

// NewWatcher constructs a fully-populated Watcher. PiAgent fields are
// unexported (lowercase) to keep construction hermetic; this is the
// blessed path for building one. Tests that need an Agent without a
// Watcher can reach in via w.agent. The Watcher is seeded with
// DefaultPollInterval so production callers do not busy-poll.
func NewWatcher(binary, logDir string, retry int, once bool) Watcher {
	return Watcher{
		agent:        PiAgent{binary: binary, logDir: logDir},
		RetryCount:   retry,
		Once:         once,
		PollInterval: DefaultPollInterval,
	}
}

// warn emits a Warning event to the observer when one is wired.
// Centralised so cleanup-step call sites read as `w.warn(...)` and
// stay silent in log mode without an observer.
func (w Watcher) warn(path, change, msg string) {
	if w.observer != nil {
		w.observer.Observe(Warning{Path: path, Change: change, Msg: msg})
	}
}

// laneDigest returns the workflow-scoped identity digest for change.
// When WorkflowName is set the digest hashes name + NUL + change so
// equal changes in different workflows get distinct lanes and log
// paths; when WorkflowName is empty (legacy single-workflow mode
// and OpenSpec compatibility) the digest hashes the change alone so
// existing tests and on-disk branches keep working.
func (w Watcher) laneDigest(change string) string {
	if w.WorkflowName != "" {
		return workflowIdentityDigest(w.WorkflowName, change)
	}
	return customChangeDigest(change)
}

func (w Watcher) workResolved(ctx context.Context, path, change string) error {
	current, err := GetCurrentCommit(path)
	if err != nil {
		return err
	}
	ref, err := originalRef(path)
	if err != nil {
		return err
	}
	if ref == "" {
		w.warn(path, change, "detached HEAD; switch to a branch first")
		return fmt.Errorf("detached HEAD on %s", path)
	}

	if w.customMode() {
		digest := w.laneDigest(change)
		created, err := ensureWorkflowLane(path, digest)
		if err != nil {
			return err
		}
		attemptTip, err := GetCurrentCommit(path)
		if err != nil {
			return err
		}
		if w.observer != nil {
			w.observer.Observe(ChangeStarted{Path: path, Change: change})
		}
		template := w.PromptTemplate
		if template == "" {
			template = defaultPromptTemplate
		}
		logPath, runErr := w.agent.Run(ctx, path, digest, renderTemplate(template, change))
		if logPath != "" && w.observer != nil {
			w.observer.Observe(LogPath{Path: logPath, Change: change})
		}
		if runErr != nil {
			return w.rollbackWorkflowLane(path, change, digest, ref, attemptTip, created, runErr)
		}
		w.catchUpCustomCommit(path, change)
		if w.observer != nil {
			w.observer.Observe(ChangeDone{Path: path, Change: change})
		}
		return nil
	}

	branch := "see/" + change
	if err := ensureBranch(path, current, branch); err != nil {
		return err
	}
	if w.observer != nil {
		w.observer.Observe(ChangeStarted{Path: path, Change: change})
	}
	template := w.PromptTemplate
	if template == "" {
		template = defaultPromptTemplate
	}
	logPath, runErr := w.agent.Run(ctx, path, change, renderTemplate(template, change))
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
	return nil
}

func (w Watcher) runOnce(ctx context.Context, repos []string) error {
	for _, repo := range repos {
		select {
		case <-ctx.Done():
			return nil
		default:
			if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
				continue
			}
			if _, err := GetCurrentCommit(repo); err != nil {
				continue
			}
			if len(w.Workflows) > 0 {
				// ponytail: a workflow failure is contained — the lane is
				// rolled back before the error reaches here, so the next
				// workflow can safely run from a clean checkout.
				for _, wf := range w.Workflows {
					if _, err := w.runOneWorkflow(ctx, repo, wf); err != nil && w.observer != nil {
						w.observer.Observe(ChangeFailed{Path: repo, Change: wf.Name, Err: err.Error()})
					}
				}
				continue
			}
			lastChange, err := w.runWithRetry(ctx, repo)
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

// runOneWorkflow runs a single workflow against repo. The
// watcher copy carries the workflow's condition, prompt, commit
// template, and name so the existing runWithRetry path picks up
// the right identity. Retry scope is the workflow, not the
// repository: a retry failure for one workflow cannot block
// later workflows.
func (w Watcher) runOneWorkflow(ctx context.Context, repo string, wf WorkflowConfig) (string, error) {
	child := w
	child.Workflows = nil
	child.Condition = wf.Condition
	child.CommitTemplate = wf.Commit
	child.WorkflowName = wf.Name
	child.SetPromptTemplate(wf.Prompt)
	return child.runWithRetry(ctx, repo)
}

func (w Watcher) Watch(ctx context.Context, repos []string) error {
	if w.Once {
		if err := w.runOnce(ctx, repos); err != nil {
			return err
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := w.runOnce(ctx, repos); err != nil {
			return err
		}
		if w.PollInterval <= 0 {
			continue
		}
		timer := time.NewTimer(w.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func main() {
	var (
		pi         = flag.String("pi", "pi", "path to the pi binary")
		retry      = flag.Int("retry", 3, "retries per repo on failure")
		modeFlag   = flag.String("mode", "tui", "output mode (default \"tui\"); one of: tui, log")
		once       = flag.Bool("once", false, "run one scan and exit")
		configFlag = flag.String("config", "", "path to config.yaml (default: ~/.config/see/config.yaml); pass \"-\" to skip")
		promptFlag = flag.String("prompt", "", "override the agent prompt template; {change} is replaced with the active change name")
		interval   = flag.Duration("interval", DefaultPollInterval, "delay between completed scans in continuous mode; 0 disables the delay, negative values are rejected")
	)
	flag.Parse()

	if *interval < 0 {
		fmt.Fprintf(os.Stderr, "see: --interval must be >= 0 (got %s); pass 0 for immediate polling\n", *interval)
		os.Exit(2)
	}

	mode, err := selectRunMode(*modeFlag, term.IsTerminal(int(os.Stdout.Fd())))
	if err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
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
	w := NewWatcher(*pi, logDir, *retry, *once)
	w.PollInterval = *interval
	defer events.Close()
	w.observer = events
	// ponytail: mirror JSONL to stdout only when stdout is not a TTY
	// — `see --mode=log | jq` and `see --mode=log > log.jsonl` get the
	// stream; an interactive terminal stays silent (the JSONL file under
	// SEE_LOG_DIR is the source of truth in that case).
	// Set the mirror before resolving the watch list so the
	// resolution-layer warnings land in both the file and stdout when
	// stdout is piped.
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		events.SetMirror(os.Stdout)
	}

	// Load configuration once so prompt and watch resolution see the
	// same snapshot. --config=- bypasses both the file read and the
	// configured-prompt path; the embedded default still applies.
	cfg, err := loadStartupConfig(*configFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
		os.Exit(2)
	}
	w.SetPromptTemplate(selectPromptTemplate(*promptFlag, cfg.Prompt))
	w.SetCommitTemplate(cfg.Commit)
	w.Condition = cfg.Condition
	if err := validateCustomConfig(cfg, *promptFlag); err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
		os.Exit(2)
	}

	repos, warnings, err := resolveConfiguredTargets(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
		os.Exit(2)
	}
	for _, warn := range warnings {
		events.Observe(warn)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	if mode == modeTUI {
		if err := runTUI(ctx, &w, events, repos); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
		return
	}
	// mode == modeLog
	if err := w.Watch(ctx, repos); err != nil {
		os.Exit(1)
	}
}

// runTUI runs the watcher with an eventLogger wired as the
// Watcher.observer; the eventLogger fans events out to the bubbletea
// ChanObserver in addition to the batch-level JSONL. The bubbletea
// program owns signal handling; when it exits we cancel the
// watcher's context so the tight poll loop returns. The bubbletea
// program must complete cleanup before this function returns error.
// Otherwise, the terminal state will not be correctly restored due to
// race condition.
func runTUI(ctx context.Context, w *Watcher, events *eventLogger, repos []string) error {
	tCtx, cancel := context.WithCancelCause(ctx)
	prog, obs := tui.New(tCtx)
	events.Attach(tuiObserver{obs: obs})
	w.observer = events
	wg := &sync.WaitGroup{}
	wg.Go(func() {
		_, err := prog.Run()
		if err != nil {
			events.Observe(InfraError{Where: "tui", Err: err.Error()})
		}
		cancel(err)
	})
	wg.Go(func() {
		err := w.Watch(tCtx, repos)
		if err != nil {
			events.Observe(InfraError{Where: "watcher", Err: err.Error()})
		}
		cancel(err)
	})
	wg.Wait()
	return context.Cause(tCtx)
}

// tuiObserver forwards each Event to the bubbletea Program via
// ChanObserver.Send. The type-switch lives here (not in the tui
// package) so the tui package has no dependency on main's Event types.
type tuiObserver struct{ obs *tui.ChanObserver }

func (o tuiObserver) Observe(e Event) {
	switch e := e.(type) {
	case RepoSeen:
		o.obs.Send(tui.RepoSeenMsg{Path: e.Path, HasChange: e.HasChange})
	case ChangeStarted:
		o.obs.Send(tui.ChangeStartedMsg{Path: e.Path, Change: e.Change})
	case RetryAttempt:
		o.obs.Send(tui.RetryAttemptMsg{Path: e.Path, Change: e.Change, N: e.N, Max: e.Max, Err: e.Err})
	case ChangeDone:
		o.obs.Send(tui.ChangeDoneMsg{Path: e.Path, Change: e.Change})
	case ChangeFailed:
		o.obs.Send(tui.ChangeFailedMsg{Path: e.Path, Change: e.Change, Err: e.Err})
	case LogPath:
		o.obs.Send(tui.LogPathMsg{Path: e.Path, Change: e.Change})
	case Warning:
		o.obs.Send(tui.WarningMsg{Path: e.Path, Change: e.Change, Msg: e.Msg})
	case InfraError:
		o.obs.Send(tui.InfraErrorMsg{Where: e.Where, Err: e.Err})
	}
}
