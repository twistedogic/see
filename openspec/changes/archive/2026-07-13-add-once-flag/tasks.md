## 1. Watcher

- [x] 1.1 Add `Once bool` field to the `Watcher` struct in `main.go`.
- [x] 1.2 In `Watcher.Watch`, call `runOnce` once before the loop and return
      when `Once` is true (mirroring the loop body's error-return path).

## 2. CLI

- [x] 2.1 Register a top-level `--once` boolean flag in `main` with the
      help string "run one scan and exit".
- [x] 2.2 Assign `w.Once = *once` after `NewWatcher` so the flag value
      reaches the watcher.

## 3. TUI wiring

- [x] 3.1 In `runTUI`, after starting the watcher goroutine, start a second
      goroutine that reads from the watcher's error channel and calls
      `prog.Quit()` on the returned Bubble Tea program.

## 4. Tests

- [x] 4.1 Add `TestWatchReturnsAfterOnePassWhenOnce`: build a watcher with
      `Once: true`, an agent that records one invocation, and assert that
      `Watch` returns nil and the agent ran exactly once.
- [x] 4.2 Add `TestWatchLoopsUntilCtxCancelWhenNotOnce`: build a watcher
      with `Once: false`, an agent that records invocations, cancel the
      context after a short delay, and assert that `Watch` returns nil
      and the agent ran more than once (or zero times if cancellation
      beat the first pass — assert at least the cancellation propagated).
- [x] 4.3 Add `TestWatchStopsOnFirstPassError`: build a watcher with
      `Once: false`, an agent that returns an error, and assert that
      `Watch` returns that error and the agent ran exactly once.
- [x] 4.4 Run `go test ./...` and confirm everything passes with
      `-race`.

## 5. Spec sync

- [x] 5.1 Run `openspec sync-specs --change add-once-flag` to promote the
      delta spec into `openspec/specs/watcher/spec.md`.
- [x] 5.2 Verify `openspec validate --change add-once-flag` reports no
      issues.