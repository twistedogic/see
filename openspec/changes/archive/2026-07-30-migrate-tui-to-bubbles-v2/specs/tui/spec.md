## ADDED Requirements

### Requirement: The live status grid uses the Charm v2 table stack
The Terminal User Interface (TUI) SHALL run on `charm.land/bubbletea/v2`, render its repository grid with `charm.land/bubbles/v2/table`, and style that grid with `charm.land/lipgloss/v2`. The Bubbles table SHALL remain unfocused and SHALL NOT introduce row selection, scrolling, filtering, mouse behavior, or additional key bindings. The TUI SHALL preserve the existing summary, priority ordering, ten-row cap, responsive column visibility and widths, one-line rows, phase styling, infrastructure error, and quit footer.

#### Scenario: Bubbles table preserves the status grid
- **WHEN** retained repository states are rendered after the Charm v2 migration
- **THEN** the summary and table contain the same repository state and attention-priority projection required by the live status grid
- **AND** no more than ten complete repository rows are visible
- **AND** the active columns obey their existing terminal width thresholds
- **AND** every repository row occupies exactly one physical line

#### Scenario: The table remains non-interactive
- **WHEN** the TUI receives a table navigation key such as an arrow key, `j`, `k`, page up, or page down
- **THEN** the visible repository projection does not change because of that key
- **AND** `q` and `ctrl+c` retain their existing quit behavior

#### Scenario: Bubble Tea v2 retains alternate-screen cleanup
- **WHEN** the TUI starts and later exits normally or through context cancellation
- **THEN** its Bubble Tea v2 view declares alternate-screen rendering
- **AND** the alternate screen is released on exit
- **AND** the terminal cursor is restored
