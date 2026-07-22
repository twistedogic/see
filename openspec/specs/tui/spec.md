# tui

## Purpose

Define the live status grid that `see` renders under the `--tui` flag
and the events the watcher emits to drive it. When stdout is a
terminal, `see` SHALL render a bubbletea grid showing every scanned
repo's phase, change, retry count, age, and an optional warning glyph
in the repository column; the underlying event stream is also written
to a batch-level JavaScript Object Notation Lines (JSONL) file (see
the `event-log` capability). When stdout is not a terminal and the
operator has asked for `--mode=tui`, `see` SHALL refuse to run and exit
with a clear stderr message; `see` SHALL NOT silently fall back to log
mode.
## Requirements
### Requirement: `see` exposes a `--mode` flag selecting the output mode
`see` SHALL accept a `-mode` string flag. The flag SHALL accept the
values `tui` and `log`. The flag SHALL default to `tui`. When
`-mode` is `tui`, `see` SHALL render the live status grid via the
`tui` package and SHALL wire the `Watcher.observer` to an
`eventLogger` that fans events out to both the JSONL file and the
TUI's `ChanObserver`. When `-mode` is `log`, `see` SHALL wire the
`Watcher.observer` to an `eventLogger` with no secondary observer;
the JSONL file SHALL be the primary sink, and `see` SHALL also
mirror the JSONL line-for-line onto stdout when stdout is not a
terminal (a pipe or a redirect). When stdout IS a terminal,
`--mode=log` SHALL stay silent — the on-disk JSONL remains the
operator's only view in that case. The mirror SHALL receive the
same encoded bytes the JSONL file receives (one event per line,
in emission order) so `see --mode=log | jq` parses identically
to `cat <jsonl-file>`. The flag SHALL have no effect on agent
invocation, retry policy, or git rollback semantics.

#### Scenario: Default invocation renders the TUI
- **WHEN** `see` is invoked with no flags
- **THEN** mode resolves to `tui`
- **THEN** an observer IS wired to the `Watcher`
- **THEN** the live status grid renders via the TUI package
- **AND** a batch-level JSONL file is created under the log
  directory

#### Scenario: `--mode=log` is silent and writes only to JSONL
- **WHEN** `see --mode=log` is invoked and stdout IS a terminal
- **THEN** an observer IS wired to the `Watcher`
- **THEN** no `log.Printf` output is written to stderr
- **THEN** stdout is empty for the lifetime of the process
- **THEN** every watcher event lands in the JSONL file in
  emission order
- **THEN** the exit status on first-repo failure is non-zero,
  identical to a pre-TUI invocation

#### Scenario: `--mode=log` mirrors JSONL to stdout when stdout is not a TTY
- **WHEN** `see --mode=log` is invoked and stdout IS NOT a terminal
  (piped output, redirected to a file, captured by a CI runner)
- **THEN** an observer IS wired to the `Watcher`
- **THEN** every watcher event lands in the JSONL file in
  emission order
- **THEN** every watcher event ALSO lands on stdout, encoded as
  one line per event, byte-identical to the on-disk JSONL
- **THEN** no `log.Printf` output is written to stderr
- **THEN** the exit status on first-repo failure is non-zero,
  identical to a pre-TUI invocation

#### Scenario: `--mode=log` JSONL stream is pipe-parsable as line-delimited JSON
- **WHEN** `see --mode=log | jq` runs against any fixture
- **THEN** `jq` decodes one record per line on stdout
- **AND** each decoded record's fields match the underlying
  `Event` payload (e.g., `RepoSeen.Path`, `ChangeStarted.Change`)

### Requirement: `--mode=tui` requires a TTY
When `-mode` is `tui`, `see` SHALL require stdout to be a terminal.
When stdout is not a terminal (piped output, redirected to a file,
run inside a non-interactive CI step), `see` SHALL exit with status
`2`, SHALL write a single-line message to stderr of the form
`see: --mode=tui requires a TTY; rerun with --mode=log`, and SHALL
NOT proceed with any watcher work. `see` SHALL NOT silently fall
back to log mode; the operator MUST opt in to log mode explicitly
with `--mode=log`.

#### Scenario: Piped `--mode=tui` exits non-zero with a hint
- **WHEN** `see --mode=tui | cat` runs against any fixture
- **THEN** stderr contains the TTY-required message
- **THEN** no `log.Printf` lines are emitted
- **THEN** no TUI is rendered
- **THEN** exit status is `2`

### Requirement: `Watcher` emits events through an `Observer` at phase boundaries
`Watcher` SHALL expose an `Observer` interface. When the observer is non-nil, `Watcher.work` SHALL emit the following events at the named boundaries:

