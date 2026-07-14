# tui delta — apply-ponytail-audit

## MODIFIED Requirements

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
- `RetryAttempt` — when the watcher's retry loop re-invokes `work`
  after a previous failure. Carries the repo path, change name,
  attempt number, and the previous error.
- `ChangeDone` — after `git commit` succeeds on a fully-archived
  change. Carries the repo path and change name.
- `ChangeFailed` — when `work` returns a non-nil error after the
  retry loop has exhausted all attempts. Carries the repo path,
  change name, and the final error.
- `Warning` — when a rollback, completion, or pre-run check step
  reports a failure that is not itself the reason `work` returns an
  error. Carries the repo path, change name, and the warning message.
  Emitted alongside (not in place of) `ChangeFailed` or other
  boundary events.

When the observer is `nil`, `Watcher` SHALL behave identically to a
build without the observer seam: no nil dereference, no event emission
attempt, no extra cost beyond a single interface-nil check per
boundary.

#### Scenario: Observer receives the expected event sequence for a successful run
- **WHEN** `Watcher.runOnce` runs against a repo with one active change
  and the agent succeeds
- **THEN** the observer receives, in order: `RepoSeen`,
  `ChangeStarted`, `LogPath`, `ChangeDone`

#### Scenario: Observer receives retry events on a recoverable failure
- **WHEN** `Watcher.runOnce` runs against a repo with one active change,
  the agent fails on the first attempt, and succeeds on the second
- **THEN** the observer receives, in order: `RepoSeen`,
  `ChangeStarted`, `LogPath`, `ChangeStarted`, `LogPath`, `ChangeDone`

#### Scenario: Observer receives ChangeFailed after retries are exhausted
- **WHEN** `Watcher.runOnce` runs against a repo with one active change
  and the agent fails on every retry
- **THEN** the observer receives, in order: `RepoSeen`,
  `ChangeStarted`, `LogPath`, `RetryAttempt`, `RetryAttempt`,
  `RetryAttempt`, `ChangeFailed`

#### Scenario: Repo with no active change still emits RepoSeen
- **WHEN** `Watcher.runOnce` runs against a git repo with no
  `openspec/changes/` entries other than `archive/`
- **THEN** the observer receives exactly `RepoSeen` with
  `HasOpenspec = false`
- **THEN** `Watcher.runOnce` continues to the next repo without
  emitting `ChangeStarted`, `RetryAttempt`, `ChangeDone`, or
  `ChangeFailed`

#### Scenario: nil observer is safe
- **WHEN** `Watcher{agent: a, RetryCount: n}` is constructed with no
  observer set
- **THEN** `Watcher.work` and `Watcher.runOnce` complete without
  panic and with the same observable behavior as today's build

#### Scenario: Rollback hiccup emits a Warning alongside ChangeFailed
- **WHEN** `Watcher.runOnce` runs against a repo with one active
  change and the agent fails, then `git switch` back to the original
  ref fails during rollback
- **THEN** the observer receives a `Warning` event for the switch
  failure
- **AND** the observer receives the `ChangeFailed` event with the
  original agent error

### Requirement: `--tui` renders a live status grid of every scanned repo
When `-tui` is `true` and stdout is a terminal, `see` SHALL render a
status grid via Bubble Tea in the alternate screen. The grid SHALL
contain one row per scanned repo. Each row SHALL display, in order:

- **REPO** — the basename of the repo directory, with a trailing
  `⚠` glyph when the row has an active `Warning` event that has not
  been cleared
- **CHANGE** — the active change name, or `—` if none
- **PHASE** — one of `idle`, `working`, `done`, `failed`,
  rendered with a phase-specific glyph and color. A row whose repo
  has no `openspec/changes/` entries SHALL render at `idle` with
  `—` in the CHANGE column.
- **RETRY** — `n/max` while retrying, `—` otherwise
- **AGE** — elapsed time since the current phase started, or `—`

The grid SHALL include a footer line with a live count of rows by
phase plus a count of rows with an active warning (e.g. `2 done ·
1 working · 1 idle · 1 failed · 1 warning`) and a key hint
`[q] quit`. A `Warning` SHALL be cleared on the next `ChangeStarted`
event for the same repo. The TUI SHALL own signal handling: pressing
`q` or sending SIGINT SHALL exit the program and restore the
terminal.

