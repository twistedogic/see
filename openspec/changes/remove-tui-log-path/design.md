## Context

`see`'s TUI (`tui` package) renders a priority grid of scanned repos
via Bubble Tea. Two implementation facts combine to corrupt that grid:

1. **Unbounded path render.** `renderRow` (`tui/view.go`) appends the
   per-invocation log path as a raw second line under its row when the
   row carries a `LogPath`:
   ```go
   if r.LogPath != "" {
       row += "\n      " + r.LogPath   // no truncation, no width bound
   }
   ```
   The other columns are width-bounded (`colRepo.Width(24)`, etc.);
   the path line is not. A real path
   (`<logDir>/<repo>--<change>--<UTC-ts>--<pid>.jsonl`) is ~90–100
   characters, so it wraps to a third physical line on any terminal
   narrower than the path.

2. **Mismatched height budget.** `fitToHeight` assumes a row with a
   `LogPath` is exactly two physical lines:
   ```go
   rowLines := 1
   if r.LogPath != "" { rowLines = 2 }
   ```
   A wrapped path spends three lines, so the budget undercounts;
   `View()` emits more rows than fit the terminal height, and the
   next Bubble Tea paint smears the frame.

A third fact makes the clutter persistent: `LogPathMsg` sets
`r.LogPath` but no lifecycle message (`ChangeDone`, `ChangeFailed`,
idle `RepoSeen`) ever clears it, so a repo that ran once keeps a
stale path on its row indefinitely.

The path is already durable through two other sinks, neither of
which can corrupt an interactive TTY:

- the **batch JSONL event file** (every event is written there,
  always, in both modes), and
- the **`--mode=log` stdout mirror**, which is wired *only* when
  stdout is not a terminal (`!term.IsTerminal`), i.e. piped to `jq`
  or a file — never to the interactive terminal the TUI owns.

So the TUI is the one sink that both (a) renders to a TTY and (b)
renders the path. Removing it from the TUI removes the only path-to-
frame channel that can garble, and leaves the durable sinks intact.

## Goals / Non-Goals

**Goals:**

- Stop the TUI from rendering the per-invocation agent log path.
- Guarantee every repository row is exactly one physical line, so
  the height budget and the rendered output can never diverge.
- Delete the now-dead TUI-side plumbing for the path (field, msg,
  model case, observer case) rather than leave it inert.
- Preserve the `LogPath` **event** and its durable sinks (batch
  JSONL file, `--mode=log` stdout mirror) unchanged.

**Non-Goals:**

- Changing what the `LogPath` event carries, when it is emitted, or
  which sinks receive it. The `watcher` and `event-log` contracts
  are untouched.
- Truncating or otherwise still-rendering the path in the TUI. The
  decision is to not render it at all (see Decisions).
- Clearing-on-done or any success-conditional display policy. Once
  the path is never rendered, "when to clear it" is moot.
- Adding a CLI flag or config knob to toggle TUI path display.

## Decisions

### Decision 1: TUI-render-only — keep the event, drop only the render

**Choice.** The watcher continues to emit `LogPath` on a successful
`agent.Run`, and the event continues to reach the batch JSONL file
and the `--mode=log` stdout mirror. Only the TUI stops consuming it.

**Rationale.** The reported defect — "the broadcast malforms the
TUI" — is localized to the interactive TTY frame. The two other
sinks are categorically incapable of causing it: the JSONL file is
not a terminal at all, and the stdout mirror is wired exclusively
when stdout is *not* a terminal. Killing the event to fix a TUI-only
defect would also destroy a tested, durable contract for no gain:
`watcher` spec requires the event in both modes, `event-log` spec
requires it in the JSONL stream, and `TestWorkEmitsLogPathOn
SuccessfulCapture` (and siblings) assert it. The smallest change
that cures the defect is to stop the one sink that both renders to a
TTY and renders the path.

**Alternatives considered.**

