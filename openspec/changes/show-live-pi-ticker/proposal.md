## Why

The Terminal User Interface (TUI) shows that `pi` is working but hides what it is currently doing, forcing operators to inspect a large per-invocation JavaScript Object Notation Lines (JSONL) file for progress. A bounded one-row ticker can expose useful live activity without letting raw agent output corrupt or dominate the alternate-screen grid.

## What Changes

- Add a single animated ticker row at the bottom of the TUI showing the latest meaningful `pi` activity.
- Keep `pi ›` fixed on the left and the existing `[q] quit` hint fixed on the right; horizontally scroll only overflowing activity text between them.
- Parse `pi --mode json` output as it is captured and summarize assistant text completions, tool starts, tool completions, and non-JSON diagnostics.
- Ignore token deltas, thinking content, repeated tool progress snapshots, and other noisy events.
- Preserve the complete raw `pi` byte stream in the existing per-invocation JSONL file and prevent those bytes from writing directly to the terminal.
- Keep activity delivery out of the batch-level watcher event stream so its schema and event ordering remain unchanged.
- Depend on `migrate-tui-to-bubbles-v2`; this change does not alter the repository table migration.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `tui`: Add the live animated `pi` activity ticker and refine the silent-agent-output contract to permit only parsed, bounded activity summaries inside the TUI.

## Impact

- Affects `PiAgent` output capture, the internal agent/watcher callback boundary, TUI messages and model state, footer rendering, animation scheduling, and tests.
- Does not change command-line flags, workflow execution, retry behavior, the batch-level JSONL schema, or the per-invocation raw JSONL format.
- Must be implemented after `migrate-tui-to-bubbles-v2` so the ticker targets the settled Bubble Tea v2 model and view contract.
