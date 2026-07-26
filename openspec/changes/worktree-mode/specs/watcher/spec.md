## MODIFIED Requirements

### Requirement: Watcher creates or resumes a persistent custom automation branch

*(Branch mode only — see lane-isolation for the worktree-mode
contract.)* For an active workflow change in branch mode, `Watcher`
SHALL use a branch named `see/<digest>`, where the digest is derived
from the workflow name and normalized change. If the lane does not
exist, it SHALL be created at the current commit. If it exists, the
watcher SHALL switch to it only when the working tree is clean and
SHALL resume its current tip without resetting prior commits. The
watcher SHALL permit switching from another clean branch or workflow
lane.

#### Scenario: First custom run creates its lane
- **WHEN** custom change `add-dark-mode` resolves to branch `see/<digest>` and that branch does not exist
- **THEN** the branch is created at the captured current commit before the agent runs
- **AND** the agent runs with `see/<digest>` checked out

#### Scenario: Repeated custom run resumes its lane
- **WHEN** `see/<digest>` is checked out with successful commits from an earlier pass
- **AND** the condition emits the same custom change again
- **THEN** the watcher runs the agent from the existing branch tip
- **AND** no earlier commit is reset or deleted

#### Scenario: Existing lane is not reset from another branch
- **WHEN** `see/<digest>` exists but the repository is checked out on `main`
- **AND** the condition resolves the change whose branch is `see/<digest>`
- **THEN** the watcher switches to the lane without resetting prior commits
- **AND** prior commits remain reachable
- **AND** the agent runs

#### Scenario: Distinct workflows have distinct lanes
- **WHEN** two workflows emit the same normalized change
- **THEN** they use different branch names
- **AND** work on one lane cannot reset or delete the other lane

#### Scenario: Clean checkout switches to an existing lane
- **WHEN** a requested workflow lane exists and another branch is checked out with a clean working tree
- **THEN** the watcher switches to the requested lane
- **AND** it preserves the lane's existing commits

#### Scenario: Dirty checkout blocks lane switching
- **WHEN** a requested workflow lane exists and the current checkout has tracked or non-ignored untracked changes
- **THEN** the workflow fails before branch mutation
- **AND** the changes remain unchanged

### Requirement: Watcher rolls back only the failed custom attempt

*(Branch mode only — see lane-isolation for the worktree-mode
contract.)* Immediately before invoking an agent in branch mode,
`Watcher` SHALL capture the selected workflow lane tip. If the agent
fails on an existing lane, the watcher SHALL reset tracked state to
that tip, remove non-ignored untracked files created by the attempt,
preserve ignored files and earlier lane commits, and leave the clean
lane available for subsequent workflows. If the lane was created by
the attempt, the watcher SHALL return to the branch that was checked
out before that workflow, restore its captured commit, and delete
only the new lane.

#### Scenario: Failure on an existing lane preserves history
- **WHEN** an existing custom lane has commits `A`, `B`, and `C`
- **AND** the agent creates edits or commits after `C` and then fails
- **THEN** the lane tip is restored to `C`
- **AND** commits `A`, `B`, and `C` remain reachable from the lane
- **AND** the lane remains checked out
- **AND** the agent error is returned

#### Scenario: Failure removes newly-created lane
- **WHEN** a custom lane is created for the first time and the agent fails
- **THEN** the original branch and captured commit are restored
- **AND** the new custom lane is deleted
- **AND** the agent error is returned

#### Scenario: Agent-created untracked files are removed
- **WHEN** the custom working tree was clean before the agent ran
- **AND** the failing agent creates an untracked non-ignored file
- **THEN** rollback removes that file

#### Scenario: Ignored files survive rollback
- **WHEN** a failing custom agent creates or modifies an ignored file
- **THEN** rollback does not delete that ignored file

#### Scenario: Existing workflow lane preserves history after failure
- **WHEN** an existing workflow lane has prior commits and its agent fails after making changes
- **THEN** the lane is reset to its pre-attempt tip
- **AND** prior commits remain reachable
- **AND** later workflows may run after cleanup

#### Scenario: New workflow lane is removed after failure
- **WHEN** an agent fails during the first attempt on a new workflow lane
- **THEN** the pre-workflow branch and commit are restored
- **AND** only the newly created lane is deleted
- **AND** later workflows may run from the restored clean checkout

### Requirement: Watcher creates a per-change branch before running the agent

