# event-log delta — add-jsonl-entry-timestamp

## MODIFIED Requirements

### Requirement: see writes a batch-level JSONL event stream
When `see` starts, after the run mode has been resolved but before
the watcher begins, `see` SHALL create one JSONL file at
`$SEE_LOG_DIR/see--<utc-timestamp>--<pid>.jsonl` (or
`os.UserCacheDir()/see/logs/see--<utc-timestamp>--<pid>.jsonl` if
`SEE_LOG_DIR` is unset). Each `Event` value the watcher emits
SHALL be wrapped in an envelope `{"ts": "<RFC3339Nano UTC>",
"event": <event-payload>}` and written as one line to that file,
in the order the events fire, before the file is flushed for that
event. The `ts` field SHALL be the observed-at time the
`eventLogger` marshalled the event (not the wall-clock time the
watcher created the event struct), expressed as the Go layout
`time.RFC3339Nano` formatted on a value in UTC. Under concurrent
producers this keeps the line ordering monotonic against the
wall clock of the writer, not the wall clock of the producer.

The envelope wraps but does not modify the underlying event
payload: a line for `RepoSeen{Path: "/x", HasOpenspec: true}`
SHALL marshal to
`{"ts":"<rfc3339nano>","event":{"Path":"/x","HasOpenspec":true}}`,
with the inner payload byte-identical to its pre-envelope shape.

#### Scenario: One file per batch
- **WHEN** `see` is invoked twice in sequence from the same
  process identifier (PID)
- **THEN** two distinct JSONL files exist under the log directory
- **AND** each file's name encodes a unique UTC timestamp

#### Scenario: Every watcher event lands in the JSONL
- **WHEN** `Watcher.runOnce` runs against one repo with one active
  change and the agent succeeds
- **THEN** the JSONL contains, in order, `RepoSeen`,
  `ChangeStarted`, `LogPath`, `ChangeDone`
- **THEN** every line is valid JSON encoding of
  `{"ts": <rfc3339nano-string>, "event": <event-payload>}`
- **AND** the inner `event` payload round-trips the original
  `Event` field set without renaming or reordering

#### Scenario: Each line carries an RFC3339Nano UTC timestamp
- **WHEN** the JSONL is read with `time.Parse(time.RFC3339Nano, ...)`
  on every `ts` value
- **THEN** the parse succeeds for every line
- **THEN** the parsed time lies within one second of the wall
  clock at process exit (sanity bound; the test runs within
  the same process)

#### Scenario: File sink and stdout mirror carry the same envelope
- **WHEN** `see --mode=log` runs with stdout piped and the
  mirror sink is wired
- **THEN** the JSONL file and the stdout stream carry
  byte-identical lines
- **AND** each line decodes to the same `{ts, event}` envelope
  on both sinks

#### Scenario: Envelope marshalling failure does not crash the watcher
- **WHEN** the inner payload fails to marshal (an unreachable
  case in the current `Event` types; pinned for future-proofing)
- **THEN** the `Observe` call is a no-op for that event
- **THEN** no panic, no log line, no exit; the watcher continues
  to the next event
