## Context

The `event-log` capability has, since the `silent-tui` change
of 2026-07-14, produced one bare `Event` payload per JSONL line:
`{"Path": "...", "HasOpenspec": true}` or similar. That contract
is the post-mortem source of truth, but it lost the timestamp
the moment an `Event` struct was constructed — `RepoSeen`
carries no creation time, `ChangeStarted` carries no creation
time, and the file name encodes UTC to the second. Operators
wanting to correlate two events emitted in the same second had
no contract surface to lean on; they had to invent wall-clock
proxies or rely on the watcher's tight poll loop being slow
enough to interleave between events. Neither addresses
nano-second ordering when the watcher fires `RepoSeen →
ChangeStarted → LogPath → ChangeDone` for a one-second agent run.

The mirror sink (added in `add-jsonl-stdout-mirror`) makes the
gap more visible: a pipe consumer parsing the stream sees the
ordering issue too. Whatever shape lands on the file has to
land on stdout byte-for-byte; the envelope lands on both.

## Goals / Non-Goals

**Goals:**

- Give every JSONL line an observed-at timestamp precise enough
  to break ties within a single UTC second, expressed in a
  parseable standard format.
- Land the same shape on the file sink and the stdout mirror;
  the two sinks SHALL stay byte-identical.
- Encode once per Observe call; the file, mirror, and secondary
  observer SHALL share one marshalled payload.

**Non-Goals:**

- No change to the agent-run per-invocation JSONL (the one
  captured by `PiAgent.Run`); it's plain stdout/stderr capture,
  not structured events, and its consumers can compute their
  own envelope if they need one.
- No struct-level `time.Time` field on `Event` types. The
  envelope is built at the marshal layer so the watcher's
  business logic and the TUI's typed adapter path stay
  unchanged.
- No timezone or locale options. `time.Time` in UTC is the
  only flavor; RFC3339Nano gives nanosecond precision when
  present, drops trailing zeros when not.

## Decisions

- **Envelope, not a per-Event `Ts` field.** Adding
  `time.Time` to every concrete `Event` struct (seven types:
  `RepoSeen`, `ChangeStarted`, `RetryAttempt`, `ChangeDone`,
  `ChangeFailed`, `LogPath`, `Warning`, `InfraError`) means
  eight small edits, and any future event type has to remember
  to set it. The marshal-layer envelope keeps the watcher and
  the TUI typed adapters untouched: `Observe(e)` continues to
  receive the raw `Event` and continues to dispatch the raw
  `Event` to `secondary`. The only thing that knows about the
  envelope is `eventLogger.Observe`.

- **`time.Time` captured at marshal, not at struct
  construction.** "When did this event happen?" is a question
  with two answers in this codebase: when the watcher built
  the struct, or when the observer saw it. The first is
  closer to "event happened"; the second is closer to "the
  observer has a complete picture of when each line landed."
  Under the watcher's sequential single-goroutine model the
  two are within microseconds; under a future concurrent
  producer they diverge. Picking marshal-time means the
  ordering information the envelope carries is the ordering
  the consumer will see in the file — the only monotonic
  ordering source the batch-level logger has access to. If
  the watcher ever ships a per-event `ObservedAt` field, this
  change is the obvious migration target.

- **RFC3339Nano, not RFC3339.** RFC3339 by itself has only
  second precision — exactly the gap the envelope is meant to
  close. RFC3339Nano preserves trailing zeros when nanos are
  zero and emits them when present, giving nanosecond
  granularity without forcing callers to guess the width.

- **`json.RawMessage` for the inner payload, not `any`.** A
  `map[string]any` round-trip drops struct field order and
  re-encodes numbers in Go's natural representation; a
  `json.RawMessage` from the first marshal call lets the
  envelope marshal those bytes verbatim. This keeps the
  inner payload byte-identical to what the previous behavior
  produced, so a future `git blame` on `eventLogger`'s output
  stays readable.

- **Best-effort envelope marshalling.** If the envelope
  marshal fails (which it shouldn't for the shapes involved,
  but worth pinning), the observer call is a no-op for that
  event, exactly as the pre-encode-error path was. Losing a
  trailing event is preferable to crashing the watcher.

## Risks / Trade-offs

- [External consumers parsing the previous JSONL shape
  (`jq .Path`) break under the new envelope] → Mitigation:
  this is a binary in early release; the JSONL is operator-
  facing observability, not a stable public API; the
  breaking change is called out in the proposal's `What
  Changes` and the spec's `MODIFIED` requirement. For
  consumers that can't update, `jq '.event.Path'` adapts the
  same query.

- [The envelope doubles per-line bytes for short payloads
  (`RepoSeen` on a path with no openspec goes from ~30 chars
  to ~70 chars)] → Acceptable: the JSONL is observability,
  not transport; doubling per-line weight is still <1 ms per
  kilo-event, well below the per-repo poll loop's cost.

- [Operators may want to discover the timestamp without
  parsing the envelope (e.g., for `grep`)] → Acceptable;
  `grep '"ts":'` finds the field, and an operator-facing tool
  can be written on top of the JSONL when needed.

## Migration Plan

Single-binary change; the JSONL file shape is the only
observable surface. Migration:

1. Land the spec + delta in this change.
2. Run `go test ./...` and confirm
   `TestEventLoggerStampsObservedAtOnEachEntry` plus the
   envelope-aware rewrites of `TestEventLoggerWritesJSONL`
   and `TestEventLoggerMirrorsEncodedEvents` pass.
3. Verify by hand: `see --mode=log` produces JSONL with `ts`
   fields at the top of every line; the on-disk JSONL file
   matches the stdout mirror byte-for-byte under
   `--mode=log` with stdout piped.
4. Archive this change; the `event-log` capability base spec
   becomes authoritative for the envelope shape and the
   project's external changelog (or README, when one exists)
   should mention the breaking JSONL change for any
   script-driven consumers.
5. No rollback path needed; the envelope is built at marshal
   time and the previous behavior is one revert away.

## Open Questions

- Should the spec also require the mirror and the file to be
  byte-for-byte identical (not just structurally equivalent)?
  Default: pin it as a task verification step; not a separate
  requirement because it follows mechanically from "encode
  once, write twice."
- Should `pi --mode json` (the agent invocation) gain the
  same envelope? Out of scope here — the agent is a separate
  binary with its own output contract; revisit only if
  consumers ask.
