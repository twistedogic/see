## MODIFIED Requirements

### Requirement: `see` exposes a `--mode` flag selecting the output mode
`see` SHALL accept a `-mode` string flag. The flag SHALL accept the
values `tui` and `log`. The flag SHALL default to `tui`. When
`-mode` is `tui`, `see` SHALL render the live status grid via the
`tui` package and SHALL wire the `Watcher.observer` to an
`eventLogger` that fans events out to both the JSONL file and the
TUI's `ChanObserver`. When `-mode` is `log`, `see` SHALL wire the
`Watcher.observer` to an `eventLogger` with no secondary observer;
the JSONL file SHALL be the only output. The flag SHALL have no
effect on agent invocation, retry policy, or git rollback
semantics.

#### Scenario: Default invocation renders the TUI
- **WHEN** `see` is invoked with no flags
- **THEN** mode resolves to `tui`
- **THEN** an observer IS wired to the `Watcher`
- **THEN** the live status grid renders via the TUI package
- **AND** a batch-level JSONL file is created under the log
  directory

#### Scenario: `--mode=log` is silent and writes only to JSONL
- **WHEN** `see --mode=log` is invoked
- **THEN** an observer IS wired to the `Watcher`
- **THEN** no `log.Printf` output is written to stderr
- **THEN** stdout is empty for the lifetime of the process
- **THEN** every watcher event lands in the JSONL file in
  emission order
- **THEN** the exit status on first-repo failure is non-zero,
  identical to a pre-TUI invocation
- **THEN** TTY state has no effect on `--mode=log` behavior

### Requirement: Watcher emits events through an `Observer` at phase boundaries
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
- **WHEN** `Watcher{agent: a, RetyCount: n}` is constructed with no
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
- **PHASE** — one of `idle`, `working`, `done`, `failed`, `no-spec`,
  rendered with a phase-specific glyph and color
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
- **THEN** the non-openspec repo shows `no-spec` with `—` for change
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

### Requirement: Agent output does not corrupt the TUI
In both `--mode=tui` and `--mode=log`, `PiAgent.Run` SHALL direct
the agent's stdout and stderr to the per-invocation JSONL file in
the log directory. The agent's bytes SHALL NOT reach stdout or
stderr of the `see` process under any mode. Per-invocation JSONL
files are guaranteed to exist for every successful `PiAgent.Run`
call; the file path is reported via the `LogPath` event in both
modes.

#### Scenario: Agent output reaches the per-invocation JSONL
- **WHEN** `--tui` is set and the agent writes lines to its stdout
  and stderr
- **THEN** those lines appear in the per-invocation JSONL file
  whose path was emitted via the `LogPath` event
- **THEN** those lines do not appear inside the TUI's alternate
  screen
- **THEN** those lines do not appear on the `see` process's
  stdout or stderr

## REMOVED Requirements

### Requirement: Log mode prints the log path to stderr
**Reason**: Log mode is now silent. The `log.Printf("see: log →
%s", path)` line in `Watcher.work` is removed; the log path is
discoverable via the `LogPath` event in the batch-level JSONL.

**Migration**: Operators who scripted against this stderr line
must switch to parsing the JSONL for `LogPath` events. No code
path in `see` itself depends on the line.
