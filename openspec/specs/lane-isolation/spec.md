# lane-isolation

## Purpose

Define the three isolation modes `see` offers for the per-change
lane (`see/<digest>`): branch mode (the historical default), worktree
mode with auto-merge, and worktree mode with manual review. Lane
isolation is the operator-facing contract for "where does the agent
run, and what happens to my branch on success or failure". This
capability scopes the new modes; the existing branch-mode contract
lives in the watcher capability and remains authoritative for
`worktree: false`.
## Requirements
### Requirement: Three isolation modes are explicit and named

`see` SHALL support three explicit isolation modes, selected by the pair
`(worktree, auto_merge)`:

- **branch**: `worktree: false` (default). The agent runs in the
  operator's checkout. The lane (`see/<digest>`) is created on that
  checkout. On success the operator's checkout is left checked out on
  the lane. The lane lives until the operator removes it.
- **worktree + auto-merge**: `worktree: true, auto_merge: true`
  (default when `worktree: true`). The agent runs in a `git worktree`
  linked to the operator's checkout. The operator's checkout is never
  switched. On success the lane is rebased onto the operator's current
  branch tip and fast-forward merged into it; the lane and worktree
  are removed after merge.
- **worktree + manual-merge**: `worktree: true, auto_merge: false`.
  The agent runs in a `git worktree` linked to the operator's checkout.
  The operator's checkout is never switched. On success the lane is
  rebased onto the operator's current branch tip and left for manual
  review; the lane and worktree are preserved.

#### Scenario: Default mode is branch

- **WHEN** the configuration does not set `worktree` or sets it to `false`
- **THEN** the watcher uses branch mode for every watched repository

#### Scenario: worktree with auto_merge selects auto-merge mode

- **WHEN** `worktree: true` and `auto_merge: true` (or unset)
- **THEN** the watcher uses worktree + auto-merge mode for every
  watched repository

#### Scenario: worktree without auto_merge selects manual-merge mode

- **WHEN** `worktree: true` and `auto_merge: false`
- **THEN** the watcher uses worktree + manual-merge mode for every
  watched repository

### Requirement: --worktree flag and config field select worktree mode

`see` SHALL expose a `--worktree` command-line flag and a top-level
`worktree` configuration field. Both SHALL be boolean. The default
SHALL be `false` (branch mode).

#### Scenario: --worktree enables worktree mode at startup

- **WHEN** `see --worktree` is invoked with `worktree: false` in the
  configuration
- **THEN** worktree mode is enabled for the duration of the process
- **AND** the auto-merge default (`true`) applies

#### Scenario: --worktree=false overrides config

- **WHEN** the configuration sets `worktree: true` and `see` is invoked
  with `--worktree=false`
- **THEN** branch mode is used for the duration of the process

#### Scenario: Config worktree: true without flag

- **WHEN** the configuration sets `worktree: true` and `--worktree` is
  not passed
- **THEN** worktree mode is enabled with `auto_merge: true`

### Requirement: --auto-merge flag and config field control merge-back in worktree mode

`see` SHALL expose an `--auto-merge` command-line flag and a top-level
`auto_merge` configuration field. Both SHALL be boolean. The default
SHALL be `true`.

`--auto-merge=false` (or `auto_merge: false`) SHALL only be effective
when worktree mode is active. When worktree mode is active and
`auto_merge` is true, the lane is fast-forward merged into the
operator's branch after a successful run. When worktree mode is
active and `auto_merge` is false, the lane is rebased onto the
operator's branch tip and left for manual review.

#### Scenario: --auto-merge=false opts out of merge-back

- **WHEN** worktree mode is enabled and `--auto-merge=false` is set
- **THEN** the lane is rebased onto the operator's branch tip but not
  merged
- **AND** the lane branch and worktree directory are preserved after
  the run

#### Scenario: Default auto-merge applies when unset

- **WHEN** worktree mode is enabled and `auto_merge` is unset or
  explicitly `true`
- **THEN** the lane is rebased and fast-forward merged into the
  operator's branch after a successful run
- **AND** the lane branch and worktree directory are removed after
  merge

#### Scenario: --auto-merge in branch mode is ignored

- **WHEN** branch mode is active (worktree: false) and `--auto-merge`
  is set
- **THEN** the `--auto-merge` value is not consulted for branch mode
  behavior
- **AND** no error or warning is emitted for the ignored value

