## Context

The Terminal User Interface (TUI) currently uses Bubble Tea v1 for the update loop and Lip Gloss v1 plus local functions for table headers, cells, truncation, and height fitting. Repository lifecycle state, priority ordering, responsive column selection, summary counts, and the ten-row cap are application behavior and must remain owned by `see`.

Bubbles v2 provides a table component, but it imports Bubble Tea v2 and Lip Gloss v2. The three Charm modules therefore have to migrate together. Bubble Tea v2 also changes the model view from `string` to `tea.View`, replaces `tea.KeyMsg` with `tea.KeyPressMsg`, and moves alternate-screen selection from a program option into declarative view state.

Two active changes refine the behavior that this migration must preserve: `show-age-only-while-working` changes AGE presentation, and `shorten-tui-errors` supplies concise display errors without changing the event log. They should land before this change.

## Goals / Non-Goals

**Goals:**

- Use `charm.land/bubbles/v2/table` for grid headers, cells, truncation, and viewport rendering.
- Migrate Bubble Tea and Lip Gloss to their v2 module paths as one coherent dependency update.
- Preserve current operator-visible TUI behavior and watcher/event contracts.
- Keep repository state and ranking independent of the presentation component.

**Non-Goals:**

- Add table focus, selection, scrolling, filtering, mouse support, or navigation keys.
- Add other Bubbles components such as help, spinner, list, or progress.
- Change summary contents, column thresholds or widths, row priority, the ten-row cap, or event semantics.
- Redesign the TUI or create a reusable table abstraction.

## Decisions

### Use the Bubbles table only at the presentation boundary

`Model` will remain the source of truth for `RepoRow` values, lifecycle transitions, activity sequencing, and terminal dimensions. Rendering will project the already-prioritized and height-fitted repository rows into Bubbles `table.Column` and `table.Row` values.

The table will not become the domain model. This avoids synchronizing lifecycle state into a presentation component and keeps summary counts based on all retained repositories, including rows outside the visible projection.

Using Bubbles `list` instead was rejected because the interface is a columnar status grid, not a collection of titled items. Keeping the custom grid was rejected because adopting the standard table is the purpose of this change.

### Preserve the non-interactive capped viewport

The table will remain unfocused and receive only the rows selected by the existing priority and height rules. Its built-in cursor and scrolling key map will therefore not change input behavior; only `q` and `ctrl+c` remain application key handling.

Passing every retained repository to a focused table was rejected because it would replace the established automatic attention-priority viewport with user-controlled navigation and would change the meaning of the visible counter.

### Override table defaults to preserve layout

Bubbles table defaults add horizontal cell padding, bold headers, and selected-row styling. The TUI will provide table styles with no added padding and no visible selection so existing column-width arithmetic remains valid. Existing phase strings can retain their Lip Gloss colors as table cell values.

Responsive column construction remains local: AGE is included at its existing threshold, and ERROR receives the terminal width left after active fixed columns and appears only when that remainder reaches its minimum width. The table handles cell-width truncation after those columns are selected.

Accepting default styles was rejected because padding would make rows exceed the calculated terminal width and selection highlighting would introduce an interaction state the TUI does not expose.

### Isolate Bubble Tea v2's declarative view from content rendering

The model's required `View()` method will return `tea.View`, set `AltScreen`, and use a separate string-rendering boundary for summary, table, infrastructure error, and footer content. Rendering tests can inspect that content without depending on terminal control behavior.

Program construction retains `tea.WithContext` but removes `tea.WithAltScreen`, because Bubble Tea v2 declares alternate-screen behavior on the returned view. Key handling and key-message test fixtures migrate to `tea.KeyPressMsg`.

### Migrate all Charm modules together

Imports will move to:

- `charm.land/bubbletea/v2`
- `charm.land/bubbles/v2/table`
- `charm.land/lipgloss/v2`

The old `github.com/charmbracelet/bubbletea` and `github.com/charmbracelet/lipgloss` direct dependencies will be removed. The implementation will use mutually compatible stable v2 releases and let Go's Minimal Version Selection resolve transitive versions.

A mixed v1/v2 Charm stack was rejected because Bubbles v2's component types use Bubble Tea v2 messages and Lip Gloss v2 styles, making parallel stacks both incompatible at the model boundary and unnecessarily duplicative.

## Risks / Trade-offs

- **[Bubbles cell sizing differs from the custom renderer]** → Use zero-padding styles and retain width-threshold and one-line regression tests at narrow and wide terminal sizes.
- **[The table's cursor leaks a selected style into output]** → Keep the table unfocused and make selected styling visually identical to ordinary cells; test multi-row output.
- **[Bubble Tea v2 changes terminal lifecycle behavior]** → Declare alternate-screen mode in every returned `tea.View` and retain shutdown tests around `q`, context cancellation, and program errors.
- **[The migration obscures behavior changes from active TUI proposals]** → Apply `show-age-only-while-working` and `shorten-tui-errors` first, then require the migration tests to preserve both outcomes.
- **[The table is initially used without its navigation features]** → Intentional: standard rendering removes local machinery without expanding the user interface. Add focus and scrolling only through a later behavior proposal.

## Migration Plan

1. Land the two active TUI behavior changes.
2. Add regression coverage that characterizes current grid content, responsive columns, priority cap, and non-interactive keys through the Bubble Tea v2-facing model contract.
3. Upgrade the three Charm dependencies and adapt Bubble Tea model, view, key, and program construction interfaces.
4. Replace custom header and cell joining with a zero-padding, unfocused Bubbles table fed by the existing visible-row projection.
5. Remove rendering helpers and styles made redundant by the table, then run the bounded Go test suite and strict OpenSpec validation.

Rollback is a normal source revert: there is no stored data, configuration, or external protocol migration.

## Open Questions

None.
