## Context

The Terminal User Interface (TUI) model records `StartedAt` when `ChangeStartedMsg` moves a repository into `working`. Rendering currently shows `time.Since(StartedAt)` whenever that timestamp is nonzero, regardless of the row's current phase. Because completion, failure, and idle transitions do not clear the timestamp, AGE can display and continue increasing after active work has stopped.

## Goals / Non-Goals

**Goals:**

- Make AGE a live duration of active work only.
- Show the grid's existing `—` no-value placeholder for every non-working phase.
- Preserve the existing AGE column width threshold and one-second refresh behavior.

**Non-Goals:**

- Record or display completed-run duration.
- Track separate timestamps for every phase.
- Change phase transitions, watcher events, viewport ordering, or column widths.

## Decisions

### Gate AGE rendering on the working phase

Row rendering will calculate elapsed AGE only when the row is in `PhaseWorking` and `StartedAt` is nonzero. All other states render `—`.

This keeps presentation semantics at the rendering boundary and avoids adding timestamps or resetting model state solely to hide a value. Clearing `StartedAt` on every transition was rejected because it spreads a display rule across event handlers and would still require defensive rendering for inconsistent state.

### Keep the existing periodic tick

The existing one-second tick remains unchanged so working AGE continues to advance between lifecycle events. Although no row needs a changing AGE outside `working`, conditionally scheduling ticks is outside this change and would add lifecycle complexity for no user-visible benefit.

## Risks / Trade-offs

- **[Completed and failed rows no longer reveal run duration]** → Intentional: AGE communicates current activity, not historical duration; logs remain the source for historical timing.
- **[A working row somehow has no start timestamp]** → Render `—` rather than inventing a duration, preserving the established no-value behavior.
