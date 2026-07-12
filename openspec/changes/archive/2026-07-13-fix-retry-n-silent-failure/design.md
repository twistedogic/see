## Context

`retryN` retries a function up to N times and currently takes
`func() (bool, error)`. The two return values give callers a way to express
"attempt finished without error but did not complete the work" via
`(false, nil)`. The loop then runs `err = e`, clobbering any prior error with
nil. The watcher reports success and the change is silently dropped.

The only caller today is `runOnce` calling `work`, which never actually emits
`(false, nil)` — but the signature permits it, the loop mishandles it, and
the bug is real.

## Goals / Non-Goals

**Goals:**
- Make the silent failure unreachable by removing the state that enables it.
- Keep `Watcher.work`'s internal commit-gating logic intact (the `done` local
  must still drive the post-agent `git add` / `git commit`).
- Land a regression test that pins the new contract.

**Non-Goals:**
- Adding metrics or per-repo outcome reporting (would justify keeping `done`
  externally visible; out of scope for this bug fix).
- Changing the retry policy itself (backoff, max attempts, jitter).
- Touching `runOnce`'s abort-on-first-error behavior.

## Decisions

**Drop `complete bool` entirely rather than just discarding it in the lambda.**

The minimal diff would change `retryN`'s callback signature while leaving
`work`'s unchanged, with the lambda discarding `done`. That's tempting because
it touches fewer lines. But it leaves `work`'s `(bool, error)` signature
documenting a state no caller reads. The honest move is to delete it from
both layers: `work` returns `error`, and `retryN` takes `func() error`.
Inside `work`, the local `done` variable stays — it still gates the commit —
it just no longer flows out.

**New contract property: `error == nil` iff the latest attempt succeeded.**

Collapsing the success signal to `nil` means callers can never express
"finished but failed" or "unfinished but no error." The bug class disappears.

**Test strategy: TDD as drive-design, not bug-reproduction.**

A literal regression test in the *old* signature (e.g.,
`(false, priorErr), (false, nil)` → expect `priorErr`) would prove the old
behavior but cannot survive the signature change. We add it anyway, run it
red, then refactor — at which point the test must be deleted (it no longer
compiles). The new-contract property test (`retryN` returns the last
non-nil error when every attempt errors) is what stays. A code comment
documents the historical bug so the rationale isn't lost with the test.

## Risks / Trade-offs

- **Future caller wants per-attempt outcome visibility** → reintroduce the
  bool at `work`'s boundary. Not needed today; AGENTS.md's "long-term
  maintainability" preference argues for the cleaner contract now and the
  comment-marked seam for later.
- **Zero-retry (`N=0`) silently no-ops** → unchanged from current code. Not
  in scope.
- **`work`'s `done` becomes load-bearing internally only** → if a future
  caller wants "did this repo complete?" they must re-derive it from error
  semantics. Acceptable: the watcher only needs pass/fail.