## Why

The Terminal User Interface (TUI) truncates long failure messages before their actionable detail appears. For example, a dirty-working-tree failure spends the available ERROR cell on a repeated repository path and hides the fact that the tree is dirty and must be committed or stashed.

## What Changes

- Introduce a custom dirty-working-tree error that provides both a full diagnostic and a concise summary.
- Carry the concise summary through watcher events only for in-memory TUI consumption while retaining the full error in JavaScript Object Notation Lines (JSONL) event logs.
- Render the concise summary in retry, final-failure, and watcher infrastructure error locations when an error provides one.
- Preserve existing full-message behavior for errors without a concise summary.
- Replace the two duplicate dirty-working-tree error constructions with the shared custom error.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `tui`: Render an error-provided concise summary instead of its full diagnostic while preserving the existing fallback for ordinary errors.

## Impact

- Affected code: watcher error construction and event emission in `main.go`, plus the main-to-TUI observer adapter.
- Event logs retain the existing exported `Err` field and full diagnostic text; no JSONL schema change is required.
- TUI message types, model state, column widths, truncation, and rendering rules remain unchanged.
- No new dependencies or command-line configuration are introduced.