- `RepoSeen` — after a repository's selected work resolver runs and before agent invocation. Carries the repo path and `HasChange`, which reports whether that resolver produced an active change.
- `ChangeStarted` — after a custom or OpenSpec-compatible change is selected and immediately before the agent is invoked. Carries the repo path and normalized change value.
- `RetryAttempt` — when the watcher's retry loop re-invokes `work` after a previous failure. Carries the repo path, most recently resolved change value when available, attempt number, and the previous error.
- `ChangeDone` — after a custom run succeeds or an OpenSpec-compatible change satisfies its existing completion contract. Carries the repo path and change value.
- `ChangeFailed` — when `work` returns a non-nil error after the retry loop has exhausted all attempts. Carries the repo path, most recently resolved change value when available, and the final error.
- `Warning` — when a rollback, completion, or pre-run check step reports a failure that is not itself the reason `work` returns an error. Carries the repo path, change value when available, and the warning message. Emitted alongside, not in place of, `ChangeFailed` or other boundary events.

When the observer is `nil`, `Watcher` SHALL behave identically to a build without the observer seam: no nil dereference, no event emission attempt, and no extra cost beyond a single interface-nil check per boundary.

#### Scenario: Observer receives the expected event sequence for a successful run
- **WHEN** `Watcher.runOnce` runs against a repo whose selected resolver returns one change and the agent succeeds
- **THEN** the observer receives, in order: `RepoSeen`, `ChangeStarted`, `LogPath`, `ChangeDone`
- **AND** `RepoSeen.HasChange` is true

#### Scenario: Observer receives retry events on a recoverable failure
- **WHEN** `Watcher.runOnce` runs against a repo with one resolved change, the agent fails on the first attempt, and succeeds on the second
- **THEN** the observer receives `RepoSeen`, change and retry boundary events, and finally `ChangeDone`
- **AND** every change-bearing event uses the value returned by the selected resolver for that attempt

#### Scenario: Observer receives ChangeFailed after retries are exhausted
- **WHEN** `Watcher.runOnce` runs against a repo with one resolved change and the agent fails on every retry
- **THEN** the observer receives exactly one `ChangeFailed` after the final attempt
- **AND** `ChangeFailed` carries the final error and most recently resolved change value

#### Scenario: Repo with no custom change still emits RepoSeen
- **WHEN** custom mode's condition exits with status `1`
- **THEN** the observer receives exactly `RepoSeen` with `HasChange = false`
- **THEN** `Watcher.runOnce` continues to the next repo without emitting `ChangeStarted`, `RetryAttempt`, `ChangeDone`, or `ChangeFailed`

#### Scenario: Repo with no OpenSpec change still emits RepoSeen
- **WHEN** compatibility mode runs against a git repo with no `openspec/changes/` entries other than `archive/`
- **THEN** the observer receives exactly `RepoSeen` with `HasChange = false`
- **THEN** `Watcher.runOnce` continues to the next repo without emitting `ChangeStarted`, `RetryAttempt`, `ChangeDone`, or `ChangeFailed`

#### Scenario: Condition failure is observable
- **WHEN** a custom condition fails on every retry before producing a change
- **THEN** the observer receives `RepoSeen` with `HasChange = false`
- **AND** receives `ChangeFailed` with an empty change and the final condition error

#### Scenario: nil observer is safe
- **WHEN** `Watcher{agent: a, RetryCount: n}` is constructed with no observer set
- **THEN** `Watcher.work` and `Watcher.runOnce` complete without panic and with the same non-event behavior

#### Scenario: Rollback hiccup emits a Warning alongside ChangeFailed
- **WHEN** `Watcher.runOnce` processes a change and the agent fails, then a rollback step also fails
- **THEN** the observer receives a `Warning` event for the cleanup failure
- **AND** the observer receives the `ChangeFailed` event with the original agent error

### Requirement: `--tui` renders a live status grid of every scanned repo
When `-tui` is `true` and stdout is a terminal, `see` SHALL render a status grid via Bubble Tea in the alternate screen. The grid SHALL contain one row per scanned repo. Each row SHALL display, in order:

- **REPO** — the basename of the repo directory, with a trailing `⚠` glyph when the row has an active `Warning` event that has not been cleared.
- **CHANGE** — the custom condition's normalized change, the active OpenSpec fallback change name, or `—` if the selected resolver found no change.
- **PHASE** — one of `idle`, `working`, `done`, or `failed`, rendered with a phase-specific glyph and color. A row whose selected resolver found no change SHALL render at `idle` with `—` in the CHANGE column.
- **RETRY** — `n/max` while retrying, `—` otherwise.
- **AGE** — elapsed time since the current phase started, or `—`.