### Requirement: worktree_root config field controls worktree location

`see` SHALL accept a top-level `worktree_root` configuration field as
a string path. The default SHALL be `~/.cache/see/worktrees`. The
field SHALL be tilde-expanded using the same rule as `root_dir`.

`worktree_root` SHALL only be effective when worktree mode is active.
In worktree mode, every new worktree SHALL be created at
`<worktree_root>/<repo-basename>--<digest>/`.

#### Scenario: Default worktree_root is used

- **WHEN** worktree mode is enabled and `worktree_root` is unset
- **THEN** worktrees are created under `<home>/.cache/see/worktrees/`

#### Scenario: Custom worktree_root is used

- **WHEN** worktree mode is enabled and the configuration sets
  `worktree_root: ~/Dev/.see-worktrees`
- **THEN** worktrees are created under `<home>/Dev/.see-worktrees/`

#### Scenario: tilde in worktree_root is expanded

- **WHEN** the home directory is `/home/alice` and `worktree_root:
  "~/see-worktrees"`
- **THEN** the resolved worktree root is `/home/alice/see-worktrees`

### Requirement: Validation rejects invalid combinations at startup

`see` SHALL validate the `(worktree, auto_merge, worktree_root)`
combination at startup, before the watcher begins, and reject any of
the following with an actionable error and exit status `2`:

- `auto_merge: true` with `worktree: false`.
- `auto_merge: false` with `worktree: false` (same reason: auto-merge
  has no meaning outside worktree mode).
- `worktree_root: <non-empty>` with `worktree: false`.

Validation SHALL NOT fire when only `worktree: true` is set; in that
case `auto_merge` defaults to `true` and `worktree_root` defaults to
the standard hidden location.

#### Scenario: auto_merge without worktree is rejected

- **WHEN** the configuration sets `worktree: false` and
  `auto_merge: true`
- **THEN** `see` prints an actionable error identifying the
  configuration file and the `auto_merge` field
- **AND** exits with status `2`
- **AND** the watcher does not start

#### Scenario: worktree_root without worktree is rejected

- **WHEN** the configuration sets `worktree: false` and
  `worktree_root: ~/somewhere`
- **THEN** `see` prints an actionable error identifying the
  configuration file and the `worktree_root` field
- **AND** exits with status `2`
- **AND** the watcher does not start

#### Scenario: --auto-merge without --worktree is rejected

- **WHEN** `see --auto-merge=false` is invoked without `--worktree`
  and the configuration has `worktree: false`
- **THEN** `see` prints an actionable error identifying the
  `--auto-merge` flag and the requirement for `--worktree`
- **AND** exits with status `2`

### Requirement: Worktree mode runs the agent in a git worktree

When worktree mode is active, before invoking the agent, `see` SHALL
ensure a `git worktree` exists at `<worktree_root>/<repo-basename>--<digest>/`,
linked to the operator's checkout's `.git`. The lane branch SHALL be
named `see/<digest>` (same naming as branch mode).

`see` SHALL prune stale worktree metadata (`git worktree prune`)
before each worktree ensure, then either reuse the existing worktree
(via `git worktree add --force`) or create a new one (via
`git worktree add -B see/<digest> <path> <start-point>`). The
start-point SHALL be the operator's current `HEAD` commit when the
lane branch does not exist, and the existing lane branch tip when it
does. Reusing the lane preserves prior commits.

The agent SHALL be invoked with `cwd` set to the worktree directory.
The operator's checkout SHALL NOT be modified by `see` during worktree
ensure, worktree-based agent invocation, rebase, or merge.

#### Scenario: First worktree run creates the worktree and lane

- **WHEN** worktree mode is enabled for a repo at `~/Dev/playground`
  with `HEAD` at `main @ abc1234`, an active change resolves to digest
  `XYZ`, and no `see/XYZ` lane or worktree exists
- **THEN** `~/.cache/see/worktrees/playground--XYZ/` exists
- **AND** its `.git` file points at
  `~/Dev/playground/.git/worktrees/XYZ`
- **AND** the lane branch `see/XYZ` exists at `abc1234`
- **AND** the operator's checkout `~/Dev/playground` is unchanged
- **AND** the agent is invoked with `cwd` set to the worktree
  directory

#### Scenario: Reused worktree preserves prior lane commits

- **WHEN** worktree mode is enabled, a previous run created a lane
  with two successful commits, and the same change resolves on a later
  pass
