## Context

A whole-repo `ponytail-audit` produced a ranked list of ~12
over-engineering findings against `see`. The findings are
localized refactors (a typed-method-per-message collapse, a
duplicate `os.ReadDir`, a hand-rolled `basename`, a
string-contains where a clean exit code works, a `phase`
enum entry whose glyph duplicates an existing one, a
single-call-site `retryN` wrapper, a `(runMode, string)`
return type that wants to be `(runMode, error)`, a
constructor that hands back a half-built struct, a typo in
a public field name, and a few related cleanups). The
project guideline in `AGENTS.md` ranks correctness,
readability, simplicity, and long-term maintainability
above short-term effort, so paying these down together is
the right call. There is no new user-facing behavior in
this change — every spec-level requirement stays
byte-identical except where the audit makes the spec
itself cleaner (the `no-spec` phase collapses into `idle`;
the `retryN` free function collapses into the watcher's
inlined loop; the `selectRunMode` return type switches
to `(runMode, error)`; the `RetyCount` field renames to
`RetryCount`).

## Goals / Non-Goals

**Goals:**

- Reduce the production-code line count by ~70 lines with
  no behavior change observable from the CLI, the JSONL
  log, or the TUI grid.
- Keep every existing test in `main_test.go` and
  `tui/tui_test.go` passing without rewrites that go
  beyond rename / signature adjustments already implied by
  the refactor itself.
- Rename the public `RetyCount` field to `RetryCount` (a
  long-standing typo) and absorb the two zero-default
  knobs (`RetryCount`, `Once`) into a single
  `NewWatcher` constructor that fully populates the
  struct.
- Make the spec descriptions of behavior match the
  refactored implementation: the `no-spec` phase goes
  away, the `retryN` helper goes away, the `selectRunMode`
  return type changes, and the `RetyCount` typo is
  corrected wherever it appears in scenario prose.

**Non-Goals:**

- No new features, flags, or output formats.
- No new dependencies, no removed dependencies.
- No restructuring of the `tui` / `main` package boundary
  (the `Event` ↔ `*Msg` duplication that motivated the
  `tuiObserver` adapter is structurally forced by Go's
  cyclic-import rules with `package main`; the audit
  found nothing actionable there).
- No performance or correctness fixes — the audit is
  scoped to over-engineering only. Any bugs found during
  implementation go to a follow-up change.

## Decisions

- **`ChanObserver` collapses to one `Send(tea.Msg)`
  method.** The 8 typed methods each just wrap `push()`
  with a `*Msg` literal. The `tuiObserver` adapter in
  `main.go` already builds the literals, so the wrapper
  methods are pure indirection. The new `Send` takes any
  `tea.Msg`; the test for "secondary observer receives
  events" still passes because the test goes through the
  `eventLogger`, not through `ChanObserver` directly.
  *Alternative considered:* keep the typed methods and
  de-duplicate `push` instead. Rejected — leaves the
  indirection in place and just hides it.

- **`PhaseNoSpec` collapses into `PhaseIdle`.** The two
  phases render the same glyph (`○`), and the row already
  carries `HasOpenspec` so the model can disambiguate
  without a dedicated phase. The view layer decides
  whether to render `—` for the change column based on
  `!HasOpenspec`, not on the phase. The TUI's "1 no-spec"
  footer counter is replaced with "1 idle" (and the
  `TestViewHandlesNoSpecRepo` test is updated to match).
  *Alternative considered:* keep `PhaseNoSpec` and only
  drop the duplicate glyph. Rejected — the only thing
  the two phases actually distinguish is the label, and
  the label is more useful as a count of all idle rows
  than as a separate "no-spec" bucket.

