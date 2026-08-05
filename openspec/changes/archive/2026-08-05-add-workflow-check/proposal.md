## Why

A configured workflow's agent self-certifies its own output: after a
successful run, `see` stages and lands the agent's changes (a catch-up
commit in branch mode, a rebase plus fast-forward merge in worktree +
auto-merge) with no independent verification. There is no in-process
lever to say "run my own command before you trust the agent's work."
Operators who want a test suite, linter, or `openspec validate` to gate
landing can only inspect after the fact.

## What Changes

- Add an optional `check` string field to each workflow entry. When
  present and nonblank, `see` runs the command as a quality gate after a
  successful agent run and before any git landing operation.
- The check uses the same platform-shell contract as `condition`
  (`/bin/sh -c` on Unix, `cmd.exe /C` on Windows; watcher context
  attached; own process group on Unix), executed in the agent's working
  directory (the lane checkout in branch mode, the worktree directory in
  worktree mode) so it observes the agent's output.
- Exit semantics are binary: status `0` passes and landing proceeds; any
  nonzero status fails the check. Standard error is captured into the
  failure.
- The `{change}` token is substituted in `check` under the same rule as
  `prompt`, `condition`, and `commit`.
- The check runs only when the agent left working-tree changes to land;
  an idempotent no-op run skips the check (and the catch-up commit),
  preserving the existing warning-free no-op.
- On a failed check, `see` rolls back to a clean slate through the
  existing per-mode rollback (branch mode: reset to the pre-attempt lane
  tip and remove the attempt's untracked files; worktree mode: remove the
  worktree and delete the lane; the operator's checkout is untouched in
  worktree mode). No commit is ever created for failed-check work.
- A check failure returns an error and therefore flows through the
  existing retry loop (`runWithRetry`), giving the agent up to
  `RetryCount` fresh attempts per poll to satisfy the gate. When retries
  are exhausted, the terminal event is a new `CheckFailed` event
  (distinct from `ChangeFailed`) so the Terminal User Interface (TUI)
  and the JavaScript Object Notation Lines (JSONL) stream can report
  "check failed" honestly.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `workflow-condition`: the `{change}` token-render requirement extends
  to `check`; a new requirement defines the check gate, its shell
  contract, binary exit semantics, no-op skip, `{change}` substitution,
  and rollback-to-clean-slate on failure.
- `workflow-files`: the `.md` frontmatter accepted-keys set gains
  `check` as an optional key.
- `lane-isolation`: the worktree + auto-merge and worktree +
  manual-merge success requirements gain a check gate before the
  catch-up commit, and the worktree rollback requirement adds check
  failure to its trigger set.
- `watcher`: the branch-mode custom success path gains the check gate
  before staging; the branch-mode custom rollback requirement adds check
  failure to its triggers; a new requirement defines the `CheckFailed`
  event and its selection as the terminal event (in place of
  `ChangeFailed`) when the final attempt failed at the check.
- `event-log`: the batch-level JSONL stream gains a `CheckFailed`
  payload variant mirroring `ChangeFailed` plus the rendered check
  command, exit code, and captured standard error.

## Impact

- `config.go`: new `Check string \`yaml:"check"\`` field on
  `WorkflowConfig`; validation that a present `check` is nonblank
  (whitespace-only rejected) alongside the existing per-field checks.
- `workflow_files.go`: new `Check string` field on the frontmatter
  decode struct, threaded onto the produced `WorkflowConfig`, and added
  to the accepted-keys set.
- `main.go`: a `runCheck` helper mirroring `resolveCustomCondition`'s
  shell/context/process-group plumbing with binary exit semantics and
  stderr capture; a `checkFailedError` sentinel; insertion of the check
  gate into the three landing paths (branch custom catch-up, worktree
  auto-merge, worktree manual-merge), each routing failure to its
  existing rollback; `runOnce` selects `CheckFailed` vs `ChangeFailed`
  as the terminal event via `errors.As`.
- `tui/`: a new `CheckFailed` case that renders like `ChangeFailed`
  (error column / failed state) with a "check failed" label.
- `AGENTS.md`: the workflow-entry schema gains `check`; the frontmatter
  keys list gains `check`; a "Check gate" subsection documents the
  contract, ordering, no-op skip, rollback, and retry behavior.
- `config.example.yaml`: a commented `check:` line in the example
  `workflows:` entry.
- No new dependencies. No breaking changes: a workflow with no `check`
  field behaves exactly as today.
