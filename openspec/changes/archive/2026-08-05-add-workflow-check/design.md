# Design — add-workflow-check

## The seam

The check is a per-workflow quality gate inserted between "agent run
succeeded" and "any git operation that lands work." It runs over the
agent's *uncommitted* working-tree output, before the catch-up commit in
every mode. This ordering is forced by the rollback-to-clean-slate
decision (see Alternatives): because a failed check discards the attempt
entirely, there is never a reason to commit work that might be thrown
away.

```
resolve change → ensure lane/worktree → run agent
   │ (success)
   ▼
working tree dirty? ── no ──▶ no-op (skip check + commit)   [unchanged]
   │ yes
   ▼
workflow has check? ── no ──▶ land (existing path)          [unchanged]
   │ yes
   ▼
run check (sh -c, in agent's cwd, {change} rendered)
   │
   ├─ exit 0 ──▶ land (catch-up commit / rebase + merge)    [unchanged mechanics]
   └─ nonzero ──▶ rollback to clean slate + return *checkFailedError
```

## Per-mode landing and rollback

| mode | check cwd | on pass | on fail (clean slate) |
|---|---|---|---|
| branch + custom | lane checkout (= operator's) | catch-up commit; lane stays | reset --hard to pre-attempt tip + `git clean -fd`; lane kept if pre-existing |
| worktree + auto | worktree dir | catch-up commit + rebase + ff-merge; lane/worktree removed | worktree removed + lane deleted (`-D`); operator untouched |
| worktree + manual | worktree dir | catch-up commit + rebase; lane/worktree left for review | worktree removed + lane deleted (`-D`); operator untouched |

All three failure paths reuse the existing rollback functions
(`rollbackWorkflowLane`, `rollbackWorktree`) unchanged; a check failure
is simply a new caller of each.

## No-op skip

The check runs only when the agent left working-tree changes to land,
detected the same way the existing no-op is: the working tree is dirty
(tracked or non-ignored untracked changes) after the agent run. A clean
working tree means the agent committed everything itself or changed
nothing — the existing warning-free no-op — and the check is skipped,
so an idempotent successful poll never spins up a test suite.

## Retry and the terminal event

A check failure returns a `*checkFailedError` sentinel carrying the
rendered command, exit code, and captured stderr. Because it is an
error, it flows through `runWithRetry` and is retried like an agent
error (the agent gets up to `RetryCount` fresh attempts per poll to
satisfy the gate; each retry re-resolves the change and re-runs the
agent on a clean slate). `RetryAttempt` events carry the prior error's
summary between attempts, unchanged.

The terminal event emitted after the final attempt is selected by type:

- `errors.As(err, &checkFailedError{})` → emit `CheckFailed` (carries
  command, exit code, stderr)
- otherwise → emit `ChangeFailed` (unchanged)

This avoids double-emission: `CheckFailed` replaces `ChangeFailed` for
the check-failure outcome, never both.

## Check command execution

`runCheck` mirrors `resolveCustomCondition`'s shell plumbing (same
`exec` setup, context attachment, Unix process-group isolation,
cancellation) but with binary semantics: any nonzero exit is a failure,
and stderr is captured into the `checkFailedError`. It runs in the
agent's `cwd` (the lane checkout in branch mode, the worktree directory
in worktree mode) so the command observes exactly what the agent
produced.

## Alternatives considered

- **Hold / drafts-branch on failure (rejected).** Keep the agent's work
  committed to the lane but block the merge until the check passes, so
  progress accumulates across polls. Rejected because the operator chose
  clean-slate rollback: simpler lane lifecycle, no unbounded
  accumulation, no ambiguity about what a "failing" lane contains. The
  cost is that partial progress is discarded; acceptable per the
  decision.
- **Terminate the poll on check failure (rejected).** Emit `CheckFailed`
  immediately and do not retry within the poll. Cheaper when checks are
  persistently unfixable, but it removes the agent's chance to satisfy
  the gate within a poll. Rejected because "pass the check" is the task,
  and retrying the task on failure is consistent with how agent errors
  already behave; `RetryCount` bounds the cost.
- **Message-only vs structural event (chosen: structural).** A check
  failure could surface only as a `ChangeFailed` whose message says
  "check failed." Chosen a distinct `CheckFailed` event because the TUI
  and JSONL consumers benefit from distinguishing "the agent failed"
  from "the agent's output failed my gate," and the cost is one sentinel
  type, one `errors.As` branch, and one TUI case.
