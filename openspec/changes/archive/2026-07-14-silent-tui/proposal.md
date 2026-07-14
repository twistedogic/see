## Why

Today, `see` writes ~18 `log.Printf` lines from the watcher plus two
`fmt.Fprintln(os.Stderr, ...)` calls from `runTUI` while the Terminal
User Interface (TUI) is running. Bubbletea owns the alternate screen;
anything written to standard output (stdout) or standard error (stderr)
in that window corrupts the rendering and leaks text the operator did
not ask for. `--mode=log` is in the same boat — its `log.Printf`
output is the only persistent record of what the watcher did, and it
disappears when the process exits.

The change makes both modes silent (no stdout, no stderr) and
redirects every event the watcher emits into a single batch-level
JavaScript Object Notation Lines (JSONL) file. The TUI becomes a pure
view of the event stream; the JSONL is the source of truth that
survives process exit. The table also loses its text-heavy error
column in favour of a single warning glyph, since all message text is
already in the JSONL.

## What Changes

- Remove every `log.Printf` call in `main.go` and `PiAgent.Run`. Each
  site migrates to an `Observer` event.
- Add two new event types: `Warning{Path, Change, Msg}` for per-repo
  cleanup/diagnostic warnings and `InfraError{Where, Err}` for
  watcher-level or TUI-level failures.
- Introduce `ensureLogDir()` invoked in `main()` before the watcher
  starts. On failure, print a single stderr message and exit `2`.
  Pre-TUI-mode startup errors remain allowed to write to stderr.
- Reframe the per-invocation JSONL invariant: `PiAgent.Run` no longer
  carries a fallback path. Given a pre-validated log directory, every
  invocation produces a non-empty JSONL file or surfaces the failure
  via `ChangeFailed`.
- Add a batch-level event logger that writes one JSONL per `see`
  process (`$SEE_LOG_DIR/see--<utc-timestamp>--<pid>.jsonl`). Every
  event the watcher emits flows through this logger. In TUI mode the
  logger fans out to the bubbletea observer; in log mode the JSONL
  is the only output.
- Reshape the TUI grid: drop the `ERR` column, render a `⚠` glyph
  in the `REPO` column when the row carries an active warning, and
  add a `warning` counter to the footer.
- Render `InfraError` as a banner between the grid body and the
  footer.
- Delete `TestPiAgentProceedsWhenLogDirCannotBeCreated`. Replace with
  a test that pins the new `ensureLogDir` exit-2 contract.
- **BREAKING**: `--mode=log` no longer writes `log.Printf` lines to
  stderr. Operators who previously tailed stderr in scripts must
  switch to tailing the JSONL.
- **BREAKING**: per-invocation JSONL files are now guaranteed to
  exist for every agent run; code that branched on `logPath == ""`
  must be updated.

## Capabilities

### New Capabilities

- `event-log`: the batch-level JSONL event stream, the
  `ensureLogDir` startup check, the `eventLogger` type that fans
  events out to JSONL (always) and to the TUI observer (in TUI
  mode), and the `Warning` / `InfraError` event shapes.

### Modified Capabilities

- `tui`: the grid layout (no `ERR` column, `⚠` glyph in `REPO`,
  warning counter in footer, `InfraError` banner), the `--mode=log`
  contract (silent + JSONL, not stderr), the agent-output-routing
  requirement (now directed to the JSONL file in both modes), and
  the TTY-required-message wording stays as the pre-TUI-mode
  startup-error exception.
- `watcher`: the `PiAgent` JSONL contract loses its "proceed without
  capture" fallback paths; the per-invocation filename requirement
  stays but is now unconditional; the `LogPath` event becomes
  unconditional (no observer-not-wired branch); the `Log mode
  prints the log path to stderr` requirement is removed.

## Impact

- `main.go`: ~20 write sites removed; `ensureLogDir`, `eventLogger`,
  `Warning`, `InfraError` added; `selectRunMode`'s pre-TUI stderr
  message stays.
- `tui/`: `RepoRow` gains a `Warning bool` field, drops `LastErr`
  rendering; `Model` gains an `infraErr string` field; `ChanObserver`
  gains `Warning()` and `InfraError()` methods; `View` drops the
  two-threshold column-width logic.
- New file: `eventlog.go` (the batch-level logger) lives in `main`'s
  package alongside `main.go`.
- `main_test.go`: deletes
  `TestPiAgentProceedsWhenLogDirCannotBeCreated`; adds a test for
  the `ensureLogDir` exit-2 contract.
- `openspec/specs/tui/spec.md` and `openspec/specs/watcher/spec.md`:
  delta specs reflect the new contracts.
- Operators running `--mode=log` interactively will see no terminal
  output. README and any docs that mention "log mode prints to
  terminal" need updating (not in scope of this change).
