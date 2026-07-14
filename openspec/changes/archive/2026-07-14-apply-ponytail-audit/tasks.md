## 1. Watcher core refactor

- [x] 1.1 Rename `Watcher.RetyCount` to `Watcher.RetryCount` and
  update every reference (in `main.go`, `main_test.go`, the
  `tui` spec delta, and any test that constructs a `Watcher`
  literal). Keep the field exported.
- [x] 1.2 Unexport `PiAgent.Binary` and `PiAgent.LogDir` to
  lowercase. Update `main.go` and `main_test.go` so neither
  constructs a `PiAgent` literal outside the new constructor.
- [x] 1.3 Extend `NewWatcher` to take
  `binary, logDir string; retry int; once bool` and return a
  `Watcher` whose `agent`, `RetryCount`, and `Once` fields
  are fully populated. Update `main()` to call the new
  signature with `*pi`, `logDir`, `*retry`, `*once`.
- [x] 1.4 Delete `repoHasOpenspec` from `main.go`. Update
  `Watcher.runOnce` to call
  `len(ListActiveOpenSpecChanges(repo)) > 0` in its place.
  Remove the `ponytail:` comment that admits the duplication.
- [x] 1.5 Replace `ensureBranch`'s
  `git branch --list <name>` + `strings.Contains` with
  `git show-ref --verify --quiet refs/heads/<name>` and a
  clean exit-code check. Keep the `-c name` / `name` switch
  decision driven by the exit code.
- [x] 1.6 Inline `retryN` into `Watcher.runOnce`. Replace the
  `retryN(w.RetryCount, ...)` call with an explicit `for
  attempt := 1; attempt <= w.RetryCount; attempt++ { ... }`
  loop that preserves the same "return last error on
  exhaustion, return nil on first success" contract.
- [x] 1.7 Rewrite
  `TestRetryNReturnsLastErrorWhenAllAttemptsFail` as
  `TestRunOnceRetryLoopReturnsLastErrorWhenAllAttemptsFail`
  and point it at `Watcher.runOnce` with a `fakeAgent` that
  fails three times. The scenario pins the same contract
  (last error returned) but exercises the inlined loop.
- [x] 1.8 Remove the standalone `retryN` function and its
  docstring from `main.go`.

## 2. TUI refactor

- [x] 2.1 In `tui/program.go`, delete the 8 typed methods on
  `ChanObserver` (`RepoSeen`, `ChangeStarted`, `RetryAttempt`,
  `ChangeDone`, `ChangeFailed`, `LogPath`, `Warning`,
  `InfraError`). Replace the unexported `push` with an
  exported `Send(tea.Msg)` that takes any `tea.Msg` and
  forwards it to the program. Keep the recover-from-send-on-
  exited-program behavior.
- [x] 2.2 In `main.go`'s `tuiObserver.Observe`, replace each
  `o.obs.<TypedMethod>(...)` call with a direct
  `o.obs.Send(<Msg>{...})` call that builds the literal
  inline. Drop the method-name indirection.
- [x] 2.3 In `tui/model.go`, delete the `PhaseNoSpec`
  constant. In `Phase.String()` and `Phase.Glyph()`, delete
  the `case PhaseNoSpec` arms (both return the same string
  as `PhaseIdle`). In `RepoSeenMsg` handling in `Update`,
  replace `r.Phase = PhaseNoSpec; r.Change = "—"` with
  `r.Phase = PhaseIdle; r.Change = "—"` (or omit the
  change-column mutation and let the view layer derive the
  em-dash from `!r.HasOpenspec`).
- [x] 2.4 In `tui/view.go`, drop the `glyphNoSpec` variable
  and the `case PhaseNoSpec` arm of `phaseString`. In the
  footer's `renderFooter`, drop the `nospec` counter and
  the `1 no-spec` summary segment. In `renderRow`, render
  the change column as `"—"` whenever `r.HasOpenspec` is
  false (in addition to the existing `truncate`).
- [x] 2.5 In `tui/model.go`, replace the hand-rolled
  `basename` helper with `path/filepath.Base`. Update the
  import list to include `path/filepath`. Delete the
  hand-rolled function.
- [x] 2.6 In `tui/tui_test.go`, update
  `TestViewHandlesNoSpecRepo` to assert `idle` (not
  `no-spec`) in the rendered view and to keep the em-dash
  assertion. Update `TestViewFooterCountsByPhase` to assert
  `"1 idle"` for the no-spec row instead of `"1 no-spec"`.

## 3. Misc local refactors

- [x] 3.1 In `eventlog.go` (or `main.go` — pick the one
  that keeps the helper in the same package as its
  callers; `eventlog.go` is the default), add an
  unexported `logFilename(stem string) string` helper that
  returns `fmt.Sprintf("%s--%s--%d.jsonl", stem,
  time.Now().UTC().Format("20060102T150405"), os.Getpid())`.
  Replace the body of `pathFor` and `eventLogPath` with a
  call to this helper, passing the appropriate stem.
- [x] 3.2 In `main.go`, change `selectRunMode`'s signature
  from `(runMode, string)` to `(runMode, error)`. Build
  the existing strings with `errors.New(...)` or
  `fmt.Errorf(...)` and return them on the failure paths.
  In `main()`, replace the manual
  `fmt.Fprintln(os.Stderr, msg); flag.Usage(); os.Exit(2)`
  block with `fmt.Fprintln(os.Stderr, "see:", err)` followed
  by `flag.Usage(); os.Exit(2)`.
- [x] 3.3 In `tui/view.go`, simplify `truncate` to drop the
  `if n <= 1` special-case branch. The general path
  already returns `""` for `n <= 0` and `"…"` for
  `n == 1`; the branch was a duplicate safety net. Keep
  the `n <= 0` guard if needed (the general path returns
  the empty slice / wrong result for `n == 0`, so a single
  `if n <= 0 { return "" }` guard is the cleanest form).
- [x] 3.4 In `main.go`, drop the "sealed interface" sentence
  from the `Event` interface doc comment. Replace it with
  a one-line note that the marker-method pattern is a
  soft convention, not a hard guarantee (Go interfaces are
  not sealed). Keep the rest of the comment.

## 4. Verification

- [x] 4.1 Run `go test ./...` and confirm every test in
  `main_test.go` and `tui/tui_test.go` passes. The expected
  test-rewrite exceptions are listed in tasks 1.7, 2.6, and
  3.2 — no other test should need changes.
- [x] 4.2 Run `go vet ./...` and confirm clean output.
- [x] 4.3 Run `gofmt -l .` and confirm no files need
  reformatting.
- [x] 4.4 Run `openspec validate apply-ponytail-audit` and
  confirm the change is still valid after the implementation
  edits (the spec deltas were validated before the code
  changes; this is a sanity check that nothing accidentally
  drifted).
