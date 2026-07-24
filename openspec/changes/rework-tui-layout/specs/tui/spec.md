## MODIFIED Requirements

### Requirement: `--tui` renders a live status grid of every scanned repo
When `-tui` is `true` and stdout is a terminal, `see` SHALL render a live status view via Bubble Tea in the alternate screen. The view SHALL retain state for every scanned git repository, but SHALL render a summary table followed by a priority viewport containing no more than ten repository entries. The summary SHALL count all retained repositories and SHALL include the total, counts by phase, active warning count, and the number of visible entries as `visible / total`.

The visible repository entries SHALL be ranked by attention priority and then activity recency:

1. repositories in `working` phase;
2. repositories in `failed` phase;
3. repositories with an active warning glyph;
4. all remaining repositories.

Within each priority class, the most recently meaningful activity SHALL appear first, with stable discovery order as the tie-breaker. `ChangeStarted`, `RetryAttempt`, `ChangeDone`, `ChangeFailed`, and `Warning` messages SHALL count as meaningful activity. Repeated `RepoSeen` messages for an existing repository SHALL NOT refresh its activity order; the first `RepoSeen` for a new repository SHALL establish its stable discovery order.

Each visible row SHALL display, in order:

- **REPO** — the basename of the repo directory, with a trailing `⚠` glyph when the row has an active `Warning` event that has not been cleared.
- **CHANGE** — the custom condition's normalized change, the active OpenSpec fallback change name, or `—` if the selected resolver found no change.
- **PHASE** — one of `idle`, `working`, `done`, or `failed`, rendered with a phase-specific glyph and color. A row whose selected resolver found no change SHALL render at `idle` with `—` in the CHANGE column.
- **RETRY** — `n/max` while retrying, `—` otherwise.
- **AGE** — elapsed time since the current phase started, or `—`.

The existing log-path continuation SHALL remain associated with its repository row when present. The TUI SHALL render fewer than ten entries when the terminal height cannot fit the summary, table header, footer, infrastructure error, and complete row entries. It SHALL NOT render more than ten entries or split a repository entry because of the height budget.

The phase and warning counts SHALL appear in the top summary rather than being duplicated in the footer. The footer SHALL retain the `[q] quit` key hint. A `Warning` SHALL be cleared on the next `ChangeStarted` event for the same repo. The TUI SHALL own signal handling: pressing `q` or sending `SIGINT` SHALL exit the program and restore the terminal.

#### Scenario: Summary counts all retained repositories
- **WHEN** the TUI has retained 47 repository rows and only ten fit in the priority viewport
- **THEN** the summary reports a total of 47 repositories
- **THEN** each phase and warning count includes rows outside the visible viewport
- **THEN** the summary reports the visible count as `10 / 47`

#### Scenario: Priority viewport is capped at ten repository entries
- **WHEN** more than ten repositories are retained
- **THEN** the rendered repository section contains no more than ten repository entries
- **THEN** all retained rows remain available for summary counting and future viewport selection

#### Scenario: Working, failed, and warning rows take priority
- **WHEN** the retained rows include working, failed, warning-bearing, done, and idle repositories
- **THEN** working repositories appear before failed repositories
- **THEN** failed repositories appear before warning-bearing repositories
- **THEN** warning-bearing repositories appear before remaining repositories
- **THEN** rows within each priority class are ordered by most recent meaningful activity

#### Scenario: Repeated repository scans do not refresh activity order
- **WHEN** an existing repository receives repeated `RepoSeen` messages without another lifecycle event
- **THEN** its activity position does not change solely because of those messages
- **THEN** a repository with a later meaningful lifecycle event can rank ahead of it

#### Scenario: New repositories receive stable discovery order
- **WHEN** a repository is first observed by `RepoSeen` and has not emitted a lifecycle event
- **THEN** it can participate in viewport selection using its discovery order
- **THEN** subsequent `RepoSeen` messages do not continuously move it ahead of other rows

#### Scenario: Grid shows the retained state of repositories across workflow modes
- **WHEN** `see --tui` scans repositories whose selected resolvers return changes, a repository whose resolver returns no change, and a non-git directory
- **THEN** the model retains one row per git repository and no row for the non-git directory
- **THEN** the visible viewport shows at most ten prioritized git-repository rows
- **THEN** repositories with changes show their resolved custom or OpenSpec-compatible change values
- **THEN** the repository without a change shows `idle` with `—` for change and retry

#### Scenario: Custom change is displayed rather than its hash
- **WHEN** a custom condition emits `add-dark-mode` and selects `see/<digest>`
- **THEN** the CHANGE column displays `add-dark-mode`
- **AND** it does not display the digest as the change label

#### Scenario: Phase transitions update the corresponding row in place
- **WHEN** a repo's phase transitions from `working` to `done`
- **THEN** the retained row for that repo SHALL show `done` on the next render
- **THEN** no other retained row SHALL change
- **THEN** the row's activity position SHALL be refreshed by the meaningful completion event

#### Scenario: Warning event adds the warning glyph and updates the summary
- **WHEN** the watcher emits a `Warning` event for a repo whose row is currently `done`
- **THEN** the REPO cell for that repo SHALL display the repo name followed by `⚠`
- **AND** the top summary's warning counter SHALL increment by one
- **AND** the row's PHASE SHALL remain `done`
- **AND** the row SHALL be ranked in the warning priority class

#### Scenario: ChangeStarted clears the warning glyph and refreshes activity
- **WHEN** the watcher emits a `Warning` event for a repo and later emits a `ChangeStarted` event for the same repo
- **THEN** the `⚠` glyph SHALL be removed from the REPO cell
- **AND** the top summary's warning counter SHALL decrement by one
- **AND** the row SHALL be ranked in the working priority class

#### Scenario: Short terminals render complete entries without exceeding the cap
- **WHEN** the terminal height cannot fit the summary, header, footer, and ten complete repository entries
- **THEN** the TUI renders only the number of complete entries that fit
- **THEN** a log-path continuation is rendered together with its repository row or omitted with that row
- **THEN** no more than ten repository entries are rendered

#### Scenario: `q` and SIGINT share the same exit-status rule
- **WHEN** the user presses `q` while the TUI is running, OR `SIGINT` is delivered to a running `see --tui` process
- **THEN** the program SHALL exit with a non-zero status if and only if `Watcher.Watch` returned a non-nil error before cancellation or `tea.Program.Run` returned an error
- **AND** the program SHALL otherwise exit with status `0`
- **AND** the terminal's cursor is restored
- **AND** the alternate screen is released
