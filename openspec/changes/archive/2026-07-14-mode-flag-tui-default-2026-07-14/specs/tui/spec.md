## MODIFIED Requirements

### Requirement: `see` exposes a `--mode` flag selecting the output mode
`see` SHALL accept a `-mode` string flag. The flag SHALL accept the
values `tui` and `log`. The flag SHALL default to `tui`. When `-mode`
is `tui`, `see` SHALL render the live status grid via the `tui`
package and SHALL wire the `Watcher.observer` to the TUI. When
`-mode` is `log`, `see` SHALL behave exactly as `see` did before the
TUI was introduced: write `log.Printf` output to stderr, halt the
watcher on the first failed repo, and exit with a non-zero status on
failure. The flag SHALL have no effect on agent invocation, retry
policy, or git rollback semantics.

#### Scenario: Default invocation renders the TUI
- **WHEN** `see` is invoked with no flags
- **THEN** mode resolves to `tui`
- **THEN** an observer IS wired to the `Watcher`
- **THEN** the live status grid renders via the TUI package

#### Scenario: `--mode=log` reproduces the pre-TUI log behavior
- **WHEN** `see --mode=log` is invoked
- **THEN** no observer is wired to the `Watcher`
- **THEN** progress and errors are written via `log.Printf` to
  stderr
- **THEN** the exit status on first-repo failure is non-zero,
  identical to a pre-TUI invocation
- **THEN** TTY state has no effect on `--mode=log` behavior

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

### Requirement: Watcher semantics are unchanged under `--mode=tui`
`-mode=tui` SHALL NOT alter watcher semantics. Under `-mode=tui`,
`see` SHALL:

- Process repos sequentially in the order `os.ReadDir` returns them.
- Halt the watcher on the first repo whose `Watcher.work` exhausts
  `retryN` attempts. The TUI SHALL NOT auto-skip or auto-retry failed
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

## REMOVED Requirements

### Requirement: `--tui` falls back to log mode when stdout is not a terminal

The previous requirement stated that `see --tui` (with a non-TTY
stdout) SHALL warn and fall back to log mode. That fallback contract
is removed: `--mode=tui` now exits with status 2 instead of falling
back. The corresponding "Piped `--tui` falls back to log mode"
scenario is removed with it. `--mode=log` is the explicit opt-in for
non-TTY operators.

## ADDED Requirements

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
named `selectRunMode(mode string, isTTY bool) (runMode, string)`.
The function SHALL be free of side effects (no I/O, no exit calls,
no flag-package interaction) so it can be unit-tested directly. The
function SHALL return `modeUnknown` plus a stderr message for both
unknown values and missing-TTY cases; the caller (`main()`) prints
the message, calls `flag.Usage()`, and exits with status `2`.

#### Scenario: `selectRunMode` resolves the valid matrix
- **WHEN** `selectRunMode("log", true)` is called
- **THEN** it returns `modeLog` and an empty message

- **WHEN** `selectRunMode("log", false)` is called
- **THEN** it returns `modeLog` and an empty message

- **WHEN** `selectRunMode("tui", true)` is called
- **THEN** it returns `modeTUI` and an empty message

- **WHEN** `selectRunMode("tui", false)` is called
- **THEN** it returns `modeUnknown` and the TTY-required message

- **WHEN** `selectRunMode("foo", true)` is called
- **THEN** it returns `modeUnknown` and the unknown-mode message
  naming `foo`

- **WHEN** `selectRunMode("", true)` is called
- **THEN** it returns `modeUnknown` and the unknown-mode message
  naming an empty value