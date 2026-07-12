## Why

`retryN` returns `nil` when a later attempt returns `(false, nil)` because the
loop clobbers the prior error with `err = e`. The watcher can then claim a
change succeeded when no attempt ever completed. The `complete bool` return on
both `retryN`'s callback and `work()` enables the bug: callers can emit an
ambiguous "not done, no error" state that the loop mishandles. Collapsing the
contract to `func() error` removes the ambiguity and the bug in one move.

## What Changes

- Change `retryN`'s callback signature from `func() (bool, error)` to
  `func() error`. Remove the `complete bool` from the success signal.
- Change `Watcher.work` from `(bool, error)` to `error`. Keep the internal
  `done` local: it still gates the post-agent `git add` / `git commit`.
- Update the caller in `Watcher.runOnce` to match the new signatures.
- Update `TestWorkCommitsOnSuccess` for the new `work` return shape.
- Add a regression test that asserts `retryN` returns the last non-nil error
  when every attempt errors. Record the original silent-failure scenario in a
  comment so the rationale survives.

**BREAKING**: the internal signatures of `retryN` and `Watcher.work` change.
Only `main_test.go` is affected — no external API.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none — this change does not alter user-visible behavior, only the internal
contract between `retryN`, `work`, and the watcher loop. Existing specs, if
any were present, would not need delta updates.)

## Impact

- `main.go`: `retryN`, `Watcher.work`, `Watcher.runOnce`.
- `main_test.go`: `TestWorkCommitsOnSuccess` signature update; one new test.
- No dependency changes.
- No CLI flag changes.
- No change to git, OpenSpec, or `pi` integration behavior.