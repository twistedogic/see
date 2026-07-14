## Why

A whole-repo `ponytail-audit` of `see` flagged ~12 over-engineering
findings: a sealed-Event-claim that's a lie, an `Event` ↔ `*Msg`
duplication (structurally forced by Go's import rules), an
`Agent` interface with a single production implementation, a
`basename` reimplementation, a `repoHasOpenspec` helper that
re-scans what `ListActiveOpenSpecChanges` already scanned, a
`retryN` wrapper for one call site, a `PhaseNoSpec` whose glyph
duplicates `PhaseIdle`, an `ensureBranch` that string-contains
`git branch --list` output, a `NewWatcher` that constructs an
agent with empty `LogDir` so `main()` can overwrite it, two
zero-default knobs (`RetyCount`, `Once`) and a typo in the first,
a `selectRunMode` that returns `(runMode, string)` instead of
`(runMode, error)`, and a `truncate` with a special-case branch
the general path already covers. The project guideline in
`AGENTS.md` ranks correctness, readability, simplicity, and
long-term maintainability above short-term effort, so paying
down this debt now is the right move.

## What Changes

- Drop the `ChanObserver` typed methods (`RepoSeen`,
  `ChangeStarted`, …, `InfraError`) and replace with a single
  `Send(tea.Msg)`. The `tuiObserver` adapter in `main.go` builds
  the `*Msg` literal directly.
- Delete `repoHasOpenspec`; `runOnce` calls
  `len(ListActiveOpenSpecChanges(repo)) > 0` instead. The
  `ponytail:` comment that already admits the duplication goes
  with it.
- Replace the hand-rolled `basename` in `tui/model.go` with
  `path/filepath.Base`.
- Extract a `logFilename(stem string) string` helper for the
  shared `<name>--<utc-20060102T150405>--<pid>.jsonl` pattern
  used by `pathFor` and `eventLogPath`.
- Collapse `PhaseNoSpec` into `PhaseIdle`. The "no openspec
  change" state is already carried on `RepoRow.HasOpenspec`; the
  row renders `—` for change and stays at the idle phase. Drop
  the duplicate glyph, the duplicate case in `Phase.String()`, the
  duplicate case in `Phase.Glyph()`, the `case PhaseNoSpec` in
  `phaseString`, and the `case PhaseNoSpec` arm in the footer's
  phase counter.
- Inline `retryN` into `Watcher.runOnce`. Keep the
  `TestRetryNReturnsLastErrorWhenAllAttemptsFail` test; rename
  the function it tests (or add a thin wrapper that delegates to
  the inlined loop) so the contract stays pinned.
- Change `selectRunMode`'s return type from `(runMode, string)`
  to `(runMode, error)`. `main()` prints the error and calls
  `flag.Usage()`; the `TestSelectRunMode` test follows the new
  shape.
- Replace `ensureBranch`'s `git branch --list <name>` +
  string-contains with `git show-ref --verify --quiet
  refs/heads/<name>`.
- Extend `NewWatcher` to take `binary, logDir string; retry int;
  once bool` and return a `Watcher` whose `agent`, `RetyCount`,
  and `Once` fields are fully populated. Unexport `PiAgent`'s
  fields. Rename `RetyCount` to `RetryCount` (typo fix) and
  collapse the two zero-default knobs.
- Simplify `truncate` to drop the `n <= 1` special case; the
  general path already returns `"…"` for `n == 1` and `""` for
  `n <= 0` is the only edge to keep.
- Drop the "sealed interface" sentence on the `Event` comment;
  Go interfaces are not sealed. The marker-method pattern stays
  for now (structurally forced) but the misleading wording goes.

## Capabilities

### New Capabilities

None. This change is a pure refactor — it does not add any
user-visible behavior, command, flag, or output format. All
spec-level requirements (the event sequence, the TUI grid
columns, the run mode dispatch, the retry contract) stay
byte-identical; only the implementation underneath changes.

### Modified Capabilities

- `tui`: `ChanObserver` collapses from 8 typed methods to
  one `Send(tea.Msg)`; the `PhaseNoSpec` value disappears
  (replaced by `PhaseIdle` with `RepoRow.HasOpenspec = false`);
  the `selectRunMode` dispatcher returns `(runMode, error)`
  instead of `(runMode, string)`; scenario prose that
  referenced `RetyCount` or the `no-spec` phase is updated.
- `watcher`: `RetyCount` field renamed to `RetryCount` and
  populated by `NewWatcher`; `repoHasOpenspec` deleted;
  `ensureBranch` switches to `git show-ref`; the inlined
  retry loop is asserted by a new requirement
  ("Watcher's retry loop returns the error from the final
  attempt") that takes over from the removed
  `retry-helper` requirement.
- `retry-helper`: the `retryN` free function goes away (its
  body is inlined into `Watcher.runOnce`); the
  `TestRetryNReturnsLastErrorWhenAllAttemptsFail` test is
  rewritten to drive `runOnce` end-to-end. The capability
  is marked `REMOVED` and the contract is re-asserted on
  the `watcher` capability as an `ADDED` requirement.

## Impact

- **Code**: ~70 lines deleted across `main.go`, `eventlog.go`,
  `tui/program.go`, `tui/model.go`, `tui/view.go`. Net zero
  new files.
- **Dependencies**: zero added, zero removed.
- **Public surface**: `Watcher.RetyCount` becomes
  `Watcher.RetryCount`. The `NewWatcher` signature changes
  (adds `logDir` and `once`). `PiAgent`'s `Binary` and `LogDir`
  fields become unexported; external callers that constructed
  `PiAgent` literally will need to switch to `NewWatcher`.
  This is a CLI binary, so "external callers" means tests and
  any future embedder.
- **Behavior**: none. All existing tests must continue to
  pass; the run-mode dispatch, the event sequence, the TUI
  grid, the retry contract, and the JSONL output are
  byte-identical.
- **Risk**: low. Each individual finding is a localized
  refactor with a passing test already in the suite (or in
  the case of the typo fix, a renamed field with no test
  that needs updating).
