Note: this change is being proposed retroactively to match code
already in place. During the original edit, the JSONL
envelope was added in `eventLogger.Observe`, three test
cases were updated, and a direct edit to the base
`event-log/spec.md` was applied in flight. The direct base
edit has since been reverted; the canonical change-of-record
is this proposal's MODIFIED delta, which becomes the new
base content when this change is archived. Tasks below are
recorded as `[x]` because the implementation is already on
disk; running `/opsx-apply` against this change has no work
to do.

## 1. eventLogger envelope on Observe

- [x] 1.1 In `eventlog.go`, swap the embedded
  `*json.Encoder` usage in `Observe` for a two-marshal
  pattern: first `json.Marshal(e)` to capture the inner
  payload as `json.RawMessage`, then `json.Marshal` an
  anonymous struct `{Ts string \`json:"ts"\`; Event
  json.RawMessage \`json:"event"\`}` with
  `time.Now().UTC().Format(time.RFC3339Nano)` as `Ts` and
  the captured payload as `Event`. Append `\n` once.
- [x] 1.2 Keep the existing best-effort write semantics:
  marshal failures (inner or outer) are swallowed because
  the JSONL is observability, not correctness.
- [x] 1.3 The same encoded bytes land on the file sink,
  the (optional) mirror sink, and the secondary observer's
  `Observe(Event)` path. The secondary observer continues
  to receive the typed `Event` value, not the bytes; the
  envelope is purely a marshal-layer construct.

## 2. Test surface

- [x] 2.1 Add `TestEventLoggerStampsObservedAtOnEachEntry`
  to `main_test.go`. Stands up an `eventLogger`, observes
  two events bracketed by `time.Now().UTC()` samples,
  reads the JSONL back, and asserts each line decodes to
  `{ts: <rfc3339nano>, event: <...>}`, the `ts` parses,
  and the parsed time lies within a ±1s tolerance of the
  bracketing samples.
- [x] 2.2 Update `TestEventLoggerWritesJSONL` to read each
  line through the envelope, comparing the first line's
  `event.HasOpenspec` and `event.Path` against the original
  payload, and the last line's `event.Where`/`event.Err`
  against the InfraError event.
- [x] 2.3 Update `TestEventLoggerMirrorsEncodedEvents` to
  read each mirrored line through the envelope, comparing
  `event.Path` / `event.HasOpenspec` for the first line and
  `event.Change` for the second.

## 3. Verification

- [x] 3.1 `go test ./...` is green with `-race`. All three
  eventLogger tests pass; the envelope-aware shape is
  pinned.
- [x] 3.2 Manual smoke: `see --mode=log | head -1 | jq`
  returns one decoded record with a non-empty `ts` and an
  `event` field matching the input event's payload.
- [x] 3.3 `openspec validate add-jsonl-entry-timestamp`
  returns clean.
- [x] 3.4 Archive this change via `openspec archive
  add-jsonl-entry-timestamp --yes`. The MODIFIED delta on
  the `event-log` capability materializes into the base
  spec, replacing the requirement's old "JSON-encoded
  directly" wording with the envelope contract.

## 4. Cleanup

- [x] 4.1 Confirm the diff at
  `openspec/specs/event-log/spec.md` against the previous
  commit is empty (the direct base edit was reverted
  earlier in this change's lifecycle; the archive step
  re-applies the same wording via the delta).
- [x] 4.2 The archived change lives at
  `openspec/changes/archive/<date>-add-jsonl-entry-timestamp/`
  per the project's normal flow. Any external consumer
  documentation (README, when one exists) should call out
  the JSONL line-shape change so existing
  `jq .Path` queries can be updated to `jq .event.Path`.