- **THEN** the worktree is reused (no recreate)
- **AND** the lane's prior two commits remain reachable
- **AND** the agent is invoked from the lane's current tip

#### Scenario: Stale worktree directory is recovered

- **WHEN** a prior run left a stale worktree directory at the expected
  path but the lane branch has been removed externally
- **THEN** `see` calls `git worktree prune` and either removes or
  reuses the path before the next attempt
- **AND** the agent is invoked against the recovered worktree

#### Scenario: Operator's checkout is not switched

- **WHEN** worktree mode is enabled and an agent attempt begins
- **THEN** the operator's checkout remains on its original branch
- **AND** no `git switch`, `git checkout`, or `git reset` runs against
  the operator's checkout as part of the attempt

### Requirement: Worktree mode defends against a dirty tree at attempt start

When worktree mode is active, `see` SHALL run the dirty-tree check
(`git status --porcelain`) against the operator's checkout at the
start of each attempt. If the operator's checkout is dirty (tracked
or non-ignored untracked changes), the attempt SHALL fail before
worktree creation or agent invocation. Ignored files SHALL NOT make
the working tree dirty for this check.

The check exists as defense in depth against condition commands that
write to the operator's checkout. The worktree itself is unaffected
by operator-side dirty state, but the rebase target branch tip is
captured from the operator's checkout and a dirty tree during the run
is surfaced separately before merge.

#### Scenario: Dirty tree blocks a worktree-mode attempt

- **WHEN** worktree mode is enabled and the operator's checkout has an
  unstaged tracked edit
- **THEN** the attempt fails with an actionable dirty-working-tree
  error before the worktree is created
- **AND** the operator's edit is preserved
- **AND** the agent is not invoked

#### Scenario: Ignored file does not block a worktree-mode attempt

- **WHEN** worktree mode is enabled and the operator's checkout has
  only ignored files beyond committed state
- **THEN** the watcher proceeds with worktree creation and agent
  invocation

### Requirement: Worktree + auto-merge rebases onto the operator's branch and fast-forward merges

When worktree mode is active and `auto_merge` is true, on a successful
agent run `see` SHALL:

0. Gate the agent's working-tree output, in order, each able to
   short-circuit to rollback:
   a. If the workflow defines a `check` and the agent left working-tree
      changes to land, run the check gate (see workflow-condition). On a
      failed check, trigger the rollback described in the rollback
      requirement and return the check-failure error.
   b. If the workflow has a resolved `measure` gate, run the candidate
      measure and require the candidate to be strictly greater than the
      baseline (see workflow-condition). On a measure failure, trigger
      the rollback described in the rollback requirement and return the
      measure-failure error. The candidate measure SHALL run regardless
      of whether the working tree is dirty, because a non-deterministic
      metric may differ on an unchanged tree.
   On a passed check — or when no check is defined — and on improvement
   — or when no measure gate is resolved — continue.
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

