# tui

## Purpose

Define the live status grid that `see` renders under the `--tui` flag, the
events the watcher emits to drive it, and the PTY-detection fallback
behavior. When stdout is a terminal, `see` SHALL render a bubbletea grid
showing every scanned repo's phase, change, retry count, age, and last
error. When stdout is not a terminal, `see` SHALL warn and fall back to
log mode.

## Requirements

### Requirement: `see` exposes a `--tui` flag
`see` SHALL accept a boolean `-tui` flag. The flag SHALL default to
`false`. When `false`, `see` SHALL behave exactly as it does today:
write `log.Printf` output to stderr, halt the watcher on the first
failed repo, and exit with a non-zero status on failure. The flag
SHALL have no effect on agent invocation, retry policy, or git
rollback semantics.

#### Scenario: Default invocation is unchanged
- **WHEN** `see` is invoked with no flags
- **THEN** no observer is wired to the `Watcher`
- **THEN** progress and errors are written via `log.Printf` to stderr
- **THEN** the exit status on first-repo failure is non-zero, identical
  to today

#### Scenario: `--tui=false` is equivalent to the default
- **WHEN** `see` is invoked with `-tui=false`
- **THEN** behavior is byte-for-byte identical to the no-flag case

### Requirement: `Watcher` emits events through an `Observer` at phase boundaries
`Watcher` SHALL expose an `Observer` interface. When the observer is
non-nil, `Watcher.work` SHALL emit the following events at the named
boundaries:

- `RepoSeen` — after `os.ReadDir` enumerates a repo (and before any
  active-change lookup). Carries the repo path and whether
  `openspec/changes/` exists (other than `archive/`).
- `ChangeStarted` — after a change is picked from the active list and
  immediately before the agent is invoked. Carries the repo path and
  change name.
- `RetryAttempt` — when `retryN` re-invokes `work` after a previous
  failure. Carries the repo path, change name, attempt number, and
  the previous error.
- `ChangeDone` — after `git commit` succeeds on a fully-archived
  change. Carries the repo path and change name.
- `ChangeFailed` — when `work` returns a non-nil error after exhausting
  `retryN`. Carries the repo path, change name, and the final error.

When the observer is `nil`, `Watcher` SHALL behave identically to a
build without the observer seam: no nil dereference, no event emission
attempt, no extra cost beyond a single interface-nil check per
boundary.

#### Scenario: Observer receives the expected event sequence for a successful run
- **WHEN** `Watcher.runOnce` runs against a repo with one active change
  and the agent succeeds
- **THEN** the observer receives, in order: `RepoSeen`,
  `ChangeStarted`, `ChangeDone`

#### Scenario: Observer receives retry events on a recoverable failure
- **WHEN** `Watcher.runOnce` runs against a repo with one active change,
  the agent fails on the first attempt, and succeeds on the second
- **THEN** the observer receives, in order: `RepoSeen`,
  `ChangeStarted`, `RetryAttempt`, `ChangeStarted`, `ChangeDone`

#### Scenario: Observer receives ChangeFailed after retries are exhausted
- **WHEN** `Watcher.runOnce` runs against a repo with one active change
  and the agent fails on every retry
- **THEN** the observer receives, in order: `RepoSeen`,
  `ChangeStarted`, `RetryAttempt`, `RetryAttempt`, `RetryAttempt`,
  `ChangeFailed`

#### Scenario: Repo with no active change still emits RepoSeen
- **WHEN** `Watcher.runOnce` runs against a git repo with no
  `openspec/changes/` entries other than `archive/`
- **THEN** the observer receives exactly `RepoSeen` with
  `HasOpenspec = false`
- **THEN** `Watcher.runOnce` continues to the next repo without
  emitting `ChangeStarted`, `RetryAttempt`, `ChangeDone`, or
  `ChangeFailed`

#### Scenario: nil observer is safe
- **WHEN** `Watcher{agent: a, RetyCount: n}` is constructed with no
  observer set
- **THEN** `Watcher.work` and `Watcher.runOnce` complete without
  panic and with the same observable behavior as today's build

### Requirement: `--tui` renders a live status grid of every scanned repo
When `-tui` is `true` and stdout is a terminal, `see` SHALL render a
status grid via Bubble Tea in the alternate screen. The grid SHALL
contain one row per scanned repo. Each row SHALL display, in order:

