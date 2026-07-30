## Context

`PiAgent.Run` invokes `pi --mode json --no-session` and directs both standard output and standard error to one per-invocation JavaScript Object Notation Lines (JSONL) file. The Terminal User Interface (TUI) receives lifecycle events such as `ChangeStarted` and `ChangeDone`, but receives no output while the agent runs. The `LogPath` event is emitted only after `Run` returns and is intentionally not consumed by the TUI.

Pi's JSON mode emits a verbose event stream containing token-level text and thinking deltas, complete message snapshots, tool calls, tool progress, and tool results. Displaying raw lines would overwhelm the grid and could render terminal control sequences supplied by agent content. The useful operator signal is much smaller: completed assistant narrative and the current tool's concise identity and outcome.

This change follows `migrate-tui-to-bubbles-v2`, so it targets Bubble Tea v2's model and declarative view. Watcher runs are sequential; at most one `pi` process is active, so one global activity ticker is sufficient.

## Goals / Non-Goals

**Goals:**

- Show the latest meaningful `pi` activity live in one animated footer row.
- Preserve the raw per-invocation JSONL byte stream unchanged.
- Keep the batch-level watcher event schema and ordering unchanged.
- Bound and sanitize all terminal-visible agent content.
- Animate only when content exceeds the available footer width.

**Non-Goals:**

- Show raw JSON, thinking content, token deltas, full tool output, or an activity history.
- Add a selectable output pane, scrolling controls, pause controls, configuration, or another command-line flag.
- Reintroduce the per-invocation log path into the TUI.
- Change agent execution, retries, workflow ordering, or log-mode output.

## Decisions

### Tee capture through a best-effort semantic parser

In TUI mode, `PiAgent.Run` will write every output byte to the existing invocation file before feeding the same bytes to a newline accumulator. Complete lines will be decoded with Go's standard `encoding/json` package. Parse failures are treated as plain diagnostic lines for the ticker; parser or display failures never fail the agent after the raw write succeeds.

The parser will emit only:

- completed assistant text from text-completion events;
- tool starts summarized from the tool name and useful known argument;
- tool completions correlated by tool-call identifier and marked success or failure; and
- non-JSON diagnostic lines.

Thinking events, text deltas, tool progress updates, session/message snapshots, and unknown events are ignored. Known tools use `command` for `bash` and `path` for `read`, `edit`, and `write`; unknown tools display only their name. Successful tool results show status plus the original summary, not result content. Failed results show a bounded first diagnostic when one is safely extractable, otherwise the failed tool summary.

The line accumulator will impose a finite parsing limit. Oversized lines continue to the raw file but are skipped for display until their newline, preventing a large message snapshot from growing TUI parser memory without bound.

Alternatives considered:

- Tail the invocation file: rejected because its path is currently reported only after the process exits, and file polling adds latency and lifecycle cleanup.
- Run `pi` in text mode: rejected because it would lose the structured events needed for tool activity and change the durable JSONL format.
- Display raw JSON: rejected because real logs contain thousands of repetitive snapshots and deltas.

### Deliver activity through a TUI-only callback

The internal agent call boundary will accept an optional activity callback. `runTUI` wires it directly to a TUI message sender; log mode leaves it nil, preserving the current direct-to-file path and output behavior. The watcher supplies no activity events to `eventLogger`.

This side channel is intentionally separate from the sealed watcher `Event` stream. Adding transient ticker updates there would insert new records between established phase events, change the public batch JSONL ordering, and duplicate data already preserved in the invocation log. The TUI remains a view of watcher events for lifecycle state while this explicitly presentation-only channel summarizes the agent's separate raw stream.

A callback was preferred over importing the TUI package into `PiAgent`, preserving the main-to-TUI adapter boundary and keeping fake agents simple.

### Render one combined ticker and quit footer

The existing footer becomes one physical line:

```text
pi › <activity window>    [q] quit
```

The prefix and quit hint remain fixed. Only the middle activity window moves. Before the first activity it displays `waiting`; `ChangeStarted` resets it to `starting`. The latest semantic activity replaces the previous text and resets the marquee offset to zero. The last activity remains visible after completion until another run starts.

Combining ticker and hint avoids taking height from the repository viewport. A separate output pane and a second footer row were rejected because the user selected a one-row news-ticker presentation.

### Use a bounded, width-aware marquee

When the sanitized activity fits, it remains static and no animation command is scheduled. When it overflows, a marquee tick advances the middle window by one terminal display cell at a short fixed interval. The logical loop is the activity followed by a visible separator and the activity again, producing a gap before repetition.

Terminal display-width functions, rather than rune counts or byte slicing, will calculate and cut the window so wide Unicode characters do not wrap or split incorrectly. On resize or new activity, the offset is normalized; if the terminal is too narrow for all parts, the quit hint takes priority, followed by the fixed `pi ›` prefix, and no content may wrap.

The marquee uses its own tick only while overflow exists. The existing one-second AGE tick remains unchanged; using that tick for animation was rejected as visibly jerky, while accelerating the AGE tick would redraw the whole TUI unnecessarily when no ticker is moving.

A fixed speed is intentionally not configurable. Add configuration only if real use shows the selected readable default is inadequate.

### Sanitize display text without modifying the raw log

Before an activity reaches model state, terminal escape sequences, carriage returns, line feeds, and remaining control characters will be removed or collapsed to spaces. The sanitized text is then bounded for retained model memory. The invocation JSONL file retains the exact original bytes.

This is required because assistant text, tool arguments, and diagnostics are not trusted terminal instructions. Truncation alone is insufficient: an escape sequence can alter terminal state without occupying visible width.

## Risks / Trade-offs

- **[The summary parser depends on pi's documented JSON event shapes]** → Decode only the small fields required, ignore unknown fields and event types, and keep malformed lines non-fatal.
- **[Merging standard output and standard error can present a diagnostic as non-JSON]** → Preserve the merged raw stream and intentionally render complete non-JSON lines as sanitized diagnostics.
- **[Frequent activity prevents a long line from completing one marquee cycle]** → Reset to the newest information; recency is more useful than finishing stale text.
- **[Animated text can distract]** → Animate only overflowing content, keep prefix and quit hint stationary, and stop scheduling immediately when content fits.
- **[A callback could block the process output copier]** → Emit only semantic completion/start/end events rather than token or progress deltas; Bubble Tea message delivery remains bounded by the active program lifecycle.
- **[Agent content contains terminal escapes or very large records]** → Sanitize before rendering and cap parser/display memory while preserving raw bytes on disk.

## Migration Plan

1. Complete and archive `migrate-tui-to-bubbles-v2`.
2. Add parser regression tests using representative pi JSON events, malformed diagnostics, oversized lines, and terminal escape content.
3. Add callback plumbing while retaining byte-for-byte invocation log tests and unchanged batch event-order tests.
4. Add ticker model state, responsive one-line rendering, and conditional marquee scheduling with deterministic tests.
5. Run the bounded Go test suite and strict OpenSpec validation.

Rollback is a normal source revert; no persistent data or configuration migration is involved.

## Open Questions

None.
