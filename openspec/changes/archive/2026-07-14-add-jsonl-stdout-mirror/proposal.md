## Why

The 2026-07-14 `silent-tui` change made `--mode=log` write its
output exclusively to a JSONL file under `SEE_LOG_DIR` and
promised the operator that "stdout is empty for the lifetime of
the process." That contract is correct on a terminal but wrong
for any operator who wants to consume the stream from the shell:
there is no way to pipe `see --mode=log | jq`, redirect the
events to a file (`> events.jsonl`), hand them to a log
collector, or follow them with `tail`. Operators either have
to know the cache directory, find the file with a glob, then
tail it — or copy the JSONL semantics verbatim into their own
script. The mirror makes the JSONL streamable without changing
what the JSONL file means (still the source of truth) and
without adding a flag.

## What Changes

- Add an optional second sink (`mirror io.Writer`) to the
  batch-level `eventLogger`. When non-nil, the same JSON-encoded
  bytes the logger writes to its file land on the mirror line
  by line, in emission order.
- In `main()`, when `--mode=log` is in effect AND stdout is not
  a terminal, wire `os.Stdout` as the mirror. TTY stdout keeps
  the silent behavior; non-TTY stdout (pipes, redirects, CI
  captures) gets the stream.
- The existing `--mode=log` contract for a TTY (silent, JSONL
  file only) is unchanged. The TUI mode is unchanged.
- **BREAKING (spec-only, no consumer impact today)**: the
  `tui` capability's `--mode=log` scenario that reads "stdout is
  empty for the lifetime of the process" is replaced with two
  scenarios — one for TTY stdout (silent), one for non-TTY
  stdout (streamed). The "TTY state has no effect on
  `--mode=log` behavior" wording is replaced with the actual
  conditional. No file format, flag, or public API changes.

## Capabilities

### New Capabilities

None. This change folds into existing capabilities.

### Modified Capabilities

- `tui`: the `--mode=log` requirement in
  `openspec/specs/tui/spec.md` is rewritten to describe the
  TTY-conditional stdout behavior, and the corresponding
  scenario is split into two (TTY silent; non-TTY streams
  JSONL).

## Impact

- **Code**: `eventlog.go` adds a `mirror io.Writer` field and
  a `SetMirror(io.Writer)` method on `*eventLogger`; the
  `Observe` method writes to file + mirror + secondary using
  one marshalling per event. `main.go` adds a five-line
  `term.IsTerminal`-gated call to `events.SetMirror(os.Stdout)`
  on the log branch.
- **Tests**: one new
  `TestEventLoggerMirrorsEncodedEvents` pins the SetMirror
  contract directly. The terminal/integration behavior is
  pinned by the run-mode scenarios.
- **Dependencies**: zero added, zero removed.
- **Behavior**: `--mode=log` on a TTY behaves as before
  (silent); on a pipe/redirect, stdout now receives the same
  JSONL the on-disk file receives. The JSONL file is the
  source of truth in both cases; the mirror is a convenience
  sink for operators.
- **Risk**: low. The mirror sink is opt-in at construction;
  existing call sites are unaffected. The TTY-only branch
  matches the silent-tui behavior byte-for-byte.
