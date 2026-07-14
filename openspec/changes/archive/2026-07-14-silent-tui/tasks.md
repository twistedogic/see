## 1. Event types

- [x] 1.1 Add `Warning{Path, Change, Msg}` to the `Event` interface in `main.go`; implement `isEvent()`
- [x] 1.2 Add `InfraError{Where, Err}` to the `Event` interface in `main.go`; implement `isEvent()`

## 2. ensureLogDir and event-log package

- [x] 2.1 Add `ensureLogDir()` in `main.go`: reads `SEE_LOG_DIR`, falls back to `os.UserCacheDir()/see/logs`, runs `MkdirAll(0o755)`, returns the directory and a wrapped error on failure
- [x] 2.2 Add `eventLogPath(dir string) string` returning `dir/see--<utc-ts>--<pid>.jsonl`
- [x] 2.3 Create `eventlog.go` in the `main` package with an `eventLogger` type that holds `*os.File`, `*json.Encoder`, and an optional secondary `Observer`
- [x] 2.4 Implement `eventLogger.Observe(Event)` that encodes to the file first, then forwards to the secondary observer if non-nil; swallow encode errors best-effort
- [x] 2.5 Implement `eventLogger.Close()` that closes the file; return any close error

## 3. PiAgent.Run: remove fallback, surface capture failure

- [x] 3.1 Replace `logPathFor`'s fallback branch with `pathFor(repo, change)` — pure filename computation, no `MkdirAll`
- [x] 3.2 In `PiAgent.Run`, accept the log directory as a parameter; compute `logPath` directly from it
- [x] 3.3 Replace the `if logErr == nil { ... } else { log.Printf(...) }` cascade with a single `os.Create` call; on error, return `("", err)` without invoking the agent
- [x] 3.4 Remove the `log.Printf("see: log file create failed ...")`, `log.Printf("see: log dir unavailable ...")`, and `log.Printf("see: log → %s", ...)` calls

## 4. Migrate watcher log.Printf sites to events

- [x] 4.1 In `Watcher.work`, replace `log.Printf("detached HEAD on %s; switch to a branch first", path)` with an observer `Warning` event carrying the same message; keep the returned error
- [x] 4.2 Replace `log.Printf("no active change on %s", path)` — emit nothing (no-op case stays silent) or, if the design decides to surface it, emit a `Warning` instead
- [x] 4.3 Replace `log.Printf("working %q on %s", change, path)` — delete (the `ChangeStarted` event already covers this)
- [x] 4.4 Replace `log.Printf("failed %q on %s: %v", change, path, runErr)` — delete (the `ChangeFailed` event already carries this)
- [x] 4.5 Replace each rollback-step `log.Printf` (switch / reset --hard / branch -D) with a `Warning` event that names the step
- [x] 4.6 Replace `log.Printf("completed %q on %s", change, path)` — delete (the `ChangeDone` event already covers this)
- [x] 4.7 Replace the `git add failed` and `git commit failed` `log.Printf` calls with `Warning` events
- [x] 4.8 Replace the `merge --no-ff failed`, `merge --abort failed`, and `branch -d failed` `log.Printf` calls with `Warning` events
- [x] 4.9 Replace `log.Printf("skipping %s: no commits", repo)` in `runOnce` — delete or migrate to `Warning`

## 5. main() and runTUI() wiring

- [x] 5.1 In `main()`, call `ensureLogDir()` immediately after `selectRunMode` resolves; on error print to stderr and `os.Exit(2)`
- [x] 5.2 Construct the `eventLogger` in `main()` with the validated directory; defer `Close()`
- [x] 5.3 In `runTUI`, attach the bubbletea `ChanObserver` as the secondary observer on the `eventLogger`; assign the `eventLogger` as `Watcher.observer`
- [x] 5.4 In `runTUI`, replace `log.Printf("watcher: %v", err)` with `events.Observe(InfraError{Where: "watcher", Err: err})`
- [x] 5.5 In `runTUI`, replace `fmt.Fprintln(os.Stderr, "see:", runErr)` with `events.Observe(InfraError{Where: "tui", Err: runErr})` (only emit if non-nil)
- [x] 5.6 Pass the log directory into `PiAgent` via a constructor field (e.g. `PiAgent{Binary, LogDir}`) or via the `Watcher` struct

## 6. TUI package changes

- [x] 6.1 Add `Warning bool` to `RepoRow`
- [x] 6.2 Add `WarningMsg{Path, Change, Msg}` and `InfraErrorMsg{Where, Err}` to `tui/events.go`
- [x] 6.3 Add `Warning()` and `InfraError()` methods on `ChanObserver` that send the messages on the bubbletea program channel
- [x] 6.4 In `tui/model.go`, add `infraErr string` to `Model`
- [x] 6.5 In `Model.Update`, handle `WarningMsg` by setting `row.Warning = true` and clearing it on the next `ChangeStartedMsg` for the same repo
- [x] 6.6 In `Model.Update`, handle `InfraErrorMsg` by setting `model.infraErr = msg.Err`
- [x] 6.7 In `tui/view.go`, drop the `showErr` column-width threshold; rename `showAge` to a single threshold; render the `⚠` suffix in `renderRow` when `r.Warning` is true; remove all `LastErr` rendering
- [x] 6.8 In `renderFooter`, add a `warning` counter that walks `m.rows` and counts rows with `Warning == true`; append `· N warning` to the summary when non-zero
- [x] 6.9 In `View`, render the `infraErr` banner between the body and the footer when non-empty

## 7. tuiObserver type-switch in main.go

- [x] 7.1 Add `case Warning:` and `case InfraError:` branches in `tuiObserver.Observe` that call the matching `ChanObserver` method

## 8. Test updates

- [x] 8.1 Delete `TestPiAgentProceedsWhenLogDirCannotBeCreated` from `main_test.go`
- [x] 8.2 Add `TestEnsureLogDirExitsWhenLogDirCannotBeCreated`: set `SEE_LOG_DIR` to a path under a regular file, invoke `main` (or a refactored entry point that calls `ensureLogDir`), assert exit `2` and a stderr line naming the directory
- [x] 8.3 Add `TestPiAgentRunSurfacesFileCreateError`: with a read-only per-run path (e.g. file exists where the new file would go), assert `Run` returns `("", err)` and does not invoke the agent
- [x] 8.4 Add a test that wires an `eventLogger` over a `t.TempDir`, observes a sequence of events, then reads the JSONL back and asserts each event round-trips through JSON

## 9. Validation

- [x] 9.1 Run `go build ./...` and `go test ./...` and confirm green
- [x] 9.2 Run `openspec validate silent-tui` and confirm green
- [x] 9.3 Manual smoke: invoke `see --mode=tui` against a fixture with one repo and one change; confirm the grid renders with no stdout/stderr noise
- [x] 9.4 Manual smoke: invoke `see --mode=log` against the same fixture; confirm stdout and stderr are empty and the JSONL under `SEE_LOG_DIR/see--<ts>--<pid>.jsonl` contains the full event timeline
- [x] 9.5 Manual smoke: set `SEE_LOG_DIR` to an invalid path; confirm `see` exits `2` with a single stderr line and no JSONL
