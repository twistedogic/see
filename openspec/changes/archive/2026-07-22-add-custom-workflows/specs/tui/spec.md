## MODIFIED Requirements

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