*(Branch mode only — see lane-isolation for the worktree-mode
contract.)* In OpenSpec compatibility mode running in branch mode,
when `Watcher.work` begins processing an active change, it SHALL
capture the current commit SHA and original branch ref, then create
or reuse `see/<change>`. After the branch exists, compatibility mode
SHALL pin its tip to the captured SHA via `git reset --hard <sha>` so
the agent starts from known state regardless of prior branch drift.

In custom mode running in branch mode, the watcher SHALL instead
follow the persistent custom automation branch requirement in this
change: create `see/<digest>` once, resume it only when it is
already checked out, and never reset its prior successful commits
before an attempt.

#### Scenario: Compatibility first run creates a branch
- **WHEN** compatibility-mode work runs on branch `main` with active OpenSpec change `task-1`
- **THEN** `see/task-1` exists before the agent runs
- **THEN** the working tree is checked out on `see/task-1`
- **THEN** the tip of `see/task-1` is the captured SHA

#### Scenario: Compatibility re-run reuses and resets an existing branch
- **WHEN** compatibility-mode work finds an existing `see/<change>` branch
- **THEN** it switches to the existing branch instead of erroring
- **THEN** it resets the branch tip to the captured SHA before the agent runs

#### Scenario: Compatibility reused branch with drifted tip
- **WHEN** compatibility mode finds `see/<change>` at a descendant or unrelated commit
- **THEN** it switches to the branch, resets it to the captured SHA, and proceeds from that SHA

#### Scenario: Custom re-run preserves an existing lane
- **WHEN** custom mode resolves a change whose hashed branch is already checked out with prior successful commits
- **THEN** the watcher starts the next agent attempt from that branch tip
- **AND** it does not reset or delete the prior successful commits

### Requirement: Watcher leaves HEAD on the per-change branch after a successful run

*(Branch mode only — see lane-isolation for the worktree-mode
contract.)* In branch mode, when the agent returns nil and the change
is archived, `Watcher.work` SHALL run `git add -A` and `git commit -m
"see: apply openspec change <change>"` on `see/<change>` so that any
files the agent left dirty are absorbed into a single `see`-owned
commit. `Watcher.work` SHALL then emit `ChangeDone` and return nil.
`Watcher.work` SHALL NOT switch back to the original branch ref,
SHALL NOT run `git merge --no-ff see/<change>`, and SHALL NOT delete
`see/<change>` via `git branch -d` on this code path. The original
branch ref captured at the start of `work` is not modified by this
success path. The rollback path on agent failure is unaffected and is
governed by the existing "Watcher rolls back the branch on agent
failure" Requirement.

#### Scenario: Successful run leaves HEAD on the workspace branch
- **WHEN** `Watcher.work` started on `main` with SHA `A`, the agent
  succeeded, and the change is archived
- **THEN** the working tree is checked out on `see/<change>` when
  `Watcher.work` returns
- **THEN** `see/<change>` is not deleted by `work`
- **THEN** the catch-up commit's subject
  `see: apply openspec change <change>` is reachable from
  `see/<change>`'s tip

#### Scenario: Original branch tip is unchanged on success
- **WHEN** `Watcher.work` started on `main` with SHA `A`, the agent
  succeeded, and the change is archived
- **THEN** `main` is at SHA `A` after `work` returns (no merge, no
  commit on `main`)
- **THEN** the agent's commits are reachable from `see/<change>` but
  not from `main`

### Requirement: Watcher rolls back the branch on agent failure

*(Branch mode only — see lane-isolation for the worktree-mode
contract.)* In OpenSpec compatibility mode running in branch mode,
when the agent returns a non-nil error, `Watcher.work` SHALL restore
the repository to its pre-run state by switching back to the
original branch ref, resetting hard to the captured commit SHA,
deleting `see/<change>`, and returning the agent error.

In custom mode running in branch mode, rollback SHALL follow the
persistent-lane requirement in this change. A failed attempt on a
pre-existing lane SHALL restore that lane to its pre-attempt tip
without deleting it. A failed attempt that created a new lane SHALL
restore the original branch and delete only that newly-created lane.
In both modes, a detached HEAD SHALL remain unsupported and SHALL be
rejected before branch mutation.

#### Scenario: Compatibility agent failure deletes the disposable branch
- **WHEN** compatibility-mode work starts on `main` at SHA `A` and the agent errors after editing or committing on `see/<change>`
- **THEN** the working tree is on `main` after rollback
- **THEN** `main` is reset to SHA `A`
- **THEN** `see/<change>` no longer exists
- **THEN** `Watcher.work` returns the agent error

