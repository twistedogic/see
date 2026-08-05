# lane-isolation delta — add-workflow-check

## MODIFIED Requirements

### Requirement: Worktree + auto-merge runs the check gate, rebases onto the operator's branch, and fast-forward merges

When worktree mode is active and `auto_merge` is true, on a successful
agent run `see` SHALL:

0. If the workflow defines a `check` and the agent left working-tree
   changes to land, run the check gate (see workflow-condition). On a
   failed check, trigger the rollback described in the rollback
   requirement and return the check-failure error. On a passed check —
   or when no check is defined, or when the agent left no working-tree
   changes — continue.
1. Run `git add -A` and (if the index differs from `HEAD`) a catch-up
   commit on `see/<digest>` inside the worktree.
2. Run `git rebase <operator-ref>` inside the worktree, where
   `<operator-ref>` is the operator's *current* branch (the
   `originalRef` captured at attempt start) at its current tip — not
   the commit captured at attempt start, so commits the operator made
   during the agent's run replay on top.
3. Re-check the operator's checkout's dirty state. If dirty, abort
   the merge and trigger full rollback.
4. Run `git merge --ff-only see/<digest>` in the operator's checkout.
   If the fast-forward fails (the operator committed between steps 2
   and 4), abort the merge and trigger full rollback.
5. Run `git worktree remove --force <worktree_path>` and `git branch
   -d see/<digest>` to clean up.