#### Scenario: Grid shows every scanned repo including non-openspec ones
- **WHEN** `see --tui` runs in a working directory with two git repos
  that have active changes, one git repo without `openspec/`, and one
  non-git directory
- **THEN** the grid contains exactly three rows
- **THEN** the openspec repos show `working` or `done` based on the
  watcher's progress
- **THEN** the non-openspec repo shows `idle` with `—` for change
  and retry
- **THEN** the non-git directory is not represented in the grid

#### Scenario: Phase transitions update the corresponding row in place
- **WHEN** a repo's phase transitions from `working` to `done`
- **THEN** the row for that repo SHALL show `done` on the next render
- **THEN** no other row SHALL change

#### Scenario: Warning event adds the ⚠ glyph to the REPO column
- **WHEN** the watcher emits a `Warning` event for a repo whose row
  is currently `done`
- **THEN** the REPO cell for that repo SHALL display the repo name
  followed by `⚠`
- **AND** the footer's `warning` counter SHALL increment by one
- **AND** the row's `PHASE` SHALL remain `done` (the warning is a
  state modifier, not a phase transition)

#### Scenario: ChangeStarted clears the ⚠ glyph
- **WHEN** the watcher emits a `Warning` event for a repo and later
  emits a `ChangeStarted` event for the same repo
- **THEN** the `⚠` glyph SHALL be removed from the REPO cell
- **AND** the footer's `warning` counter SHALL decrement by one

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

### Requirement: Watcher semantics are unchanged under `--mode=tui`
`-mode=tui` SHALL NOT alter watcher semantics. Under `-mode=tui`,
`see` SHALL:

- Process repos sequentially in the order `os.ReadDir` returns them.
- Halt the watcher on the first repo whose retry loop exhausts
  all attempts. The TUI SHALL NOT auto-skip or auto-retry failed
  repos on the operator's behalf.
- Honor `-retry` exactly as in log mode.
- Honor `-pi` exactly as in log mode.
- Emit the same `git` commands in the same order as in log mode.

#### Scenario: First-repo failure halts the watcher under `--mode=tui`
- **WHEN** the first repo in scan order fails all retry attempts
- **THEN** the TUI exits with a non-zero status
- **THEN** the error message from `ChangeFailed` is preserved (shown
  on the final TUI frame before exit)

#### Scenario: Retry policy is honored under `--mode=tui`
- **WHEN** `--mode=tui -retry=5` is set and a repo fails on
  attempts 1-3 before succeeding on attempt 4
- **THEN** the agent is invoked four times for that repo
- **THEN** the TUI shows `RETRY` values `1/5`, `2/5`, `3/5`, then
  transitions to `done`

### Requirement: `see` extracts a testable `selectRunMode` dispatcher
The flag-to-mode resolution in `main()` SHALL live in a pure function
named `selectRunMode(mode string, isTTY bool) (runMode, error)`.
The function SHALL be free of side effects (no I/O, no exit calls,
no flag-package interaction) so it can be unit-tested directly. The
function SHALL return `modeUnknown` plus a non-nil error for both
unknown values and missing-TTY cases; the caller (`main()`) prints
the error message (prefixed with `see: `), calls `flag.Usage()`, and
exits with status `2`.

#### Scenario: `selectRunMode` resolves the valid matrix
- **WHEN** `selectRunMode("log", true)` is called
- **THEN** it returns `modeLog` and a nil error

- **WHEN** `selectRunMode("log", false)` is called
- **THEN** it returns `modeLog` and a nil error

- **WHEN** `selectRunMode("tui", true)` is called
- **THEN** it returns `modeTUI` and a nil error

- **WHEN** `selectRunMode("tui", false)` is called
- **THEN** it returns `modeUnknown` and an error whose message
  equals `see: --mode=tui requires a TTY; rerun with --mode=log`

- **WHEN** `selectRunMode("foo", true)` is called
- **THEN** it returns `modeUnknown` and an error whose message
  equals `see: unknown --mode="foo" (want: tui, log)`

- **WHEN** `selectRunMode("", true)` is called
- **THEN** it returns `modeUnknown` and an error whose message
  equals `see: unknown --mode="" (want: tui, log)`