#### Scenario: Detached HEAD remains unsupported
- **WHEN** either mode begins on a detached HEAD
- **THEN** `Watcher.work` returns an error before branch mutation
- **THEN** the working tree is unchanged

#### Scenario: Existing custom lane survives failure
- **WHEN** a custom agent fails while running on a lane that existed before the attempt
- **THEN** the lane is restored to its captured pre-attempt tip
- **AND** the lane is not deleted
- **AND** it remains checked out

#### Scenario: Newly-created custom lane is removed on failure
- **WHEN** a custom agent fails during the first attempt on a newly-created lane
- **THEN** the original branch and commit are restored
- **AND** only the newly-created lane is deleted

### Requirement: Watcher leaves the final usable workflow lane checked out

*(Branch mode only — see lane-isolation for the worktree-mode
contract.)* After all workflows for a repository are processed in
branch mode, `Watcher` SHALL leave the most recently usable active
workflow lane checked out. If no workflow was active, it SHALL leave
the branch that was checked out when repository processing began. A
successful workflow SHALL not merge its lane into the starting
branch.

#### Scenario: Final active workflow lane remains checked out
- **WHEN** two workflows run successfully for one repository
- **THEN** the second workflow's lane is checked out when repository processing ends
- **AND** both workflow lanes retain their commits

#### Scenario: No active workflow preserves the starting branch
- **WHEN** all workflow conditions exit with status `1`
- **THEN** no workflow lane is created or switched to
- **AND** the starting branch remains checked out

#### Scenario: A failed final workflow leaves a safe usable checkout
- **WHEN** the final active workflow fails and rollback succeeds
- **THEN** the cleaned workflow lane remains available as the final lane when it existed before the attempt
- **AND** no later workflow remains to run

## ADDED Requirements

### Requirement: Watcher dispatches into the configured isolation mode

`Watcher.work` (or its successor) SHALL select an isolation mode based
on the resolved `(worktree, auto_merge, worktree_root)` configuration
that `main()` constructs from CLI flags and config fields. The
selection SHALL follow the lane-isolation capability's "Three
isolation modes are explicit and named" requirement:

- When `worktree` is false (the default), the watcher SHALL use
  branch mode and SHALL follow the existing branch-mode requirements
  (per-change branch creation, leave-HEAD-on-lane after success,
  per-mode rollback, final-lane-checked-out). The `auto_merge` and
  `worktree_root` values SHALL be ignored in branch mode.

- When `worktree` is true, the watcher SHALL use worktree mode and
  SHALL follow the lane-isolation capability's worktree-mode
  requirements (worktree creation, lane rebased onto operator's
  branch tip, fast-forward merge when `auto_merge` is true, rollback
  with worktree removed and lane deleted on failure). The existing
  branch-mode requirements SHALL NOT apply.

The dispatch SHALL be evaluated once per `Watcher.work` invocation
against the resolved configuration; runtime mode switching within a
single `Watcher.work` call is not supported.

#### Scenario: Default config selects branch mode

- **WHEN** `see` is invoked with no `--worktree` flag and the
  configuration has no `worktree:` field
- **THEN** every `Watcher.work` call uses branch mode
- **AND** the existing branch-mode requirements apply unchanged

#### Scenario: --worktree flag selects worktree mode

- **WHEN** `see` is invoked with `--worktree`
- **THEN** every `Watcher.work` call uses worktree mode
- **AND** the lane-isolation worktree-mode requirements apply
- **AND** the existing branch-mode requirements do not apply

#### Scenario: --worktree overrides worktree: false

- **WHEN** the configuration sets `worktree: false` and `see` is
  invoked with `--worktree`
- **THEN** worktree mode is selected for every `Watcher.work` call
- **AND** the lane-isolation worktree-mode requirements apply

#### Scenario: Configuration worktree: true without flag

- **WHEN** the configuration sets `worktree: true` and `--worktree`
  is not passed
- **THEN** worktree mode is selected
- **AND** the lane-isolation worktree-mode requirements apply

#### Scenario: auto_merge is ignored in branch mode

- **WHEN** branch mode is active and `auto_merge: true` is set in
  the configuration or `--auto-merge=true` is passed
- **THEN** the watcher proceeds with branch mode
- **AND** the `auto_merge` value is not consulted for behavior
- **AND** no error is emitted for the unused value

#### Scenario: worktree_root is ignored in branch mode

- **WHEN** branch mode is active and `worktree_root: <path>` is set
  in the configuration
- **THEN** the watcher proceeds with branch mode
- **AND** the `worktree_root` value is not consulted for behavior
- **AND** no error is emitted for the unused value