The grid SHALL include a footer line with a live count of rows by phase plus a count of rows with an active warning, for example `2 done · 1 working · 1 idle · 1 failed · 1 warning`, and a key hint `[q] quit`. A `Warning` SHALL be cleared on the next `ChangeStarted` event for the same repo. The TUI SHALL own signal handling: pressing `q` or sending `SIGINT` SHALL exit the program and restore the terminal.

#### Scenario: Grid shows every scanned repo across workflow modes
- **WHEN** `see --tui` scans repositories whose selected resolvers return changes, a repository whose resolver returns no change, and a non-git directory
- **THEN** the grid contains one row per git repository and no row for the non-git directory
- **THEN** repositories with changes show their resolved custom or OpenSpec-compatible change values
- **THEN** the repository without a change shows `idle` with `—` for change and retry

#### Scenario: Custom change is displayed rather than its hash
- **WHEN** a custom condition emits `add-dark-mode` and selects `see/<digest>`
- **THEN** the CHANGE column displays `add-dark-mode`
- **AND** it does not display the digest as the change label

#### Scenario: Phase transitions update the corresponding row in place
- **WHEN** a repo's phase transitions from `working` to `done`
- **THEN** the row for that repo SHALL show `done` on the next render
- **THEN** no other row SHALL change

#### Scenario: Warning event adds the ⚠ glyph to the REPO column
- **WHEN** the watcher emits a `Warning` event for a repo whose row is currently `done`
- **THEN** the REPO cell for that repo SHALL display the repo name followed by `⚠`
- **AND** the footer's `warning` counter SHALL increment by one
- **AND** the row's PHASE SHALL remain `done`

#### Scenario: ChangeStarted clears the ⚠ glyph
- **WHEN** the watcher emits a `Warning` event for a repo and later emits a `ChangeStarted` event for the same repo
- **THEN** the `⚠` glyph SHALL be removed from the REPO cell
- **AND** the footer's `warning` counter SHALL decrement by one

#### Scenario: `q` and SIGINT share the same exit-status rule
- **WHEN** the user presses `q` while the TUI is running, OR `SIGINT` is delivered to a running `see --tui` process
- **THEN** the program SHALL exit with a non-zero status if and only if `Watcher.Watch` returned a non-nil error before cancellation or `tea.Program.Run` returned an error
- **AND** the program SHALL otherwise exit with status `0`
- **AND** the terminal's cursor is restored
- **AND** the alternate screen is released

### Requirement: `--tui` drains the watcher goroutine and closes the JSONL event logger before exit
When `-mode=tui` is in effect, `see` SHALL, before the process exits
in response to either `q` or SIGINT:

- cancel the watcher context, killing any in-flight agent
  subprocess via `exec.CommandContext`
- drain the watcher goroutine via the `watchErr` channel
- close the JavaScript Object Notation Lines (JSONL) event logger
  so any buffered bytes reach the file system

The `defer events.Close()` registered in `main()` before `runTUI`
runs, and the explicit `cancel()` plus `<-watchErr` wait inside
`runTUI`, are the ordered runtime hooks that satisfy this
requirement.

#### Scenario: Cleanup chain runs before exit on `q` or SIGINT
- **WHEN** the user presses `q` while the TUI is running, OR
  SIGINT is delivered to a running `see --tui` process
- **THEN** the watcher goroutine has returned before `main` exits
- **THEN** the JSONL event logger's writer has been closed before
  `main` exits

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

### Requirement: `see` rejects unknown `--mode` values
`see` SHALL reject any `-mode` value other than `tui` or `log`,
including the empty string. On rejection, `see` SHALL exit with
status `2`, SHALL write a message to stderr of the form
`see: unknown --mode="<value>" (want: tui, log)`, and SHALL print
`flag.Usage()` so the operator sees the registered flags and their
valid values.

#### Scenario: `--mode=foo` exits non-zero with usage
- **WHEN** `see --mode=foo` is invoked
- **THEN** stderr contains the unknown-mode message naming `foo`
- **THEN** stderr contains `flag.Usage()` output listing the
  registered flags
- **THEN** exit status is `2`

#### Scenario: `--mode=` (empty string) is rejected
- **WHEN** `see --mode=` is invoked
- **THEN** stderr contains the unknown-mode message naming an empty
  value
- **THEN** exit status is `2`

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

