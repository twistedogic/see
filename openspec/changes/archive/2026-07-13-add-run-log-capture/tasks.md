## 1. PiAgent capture

- [x] 1.1 Add `logPathFor(repo, change string) (string, error)` helper
      in `main.go` that returns the full file path for a `(repo,
      change)` pair and ensures the log directory exists. Reads
      `SEE_LOG_DIR` on each call; falls back to
      `os.UserCacheDir()/see/logs/`. Returns a wrapped error on
      `MkdirAll` failure.
- [x] 1.2 Modify `PiAgent.Run` to call `logPathFor(path, change)`,
      open the file, set `cmd.Stdout` and `cmd.Stderr` to the file,
      run the command, then `Close()` the file in a `defer`. On
      `logPathFor` or `os.Create` failure, log a warning to `log`
      and proceed without redirection.
- [x] 1.3 Remove the `RedirectOutput bool` field from `PiAgent`.

## 2. Agent interface

- [x] 2.1 Extend the `Agent` interface so `Run` takes `(ctx, path,
      change, prompt)` and returns `(string, error)` where the
      string is the log path (empty when capture was unavailable).
      **BREAKING**: callers and impls must update.
- [x] 2.2 Update `Watcher.work` to pass the active `change` to
      `w.agent.Run(...)` and receive the returned log path.
- [x] 2.3 Update `fakeAgent.Run` in `main_test.go` to accept and
      ignore the new `change` parameter and return a log path;
      verify every existing test that constructs `PiAgent` or calls
      `agent.Run` compiles.

## 3. LogPath event

- [x] 3.1 Add `LogPath struct{ Path, Change string }` to the sealed
      `Event` interface in `main.go`; implement `isEvent()`.
- [x] 3.2 Add a `case LogPath:` arm to `tuiObserver.Observe` that
      forwards to a new `tui.ChanObserver.LogPath(path, change)`
      method.
- [x] 3.3 Add `LogPathMsg` (or equivalent) to `tui/events.go` and a
      handler in `tui/model.go` that records the most recent log
      path per repo. LogPathMsg also seeds `r.Change` when the row
      doesn't yet know the change name.
- [x] 3.4 Render the log path in `tui/view.go` as a second line
      beneath the row it belongs to (plain text; OSC 8 hyperlinks
      are a future nicety).
- [x] 3.5 After `w.agent.Run` returns successfully and capture
      succeeded (logPath != ""), emit the `LogPath` event from
      `Watcher.work` before returning.

## 4. Log mode surfacing

- [x] 4.1 In `Watcher.work`, after `w.agent.Run` returns and capture
      succeeded, call `log.Printf("see: log → %s", path)` so log-mode
      runs make the path discoverable. (TUI mode skips this — the
      observer does the surfacing.)
- [x] 4.2 Gate the `log.Printf` on `w.observer == nil` so TUI mode
      doesn't double-print (TUI uses the LogPath event).

## 5. Tests

- [x] 5.1 Add `TestPiAgentCapturesOutputToLogFile`: invoke
      `PiAgent.Run` with a fake binary that emits known lines to
      stdout and stderr; assert the file exists at the computed
      path and contains the combined output.
- [x] 5.2 Add `TestPiAgentProceedsWhenLogDirCannotBeCreated`:
      point `SEE_LOG_DIR` at a path that cannot be created (under
      a file). Assert a warning is logged and the run's exit
      status reflects only the agent's exit code.
- [x] 5.3 Add `TestPiAgentRespectsSeeLogDir`: set
      `SEE_LOG_DIR=/tmp/see-test-XXX`, invoke `PiAgent.Run`, assert
      the file is under that directory, not under
      `UserCacheDir()/see/logs/`.
- [x] 5.4 Add `TestWorkEmitsLogPathOnSuccessfulCapture`: build a
      `Watcher` with a `recordingObserver`, run `work` against a
      fake repo, assert a `LogPath` event with the expected path
      was emitted.
- [x] 5.5 Add `TestWorkDoesNotEmitLogPathOnCaptureFailure`: same as
      5.4 but with the agent signalling capture-failure
      (empty logPath); assert `LogPath` is absent.
- [x] 5.6 Update existing tests that construct `PiAgent` or call
      `agent.Run` to use the new signature (PiAgent struct literal,
      fakeAgent.Run return shape, two pre-existing event-sequence
      tests that now insert a LogPath event).
- [x] 5.7 Run `go test ./... -race` and confirm everything passes.

## 6. Spec sync

- [x] 6.1 Run `openspec archive add-run-log-capture -y` to promote
      the delta spec into `openspec/specs/watcher/spec.md` and
      move the change to the archive directory.
- [x] 6.2 Run `openspec validate add-run-log-capture` and confirm
      no issues.
