## 1. PiAgent capture

- [ ] 1.1 Add `logPathFor(repo, change string) (string, error)` helper
      in `main.go` that returns the full file path for a `(repo,
      change)` pair and ensures the log directory exists. Reads
      `SEE_LOG_DIR` on each call; falls back to
      `os.UserCacheDir()/see/logs/`. Returns a wrapped error on
      `MkdirAll` failure.
- [ ] 1.2 Modify `PiAgent.Run` to call `logPathFor(path, change)`,
      open the file, set `cmd.Stdout` and `cmd.Stderr` to the file,
      run the command, then `Close()` the file in a `defer`. On
      `logPathFor` or `os.Create` failure, log a warning to `log`
      and proceed without redirection.
- [ ] 1.3 Remove the `RedirectOutput bool` field from `PiAgent`.

## 2. Agent interface

- [ ] 2.1 Extend the `Agent` interface so `Run` takes `(ctx, path,
      change, prompt)`. **BREAKING**: callers and impls must update.
- [ ] 2.2 Update `Watcher.work` to pass the active `change` to
      `w.agent.Run(...)`.
- [ ] 2.3 Update `fakeAgent.Run` in `main_test.go` to accept and
      ignore the new `change` parameter; verify every existing test
      that constructs `PiAgent` or calls `agent.Run` compiles.

## 3. LogPath event

- [ ] 3.1 Add `LogPath struct{ Path string }` to the sealed `Event`
      interface in `main.go`; implement `isEvent()`.
- [ ] 3.2 Add a `case LogPath:` arm to `tuiObserver.Observe` that
      forwards to a new `tui.ChanObserver.LogPath(path)` method.
- [ ] 3.3 Add `LogPathMsg` (or equivalent) to `tui/events.go` and a
      handler in `tui/model.go` that records the most recent log
      path per repo or per change.
- [ ] 3.4 Render the log path in `tui/view.go` alongside the change
      it belongs to (plain text is fine; OSC 8 hyperlinks are a
      future nicety).
- [ ] 3.5 After `w.agent.Run` returns successfully and capture
      succeeded, emit the `LogPath` event from `Watcher.work`
      before returning.

## 4. Log mode surfacing

- [ ] 4.1 In `Watcher.work`, after `w.agent.Run` returns and capture
      succeeded, call `log.Printf("see: log → %s", path)` so log-mode
      runs make the path discoverable. (TUI mode skips this — the
      observer does the surfacing.)
- [ ] 4.2 Decide whether the `log.Printf` belongs in `Watcher.work`
      (always) or only in the log-mode branch (gated on whether an
      observer is wired). **Default**: gate on `w.observer == nil`
      so TUI mode doesn't double-print. Confirm against
      `runTUI`/`runLog` wiring.

## 5. Tests

- [ ] 5.1 Add `TestPiAgentRunWritesLog`: invoke `PiAgent.Run` with a
      fake binary that emits known lines to stdout and stderr;
      assert the file exists at the computed path and contains the
      combined output.
- [ ] 5.2 Add `TestPiAgentRunProceedsWhenLogDirCannotBeCreated`:
      point `SEE_LOG_DIR` at a path that cannot be created (e.g.,
      under a file). Assert a warning is logged and the run's exit
      status reflects only the agent's exit code.
- [ ] 5.3 Add `TestPiAgentRunRespectsSeeLogDir`: set
      `SEE_LOG_DIR=/tmp/see-test-XXX`, invoke `PiAgent.Run`, assert
      the file is under `/tmp/see-test-XXX`, not under
      `UserCacheDir()/see/logs/`.
- [ ] 5.4 Add `TestWorkEmitsLogPathOnSuccessfulCapture`: build a
      `Watcher` with a `recordingObserver`, run `work` against a
      fake repo, assert the recorded event sequence ends with
      `LogPath` carrying the expected path.
- [ ] 5.5 Add `TestWorkDoesNotEmitLogPathOnCaptureFailure`: same as
      5.4 but with the log dir unwritable; assert `LogPath` is
      absent.
- [ ] 5.6 Update existing tests that construct `PiAgent` or call
      `agent.Run` to use the new signature.
- [ ] 5.7 Run `go test ./... -race` and confirm everything passes.

## 6. Spec sync

- [ ] 6.1 Run `openspec sync-specs --change add-run-log-capture`
      to promote the delta spec into `openspec/specs/watcher/spec.md`.
- [ ] 6.2 Run `openspec validate --change add-run-log-capture` and
      confirm no issues.