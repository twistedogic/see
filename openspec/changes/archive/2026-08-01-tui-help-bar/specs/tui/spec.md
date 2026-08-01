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
- **AGE** — elapsed time since active work started while PHASE is `working`, or `—` while PHASE is `idle`, `done`, or `failed`.
- **ERROR** — the row's current failure reason (`LastErr`), rendered only on terminals wide enough to hold it after the fixed columns. For a `failed` row this is the final error; for a `working` row between retry attempts it is the previous attempt's error. The cell SHALL render `—` when the row has no current failure reason.

The TUI SHALL NOT render the per-invocation agent log path anywhere in the grid. Each repository row SHALL occupy exactly one physical line, regardless of terminal width or the length of any path the watcher emits. The path remains available in the batch-level JavaScript Object Notation Lines (JSONL) event file and, under `--mode=log` with piped stdout, on the stdout mirror; the TUI is not a consumer of it. The TUI SHALL render fewer than ten entries when the terminal height cannot fit the summary, table header, footer, infrastructure error, and complete one-line row entries, and SHALL NOT render more than ten entries because of the height budget. The footer height SHALL include the activity row, the separator row, and the help row (see the live pi activity ticker requirement); the height budget SHALL account for the help row's current rendering, which is one line in the short help view and two lines in the full help view.

The ERROR column SHALL be the final column. It SHALL be shown only when the terminal width exceeds the sum of the fixed columns active at that width by at least a fixed minimum (`errMinWidth`, twenty columns); below that width the column SHALL be omitted and the row SHALL be identical to a grid without it. AGE SHALL continue to appear at its existing width threshold (`>= 80` columns) regardless of whether the ERROR column is shown. When shown, the ERROR column's width SHALL be exactly the terminal width minus that fixed-column sum, so the column cannot cause a row to exceed one physical line. Before rendering, the failure reason SHALL be collapsed to a single line by replacing every run of whitespace (including carriage returns and line feeds) with a single space, then truncated to the column width with a trailing ellipsis if it overflows; a failure reason that contains embedded newlines SHALL therefore render on exactly one physical line. The full, unmodified failure reason remains available in the batch-level JSONL file and, under `--mode=log` with piped stdout, on the stdout mirror.

The phase and warning counts SHALL appear in the top summary rather than being duplicated in the footer. The footer SHALL render a help bar from a typed `keymap` struct (the three-row layout — activity, separator, help — and the help-bar behavior, including the `?` toggle, are defined in the live pi activity ticker requirement). A `Warning` SHALL be cleared on the next `ChangeStarted` event for the same repo. The TUI SHALL own signal handling: pressing `q` or sending `SIGINT` SHALL exit the program and restore the terminal.

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

#### Scenario: AGE advances only while working
- **WHEN** a repository row is in the `working` phase with a recorded start time
- **THEN** AGE SHALL display the elapsed time since active work started
- **AND** the displayed duration SHALL continue updating once per second

#### Scenario: Non-working phases show no AGE value
- **WHEN** a repository row is in the `idle`, `done`, or `failed` phase
- **THEN** AGE SHALL display `—`
- **AND** a start time retained from earlier work SHALL NOT cause AGE to display or continue increasing

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
- **WHEN** the terminal height cannot fit the summary, header, footer (activity, separator, help), and ten complete repository entries
- **THEN** the TUI renders only the number of complete entries that fit
- **THEN** no more than ten repository entries are rendered

#### Scenario: The agent log path is not rendered in any row
- **WHEN** a repository's run emits a `LogPath` event whose path is longer than the terminal width
- **THEN** the rendered row for that repository occupies exactly one physical line
- **AND** the path string does not appear anywhere in the rendered grid
- **AND** the row's REPO, CHANGE, PHASE, RETRY, and AGE columns render unchanged
- **AND** the path still reaches the batch-level JSONL file and the `--mode=log` stdout mirror

#### Scenario: Failed row shows its failure reason in the ERROR column
- **WHEN** a repository's run exhausts its retries and the watcher emits `ChangeFailed` carrying an error, on a terminal at least 100 columns wide
- **THEN** the rendered row for that repository SHALL display the final error in the ERROR column
- **AND** the ERROR column SHALL appear as the final column after AGE
- **AND** a healthy repository (no current failure reason) in the same viewport SHALL render `—` in its ERROR cell
- **AND** the full, unmodified error SHALL still reach the batch-level JSONL file and the `--mode=log` stdout mirror

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

### Requirement: The TUI renders live pi activity as an animated ticker
The Terminal User Interface (TUI) SHALL render three footer rows beneath the repository grid:

1. an **activity row** containing a fixed `pi ›` prefix and the latest meaningful sanitized `pi` activity;
2. a **separator row** — a single horizontal rule spanning the terminal width, styled to match the help row's separator tone;
3. a **help row** rendered by `charm.land/bubbles/v2/help` from a typed `keymap` struct, showing the active key bindings (for example `q quit • ? help`).

Completed assistant text, tool execution starts, tool execution outcomes, and non-JSON diagnostics SHALL be eligible ticker activities. Thinking content, token-level text deltas, repeated tool progress updates, raw message snapshots, and unknown JSON events SHALL NOT be rendered.

Each new activity SHALL replace the previous activity and reset its display offset. Before any invocation the activity SHALL be `waiting`; a `ChangeStarted` event SHALL reset it to `starting`; the latest activity SHALL remain after the invocation ends until another invocation starts.

