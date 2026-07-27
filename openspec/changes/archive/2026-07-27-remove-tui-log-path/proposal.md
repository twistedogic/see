## Why

The TUI renders the per-invocation agent log path as an unbounded
second line under its repository row. A real path is ~90–100
characters
(`/Users/.../see/logs/<repo>--<change>--<UTC-ts>--<pid>.jsonl`),
which wraps to a third physical line on any terminal narrower than
the path. The renderer's height budget (`fitToHeight`) assumes that
row is exactly two lines, so a wrap undercounts the lines spent;
the next paint overflows the terminal height and bubbletea smears
the frame. The grid visibly corrupts whenever a row carries a path
that does not fit one line — which, on a default 80-column
terminal, is effectively every run.

Separately, the `LogPath` message sets the path on the row but
nothing ever clears it, so a repo that ran once keeps displaying a
stale path through `done`, `failed`, and the next idle cycle.

The path is already durable elsewhere: it lands in the batch-level
JavaScript Object Notation Lines (JSONL) event file and, under
`--mode=log` with piped stdout, on the stdout mirror. The TUI does
not need its own copy, and that copy is what corrupts the frame.

## What Changes

- The TUI SHALL NOT render the per-invocation agent log path
  anywhere in the grid. Each repository row SHALL occupy exactly
  one physical line, regardless of terminal width or path length.
- The height budget (`fitToHeight`) becomes single-line: every
  retained row costs exactly one line, so the budget and the
  rendered output can never diverge.
- The TUI-side plumbing for the path is deleted (the `LogPath` row
  field, the `LogPathMsg` type, the model's `LogPathMsg` case, and
  the `tuiObserver`'s `LogPath` case). Dead code is removed, not
  left inert.
- The `LogPath` **event** itself is unchanged. The watcher still
  emits it on a successful `agent.Run`, and it still reaches the
  batch JSONL file and the `--mode=log` stdout mirror. The event's
  contract (`watcher`, `event-log`) is untouched; only the TUI
  stops being a consumer.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `tui`: the grid-rendering requirement loses its log-path
  continuation clauses ("the existing log-path continuation SHALL
  remain associated with its repository row"; the short-terminal
  scenario's "a log-path continuation is rendered together with its
  repository row or omitted with that row") and gains an explicit
  prohibition on rendering the path. Each row is one physical line.
  The `LogPath` event remains in the observer sequence and the
  agent-output requirement, which are unchanged.

## Impact

- **Code**: roughly a dozen net-negative lines across two packages.
  - `tui/view.go`: remove the `if r.LogPath != ""` append in
    `renderRow`; collapse `fitToHeight` to single-line accounting
    (drop the two-line branch and the continuation comment).
  - `tui/model.go`: remove the `case LogPathMsg` update and the
    `LogPath` field on `RepoRow`.
  - `tui/events.go`: remove the `LogPathMsg` type.
  - `main.go`: remove the `case LogPath` arm of `tuiObserver`.
    The `LogPath` event type and its three emit sites are unchanged.
- **Specs**: one `MODIFIED` requirement on `tui`; no new
  capabilities. `watcher` and `event-log` are untouched.
- **Dependencies**: zero added, zero removed.
- **Behavior**: the agent log path is no longer visible in the TUI
  frame. It is unchanged in the batch JSONL file and the
  `--mode=log` stdout mirror. The garbling on narrow terminals is
  gone because no row can exceed one line.
- **Risk**: low. The change is deletion of a display path with no
  caller depending on the rendered path; the durable sinks are
  untouched and covered by existing tests. The one behavioral
  surprise — operators who read the path off the TUI must now read
  it from the JSONL file or `--mode=log` — is the intended
  trade-off and is captured by the spec delta and a regression
  test.