- **REPO** — the basename of the repo directory
- **CHANGE** — the active change name, or `—` if none
- **PHASE** — one of `idle`, `working`, `done`, `failed`, `no-spec`,
  rendered with a phase-specific glyph and color
- **RETRY** — `n/max` while retrying, `—` otherwise
- **AGE** — elapsed time since the current phase started, or `—`
- **ERR** — the last error message, or empty

The grid SHALL include a footer line with a live count of rows by
phase (e.g. `2 done · 1 working · 1 idle · 1 failed`) and a key
hint `[q] quit`. The TUI SHALL own signal handling: pressing `q` or
sending SIGINT SHALL exit the program and restore the terminal.

#### Scenario: Grid shows every scanned repo including non-openspec ones
- **WHEN** `see --tui` runs in a working directory with two git repos
  that have active changes, one git repo without `openspec/`, and one
  non-git directory
- **THEN** the grid contains exactly three rows
- **THEN** the openspec repos show `working` or `done` based on the
  watcher's progress
- **THEN** the non-openspec repo shows `no-spec` with `—` for change
  and retry
- **THEN** the non-git directory is not represented in the grid

#### Scenario: Phase transitions update the corresponding row in place
- **WHEN** a repo's phase transitions from `working` to `done`
- **THEN** the row for that repo SHALL show `done` on the next render
- **THEN** no other row SHALL change

#### Scenario: Pressing `q` exits the TUI cleanly
- **WHEN** the user presses `q` while the TUI is running
- **THEN** the program exits with status 0
- **THEN** the terminal's cursor is restored
- **THEN** the alternate screen is released

#### Scenario: SIGINT exits the TUI cleanly
- **WHEN** SIGINT is delivered to a running `see --tui` process
- **THEN** the program exits with the same status as the equivalent
  log-mode invocation (non-zero if a repo failure was in flight,
  zero otherwise)

### Requirement: `--tui` falls back to log mode when stdout is not a terminal
When `-tui` is `true` but stdout is not a terminal (piped output,
redirected to a file, run inside a non-interactive CI step), `see`
SHALL write a single-line warning to stderr of the form
`see: --tui requires a TTY; falling back to log mode` and SHALL
proceed in log mode. The fallback SHALL be deterministic: a piped
invocation SHALL produce the same stderr and stdout as an invocation
without `-tui`, plus the one warning line.

#### Scenario: Piped `--tui` falls back to log mode
- **WHEN** `see --tui | cat` runs against a small fixture
- **THEN** stderr contains the warning line
- **THEN** stderr contains the same `log.Printf` lines an unflagged
  invocation would produce
- **THEN** the exit status is the same as an unflagged invocation
  with the same fixture

### Requirement: Watcher semantics are unchanged under `--tui`
The `-tui` flag SHALL NOT alter watcher semantics. Under `-tui`,
`see` SHALL:

- Process repos sequentially in the order `os.ReadDir` returns them.
- Halt the watcher on the first repo whose `Watcher.work` exhausts
  `retryN` attempts. The TUI SHALL NOT auto-skip or auto-retry failed
  repos on the operator's behalf.
- Honor `-retry` exactly as in log mode.
- Honor `-pi` exactly as in log mode.
- Emit the same `git` commands in the same order as in log mode.

#### Scenario: First-repo failure halts the watcher under `--tui`
- **WHEN** the first repo in scan order fails all retry attempts
- **THEN** the TUI exits with a non-zero status
- **THEN** the error message from `ChangeFailed` is preserved (written
  to stderr as part of the fallback path, or shown on the final TUI
  frame before exit)

#### Scenario: Retry policy is honored under `--tui`
- **WHEN** `-tui -retry=5` is set and a repo fails on attempts 1-3
  before succeeding on attempt 4
- **THEN** the agent is invoked four times for that repo
- **THEN** the TUI shows `RETRY` values `1/5`, `2/5`, `3/5`, then
  transitions to `done`

### Requirement: Agent output does not corrupt the TUI
In `--tui` mode, `PiAgent.Run` SHALL direct the agent's stdout and
stderr to the watcher's stderr (not stdout). This SHALL be the only
difference in agent invocation between log mode and TUI mode.

#### Scenario: Agent output reaches stderr, not the TUI
- **WHEN** `--tui` is set and the agent writes lines to its stdout
- **THEN** those lines do not appear inside the TUI's alt screen
- **THEN** those lines are visible on the watcher's stderr stream
  (for example, when stderr is captured to a file or visible on a
  terminal that is not in alt-screen mode)