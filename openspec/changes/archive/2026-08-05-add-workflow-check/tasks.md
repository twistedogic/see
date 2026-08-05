## 1. Configuration

- [ ] 1.1 Add `Check string \`yaml:"check"\`` to `WorkflowConfig` in `config.go`.
- [ ] 1.2 Extend the per-workflow field validation to reject a present-but-blank/whitespace-only `check`, naming the workflow and the field (treat `check` as optional, so absent stays valid).
- [ ] 1.3 Add a commented `check:` line to the example `workflows:` entry in `config.example.yaml`.

## 2. Workflow files

- [ ] 2.1 Add `Check string` to the frontmatter decode struct in `workflow_files.go`; thread it onto the produced `WorkflowConfig`.
- [ ] 2.2 Add `check` to the accepted frontmatter-keys set so an unknown key is still rejected.

## 3. Check execution

- [ ] 3.1 Add `runCheck(ctx, cwd, command string) error` mirroring `resolveCustomCondition`'s shell/context/process-group plumbing, with binary exit semantics (nonzero = failure) and stderr capture.
- [ ] 3.2 Add a `checkFailedError` sentinel type carrying the rendered command, exit code, and stderr; implement `Error()` so `errors.As` and the summary helper work.
- [ ] 3.3 Render `{change}` into the check command via `renderTemplate` before execution.

## 4. Landing-path integration

- [ ] 4.1 Branch + custom: in `workResolved`, after a successful agent run and before `catchUpCustomCommit`, if the workflow has a check and the working tree is dirty, run it; on fail route to `rollbackWorkflowLane` with the `checkFailedError` and return it; on pass / no-check / no-changes proceed.
- [ ] 4.2 Worktree + auto-merge: run the check before the catch-up commit inside `mergeWorktreeLane`; on fail route to `rollbackWorktree` and return the error.
- [ ] 4.3 Worktree + manual-merge: run the check before `rebaseWorktreeLane`; on fail route to `rollbackWorktree` and return the error.
- [ ] 4.4 Confirm the no-op skip (clean working tree) bypasses the check in all three paths.

## 5. Terminal event

- [ ] 5.1 Add a `CheckFailed` event type (fields: `Path`, `Workflow`, `Change`, `Command`, `ExitCode`, `Err`, plus a `summary`) implementing `isEvent()`.
- [ ] 5.2 In `runOnce`, after `runWithRetry` returns the final error, select `CheckFailed` (via `errors.As` on `checkFailedError`) vs `ChangeFailed` as the terminal event; never emit both.
- [ ] 5.3 Ensure `RetryAttempt` between attempts carries the check-failure summary unchanged.

## 6. TUI

- [ ] 6.1 Add a `CheckFailed` case to the TUI event type-switch; render it like `ChangeFailed` (error column / failed state) with a "check failed" label.

## 7. Tests

- [ ] 7.1 Config: a present-but-blank `check` is rejected at startup; an absent `check` is accepted and behaves as before.
- [ ] 7.2 Workflow files: `check` is accepted in frontmatter; an unknown frontmatter key is still rejected.
- [ ] 7.3 Branch mode: a passing check precedes the catch-up commit; a failing check rolls back to the pre-attempt tip, creates no commit, and yields a `CheckFailed` terminal event; a no-op run skips the check.
- [ ] 7.4 Worktree + auto-merge: a passing check precedes rebase + ff-merge; a failing check removes the worktree, deletes the lane, leaves the operator untouched, and yields `CheckFailed`.
- [ ] 7.5 Worktree + manual-merge: a failing check rolls back and yields `CheckFailed`.
- [ ] 7.6 Retry: a check failure retries up to `RetryCount`; the final failure emits `CheckFailed`; `RetryAttempt` summaries are present between attempts.
- [ ] 7.7 `{change}` substitution in `check`; cancellation stops a running check.

## 8. Docs

- [ ] 8.1 `AGENTS.md`: add `check` to the workflow-entry schema, to the frontmatter keys list, and add a "Check gate" subsection (shell contract, ordering, no-op skip, rollback-to-clean-slate, retry behavior).
- [ ] 8.2 Run `openspec validate --change add-workflow-check` and resolve any reported delta issues.
