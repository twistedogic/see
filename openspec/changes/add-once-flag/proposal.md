## Why

`see` watches a directory of repositories in a tight loop, dispatching an agent
against any active OpenSpec change it finds. For interactive use that is exactly
what we want; for smoke tests it is the problem — there is no way to invoke the
binary, let it do one pass, and exit, so the calling process has to kill it
manually.

This change adds a `--once` flag that limits the watcher to a single pass and
makes the binary exit cleanly afterwards, in both `--mode=log` and `--mode=tui`.

## What Changes

- Add a top-level boolean flag `--once` to `see`.
- When `--once` is set, `Watcher.Watch` returns after the first call to
  `runOnce` instead of looping.
- When `--once` is set in TUI mode, the Bubble Tea program exits once the
  watcher returns, so the binary does not hang waiting for user input.
- New tests cover the once-vs-loop behavior of `Watcher.Watch`.
- The watcher spec gains a requirement describing the once-mode contract.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `watcher`: add a requirement covering `Watcher.Once` and the short-circuit
  behavior of `Watcher.Watch` when it is set. The per-pass contract (branch
  handling, rollback, merge) is unchanged.

## Impact

- `main.go`: `Watcher` gains an `Once` field; `Watcher.Watch` gains a
  short-circuit; `main` registers the flag; `runTUI` wires a goroutine that
  calls `prog.Quit()` when the watcher exits.
- `main_test.go`: new tests for `Watcher.Watch` with `Once` true and false.
- `openspec/specs/watcher/spec.md`: new requirement + scenarios.
- No new dependencies. No breaking changes to the public surface — `Watcher`
  is constructed inside `main` and tests use struct literals, so adding a
  field is source-compatible.