## Why

Every batch-level event in `SEE_LOG_DIR/see--<utc-second>--<pid>.jsonl`
is a separate JSON record, but the only temporal marker on the file
itself is the UTC second encoded in the filename. Two events
emitted within the same second — common on a fast repo with a
short RetryAttempt-to-ChangeFailed interval — lose ordering
information the moment they're loaded into a JSONL reader. The
on-disk file is the source of truth for the post-mortem timeline,
and right now that timeline has gaps the watcher can't reach.
Operators grepping across multiple batches need to correlate
events across files (different `pid`s, different UTC seconds)
without leaning on log-collector infrastructure that isn't part
of the binary's contract.

## What Changes

- Wrap each JSONL line in an envelope
  `{"ts": "<RFC3339Nano UTC>", "event": <original payload>}` at
  `eventLogger.Observe` time. The inner payload is the existing
  `Event` struct rendered exactly as before; the wrapper is added
  at the marshal layer so the watcher, agent-run JSONL format,
  and TUI adapter don't see a new field.
- Pin the timestamp to the moment `eventLogger` marshals the
  event (the observed-at capture), not the moment the watcher
  built the struct. This is the only monotonic ordering source
  the batch-level logger has; under concurrent producers the
  observer's view is the wall-clock truth for the line.
- The same envelope is mirrored to stdout in `--mode=log` when
  stdout isn't a TTY (the behavior added by
  `add-jsonl-stdout-mirror`), so a pipe consumer sees one shape
  on both the file sink and the stdout sink.
- **BREAKING**: the JSONL line shape changes from
  `<EventPayload>` to `{"ts": <string>, "event": <EventPayload>}`.
  External consumers that parsed the previous shape directly will
  see field names move one level down. Internal consumers
  (`eventLogger.secondary` → TUI adapter) are unaffected because
  they receive the typed `Event`, not the JSON bytes.

## Capabilities

### Modified Capabilities

- `event-log`: the `Requirement: see writes a batch-level JSONL
  event stream` in `openspec/specs/event-log/spec.md` is
  rewritten to pin the `{"ts", "event"}` envelope, and the
  accompanying scenario asserts each line is valid JSONL of
  that shape.

## Impact

- **Code**: one path in `eventlog.go::Observe` switches from
  `json.NewEncoder(f).Encode(e)` to
  `json.Marshal(struct{ Ts string; Event json.RawMessage }{
  time.Now().UTC().Format(time.RFC3339Nano), payload })`.
  The mirror, file, and secondary sinks all receive the same
  marshalled payload; the byte stream on every sink changes
  shape identically.
- **Tests**: one new
  `TestEventLoggerStampsObservedAtOnEachEntry` pins the
  envelope shape and the RFC3339Nano format. Existing
  `TestEventLoggerWritesJSONL` and
  `TestEventLoggerMirrorsEncodedEvents` are updated to read
  through the envelope.
- **Dependencies**: zero added; `encoding/json` (already
  imported) gains no new dependency on its own types.
- **Public surface**: none. The `Event` interface, the
  `EventLogger` API, the JSONL file path, and the file-naming
  scheme are unchanged.
- **Behavior**: the JSONL stream becomes self-describing in
  time. Two events sharing a UTC second now carry nanosecond
  ordering and can be sorted without consulting the file name.
  All other observable behavior (run modes, retry contract,
  git rollback, TUI grid, agent invocation) is unchanged.
