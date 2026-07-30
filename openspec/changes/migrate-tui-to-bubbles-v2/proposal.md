## Why

The Terminal User Interface (TUI) maintains custom table layout, truncation, and viewport code that overlaps with the standard Charm Bubbles table component. Moving the Charm dependencies to their supported v2 modules and delegating the grid to Bubbles reduces local rendering machinery while retaining `see`'s existing status behavior.

## What Changes

- Upgrade Bubble Tea and Lip Gloss from their v1 GitHub module paths to `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2`.
- Add `charm.land/bubbles/v2` and render the repository grid with its table component.
- Adapt the TUI model, alternate-screen declaration, key handling, observer bridge, and tests to Bubble Tea v2.
- Preserve the current summary, repository prioritization, ten-row cap, responsive columns, phase styling, error display, AGE updates, infrastructure error, and quit footer.
- Keep the table non-interactive: this change does not add row selection, scrolling, or new key bindings.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `tui`: Require the live status grid to use the Charm v2 stack and the Bubbles v2 table while preserving its existing operator-visible behavior.

## Impact

- Affects `go.mod`, `go.sum`, and the `tui` package and its tests.
- Replaces the existing Bubble Tea and Lip Gloss v1 dependencies with their v2 modules and adds Bubbles v2.
- Changes the internal TUI model interface from Bubble Tea v1's string view to Bubble Tea v2's declarative view; no command-line, event, or JavaScript Object Notation Lines (JSONL) contract changes.
- Should follow the active `show-age-only-while-working` and `shorten-tui-errors` changes so the migration preserves their settled behavior.
