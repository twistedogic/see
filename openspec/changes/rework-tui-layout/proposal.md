## Why

The current Terminal User Interface (TUI) renders every scanned repository in one unbounded list, so large watch sets push the actionable rows off-screen. Its phase summary already exists in the footer, but is harder to see at a glance and does not indicate how many repositories are currently visible. Reworking the layout will keep aggregate visibility while making active repository work the primary focus.

## What Changes

- Add a summary table at the top of the TUI showing counts across all known repositories: total, working, done, idle, failed, warnings, and the visible-row count.
- Replace the unbounded repository list with a priority viewport containing at most ten repository entries.
- Prioritize visible rows in this order: working repositories, failed repositories, warning-bearing repositories, then remaining repositories by most recent meaningful activity.
- Track repository activity from meaningful watcher events such as start, retry, completion, failure, and warning; repeated `RepoSeen` polling events SHALL NOT continually refresh activity order.
- Retain all repository rows and their state in the model so summary counts remain global and repositories can return to the viewport when their priority changes.
- Keep the existing repository columns, warning glyph, phase rendering, change values, retry information, age, and log-path display unless layout space requires showing fewer than ten entries.
- Move the phase and warning counts out of the footer; leave the footer for the quit hint and other control/status text.
- Update the TUI capability requirements and regression tests for the summary table, global counts, priority ordering, recency behavior, and ten-entry limit.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `tui`: Change the live status grid from rendering every scanned repository to rendering a global summary plus a priority viewport of at most ten entries.

## Impact

- Affected code: `tui/model.go`, `tui/view.go`, and `tui/tui_test.go`.
- The watcher event types and event adapter do not need new public events; the TUI will derive activity ordering from existing messages.
- The watcher, agent execution, retry behavior, repository processing order, and JSONL event output remain unchanged.
- No new dependencies or external APIs are required.