- **`retryN` inlines into `Watcher.runOnce`.** The
  function has exactly one caller and the body is a
  five-line `for` loop. The
  `TestRetryNReturnsLastErrorWhenAllAttemptsFail` test
  stays as the contract pin and is moved to target
  `Watcher.runOnce`'s retry behavior directly (or stays
  pointed at a one-line wrapper that delegates to the
  inlined loop — the choice is a test-only detail). The
  `retry-helper` spec is marked `REMOVED` because the
  capability no longer exists as a separately named
  function; the contract is re-asserted as an `ADDED`
  requirement on the `watcher` spec ("`runOnce`'s retry
  loop returns the error from the final attempt").

- **`selectRunMode` returns `(runMode, error)`.** The
  current `(runMode, string)` shape is a one-off
  pattern: the `string` is always either empty or a
  stderr message. Switching to `error` lets `main()`
  use the standard `if err != nil { fmt.Fprintln(os.Stderr,
  "see:", err); flag.Usage(); os.Exit(2) }` idiom. The
  `TestSelectRunMode` test is updated to compare
  `err.Error()` against the previous string.

- **`repoHasOpenspec` deleted; `runOnce` calls
  `len(ListActiveOpenSpecChanges(repo)) > 0` instead.**
  The current helper re-`os.ReadDir`s the same directory
  with the same `IsDir && name != "archive"` filter.
  `ListActiveOpenSpecChanges` returns `nil` on error
  (not an error), so the `len > 0` test is correct
  even when the directory is missing.

- **`ensureBranch` uses `git show-ref --verify --quiet
  refs/heads/<name>`** instead of `git branch --list
  <name>` + `strings.Contains`. The exit code is the
  clean signal; the previous code was a string match on
  a multi-line stdout payload.

- **`NewWatcher` takes `binary, logDir string; retry
  int; once bool` and returns a fully populated
  `Watcher`.** `PiAgent.Binary` and `PiAgent.LogDir`
  become unexported. `Watcher.RetyCount` renames to
  `Watcher.RetryCount`. The two zero-default knobs
  collapse into the constructor signature. `main()`
  passes `*pi`, `logDir`, `*retry`, `*once` and never
  reaches into struct fields again.

- **`tui/model.go` uses `path/filepath.Base`** for
  `basename`. No behavior change; the hand-rolled loop
  is replaced with the stdlib one-liner.

- **`logFilename(stem string) string`** is a new
  unexported helper in `eventlog.go` (or `main.go`,
  whichever keeps the import graph simple — see the
  tasks doc for the placement decision) that builds
  `<stem>--<utc-20060102T150405>--<pid>.jsonl`. Both
  `pathFor` and `eventLogPath` delegate to it.

- **`truncate` drops the `n <= 1` special case.** The
  general path returns `""` for `n <= 0` and `"…"` for
  `n == 1` already; the explicit branch was a duplicate
  safety net.

- **The "sealed interface" sentence on the `Event`
  comment is dropped.** Go interfaces are not sealed;
  the marker-method pattern is a soft convention, not a
  hard guarantee. The comment is corrected, the pattern
  is preserved.

## Risks / Trade-offs

- [`Watcher.RetryCount` is a public-field rename —
  consumers outside the `see` repo (none today, but
  possible) would need to update] → Mitigation: the
  rename is grep-able; the constructor signature is the
  only blessed construction path; any external embedder
  would also need to update for the new `NewWatcher`
  signature anyway.

- [The `PhaseNoSpec` collapse changes the TUI's footer
  count from "N no-spec" to "N idle" — a small visual
  change in the running grid] → Mitigation: the
  underlying state is identical; the `no-spec` row
  still renders with `—` for change and the `○` glyph;
  the footer count is the only visible difference and
  the spec is updated to match.

- [Inlining `retryN` removes a named, testable function
  — the existing test stays valid only if a thin
  wrapper is preserved or the test is rewritten to
  exercise `runOnce` end-to-end] → Mitigation: the test
  is rewritten to drive `runOnce` with a controlled
  agent that fails N times, which is a stronger
  contract pin than the isolated unit test.

- [`PiAgent.Binary` / `LogDir` becoming unexported
  breaks any literal struct construction in tests or
  external code] → Mitigation: only `main_test.go`
  uses `PiAgent{...}` literals today, and the
  `NewWatcher` constructor gives those tests a
  fully-populated `Watcher` to drive.

- [The "sealed interface" comment removal is a cosmetic
  change that has no test coverage] → Acceptable — the
  comment was misleading, not load-bearing.

## Migration Plan

This is a single-binary refactor with no on-disk format
changes, no flag changes, and no public API outside the
`see` repo. The migration is:

1. Land the change on a topic branch.
2. Run `go test ./...` and confirm every existing test
   still passes (with the expected test-rewrite
   exceptions called out in the tasks doc).
3. Run `go vet ./...` and `gofmt -l .` to confirm clean
   output.
4. Merge via the project's normal flow. No rollback
   plan is needed because the behavior is byte-identical
   to the pre-change build; if something goes wrong, the
   prior commit reverts cleanly.

## Open Questions

- Should the `retry-helper` spec be archived as
  `REMOVED` with a pointer to the new watcher requirement,
  or rewritten in place to describe the inlined loop?
  Default: REMOVED in this change, ADDED on the watcher
  spec — keeps the spec history linear and avoids
  maintaining a one-line "wrapper" requirement.
- Should `logFilename` live in `eventlog.go` (close to
  `eventLogPath`, the original owner) or in `main.go`
  (close to `pathFor`, the other caller)? Default:
  `eventlog.go` so the JSONL filename concern stays in
  one file; `main.go` imports the helper from the same
  package with no new import line.
