## 1. Add failing interval tests

- [ ] 1.1 Add a channel-backed observer test proving the first continuous pass is immediate and a second pass does not begin before a short configured `PollInterval` measured from the first pass completion.
- [ ] 1.2 Add a test proving context cancellation during a long configured interval makes `Watcher.Watch` return promptly without another pass.
- [ ] 1.3 Add a test proving a zero interval permits another pass without an intentional delay, then cancel the context to terminate the test deterministically.
- [ ] 1.4 Add focused tests for the five-minute default and interval validation, accepting zero and rejecting negative durations.
- [ ] 1.5 Run the focused watcher tests with a 30-second timeout and confirm the new tests fail before production edits.

## 2. Add the command-line interval setting

- [ ] 2.1 Add the shared five-minute default and `PollInterval time.Duration` to `Watcher`, then pass the parsed interval through `NewWatcher` so production construction remains fully populated.
- [ ] 2.2 Register `--interval` with the standard duration flag parser and help text describing a delay between completed scans.
- [ ] 2.3 Reject negative intervals immediately after flag parsing with an actionable error and exit status 2; preserve zero as the explicit no-delay value.
- [ ] 2.4 Run the focused default and validation tests and confirm they pass.

## 3. Pace the continuous watch loop

- [ ] 3.1 Change continuous `Watcher.Watch` to run its first pass immediately, return immediately on a pass error, and wait on either the configured timer or `ctx.Done()` after each successful pass.
- [ ] 3.2 Bypass the timer when `PollInterval` is zero, preserve the pre-pass cancellation check, and keep `Watcher.Once` free of any interval wait.
- [ ] 3.3 Remove the obsolete tight-loop comment without adding a ticker, clock abstraction, configuration-file field, event type, or Terminal User Interface (TUI) behavior.
- [ ] 3.4 Run the focused watcher tests and confirm delayed repetition, prompt cancellation, zero delay, once mode, and first-error behavior are green.

## 4. Update project guidance

- [ ] 4.1 Update `AGENTS.md` to describe the immediate first pass, five-minute default delay, `--interval=0` escape hatch, and timeout-safe unit-test approach.
- [ ] 4.2 Update `openspec/config.yaml` to replace its tight-poll-loop context and deferred-debt entry with the configurable completion-relative interval.

## 5. Validate and synchronize

- [ ] 5.1 Run `gofmt` on changed Go files, then run `go test -timeout 30s ./...` and `go vet ./...`.
- [ ] 5.2 Run `go build ./...` and manually inspect `see -h` to confirm `--interval` advertises the `5m0s` default.
- [ ] 5.3 Run `openspec validate add-watch-interval --type change --strict` and resolve every reported issue.
- [ ] 5.4 Synchronize the watcher delta into the main specification after implementation and verification.
- [ ] 5.5 Archive `add-watch-interval` only after every implementation and validation task is complete.
