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

	tea "github.com/charmbracelet/bubbletea"
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
// is validated up front by resolveLogDir.
type Agent interface {
	Run(ctx context.Context, path, change, prompt, model string) (string, error)
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

func (p PiAgent) Run(ctx context.Context, path, change, prompt, model string) (string, error) {
	args := []string{"--mode", "json", "--no-session"}
	if model = strings.TrimSpace(model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	cmd := exec.CommandContext(ctx, p.binary, args...)
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
//   - Lane exists and HEAD is on a clean branch or lane: switched
//     via `git switch`, preserving prior commits. The dirty-tree
//     guard at the top of ensureWorkflowLane is the only safety
//     check on this transition.
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
	// tree is also the rollback baseline for failed attempts. The
	// guard exists so a downstream `git switch` cannot lose tracked
	// or non-ignored untracked edits the operator has staged.
	if dirty, derr := hasUntrackedOrModified(path); derr != nil {
		return false, derr
	} else if dirty {
		return false, &dirtyWorkingTreeError{path: path}
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

	// Lane exists: switch to it. The dirty-tree guard above is
	// the only safety check; a clean working tree means there is
	// nothing to lose by leaving the current branch. `git switch`
	// without `--hard` keeps the lane's existing commits intact.
	if out, gerr := exec.Command("git", "-C", path, "switch", branch).CombinedOutput(); gerr != nil {
		return false, fmt.Errorf("see: git switch %s on %s: %w\n%s", branch, path, gerr, out)
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
	step := func(label string, args ...string) { w.gitStep(path, change, label, args...) }
	if created {
		step(fmt.Sprintf("switch back to %q", ref), "switch", ref)
	}
	step("reset failed custom attempt", "reset", "--hard", commit)
	step("clean failed custom attempt", "clean", "-fd")
	if created {
		step("delete newly-created lane "+branch, "branch", "-D", branch)
	}
	return cause
}

// catchUpCustomCommit stages all working-tree changes after a
// successful custom agent run and creates a commit with the rendered
// CommitTemplate when the index differs from HEAD. It is a
// successful no-op when the agent committed all work itself or
// made no changes; the empty-staged-diff case skips `git commit`
// entirely so an idempotent run never emits a no-changes Warning.
// `git add` and `git commit` failures return an error wrapped with
// the underlying git output; the caller surfaces the same detail
// as a Warning event before triggering rollback so the next
// workflow can run on a clean checkout. The custom lane is left
// checked out when no error is returned so the next polling pass
// resumes the same persistent branch.
func (w Watcher) catchUpCustomCommit(path, change string) error {
	return stageAndCommitIfDirty(path, w.catchUpMessage(change))
}

// catchUpMessage returns the catch-up commit message for change in
// the active mode: the rendered CommitTemplate in custom mode (where
// a workflow supplies its own commit subject) and the OpenSpec
// compatibility subject otherwise. Worktree mode reuses this so both
// modes share one message contract.
func (w Watcher) catchUpMessage(change string) string {
	if w.customMode() {
		return renderTemplate(w.CommitTemplate, change)
	}
	return fmt.Sprintf("see: apply openspec change %s", change)
}

// stageAndCommitIfDirty stages all changes under cwd and commits them
// with message when the staged index differs from HEAD. A matching
// index is a warning-free no-op. Used by both branch-mode catch-up
// (catchUpCustomCommit) and worktree-mode catch-up
// (rebaseWorktreeLane / mergeWorktreeLane).
func stageAndCommitIfDirty(cwd, message string) error {
	add := exec.Command("git", "-C", cwd, "add", "-A")
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("see: git add failed: %w\n%s", err, out)
	}
	// ponytail: diff --cached --quiet exits 0 when the index matches
	// HEAD and 1 when it differs. Skipping `git commit` on the empty
	// case keeps an idempotent run warning-free.
	diff := exec.Command("git", "-C", cwd, "diff", "--cached", "--quiet")
	if err := diff.Run(); err == nil {
		return nil
	}
	if out, err := exec.Command("git", "-C", cwd, "commit", "-m", message).CombinedOutput(); err != nil {
		return fmt.Errorf("see: git commit failed: %w\n%s", err, out)
	}
	return nil
}

// ensureWorktree creates or reuses the worktree-mode lane for digest
// linked to the repo at repoPath, at the directory
// <worktreeRoot>/<repo-basename>--<digest>. It prunes stale worktree
// metadata first, clears an orphan directory at the target path, and
// then runs `git worktree add --force -B see/<digest> <path> <start>`.
// The start point is the existing lane tip when the lane branch
// already exists (preserving prior commits) and the operator's HEAD
// otherwise. created reports whether the lane branch was newly
// created by this call. The worktree path is returned so the caller
// can invoke the agent and drive merge / rollback against it.
func ensureWorktree(repoPath, digest, worktreeRoot string) (created bool, worktreePath string, err error) {
	branch := "see/" + digest
	worktreePath = filepath.Join(worktreeRoot, filepath.Base(repoPath)+"--"+digest)
	if out, e := exec.Command("git", "-C", repoPath, "worktree", "prune").CombinedOutput(); e != nil {
		return false, "", fmt.Errorf("see: git worktree prune on %s: %w\n%s", repoPath, e, out)
	}
	showRef := exec.Command("git", "-C", repoPath, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	branchExists := showRef.Run() == nil
	created = !branchExists
	start := "HEAD"
	if branchExists {
		start = branch
	}
	// An orphan directory at the target path (e.g. left by a crash or a
	// manual rm) is not a registered worktree; git would refuse to add
	// over it. Remove it so a fresh worktree can be created. A
	// registered-but-stale worktree is reused via --force below.
	if info, statErr := os.Stat(worktreePath); statErr == nil && info.IsDir() {
		if !isRegisteredWorktree(repoPath, worktreePath) {
			if rmErr := os.RemoveAll(worktreePath); rmErr != nil {
				return false, "", fmt.Errorf("see: clear stale worktree dir %s: %w", worktreePath, rmErr)
			}
		}
	}
	if err := os.MkdirAll(worktreeRoot, 0o755); err != nil {
		return false, "", fmt.Errorf("see: create worktree root %s: %w", worktreeRoot, err)
	}
	args := []string{"-C", repoPath, "worktree", "add", "--force", "-B", branch, worktreePath, start}
	if out, e := exec.Command("git", args...).CombinedOutput(); e != nil {
		return false, "", fmt.Errorf("see: git worktree add on %s: %w\n%s", repoPath, e, out)
	}
	return created, worktreePath, nil
}

// isRegisteredWorktree reports whether worktreePath is listed by
// `git worktree list` for the repo at repoPath. Used by
// ensureWorktree to distinguish a registered (reusable) worktree from
// an orphan directory.
func isRegisteredWorktree(repoPath, worktreePath string) bool {
	out, err := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain").CombinedOutput()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			if strings.TrimSpace(strings.TrimPrefix(line, "worktree ")) == worktreePath {
				return true
			}
		}
	}
	return false
}

// rollbackWorktree cleans up a failed worktree-mode attempt and
// returns the original cause unchanged. The cleanup runs every step
// regardless of which step failed: abort any in-progress rebase (the
// rebase state lives in the worktree) and merge (state lives in the
// operator's checkout), remove the worktree directory, and delete
// see/<digest> with -D — the lane must not survive a failed attempt
// in worktree mode. The aborts are run silently because "no rebase /
// merge in progress" is the common, expected case, not a cleanup
// failure worth a Warning; the remove and delete emit a Warning on
// failure so a partial cleanup is visible.
func (w Watcher) rollbackWorktree(repoPath, digest, worktreePath string, cause error) error {
	branch := "see/" + digest
	if worktreePath != "" {
		exec.Command("git", "-C", worktreePath, "rebase", "--abort").Run()
	}
	exec.Command("git", "-C", repoPath, "merge", "--abort").Run()
	step := func(label string, args ...string) { w.gitStep(repoPath, digest, label, args...) }
	step("worktree remove "+worktreePath, "worktree", "remove", "--force", worktreePath)
	step("delete lane "+branch, "branch", "-D", branch)
	return cause
}

// rebaseWorktreeLane runs the worktree-mode catch-up commit and
// rebases see/<digest> onto operatorRef (the operator's current
// branch name, resolved to its tip at rebase time). It is the
// manual-merge success path: the lane and worktree are left in place
// for review. A rebase conflict aborts the rebase and returns the
// error; the caller triggers rollbackWorktree.
func (w Watcher) rebaseWorktreeLane(worktreePath, operatorRef, change string) error {
	if err := stageAndCommitIfDirty(worktreePath, w.catchUpMessage(change)); err != nil {
		return err
	}
	if out, err := exec.Command("git", "-C", worktreePath, "rebase", operatorRef).CombinedOutput(); err != nil {
		exec.Command("git", "-C", worktreePath, "rebase", "--abort").Run()
		return fmt.Errorf("see: rebase %s in %s failed: %w\n%s", operatorRef, worktreePath, err, out)
	}
	return nil
}

// mergeWorktreeLane is the worktree + auto-merge success path: it
// rebases see/<digest> onto operatorRef (the operator's current branch
// name, resolved to its tip at rebase time) and then fast-forward
// merges the lane into the operator's branch via ffMergeLane. A rebase
// conflict returns an error (after aborting the rebase) so the caller
// can run rollbackWorktree.
func (w Watcher) mergeWorktreeLane(repoPath, operatorRef, worktreePath, digest, change string) error {
	if err := w.rebaseWorktreeLane(worktreePath, operatorRef, change); err != nil {
		return err
	}
	return w.ffMergeLane(repoPath, worktreePath, digest, change)
}

// ffMergeLane runs the worktree + auto-merge tail: re-check the
// operator's checkout is clean, fast-forward merge see/<digest> into
// the operator's branch at repoPath, then remove the worktree and
// delete the lane. A dirty operator tree at merge time, or a
// fast-forward failure (the operator committed between rebase and
// merge), returns an error after aborting the in-progress merge so the
// caller can run rollbackWorktree. Splitting the post-rebase steps
// out makes the dirty-merge-time and fast-forward-failure rollback
// paths deterministically testable without racing a mid-run commit.
func (w Watcher) ffMergeLane(repoPath, worktreePath, digest, change string) error {
	branch := "see/" + digest
	if dirty, _ := hasUntrackedOrModified(repoPath); dirty {
		return fmt.Errorf("see: working tree on %s is dirty; commit or stash before merge runs", repoPath)
	}
	if out, err := exec.Command("git", "-C", repoPath, "merge", "--ff-only", branch).CombinedOutput(); err != nil {
		exec.Command("git", "-C", repoPath, "merge", "--abort").Run()
		return fmt.Errorf("see: merge --ff-only %s on %s failed: %w\n%s", branch, repoPath, err, out)
	}
	step := func(label string, args ...string) { w.gitStep(repoPath, change, label, args...) }
	step("worktree remove "+worktreePath, "worktree", "remove", "--force", worktreePath)
	step("delete lane "+branch, "branch", "-d", branch)
	return nil
}

// runWithRetry invokes workResolved up to w.RetryCount times on repo,
// emitting RetryAttempt events between attempts. Each attempt resolves the
// change again so retries can become idle or select a different lane.
func (w Watcher) runWithRetry(ctx context.Context, repo string) (string, error) {
	lastChange := ""
	var prevErr error
	for attempt := 1; attempt <= w.RetryCount; attempt++ {
		if prevErr != nil && w.observer != nil {
			errStr := prevErr.Error()
			w.observer.Observe(RetryAttempt{
				Path: repo, Workflow: w.WorkflowName, Change: lastChange, N: attempt, Max: w.RetryCount,
				Err: errStr, summary: summaryFor(prevErr),
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
	Path string
	// Workflow is the configured workflow name when this event
	// originates from a `workflows` entry. Empty in OpenSpec
	// compatibility mode and in the legacy single-workflow
	// path; nonblank when the watcher iterates the ordered
	// workflow sequence so JSONL consumers can disambiguate
	// equal human-readable change values from different
	// workflows.
	Workflow string
	Change   string
}

func (ChangeStarted) isEvent() {}

type RetryAttempt struct {
	Path     string
	Workflow string
	Change   string
	N, Max   int
	Err      string
	// summary carries the concise Summary() text when the source
	// error implements it; empty otherwise. Unexported so the
	// batch JavaScript Object Notation Lines (JSONL) schema is
	// unchanged and the in-memory TUI adapter can pick the right
	// text without leaking a presentation-only duplicate of Err.
	summary string
}

func (RetryAttempt) isEvent() {}

type ChangeDone struct {
	Path     string
	Workflow string
	Change   string
}

func (ChangeDone) isEvent() {}

type ChangeFailed struct {
	Path     string
	Workflow string
	Change   string
	Err      string
	// summary: see RetryAttempt.summary.
	summary string
}

func (ChangeFailed) isEvent() {}

type LogPath struct {
	Path     string
	Workflow string
	Change   string
}

func (LogPath) isEvent() {}

// Warning reports a per-repo cleanup or pre-run check step that
// failed but is not itself the reason the run returns an error.
// The Msg field carries the human-readable detail; the JSONL is the
// source of truth for the message text.
type Warning struct {
	Path     string
	Workflow string
	Change   string
	Msg      string
}

func (Warning) isEvent() {}

// InfraError reports a process-level failure: the watcher goroutine
// returned an error (Where == "watcher") or the bubbletea program
// returned an error from Run (Where == "tui"). It is emitted before
// the process exits with a non-zero status.
type InfraError struct {
	Where string
	Err   string
	// summary: see RetryAttempt.summary.
	summary string
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
	// Model is the optional per-workflow model selector. It is populated
	// by runOneWorkflow for the duration of one workflow run and is empty
	// in OpenSpec-compatibility mode.
	Model string
	// Condition selects custom workflow mode when nonblank.
	Condition string
	// CommitTemplate is the catch-up commit message rendered with
	// {change} substitution in custom mode. The startup workflow
	// validator (validateWorkflows) rejects a blank Commit whenever a
	// workflows entry is configured, so a Watcher driving any
	// workflow is expected to carry a nonblank CommitTemplate. Use
	// SetCommitTemplate to apply the trimming rule rather than
	// assigning directly.
	CommitTemplate string
	// Workflows is the ordered multi-workflow configuration. When
	// non-empty, Watcher iterates over each workflow for every
	// repository instead of falling through to the legacy
	// single-workflow path. The combined workflow identity is
	// workflow-scoped (name + change) so different workflows get
	// isolated lanes and log paths even when their normalized
	// change values collide.
	Workflows []WorkflowConfig
	// Worktree selects worktree lane isolation. When true, the agent
	// runs in a git worktree linked to the operator's checkout and
	// the operator's checkout is never switched. When false (the
	// default), the existing branch-mode path applies unchanged.
	Worktree bool
	// AutoMerge governs merge-back in worktree mode. When true (the
	// default), a successful run rebases the lane onto the operator's
	// branch tip and fast-forward merges it. When false, the rebased
	// lane is left for manual review. Ignored in branch mode.
	AutoMerge bool
	// WorktreeRoot is the parent directory for new worktrees in
	// worktree mode, already tilde-expanded and defaulted to
	// ~/.cache/see/worktrees by main(). Ignored in branch mode.
	WorktreeRoot string
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

// defaultWorktreeRoot is the parent directory for new worktrees when
// worktree mode is active and no --worktree-root / worktree_root is
// configured. It lives outside any plausible root_dir so the
// discovery layer never double-watches it. Expressed with a leading
// ~ and tilde-expanded at resolution time, mirroring root_dir.
const defaultWorktreeRoot = "~/.cache/see/worktrees"

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
// stay silent in log mode without an observer. The Workflow field
// carries the active workflow name when the warning originates
// from a workflow-mode run; the OpenSpec-compat and legacy
// single-workflow paths leave it blank.
func (w Watcher) warn(path, change, msg string) {
	if w.observer != nil {
		w.observer.Observe(Warning{Path: path, Workflow: w.WorkflowName, Change: change, Msg: msg})
	}
}

// gitStep runs git in repoPath and emits a Warning (keyed by change)
// when it fails. Shared by the cleanup/rollback paths that must run
// every step regardless of which one failed; a failed step must not
// abort the remaining cleanup.
func (w Watcher) gitStep(repoPath, change, label string, args ...string) {
	out, e := exec.Command("git", append([]string{"-C", repoPath}, args...)...).CombinedOutput()
	if e != nil {
		w.warn(repoPath, change, fmt.Sprintf("%s failed: %v\n%s", label, e, out))
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

// workResolvedWorktree drives one worktree-mode attempt. The operator's
// checkout is never switched: the agent runs in a git worktree linked to
// it. On success the lane is rebased onto the operator's current branch
// tip; when w.AutoMerge is true it is then fast-forward merged and the
// lane + worktree are removed, otherwise they are left for manual review.
// Every failure path (dirty tree, worktree creation, agent error, rebase
// conflict, merge failure) routes through rollbackWorktree, which removes
// the worktree and deletes the lane. The pre-attempt dirty-tree check is
// defense in depth against writeful condition commands; the worktree
// itself is built from a commit, not the working tree.
func (w Watcher) workResolvedWorktree(ctx context.Context, path, change, ref string) error {
	digest := w.laneDigest(change)
	if dirty, err := hasUntrackedOrModified(path); err != nil {
		return err
	} else if dirty {
		return &dirtyWorkingTreeError{path: path}
	}
	_, worktreePath, err := ensureWorktree(path, digest, w.WorktreeRoot)
	if err != nil {
		return err
	}
	if w.observer != nil {
		w.observer.Observe(ChangeStarted{Path: path, Workflow: w.WorkflowName, Change: change})
	}
	template := w.PromptTemplate
	if template == "" {
		template = defaultPromptTemplate
	}
	logPath, runErr := w.agent.Run(ctx, worktreePath, digest, renderTemplate(template, change), w.Model)
	if logPath != "" && w.observer != nil {
		w.observer.Observe(LogPath{Path: logPath, Workflow: w.WorkflowName, Change: change})
	}
	if runErr != nil {
		return w.rollbackWorktree(path, digest, worktreePath, runErr)
	}
	if w.AutoMerge {
		if err := w.mergeWorktreeLane(path, ref, worktreePath, digest, change); err != nil {
			return w.rollbackWorktree(path, digest, worktreePath, err)
		}
	} else {
		if err := w.rebaseWorktreeLane(worktreePath, ref, change); err != nil {
			return w.rollbackWorktree(path, digest, worktreePath, err)
		}
	}
	if w.observer != nil {
		w.observer.Observe(ChangeDone{Path: path, Workflow: w.WorkflowName, Change: change})
	}
	return nil
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

	// Lane isolation dispatch: worktree mode runs the agent in a git
	// worktree and never switches the operator's checkout; the default
	// branch-mode path below is unchanged when Worktree is false.
	if w.Worktree {
		return w.workResolvedWorktree(ctx, path, change, ref)
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
			w.observer.Observe(ChangeStarted{Path: path, Workflow: w.WorkflowName, Change: change})
		}
		template := w.PromptTemplate
		if template == "" {
			template = defaultPromptTemplate
		}
		logPath, runErr := w.agent.Run(ctx, path, digest, renderTemplate(template, change), w.Model)
		if logPath != "" && w.observer != nil {
			w.observer.Observe(LogPath{Path: logPath, Workflow: w.WorkflowName, Change: change})
		}
		if runErr != nil {
			return w.rollbackWorkflowLane(path, change, digest, ref, attemptTip, created, runErr)
		}
		if commitErr := w.catchUpCustomCommit(path, change); commitErr != nil {
			w.warn(path, change, commitErr.Error())
			return w.rollbackWorkflowLane(path, change, digest, ref, attemptTip, created, commitErr)
		}
		if w.observer != nil {
			w.observer.Observe(ChangeDone{Path: path, Workflow: w.WorkflowName, Change: change})
		}
		return nil
	}

	branch := "see/" + change
	if err := ensureBranch(path, current, branch); err != nil {
		return err
	}
	if w.observer != nil {
		w.observer.Observe(ChangeStarted{Path: path, Workflow: w.WorkflowName, Change: change})
	}
	template := w.PromptTemplate
	if template == "" {
		template = defaultPromptTemplate
	}
	logPath, runErr := w.agent.Run(ctx, path, change, renderTemplate(template, change), "")
	if logPath != "" && w.observer != nil {
		w.observer.Observe(LogPath{Path: logPath, Workflow: w.WorkflowName, Change: change})
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
	msg := w.catchUpMessage(change)
	if err := exec.Command("git", "-C", path, "commit", "-m", msg).Run(); err != nil {
		w.warn(path, change, fmt.Sprintf("git commit failed: %v", err))
	}
	if w.observer != nil {
		w.observer.Observe(ChangeDone{Path: path, Workflow: w.WorkflowName, Change: change})
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
					changeName, err := w.runOneWorkflow(ctx, repo, wf)
					if err != nil && w.observer != nil {
						errStr := err.Error()
						w.observer.Observe(ChangeFailed{Path: repo, Workflow: wf.Name, Change: changeName, Err: errStr, summary: summaryFor(err)})
					}
				}
				continue
			}
			lastChange, err := w.runWithRetry(ctx, repo)
			if err != nil {
				if w.observer != nil {
					errStr := err.Error()
					w.observer.Observe(ChangeFailed{Path: repo, Change: lastChange, Err: errStr, summary: summaryFor(err)})
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
	child.Model = strings.TrimSpace(wf.Model)
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
		pi            = flag.String("pi", "pi", "path to the pi binary")
		retry         = flag.Int("retry", 3, "retries per repo on failure")
		modeFlag      = flag.String("mode", "tui", "output mode (default \"tui\"); one of: tui, log")
		once          = flag.Bool("once", false, "run one scan and exit")
		configFlag    = flag.String("config", "", "path to config.yaml (default: ~/.config/see/config.yaml); pass \"-\" to skip")
		promptFlag    = flag.String("prompt", "", "override the agent prompt template; {change} is replaced with the active change name")
		interval      = flag.Duration("interval", DefaultPollInterval, "delay between completed scans in continuous mode; 0 disables the delay, negative values are rejected")
		worktreeFlag  = flag.Bool("worktree", false, "run agents in a git worktree so the operator's checkout is never switched")
		autoMergeFlag = flag.Bool("auto-merge", true, "in worktree mode, rebase and fast-forward merge the lane into the operator's branch on success; pass --auto-merge=false to leave the rebased lane for manual review")
		worktreeRoot  = flag.String("worktree-root", "", "override the worktree location (default ~/.cache/see/worktrees); only meaningful with --worktree")
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

	// Load configuration once so log-directory resolution, prompt, and
	// watch resolution see the same snapshot. --config=- bypasses both
	// the file read and the configured paths; the embedded default
	// still applies. Config loads before the log directory is resolved
	// so a configured log_dir can take effect.
	cfg, err := loadStartupConfig(*configFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
		os.Exit(2)
	}

	logDir, err := resolveLogDir(cfg)
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

	w.SetPromptTemplate(selectPromptTemplate(*promptFlag, ""))
	if err := validateWorkflows(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
		os.Exit(2)
	}
	// Drop parked (disable: true) workflows after validateWorkflows has
	// already run on the full merged list. The watcher iterates this
	// filtered slice unchanged; an empty slice restores OpenSpec mode.
	w.Workflows = filterDisabledWorkflows(cfg.Workflows)

	// Resolve the lane-isolation triple (worktree, auto_merge,
	// worktree_root) with CLI flag > config field > default precedence,
	// then validate it. flag.Visit records which flags were passed
	// explicitly so the default-true --auto-merge does not reject every
	// default branch-mode run, and so an explicit --auto-merge (either
	// form) in branch mode can be rejected per the lane-isolation
	// contract.
	explicitFlags := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicitFlags[f.Name] = true })
	worktree, autoMerge, resolvedRoot, err := resolveLaneIsolation(cfg, explicitFlags, *worktreeFlag, *autoMergeFlag, *worktreeRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
		os.Exit(2)
	}
	w.Worktree = worktree
	w.AutoMerge = autoMerge
	w.WorktreeRoot = resolvedRoot

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
	events.Attach(tuiObserver{send: obs.Send})
	w.observer = events
	wg := &sync.WaitGroup{}
	wg.Go(func() {
		_, err := prog.Run()
		if err != nil {
			errStr := err.Error()
			events.Observe(InfraError{Where: "tui", Err: errStr, summary: summaryFor(err)})
		}
		cancel(err)
	})
	wg.Go(func() {
		err := w.Watch(tCtx, repos)
		if err != nil {
			errStr := err.Error()
			events.Observe(InfraError{Where: "watcher", Err: errStr, summary: summaryFor(err)})
		}
		cancel(err)
	})
	wg.Wait()
	return context.Cause(tCtx)
}

// tuiObserver forwards each Event to the bubbletea Program via a
// `func(tea.Msg)` (typically *tui.ChanObserver.Send). The function
// field keeps the adapter trivial to test. The type-switch lives
// here so the tui package has no dependency on main's Event types.
//
// For the three failure-bearing events the observer prefers the
// concise unexported `summary` over the exported `Err` when the
// source error opted into Summary(). The full Err still reaches the
// batch JavaScript Object Notation Lines (JSONL) because the
// eventLogger serializes the event before forwarding; the in-memory
// TUI message therefore renders the short reason while JSONL readers
// see the full diagnostic.
type tuiObserver struct {
	send func(tea.Msg)
}

func (o tuiObserver) Observe(e Event) {
	switch e := e.(type) {
	case RepoSeen:
		o.send(tui.RepoSeenMsg{Path: e.Path, HasChange: e.HasChange})
	case ChangeStarted:
		o.send(tui.ChangeStartedMsg{Path: e.Path, Workflow: e.Workflow, Change: e.Change})
	case RetryAttempt:
		o.send(tui.RetryAttemptMsg{Path: e.Path, Workflow: e.Workflow, Change: e.Change, N: e.N, Max: e.Max, Err: pickSummary(e.Err, e.summary)})
	case ChangeDone:
		o.send(tui.ChangeDoneMsg{Path: e.Path, Workflow: e.Workflow, Change: e.Change})
	case ChangeFailed:
		o.send(tui.ChangeFailedMsg{Path: e.Path, Workflow: e.Workflow, Change: e.Change, Err: pickSummary(e.Err, e.summary)})
	case Warning:
		o.send(tui.WarningMsg{Path: e.Path, Workflow: e.Workflow, Change: e.Change, Msg: e.Msg})
	case InfraError:
		o.send(tui.InfraErrorMsg{Where: e.Where, Err: pickSummary(e.Err, e.summary)})
	}
}

// pickSummary returns summary when non-empty, else full. Keeps the
// three failure-bearing event sites in lockstep and lets a future
// event that forgets to populate `summary` fall back to its full
// message unchanged.
func pickSummary(full, summary string) string {
	if summary != "" {
		return summary
	}
	return full
}
