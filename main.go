package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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

// renderPrompt substitutes the literal token `{change}` in template
// with the active change name. One substitution, no escape syntax,
// no other tokens — see openspec/changes/extract-apply-prompt-flag
// for the design rationale.
func renderPrompt(template, change string) string {
	return strings.ReplaceAll(template, "{change}", change)
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

// runWithRetry invokes work up to w.RetryCount times on repo, emitting
// RetryAttempt events between attempts, and returns the name of the
// change that was being worked (or "" when no active change existed
// before the first attempt) plus the final error from the last attempt.
// A nil error short-circuits the loop; exhausting all attempts returns
// the last attempt's error.
func (w Watcher) runWithRetry(ctx context.Context, repo string) (string, error) {
	lastChange := ""
	var prevErr error
	for attempt := 1; attempt <= w.RetryCount; attempt++ {
		changeName := ""
		if cs := ListActiveOpenSpecChanges(repo); len(cs) > 0 {
			changeName = cs[0]
			lastChange = changeName
		}
		if prevErr != nil && w.observer != nil {
			w.observer.Observe(RetryAttempt{
				Path: repo, Change: changeName, N: attempt, Max: w.RetryCount,
				Err: prevErr.Error(),
			})
		}
		err := w.work(ctx, repo)
		if err == nil {
			return lastChange, nil
		}
		prevErr = err
	}
	return lastChange, prevErr
}

// Event is the watcher→observer interface. Concrete types are below;
// the marker-method pattern is a soft convention, not a hard
// guarantee (Go interfaces are not sealed). The watcher emits one of
// these at each phase boundary and the tui package type-switches on
// the concrete shape.
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
	template := w.PromptTemplate
	if template == "" {
		template = defaultPromptTemplate
	}
	logPath, runErr := w.agent.Run(ctx, path, change, renderPrompt(template, change))
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
			if w.observer != nil {
				w.observer.Observe(RepoSeen{Path: repo, HasOpenspec: len(ListActiveOpenSpecChanges(repo)) > 0})
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
		configFlag = flag.String("config", "", "path to config.yaml (default: $XDG_CONFIG_HOME/see/config.yaml); pass \"-\" to skip")
		promptFlag = flag.String("prompt", "", "override the agent prompt template; {change} is replaced with the active change name")
		interval   = flag.Duration("interval", DefaultPollInterval, "delay between completed scans in continuous mode; 0 disables the delay, negative values are rejected")
		watchFlag  multiFlag
	)
	flag.Var(&watchFlag, "watch", "watch path or shell-glob (path, ~/path, or shell-glob with *, ?, [abc]; '**' is rejected). Repeatable.")
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
	if err := validateCustomConfig(cfg, *promptFlag); err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
		os.Exit(2)
	}

	repos, warnings, err := resolveWatchList(watchFlag, cfg.Watches)
	if err != nil {
		fmt.Fprintln(os.Stderr, "see:", err)
		os.Exit(2)
	}
	for _, warn := range warnings {
		events.Observe(warn)
	}

	if mode == modeTUI {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := runTUI(ctx, &w, events, repos); err != nil {
			os.Exit(1)
		}
		return
	}
	// mode == modeLog
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	if err := w.Watch(ctx, repos); err != nil {
		os.Exit(1)
	}
}

// resolveWatchList picks the watch list from layered sources using
// the same precedence rule as selectPromptTemplate: the first source
// that contributes at least one entry wins outright, and the next
// source is not consulted. Order: CLI --watch entries, then the
// configured `watches` sequence, then the current working directory
// as the fallback when both are empty. It is a pure coordinator —
// no configuration file input/output happens here; main() loads the
// configuration once and threads cfg.Watches through this function
// alongside the CLI entries so prompt and watches share one config
// snapshot.
func resolveWatchList(cliWatches, cfgWatches []string) ([]string, []Warning, error) {
	patterns := cliWatches
	if len(patterns) == 0 {
		patterns = cfgWatches
	}
	if len(patterns) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, nil, err
		}
		patterns = []string{cwd}
	}
	return resolveTargets(patterns)
}

// multiFlag accumulates repeated --flag values into a slice.
// Implements flag.Value so the standard flag package accepts
// `--watch a --watch b` and `--watch=a,b` (the latter via a custom
// split is out of scope; one --watch per pattern keeps the surface
// simple).
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// runTUI runs the watcher with an eventLogger wired as the
// Watcher.observer; the eventLogger fans events out to the bubbletea
// ChanObserver in addition to the batch-level JSONL. The bubbletea
// program owns signal handling; when it exits we cancel the
// watcher's context so the tight poll loop returns.
func runTUI(ctx context.Context, w *Watcher, events *eventLogger, repos []string) error {
	prog, obs := tui.New()
	events.Attach(tuiObserver{obs: obs})
	w.observer = events
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	watchErr := make(chan error, 1)
	go func() { watchErr <- w.Watch(ctx, repos) }()
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
// tuiObserver implements main.Observer by translating each Event into
// a tui Msg literal and forwarding it through ChanObserver.Send. The
// type-switch lives here (not in the tui package) so the tui package
// has no dependency on main's Event types.
type tuiObserver struct{ obs *tui.ChanObserver }

func (o tuiObserver) Observe(e Event) {
	switch e := e.(type) {
	case RepoSeen:
		o.obs.Send(tui.RepoSeenMsg{Path: e.Path, HasOpenspec: e.HasOpenspec})
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
