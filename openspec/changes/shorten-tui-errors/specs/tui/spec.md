## MODIFIED Requirements

### Requirement: `--tui` renders a live status grid of every scanned repo
When `-tui` is `true` and stdout is a terminal, `see` SHALL render a live Terminal User Interface (TUI) status view via Bubble Tea in the alternate screen. The view SHALL retain state for every scanned git repository, but SHALL render a summary table followed by a priority viewport containing no more than ten repository entries. The summary SHALL count all retained repositories and SHALL include the total, counts by phase, active warning count, and the number of visible entries as `visible / total`.

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
- **ERROR** — the row's current failure display text (`LastErr`), rendered only on terminals wide enough to hold it after the fixed columns. When the error provides a concise summary, this text SHALL be that summary; otherwise it SHALL be the full failure reason. For a `failed` row this represents the final error; for a `working` row between retry attempts it represents the previous attempt's error. The cell SHALL render `—` when the row has no current failure reason.

The TUI SHALL NOT render the per-invocation agent log path anywhere in the grid. Each repository row SHALL occupy exactly one physical line, regardless of terminal width or the length of any path the watcher emits. The path remains available in the batch-level JavaScript Object Notation Lines (JSONL) event file and, under `--mode=log` with piped stdout, on the stdout mirror; the TUI is not a consumer of it. The TUI SHALL render fewer than ten entries when the terminal height cannot fit the summary, table header, footer, infrastructure error, and complete one-line row entries, and SHALL NOT render more than ten entries because of the height budget.

The ERROR column SHALL be the final column. It SHALL be shown only when the terminal width exceeds the sum of the fixed columns active at that width by at least a fixed minimum (`errMinWidth`, twenty columns); below that width the column SHALL be omitted and the row SHALL be identical to a grid without it. AGE SHALL continue to appear at its existing width threshold (`>= 80` columns) regardless of whether the ERROR column is shown. When shown, the ERROR column's width SHALL be exactly the terminal width minus that fixed-column sum, so the column cannot cause a row to exceed one physical line. Before rendering, the selected display text SHALL be collapsed to a single line by replacing every run of whitespace (including carriage returns and line feeds) with a single space, then truncated to the column width with a trailing ellipsis if it overflows; display text that contains embedded newlines SHALL therefore render on exactly one physical line. The same summary selection SHALL apply to watcher errors shown in the infrastructure error banner. The full, unmodified failure reason remains available in the batch-level JSONL file and, under `--mode=log` with piped stdout, on the stdout mirror.

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
- **THEN** no more than ten repository entries are rendered

#### Scenario: The agent log path is not rendered in any row
- **WHEN** a repository's run emits a `LogPath` event whose path is longer than the terminal width
- **THEN** the rendered row for that repository occupies exactly one physical line
- **AND** the path string does not appear anywhere in the rendered grid
- **AND** the row's REPO, CHANGE, PHASE, RETRY, and AGE columns render unchanged
- **AND** the path still reaches the batch-level JSONL file and the `--mode=log` stdout mirror

#### Scenario: Failed row uses a concise error summary when available
- **WHEN** a repository's run exhausts its retries with an error that provides a concise summary and the watcher emits `ChangeFailed`, on a terminal at least 100 columns wide
- **THEN** the rendered row for that repository SHALL display the concise summary in the ERROR column
- **AND** the ERROR column SHALL appear as the final column after AGE
- **AND** a healthy repository (no current failure reason) in the same viewport SHALL render `—` in its ERROR cell
- **AND** the full, unmodified error SHALL still reach the batch-level JSONL file and the `--mode=log` stdout mirror

#### Scenario: Failed row falls back to the full error
- **WHEN** a repository's run exhausts its retries with an error that provides no concise summary and the watcher emits `ChangeFailed`, on a terminal at least 100 columns wide
- **THEN** the rendered row for that repository SHALL display the full error in the ERROR column subject to the existing one-line collapse and truncation rules
- **AND** the full, unmodified error SHALL reach the batch-level JSONL file and the `--mode=log` stdout mirror

#### Scenario: Retry and infrastructure errors use concise summaries
- **WHEN** an error that provides a concise summary is carried by a `RetryAttempt` event or shown as a watcher infrastructure error
- **THEN** the TUI SHALL display the concise summary
- **AND** the corresponding JSONL event SHALL retain the full, unmodified error

#### Scenario: A multi-line failure reason collapses to one physical line
- **WHEN** the failure reason carried by `ChangeFailed` contains embedded carriage returns and line feeds, and the terminal is at least 100 columns wide
- **THEN** the rendered row for that repository SHALL occupy exactly one physical line
- **AND** the ERROR cell SHALL contain no newline characters
- **AND** the cell SHALL be truncated to the ERROR column width with a trailing ellipsis when the collapsed text exceeds it

#### Scenario: ERROR column is omitted below its width threshold
- **WHEN** the terminal is narrower than 100 columns, a repo has failed, and another repo is healthy
- **THEN** no ERROR column is rendered in the header or any row
- **AND** AGE SHALL still appear when the terminal is at least 80 columns wide
- **AND** each row SHALL be identical to a grid rendered without the ERROR column

#### Scenario: `q` and SIGINT share the same exit-status rule
- **WHEN** the user presses `q` while the TUI is running, OR `SIGINT` is delivered to a running `see --tui` process
- **THEN** the program SHALL exit with a non-zero status if and only if `Watcher.Watch` returned a non-nil error before cancellation or `tea.Program.Run` returned an error
- **AND** the program SHALL otherwise exit with status `0`
- **AND** the terminal's cursor is restored
- **AND** the alternate screen is released
