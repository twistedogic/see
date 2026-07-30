## ADDED Requirements

### Requirement: The TUI renders live pi activity as an animated ticker
The Terminal User Interface (TUI) SHALL render exactly one footer row containing a fixed `pi ›` prefix, the latest meaningful sanitized `pi` activity, and the fixed `[q] quit` hint. Completed assistant text, tool execution starts, tool execution outcomes, and non-JSON diagnostics SHALL be eligible ticker activities. Thinking content, token-level text deltas, repeated tool progress updates, raw message snapshots, and unknown JSON events SHALL NOT be rendered.

Each new activity SHALL replace the previous activity and reset its display offset. Before any invocation the activity SHALL be `waiting`; a `ChangeStarted` event SHALL reset it to `starting`; the latest activity SHALL remain after the invocation ends until another invocation starts.

When the activity fits between the fixed prefix and quit hint, it SHALL remain stationary and SHALL NOT schedule marquee updates. When it overflows, only the middle activity window SHALL move horizontally, one terminal display cell at a time, with a visible gap between repetitions. The prefix and quit hint SHALL remain stationary. The footer SHALL occupy exactly one physical line at every terminal size; when width is insufficient, the quit hint SHALL take precedence over the prefix and activity.

#### Scenario: Tool activity updates the ticker live
- **WHEN** the active `pi` invocation emits a tool execution start for `bash` with command `go test ./...`
- **THEN** the ticker displays a sanitized activity identifying `bash` and `go test ./...`
- **AND** the update appears before the invocation exits

#### Scenario: Tool completion replaces its start activity
- **WHEN** a displayed tool execution later completes
- **THEN** the ticker replaces the start marker with a success or failure marker for the same summarized tool
- **AND** it does not render the tool's full result body

#### Scenario: Assistant narrative replaces previous activity
- **WHEN** `pi` emits a completed assistant text event
- **THEN** the ticker displays that text with whitespace collapsed to one line
- **AND** thinking and token-delta events do not independently update the ticker

#### Scenario: Overflowing activity animates within fixed chrome
- **WHEN** sanitized activity is wider than the space between `pi ›` and `[q] quit`
- **THEN** successive marquee ticks advance the visible activity by terminal display cells
- **AND** the prefix and quit hint remain at fixed positions
- **AND** the footer remains one physical line

#### Scenario: Fitting activity does not animate
- **WHEN** sanitized activity fits in the available ticker window
- **THEN** the activity remains stationary
- **AND** no marquee tick is rearmed for that activity

#### Scenario: New activity restarts at its beginning
- **WHEN** a marquee is partway through an overflowing activity and a new meaningful activity arrives
- **THEN** the ticker replaces the old activity
- **AND** the new activity is first rendered from display offset zero

#### Scenario: Narrow terminal preserves quit access
- **WHEN** the terminal is too narrow to render the full prefix, activity, and quit hint
- **THEN** `[q] quit` remains visible when the terminal can contain it
- **AND** ticker content is omitted or width-bounded rather than wrapping

#### Scenario: Terminal controls are not interpreted
- **WHEN** assistant text, a tool argument, or a diagnostic contains terminal escape sequences or control characters
- **THEN** those controls are stripped or collapsed before the activity enters TUI state
- **AND** the original bytes remain unchanged in the per-invocation log

## MODIFIED Requirements

### Requirement: Agent output does not corrupt the TUI
In both `--mode=tui` and `--mode=log`, `PiAgent.Run` SHALL direct the agent's complete stdout and stderr byte stream to the per-invocation JavaScript Object Notation Lines (JSONL) file in the log directory. The agent's raw bytes SHALL NOT write directly to stdout or stderr of the `see` process under any mode. Per-invocation JSONL files SHALL exist for every successful `PiAgent.Run` call; the file path SHALL be reported via the `LogPath` event in both modes.

In TUI mode only, `see` SHALL parse complete output lines as a best-effort presentation side channel and MAY render only the bounded, sanitized semantic activities defined by the live ticker requirement. Parsing, sanitizing, truncating, or ignoring a line for display SHALL NOT modify the raw bytes written to the per-invocation JSONL file. These transient activities SHALL NOT be added to the batch-level watcher event stream or its JSONL schema. In log mode, no ticker callback SHALL be configured and output behavior SHALL remain unchanged.

#### Scenario: Agent output reaches the per-invocation JSONL and bounded ticker
- **WHEN** `--mode=tui` is set and the agent writes valid pi JSON events and diagnostic lines to its stdout and stderr
- **THEN** the complete original bytes appear in the per-invocation JSONL file whose path is emitted via the `LogPath` event
- **AND** those raw bytes do not write directly to the TUI, stdout, or stderr
- **AND** only eligible sanitized activity summaries may appear in the one-row ticker
- **AND** no activity summary is inserted into the batch-level watcher event stream

#### Scenario: Log mode does not parse ticker activity
- **WHEN** `--mode=log` is set and the agent writes output
- **THEN** the complete output is captured in the per-invocation JSONL file
- **AND** no TUI activity callback is configured
- **AND** existing batch-level JSONL and stdout-mirror event ordering is unchanged

#### Scenario: Oversized or malformed output cannot fail the agent
- **WHEN** an output line is malformed JSON or exceeds the ticker parser's retained-line limit
- **THEN** its original bytes are still written to the per-invocation JSONL file
- **AND** parser handling does not change the agent process's exit result
- **AND** retained parser and ticker state remain bounded