When the activity fits within the activity row's width, it SHALL remain stationary and SHALL NOT schedule marquee updates. When it overflows, only the activity window SHALL move horizontally, one terminal display cell at a time, with a visible gap between repetitions. The separator and help rows SHALL remain stationary. Each of the three footer rows SHALL occupy exactly one physical line at every terminal size; when width is insufficient, the help row SHALL use the `help` package's built-in truncation with a trailing `…` rather than wrapping, and the activity row SHALL fall back to its existing one-line collapse and display.

The `?` key SHALL toggle the help bar between the short one-line rendering and the full multi-column rendering. The toggle SHALL start in the short rendering. The full rendering SHALL be two lines tall, stacking each binding's key and description in its own column; the renderer's height budget SHALL account for the active rendering so the table viewport shrinks when the full view is showing and grows back when it is hidden.

#### Scenario: Tool activity updates the ticker live
- **WHEN** the active `pi` invocation emits a tool execution start for `bash` with command `go test ./...`
- **THEN** the ticker displays a sanitized activity identifying `bash` and `go test ./...`
- **AND** the update appears before the invocation exits

#### Scenario: Tool completion replaces its start activity
- **WHEN** a displayed tool execution later completes
- **THEN** the ticker replaces the start marker with a success or failure marker for the same summarized tool
- **AND** it does not render the tool's full result body

#### Scenario: Assistant narrative replaces previous activity
- **WHEN** `pi` emits a completed assistant text event
- **THEN** the ticker displays that text with whitespace collapsed to one line
- **AND** thinking and token-delta events do not independently update the ticker

#### Scenario: Overflowing activity animates within fixed chrome
- **WHEN** sanitized activity is wider than the activity row
- **THEN** successive marquee ticks advance the visible activity by terminal display cells
- **AND** the separator and help rows remain at fixed positions on their own lines
- **AND** the activity row remains one physical line

#### Scenario: Fitting activity does not animate
- **WHEN** sanitized activity fits in the activity row
- **THEN** the activity remains stationary
- **AND** no marquee tick is rearmed for that activity

#### Scenario: New activity restarts at its beginning
- **WHEN** a marquee is partway through an overflowing activity and a new meaningful activity arrives
- **THEN** the ticker replaces the old activity
- **AND** the new activity is first rendered from display offset zero

#### Scenario: Narrow terminal preserves quit access
- **WHEN** the terminal is too narrow to render the full help bar text
- **THEN** the help row renders a single physical line with a trailing `…` ellipsis marking the truncation rather than wrapping
- **AND** ticker content remains on its own row and is not wrapped into the help row
- **AND** the quit binding remains active (handled by the `keymap`) regardless of whether its label is visible

#### Scenario: Terminal controls are not interpreted
- **WHEN** assistant text, a tool argument, or a diagnostic contains terminal escape sequences or control characters
- **THEN** those controls are stripped or collapsed before the activity enters TUI state
- **AND** the original bytes remain unchanged in the per-invocation log

#### Scenario: `?` toggles the help bar between short and full renderings
- **WHEN** the TUI is in the short help rendering and the user presses `?`
- **THEN** the help row switches to the full multi-column rendering
- **THEN** the renderer height budget grows by one line, shrinking the table viewport by one row
- **AND** pressing `?` again returns the help row to the short rendering and restores the viewport

## ADDED Requirements

### Requirement: The TUI footer is sourced from a typed keymap
The TUI footer SHALL source its key bindings from a typed `keymap` struct, not from hand-edited string literals. The `keymap` SHALL hold one `key.Binding` field per active binding (currently `Quit` and `Help`) and SHALL implement the `charm.land/bubbles/v2/help.KeyMap` interface by providing `ShortHelp() []key.Binding` and `FullHelp() [][]key.Binding` methods. The footer rendering, the `?` toggle, and the help row's text SHALL all derive from this struct; adding a new binding SHALL be a one-field change to the struct plus, at most, a matching `key.Matches` case in the `Update` switch.

The `key.Binding.Keys` slice SHALL declare the actual key presses the binding matches (for example `["q", "ctrl+c"]` for `Quit`). The `key.Binding.Help` field SHALL declare the display label (for example `WithHelp("q", "quit")`). The label and the matcher SHALL NOT drift: the matcher's source of truth is `Keys`, the label's source of truth is `Help`, and the help bar's rendered text is the `Help` field — there SHALL be no parallel string literal that has to be kept in sync with the matcher.

#### Scenario: Footer chrome is a projection of the keymap
- **WHEN** the TUI renders its footer rows
- **THEN** the help row text SHALL be the result of `m.help.ShortHelpView(m.keys.ShortHelp())` (or `FullHelpView` when `ShowAll` is true)
- **THEN** no string literal in `tui/view.go` SHALL name a key binding the help bar is showing

#### Scenario: Adding a binding is a one-field change
- **WHEN** a new binding is added to the `keymap` struct
- **THEN** the help bar SHALL display it without any other change to the footer rendering code
- **THEN** a matching `key.Matches` case in the `Update` switch SHALL be the only handler change required to make the new key do something

#### Scenario: Key match uses the binding's `Keys`, not its `Help` label
- **WHEN** a `key.Binding` is declared with `WithKeys("q", "ctrl+c")` and `WithHelp("q", "quit")`
- **THEN** `key.Matches` on a `tea.KeyPressMsg` whose `String()` is `"q"` or `"ctrl+c"` SHALL return true
- **THEN** `key.Matches` on a `tea.KeyPressMsg` whose `String()` is `"ctrl+q"` SHALL return false
- **THEN** the help bar SHALL render the label `q quit` regardless of the matcher