If any of steps 0a (check), 0b (measure), 2, 3, or 4 fails, `see` SHALL
execute the rollback described in the rollback requirement (worktree
removed, lane deleted, operator's checkout untouched) and return the
error.

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

#### Scenario: Passing check and improvement precede the merge

- **WHEN** worktree mode with auto-merge is active, the workflow defines
  a check and a measure, the agent succeeded and left changes, the check
  exits `0`, and the candidate strictly exceeds the baseline
- **THEN** the check runs in the worktree directory before the catch-up
  commit
- **AND** the candidate measure runs in the worktree directory before
  the catch-up commit
- **AND** the merge proceeds as if no gate were defined

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

#### Scenario: Failed check short-circuits the candidate measure

- **WHEN** worktree mode with auto-merge is active, the workflow defines
  both a check and a measure, and the check exits nonzero
- **THEN** the candidate measure is not executed
- **AND** no catch-up commit is created on `see/<digest>`
- **AND** the worktree is removed and `see/<digest>` is deleted (`-D`)
- **AND** the operator's checkout is unchanged
- **AND** the attempt returns the check-failure error

#### Scenario: Non-improvement triggers rollback before any commit

- **WHEN** worktree mode with auto-merge is active, the workflow defines
  a measure, and the candidate does not strictly exceed the baseline
- **THEN** no catch-up commit is created on `see/<digest>`
- **AND** the worktree is removed and `see/<digest>` is deleted (`-D`)
- **AND** the operator's checkout is unchanged
- **AND** the attempt returns the measure-failure error

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

### Requirement: Worktree + manual-merge rebases onto the operator's branch and preserves the lane

When worktree mode is active and `auto_merge` is false, on a successful
agent run `see` SHALL:

0. Gate the agent's working-tree output, in order, each able to
   short-circuit to rollback:
   a. If the workflow defines a `check` and the agent left working-tree
      changes to land, run the check gate. On a failed check, trigger
      the rollback described in the rollback requirement and return the
      check-failure error.
   b. If the workflow has a resolved `measure` gate, run the candidate
      measure and require the candidate to be strictly greater than the
      baseline. On a measure failure, trigger the rollback described in
      the rollback requirement and return the measure-failure error. The
      candidate measure SHALL run regardless of whether the working tree
      is dirty.
   On a passed check — or when no check is defined — and on improvement
   — or when no measure gate is resolved — continue.
1. Run `git add -A` and (if the index differs from `HEAD`) a catch-up
   commit on `see/<digest>` inside the worktree.
2. Run `git rebase <operator-ref>` inside the worktree, where
   `<operator-ref>` is the operator's *current* branch tip.

The worktree directory and `see/<digest>` branch SHALL remain in place
after a successful run for the operator to inspect and merge manually.

On agent failure, check failure, measure failure, or rebase failure,
rollback SHALL apply as for auto-merge (worktree removed, lane deleted).

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

#### Scenario: Non-improvement in manual-merge mode triggers rollback

- **WHEN** worktree mode with `auto_merge: false` is active, the
  workflow defines a measure, and the candidate does not strictly exceed
  the baseline
- **THEN** no catch-up commit is created on `see/<digest>`
- **AND** the worktree is removed and `see/<digest>` is deleted (`-D`)
- **AND** the operator's checkout is unchanged
- **AND** the attempt returns the measure-failure error

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
failure, measure failure at the baseline or the candidate, rebase error,
merge error, or pre-attempt check) `see` SHALL execute the following
cleanup steps, in order, ignoring individual step failures and emitting
`Warning` events for any step that fails:

1. `git rebase --abort` if a rebase is in progress.
2. `git merge --abort` if a merge is in progress.
3. `git worktree remove --force <worktree_path>`.
4. `git branch -D see/<digest>`.

The operator's checkout SHALL remain on its original branch and SHALL
NOT be modified by any of these steps.

#### Scenario: Failure cleanup removes worktree and lane

- **WHEN** worktree mode is active and the attempt fails for any
  reason, including a failed workflow check or a failed measure gate
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

### Requirement: Worktree mode preserves per-attempt log identity

In worktree mode, `see` SHALL continue to compute the per-attempt log
filename using the existing per-change digest. The digest is unchanged
by mode selection. Two polling passes against the same change in
worktree mode SHALL use the same log filename pattern as they would
in branch mode.

#### Scenario: Log filename is per-change in worktree mode

- **WHEN** worktree mode is active and the agent is invoked for change
  `add-dark-mode` against repo `/repos/myproj`
- **THEN** the log filename uses the existing
  `<repo-basename>--<digest>--<utc-timestamp>--<pid>.jsonl` pattern
- **AND** the `digest` is computed the same way as in branch mode

### Requirement: Worktree mode replays open operator commits during rebase

When worktree mode is active, the rebase target SHALL be the
operator's branch tip at rebase time, not the commit captured at
attempt start. This ensures that commits the operator makes during
the agent's run are preserved and that the agent's commits replay on
top.

#### Scenario: Operator's mid-run commits are preserved

- **WHEN** worktree mode is active, the operator's branch tip at
  attempt start is `A`, the agent makes commit `a1` on `see/<digest>`,
  and the operator makes commit `o1` on `main` during the run
- **THEN** the rebase target is `main @ o1` (not `main @ A`)
- **AND** after the rebase, `see/<digest>` contains `main`, `o1`, and
  `a1'` (the rebased form of `a1`)
- **AND** after the fast-forward merge, `main` contains `o1` followed
  by `a1'`

#### Scenario: Operator makes no commits during the run

- **WHEN** worktree mode is active and the operator does not commit
  during the run
- **THEN** the rebase target equals the attempt-start `HEAD`
- **AND** the rebase is a no-op or a trivial replay (linear history
  preserved)

