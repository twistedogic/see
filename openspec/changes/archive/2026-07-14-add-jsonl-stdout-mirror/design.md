## Context

The `silent-tui` change (2026-07-14) intentionally silenced
`--mode=log` so the Terminal User Interface (TUI) grid could
own the alternate screen without contamination, and so the
JSONL file under `SEE_LOG_DIR` could be the durable record of
the run. Operators running interactively saw nothing on the
terminal; operators wanting the stream had to know the cache
directory, glob the file, and tail it. The proposal rejects the
second half of that tradeoff: pipe-friendly operators
(`see --mode=log | jq`, `see --mode=log > run.jsonl`, CI
captures) need the JSONL on stdout without losing the
on-disk JSONL for after-the-fact audit. The TUI mode stays
exactly as silent-tui left it; the TTY case of `--mode=log`
stays silent too (so an operator at a console can't
accidentally interleave events with their typed output).

## Goals / Non-Goals

**Goals:**

- Make the JSONL stream reachable on stdout when stdout is not
  a terminal, in `--mode=log` mode, without adding a flag or a
  TUI-mode side effect.
- Keep the on-disk JSONL file as the source of truth (it is
  still written for every event, including ones the mirror
  also receives).
- Encode each event once per Observe call; the file and the
  mirror share one marshalled payload so the two streams are
  byte-identical.

**Non-Goals:**

- No new flags, no new env vars, no new binary knobs.
- No TTY-aware behavior outside of `--mode=log` (TUI mode's
  existing TTY guard is unchanged; the per-run agent log file
  is unchanged).
- No fanout to stderr in addition to stdout; stderr remains a
  diagnostic channel only.
- No rewrite of the existing `secondary` (TUI) fan-out path;
  the new mirror sits alongside it as a third optional sink.

## Decisions

- **Mirror as an `io.Writer` field on `eventLogger`, not a new
  fan-out observer.** The existing `secondary Observer` is
  already used for the TUI adapter, which needs the typed
  `Event` value (it routes to per-method call sites). The
  mirror only needs the encoded bytes — `os.Stdout` doesn't
  care about event types. A second `Observer` here would
  require an adapter that re-marshals the event it just
  received, doubling the encoder cost and adding an
  indirection. Putting an `io.Writer` on `eventLogger` keeps
  the data path flat: `Marshal` once, `Write` to each sink.
  The existing secondary path is untouched.

- **`SetMirror` mutator rather than constructor parameter.**
  `openEventLogger` is currently called once in `main()`, the
  same place that now learns about the TTY check; threading a
  new `mirror` argument through the function or wrapping
  `eventLogger` in another constructor just to set one
  optional field is more ceremony than the change earns. A
  zero-default `mirror` field with a `SetMirror(nil)` reset
  matches the existing `Attach` method's shape (one-and-done
  setup, no lifecycle to manage).

- **TTY branch in `main()`, not in `eventLogger`.** The
  `eventLogger` package should not know about TTY detection;
  TTY semantics belong to the run-mode dispatcher in `main()`
  (already the source of the `term.IsTerminal` check for the
  TUI guard). The check is duplicated at two call sites — one
  for TUI guard, one for the mirror — but the duplication is
  two `term.IsTerminal(int(os.Stdout.Fd()))` lines, and a
  helper would obscure the per-mode reasoning.

- **No newline insertion in `SetMirror`.** `os.Stdout`
  needs the same trailing `\n` the file gets so a pipe
  reader sees one event per line. The encode-once path
  already appends `\n` once; the mirror receives the exact
  same byte slice. If a future caller wants a different
  framing, they can wrap their writer.

- **Best-effort writes.** Both the file `Write` and the
  mirror `Write` ignore returned errors. The JSONL is
  observability, not correctness; a partial failure on one
  sink shouldn't crash the watcher. (This matches the
  pre-existing swallow-on-error stance on `json.Encoder.Encode`.)

## Risks / Trade-offs

- [`os.Stdout` writes in large batches can be slow on some
  pipe sinks (e.g., a `| head` reader)] → Mitigation: the
  watcher polls slowly per repo; the actual cost is bounded
  by the number of events per pass, which is small per
  poll cycle. If this becomes load-bearing, switch the file
  sink to a buffered `bufio.Writer` first.

- [Duplicate JSONL encoding if a downstream consumer reads
  stdout AND the on-disk file — wasted bytes] → Acceptable;
  the operator picked both sinks explicitly by setting
  `SEE_LOG_DIR` and running with a redirected stdout, and
  the alternative (one or the other) breaks either the
  pipe-friendly or the audit-friendly contract.

- [A monitoring wrapper that opens `os.Stdout` mid-process
  would not see the mirror's TTY check at startup time]
  → Acceptable; the TTY check happens once in `main()` at
  process start; later redirection is the operator's
  problem.

- [Operators who relied on "stdout is empty for the lifetime
  of the process" as a sentinel for "watcher is idle" lose
  that sentinel on non-TTY runs] → Mitigation: the JSONL
  file under `SEE_LOG_DIR` is the source of truth for
  liveness; check it. The task list pins this.

## Migration Plan

Single-binary change. The migration is:

1. Land the spec + delta in this change.
2. Run `go test ./...` and confirm the new
   `TestEventLoggerMirrorsEncodedEvents` passes alongside the
   existing `TestEventLoggerWritesJSONL`,
   `TestEventLoggerFansOutToSecondary`, and the per-mode spec
   tests.
3. Verify by hand: `see --mode=log | jq` parses one event per
   line; `see --mode=log` on a TTY writes only to the
   on-disk JSONL.
4. Merge. No rollback path is needed: the mirror is opt-in
   via `SetMirror(nil)` at construction; reverting removes
   the entire `SetMirror` call from `main()` and restores
   the silent-tui contract.

## Open Questions

- Should the new `TestEventLoggerMirrorsEncodedEvents` live
  under `main_test.go` or a smaller `eventlog_test.go`? It
  currently lives with the rest of the `eventLogger` tests
  in `main_test.go`; if the test set grows, split it then.
- Should `--mode=tui` ever mirror the JSONL when stdout is
  redirected (e.g., `see --mode=tui > events.jsonl` for
  scripted runs)? Out of scope for this change; pick up in
  a follow-up if operators ask for it.
