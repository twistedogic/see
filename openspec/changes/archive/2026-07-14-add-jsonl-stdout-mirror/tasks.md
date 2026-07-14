Note: this change is being proposed retroactively to match code
already in place (`SetMirror` was added to `eventLogger` and
wired in `main()` on the `--mode=log` branch during an earlier
debugging session). Tasks below are recorded as `[x]` because
the implementation is already on disk; running `/opsx-apply`
against this change has no work to do, and archiving it
materializes the spec delta into the base `tui/spec.md`.

## 1. eventLogger mirror sink

- [x] 1.1 Add `mirror io.Writer` field to `eventLogger` in
  `eventlog.go`. Zero value is `nil` (no mirror). Document
  the contract that the mirror receives the same encoded
  bytes as the file.
- [x] 1.2 Add `SetMirror(w io.Writer)` method that
  mutates the field; pass `nil` to clear.
- [x] 1.3 Switch `eventLogger.Observe` from `*json.Encoder`
  to one-shot `json.Marshal` so the same marshalled payload
  is written to the file, the mirror, and the secondary
  observer in turn. Each successful Observe writes the same
  byte slice to all three sinks (one `\n`-terminated line
  per event). Best-effort writes: errors are swallowed on
  the file and mirror sinks, since the JSONL is
  observability, not correctness.

## 2. main() TTY branch for log mode

- [x] 2.1 After `openEventLogger` succeeds on the
  `--mode=log` branch of `main()`, evaluate
  `term.IsTerminal(int(os.Stdout.Fd()))`. When false, call
  `events.SetMirror(os.Stdout)`; when true, leave the mirror
  nil. The TUI branch is unaffected.
- [x] 2.2 Document the conditional with a one-line
  `ponytail:` comment explaining why the TTY branch stays
  silent (the on-disk JSONL is the source of truth for
  interactive operators; the mirror exists only for
  pipe/redirect consumers).

## 3. Tests

- [x] 3.1 Add `TestEventLoggerMirrorsEncodedEvents` to
  `main_test.go`. Stands up an `eventLogger` with a
  `bytes.Buffer` mirror; observes two events; asserts
  the buffer contains two valid JSONL lines that round-trip
  the underlying event payloads.
- [x] 3.2 (Continuity) Existing `TestEventLoggerWritesJSONL`
  and `TestEventLoggerFansOutToSecondary` continue to pass
  unchanged (the SetMirror is optional; the file and
  secondary paths are untouched).

## 4. Verification

- [x] 4.1 `go test ./...` is green with `-race`, including the
  three eventLogger tests.
- [x] 4.2 Manual smoke: `see --mode=log | jq .` (or `cat`)
  produces one event per line; `see --mode=log` on a TTY
  writes only to the JSONL file under `SEE_LOG_DIR`.
- [x] 4.3 `openspec validate add-jsonl-stdout-mirror`
  returns clean.
- [x] 4.4 Archive this change per the project's normal flow
  so the `tui` capability base spec reflects the
  TTY-conditional behavior and the old "TTY state has no
  effect on `--mode=log`" sentence is gone.