If any of steps 0 (check), 2, 3, or 4 fails, `see` SHALL execute the
rollback described in the rollback requirement (worktree removed, lane
deleted, operator's checkout untouched) and return the error.

#### Scenario: Successful auto-merge rebases and merges onto operator's branch

- **WHEN** worktree mode with auto-merge is active, the agent
  succeeded, the operator's branch is `main`, and the operator did
  not commit during the run
- **THEN** `see/<digest>` is rebased onto the operator's current `main`
  tip
- **THEN** `main` is fast-forward merged to `see/<digest>`
- **THEN** the worktree directory and `see/<digest>` are removed
- **THEN** the operator's checkout is on `main` with the agent's
  commits now reachable from `main`

#### Scenario: Passing check precedes the merge

- **WHEN** worktree mode with auto-merge is active, the workflow defines
  a check, the agent succeeded and left changes, and the check exits `0`
- **THEN** the check runs in the worktree directory before the catch-up
  commit
- **AND** the merge proceeds as if no check were defined

#### Scenario: Failed check triggers rollback before any commit

- **WHEN** worktree mode with auto-merge is active, the workflow defines
  a check, and the check exits nonzero
- **THEN** no catch-up commit is created on `see/<digest>`
- **AND** the worktree is removed and `see/<digest>` is deleted (`-D`)
- **AND** the operator's checkout is unchanged
- **AND** the attempt returns the check-failure error

#### Scenario: Operator commits during the agent run

- **WHEN** worktree mode with auto-merge is active, the agent commits
  on `see/<digest>`, and the operator commits on `main` during the
  same run
- **THEN** `see/<digest>` is rebased onto the operator's new `main`
  tip — the operator's commits are preserved and the agent's commits
  replay on top
- **AND** `main` is fast-forward merged to the rebased `see/<digest>`
  tip

#### Scenario: Rebase conflict triggers rollback

- **WHEN** worktree mode with auto-merge is active and `git rebase`
  exits non-zero with a conflict
- **THEN** `git rebase --abort` runs
- **AND** the worktree is removed
- **AND** `see/<digest>` is deleted (`-D`)
- **AND** the operator's checkout is unchanged
- **AND** the attempt returns the rebase error

#### Scenario: Operator dirty at merge time triggers rollback

- **WHEN** worktree mode with auto-merge is active, the rebase
  succeeded, and the operator's checkout is dirty when the merge is
  attempted
- **THEN** the merge does not run
- **AND** the worktree is removed
- **AND** `see/<digest>` is deleted (`-D`)
- **AND** the operator's dirty edits are preserved
- **AND** the attempt returns a dirty-merge-time error

#### Scenario: Fast-forward failure triggers rollback

- **WHEN** worktree mode with auto-merge is active, the rebase
  succeeded, the operator's checkout is clean, but `git merge
  --ff-only` fails because the operator committed between rebase and
  merge
- **THEN** `git merge --abort` runs
- **AND** the worktree is removed
- **AND** `see/<digest>` is deleted (`-D`)
- **AND** the operator's checkout is unchanged (operator's late commit
  is preserved on their branch)
- **AND** the attempt returns the merge error

### Requirement: Worktree + manual-merge runs the check gate, rebases onto the operator's branch, and preserves the lane

When worktree mode is active and `auto_merge` is false, on a successful
agent run `see` SHALL:

0. If the workflow defines a `check` and the agent left working-tree
   changes to land, run the check gate (see workflow-condition). On a
   failed check, trigger the rollback described in the rollback
   requirement and return the check-failure error. On a passed check —
   or when no check is defined, or when the agent left no working-tree
   changes — continue.
1. Run `git add -A` and (if the index differs from `HEAD`) a catch-up
   commit on `see/<digest>` inside the worktree.
2. Run `git rebase <operator-ref>` inside the worktree, where
   `<operator-ref>` is the operator's *current* branch tip.

The worktree directory and `see/<digest>` branch SHALL remain in place
after a successful run for the operator to inspect and merge manually.

On agent failure, check failure, or rebase failure, rollback SHALL apply
as for auto-merge (worktree removed, lane deleted).

#### Scenario: Successful manual-merge rebases onto operator's branch

- **WHEN** worktree mode with `auto_merge: false` is active and the
  agent succeeded
- **THEN** `see/<digest>` is rebased onto the operator's current branch
  tip
- **AND** the worktree directory and `see/<digest>` remain in place for
  manual review

#### Scenario: Failed check in manual-merge mode triggers rollback

- **WHEN** worktree mode with `auto_merge: false` is active, the
  workflow defines a check, and the check exits nonzero
- **THEN** no catch-up commit is created on `see/<digest>`
- **AND** the worktree is removed and `see/<digest>` is deleted (`-D`)
- **AND** the operator's checkout is unchanged
- **AND** the attempt returns the check-failure error

#### Scenario: Rebase conflict in manual-merge mode triggers rollback

- **WHEN** worktree mode with `auto_merge: false` is active and the
  rebase fails
- **THEN** rollback runs (worktree removed, lane deleted) and the
  attempt returns the rebase error

#### Scenario: Agent failure in manual-merge mode triggers rollback

- **WHEN** worktree mode with `auto_merge: false` is active and the
  agent fails
- **THEN** rollback runs (worktree removed, lane deleted) and the
  attempt returns the agent error

### Requirement: Worktree-mode rollback restores operator's checkout and deletes lane

When worktree mode is active, on any failure path (agent error, check
failure, rebase error, merge error, or pre-attempt check) `see` SHALL
execute the following cleanup steps, in order, ignoring individual step
failures and emitting `Warning` events for any step that fails:

1. `git rebase --abort` if a rebase is in progress.
2. `git merge --abort` if a merge is in progress.
3. `git worktree remove --force <worktree_path>`.
4. `git branch -D see/<digest>`.

The operator's checkout SHALL remain on its original branch and SHALL
NOT be modified by any of these steps.

#### Scenario: Failure cleanup removes worktree and lane

- **WHEN** worktree mode is active and the attempt fails for any
  reason, including a failed workflow check
- **THEN** the worktree directory is removed (or `Warning` is emitted if
  the remove fails)
- **AND** `see/<digest>` is deleted (or `Warning` is emitted if the
  delete fails)
- **AND** the operator's checkout is on its original branch
- **AND** the operator's checkout's commit history is unchanged

#### Scenario: Stale worktree dir on next attempt

- **WHEN** a previous attempt failed and left a stale worktree directory
- **THEN** the next attempt's `git worktree prune` removes the stale
  metadata
- **AND** `git worktree add -B` creates a fresh worktree on the existing
  lane branch