- *Kill the `LogPath` event entirely.* Rejected: it would break the
  `watcher`/`event-log` contracts and their tests, and remove a
  useful machine-readable record, to fix a TUI rendering bug. Over-
  reach.
- *Clear the path on `ChangeDone` (and idle), keep it while
  working/failed.* Rejected as the primary fix: it mitigates but
  does not cure the wrap — a `working` row with a long path still
  wraps and smears mid-run. It also reintroduces a success-
  conditional display policy that complicates the model for a
  benefit (live tailing from the TUI) the operator did not ask to
  keep. If live tailing later becomes a requirement, a width-bounded
  detail view is the right shape, not a per-row second line.
- *Truncate the path to terminal width, keep rendering it.* Rejected
  as the primary fix: it cures the wrap but preserves a row that
  costs two lines (halving viewport capacity) to show a path the
  operator can already read from the JSONL file or `--mode=log`.
  Truncation is also lossy in the wrong direction — the useful part
  of the path is the filename tail, which naive truncation drops
  first.

### Decision 2: Delete the dead TUI-side plumbing, do not leave it inert

**Choice.** Remove, in one pass: the `r.LogPath` field on
`RepoRow` (`tui/model.go`), the `LogPathMsg` type (`tui/events.go`),
the `case LogPathMsg` in the model's `Update` (`tui/model.go`), and
the `case LogPath` arm of `tuiObserver.Observe` (`main.go`). Also
remove the now-obsolete test `TestViewRendersLogPathWhenSet`
(`tui/tui_test.go`).

**Rationale.** Once `renderRow` no longer reads `r.LogPath`, that
field, its setter message, its model case, and its observer arm have
zero consumers. Leaving them is dead weight that a future reader
must trace and dismiss. Deletion is smaller in steady state than
guarded no-ops and matches the project's deletion-over-addition
norm. The `LogPath` **event type** (`main.LogPath`) and its three
emit sites are untouched — they feed the durable sinks.

The `eventLogger.secondary` (the `tuiObserver`) still receives every
event including `LogPath`; with the `case LogPath` arm removed, the
type switch simply falls through. That is a cheap no-op (one switch
miss per `LogPath` event) and is not worth filtering earlier — the
event still has to reach the file and the mirror, so it cannot be
suppressed at the source.

### Decision 3: Single-line height budget

**Choice.** Collapse `fitToHeight` (`tui/view.go`) to single-line
accounting: every retained row costs exactly one line. Drop the
`rowLines = 2` branch and its comment about the log-path
continuation.

**Rationale.** The two-line branch existed solely to budget the path
continuation. With the continuation gone, the budget is trivially
`budget -= 1` per row and can no longer disagree with the rendered
height. This is the structural fix that makes the wrap defect
impossible to reintroduce for this row type.

## Risks / Trade-offs

- **[Operators lose the path in the TUI]** → The path is still in the
  batch JSONL file (always) and on `--mode=log` stdout (when piped).
  The TUI's `[q] quit` footer does not advertise it; operators who
  relied on reading it off the frame switch to `jq` or the file.
  This is the intended trade-off and is captured by the spec delta
  and a regression test. *Accepted.*
- **[A future need to tail the live log from the TUI]** → Out of
  scope. If it arises, a width-bounded detail/selection view is the
  right shape, not a second line per row. The deletion does not
  block that: the `LogPath` event and its path are still emitted and
  available to any future consumer. *Accepted.*
- **[Removing `case LogPath` from `tuiObserver` could hide a future
  regression where the event stops being emitted]** → The event's
  emission is independently asserted by `watcher`-level tests
  (`TestWorkEmitsLogPathOnSuccessfulCapture`,
  `TestWorkDoesNotEmitLogPathOnCaptureFailure`) and governed by the
  `watcher`/`event-log` specs, which are unchanged. The TUI was
  never the canonical assertion of emission. *Mitigated.*
