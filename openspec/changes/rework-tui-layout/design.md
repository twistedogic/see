## Context

The current Terminal User Interface (TUI) stores all repository rows in `Model.rows`, preserves first-observed paths in `Model.order`, renders every row in that order, and derives phase counts in the footer. The watcher emits `RepoSeen` for every repository on every polling pass, so that event is a scan heartbeat rather than useful activity recency. The existing model already receives meaningful lifecycle messages for starts, retries, completions, failures, and warnings.

The change is limited to the TUI presentation and its in-memory view metadata. Watcher processing order, event payloads, agent execution, retry behavior, and event logging remain unchanged.

## Goals / Non-Goals

**Goals:**

- Make aggregate repository health visible at the top of the screen.
- Keep summary counts accurate for every repository known to the TUI, even when only ten rows are visible.
- Keep the visible list bounded and prioritize repositories that need attention.
- Make activity recency deterministic and independent of repeated polling heartbeats.
- Preserve the current row information and warning semantics where the terminal has room.
- Use the terminal height to avoid overflowing the summary, table header, rows, and footer.

**Non-Goals:**

- Changing watcher events or adding a new event transport.
- Changing repository scan order, retry policy, workflow behavior, or agent execution.
- Persisting TUI state between processes.
- Adding scrolling, selection, filtering, or interactive repository controls.
- Changing the JSONL event schema or log files.

## Decisions

### Retain all rows and derive a bounded render list

`Model.rows` will continue to retain every repository observed during the process. `View` will derive a render-only slice containing no more than ten rows. Summary counts will iterate over all retained rows, while the top table will separately report how many rows are visible.

This is preferred over deleting rows because repositories must be able to re-enter the viewport when their phase or activity changes, and global counts must not depend on which rows happen to be visible.

### Track meaningful activity with a monotonic sequence

The model will maintain a monotonically increasing activity sequence. Each row will store its latest activity sequence and a discovery sequence for stable fallback ordering. Existing rows will not receive a new activity sequence for repeated `RepoSeenMsg` events. A newly discovered row will receive a discovery sequence so it can appear in the viewport before any meaningful lifecycle event occurs.

`ChangeStartedMsg`, `RetryAttemptMsg`, `ChangeDoneMsg`, `ChangeFailedMsg`, and `WarningMsg` will advance the sequence for their repository. A sequence is preferable to wall-clock timestamps because Bubble Tea processes updates serially and the ordering remains deterministic in tests and when multiple events share a timestamp.

### Use status priority before activity recency

The render list will rank rows by a fixed attention class, then by descending activity sequence, then by stable discovery order. The attention classes are:

1. `PhaseWorking` — work is currently in progress.
2. `PhaseFailed` — the latest attempt failed.
3. rows with an active warning.
4. all remaining rows.

This ensures an old failure or warning is not displaced by routine completion activity. Within a class, the most recently meaningful activity appears first. The existing row phase and warning state remain the source of truth; ranking does not mutate either one.

### Put the aggregate summary above the repository table

The top section will show the total number of known repositories, counts for working, done, idle, failed, and warnings, and the viewport count in the form `visible / total`. The counts are derived on each render from all rows, so they remain live without additional event state.

The phase and warning counts will be removed from the footer to avoid duplicating the same information. The footer will retain the quit hint and any existing process-level error presentation.

### Respect terminal height while preserving the ten-row ceiling

The view will reserve space for the summary, repository header, footer, and any infrastructure error. It will render up to ten repository entries within the remaining height. A repository entry that includes a log-path continuation line consumes both physical lines. A short terminal may therefore show fewer than ten entries; it will never show more than ten or split an entry unexpectedly.

The existing width-based column behavior and truncation remain in place. The summary must use a compact layout at narrow widths rather than changing watcher or row semantics.

## Risks / Trade-offs

- **Active failures or warnings can permanently occupy viewport slots.** → This is intentional attention prioritization; newer ordinary activity fills only the remaining slots.
- **Ignoring repeated `RepoSeenMsg` events means an idle repository is not made recent by polling.** → New rows receive discovery order, and meaningful lifecycle events refresh activity; this avoids scan-order churn.
- **Log-path continuation lines reduce the number of visible entries on short terminals.** → Count the limit in repository entries, honor available height, and preserve the existing log-path observability.
- **A summary table consumes vertical space previously available to rows.** → Use the height budget and render fewer rows when necessary; the requirement is an upper bound of ten, not a promise that ten fit in every terminal.
- **Priority ordering is a presentation policy that may surprise users expecting scan order.** → Keep deterministic tie-breaking and label the section as a prioritized repository view; the global summary still covers every retained row.

## Migration Plan

No persisted data, event schema, or configuration migration is required. The change can be rolled out by updating the TUI package and its tests. Reverting the code restores the previous all-rows grid and footer summary; watcher behavior and log output are unaffected in either direction.

## Open Questions

None. The viewport is intentionally defined as an attention-prioritized list rather than a literal last-ten scan slice.
