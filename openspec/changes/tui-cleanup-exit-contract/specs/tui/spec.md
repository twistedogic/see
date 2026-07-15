## MODIFIED Requirements

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
event for the same repo. The Terminal User Interface (TUI) SHALL
own signal handling: pressing `q` or sending Signal-Interrupt
(SIGINT) SHALL exit the program and restore the terminal.

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

#### Scenario: `q` and SIGINT share the same exit-status rule
- **WHEN** the user presses `q` while the TUI is running, OR
  SIGINT is delivered to a running `see --tui` process
- **THEN** the program SHALL exit
  - with a non-zero status if and only if `Watcher.Watch` returned
    a non-nil error before the watcher's context was cancelled, OR
    `tea.Program.Run` returned an error
  - with status 0 otherwise
- **THEN** the terminal's cursor is restored
- **THEN** the alternate screen is released

## ADDED Requirements

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

#### Scenario: `q` and SIGINT leave no orphaned goroutines or unflushed JSONL
- **WHEN** the user presses `q` while the TUI is running, OR
  SIGINT is delivered to a running `see --tui` process
- **THEN** the watcher goroutine has returned before `main` exits
- **THEN** the JSONL event logger's writer has been closed before
  `main` exits
