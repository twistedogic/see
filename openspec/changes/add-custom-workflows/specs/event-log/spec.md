## MODIFIED Requirements

### Requirement: see writes a batch-level JSONL event stream
When `see` starts, after the run mode has been resolved but before the watcher begins, `see` SHALL create one JavaScript Object Notation Lines (JSONL) file at `$SEE_LOG_DIR/see--<utc-timestamp>--<pid>.jsonl`, or `os.UserCacheDir()/see/logs/see--<utc-timestamp>--<pid>.jsonl` if `SEE_LOG_DIR` is unset. Each `Event` value the watcher emits SHALL be wrapped in an envelope `{"ts": "<RFC3339Nano UTC>", "event": <event-payload>}` and written as one line to that file, in the order the events fire, before the file is flushed for that event. The `ts` field SHALL be the observed-at time the `eventLogger` marshalled the event, not the wall-clock time the watcher created the event struct, expressed as the Go layout `time.RFC3339Nano` formatted on a value in Coordinated Universal Time (UTC). Under concurrent producers this keeps line ordering monotonic against the writer's wall clock rather than a producer's wall clock.

The envelope SHALL wrap without otherwise modifying the underlying event payload. `RepoSeen` SHALL carry `Path` and the workflow-neutral `HasChange` boolean. The previous `HasOpenspec` field SHALL NOT be emitted. For example, `RepoSeen{Path: "/x", HasChange: true}` SHALL marshal as `{"ts":"<rfc3339nano>","event":{"Path":"/x","HasChange":true}}`.

#### Scenario: One file per batch
- **WHEN** `see` is invoked twice in sequence from the same process identifier (PID)
- **THEN** two distinct JSONL files exist under the log directory
- **AND** each file's name encodes a unique UTC timestamp

#### Scenario: Every watcher event lands in the JSONL
- **WHEN** `Watcher.runOnce` runs against one repo with one resolved change and the agent succeeds
- **THEN** the JSONL contains, in order, `RepoSeen`, `ChangeStarted`, `LogPath`, and `ChangeDone`
- **THEN** every line is valid JSON encoding of `{"ts": <rfc3339nano-string>, "event": <event-payload>}`
- **AND** the inner `event` payload round-trips the original `Event` field set without renaming or reordering

#### Scenario: Repository availability is workflow-neutral
- **WHEN** either a custom condition or the OpenSpec compatibility resolver produces a change
- **THEN** the `RepoSeen` event payload contains `HasChange: true`
- **AND** it does not contain a `HasOpenspec` field

#### Scenario: Repository without work reports no change
- **WHEN** the selected resolver produces no change
- **THEN** the `RepoSeen` event payload contains `HasChange: false`
- **AND** it does not contain a `HasOpenspec` field

#### Scenario: Each line carries an RFC3339Nano UTC timestamp
- **WHEN** the JSONL is read with `time.Parse(time.RFC3339Nano, ...)` on every `ts` value
- **THEN** the parse succeeds for every line
- **THEN** the parsed time lies within one second of the wall clock at process exit

#### Scenario: File sink and stdout mirror carry the same envelope
- **WHEN** `see --mode=log` runs with stdout piped and the mirror sink is wired
- **THEN** the JSONL file and stdout stream carry byte-identical lines
- **AND** each line decodes to the same `{ts, event}` envelope on both sinks

#### Scenario: Envelope marshalling failure does not crash the watcher
- **WHEN** the inner payload fails to marshal
- **THEN** the `Observe` call is a no-op for that event
- **THEN** no panic, log line, or process exit occurs
- **AND** the watcher continues to the next event
