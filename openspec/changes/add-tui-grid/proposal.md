## Why

Today `see` surfaces its progress through `log.Printf` lines written to
stderr. When watching N repos, the operator gets N interleaved
`file: line: msg` streams with no way to see, at a glance, which repo is
running, which is queued, which failed. The watcher's halt-on-first-
failure semantics are invisible until the whole process exits.

A live status grid makes the watcher's per-repo phase visible in real
time, surfaces failures as they happen (instead of as a process exit),
and gives the operator a single screen that survives a long batch run.

## What Changes

- Add a `--tui` flag (default `false`). When set and stdout is a TTY,
  `see` renders a live status grid instead of `log.Printf` output.
- Add an `Observer` interface and a small set of `Event` types to
  `main.go`. `Watcher` emits events at each phase boundary:
  `RepoSeen`, `ChangeStarted`, `RetryAttempt`, `ChangeDone`,
  `ChangeFailed`.
- Add a new `tui` package (new `tui/` directory) containing a Bubble
  Tea Model that subscribes to the watcher's events and renders the
  grid. The package owns the event channel; the watcher writes to it
  synchronously.
- When stdout is not a TTY and `--tui` is set, `see` writes a one-line
  warning to stderr and falls back to `log.Printf` mode. This keeps the
  flag safe to use in scripts and CI without surprises.
- Watcher semantics are unchanged: sequential execution, halt on first
  failure, retry policy (`-retry` flag) intact, agent invocation
  (`-pi` flag) intact, `git reset --hard` rollback intact, merge-back
  semantics from `see-isolate-agent-runs-on-branch` (when that lands)
  intact.
- The default `Observer` is a no-op. `Watcher` keeps its existing
  `log.Printf` calls so log mode and TUI mode share the same code path.

**BREAKING**: None. `--tui` is opt-in and default behavior is preserved.

## Capabilities

### New Capabilities

- `tui`: the live status grid rendered under `--tui`, the events the
  watcher emits to drive it, and the PTY-detection fallback behavior.

### Modified Capabilities

(none — `openspec/specs/` is currently empty aside from this change's
delta spec.)

## Impact

- `main.go`:
  - New types: `Event` (sealed interface with `isEvent()`),
    `RepoSeen`, `ChangeStarted`, `RetryAttempt`, `ChangeDone`,
    `ChangeFailed`, and `Observer` interface.
  - `Watcher` gains an `observer Observer` field (zero-value nil is
    the no-op default).
  - `Watcher.work` and `Watcher.runOnce` gain `w.observer.Observe(...)`
    calls at each phase boundary. The `log.Printf` calls stay.
  - `main()` adds `-tui` flag. When set with a TTY stdout, build a
    `tui.Program` wired to a new `Watcher.observer` and run it. When
    set without a TTY, warn to stderr and fall through to today's
    behavior.
  - `PiAgent.Run` in TUI mode redirects the agent's stdout/stderr to
    stderr (shell-level `2>&1` in the `exec.Command` setup) so the
    TUI's screen is not corrupted by the agent's raw output. The
    log mode keeps the existing behavior (agent output flows to the
    watcher's stdout/stderr as today).
- `main_test.go`:
  - Extend `TestWorkCommitsOnSuccess` and
    `TestRunOncePassesRepoPathToAgent` with a `recordingObserver` that
    captures the emitted event sequence and asserts the expected
    ordering.
  - New `TestObserverIsOptional` confirms that `Watcher{agent: ...}`
    with no `observer` set still runs the same as today (existing
    tests cover this transitively; add explicit assertion if needed).
- New files:
  - `tui/model.go` — `Model`, `Init`, `Update`, `View`.
  - `tui/view.go` — grid rendering via `lipgloss` styles.
  - `tui/program.go` — `NewProgram(watcher, observer)` constructor and
    signal handling.
  - `tui/tui_test.go` — snapshot tests for `View()` and a driver test
    that pumps fake events through `Update`.
- Deps: `github.com/charmbracelet/bubbletea`,
  `github.com/charmbracelet/lipgloss`. `golang.org/x/term` if
  `isatty` is not available from bubbletea's transitive deps (verify
  during implementation; if bubbletea re-exports it, no new direct
  dep).
- No flag other than `-tui` is added or changed.
- No change to OpenSpec or `pi` integration beyond the stdout/stderr
  redirect noted above.