## ADDED Requirements

### Requirement: Watcher creates a per-change branch before running the agent
When `Watcher.work` begins processing an active change, it SHALL capture
the current commit SHA and the original branch ref (the symbolic-ref
short name) at the same moment, then create or reuse a branch named
`see/<change>`. After the branch exists, `Watcher.work` SHALL pin the
branch tip to the captured SHA via `git reset --hard <sha>` so the agent
always starts from a known state, regardless of any state the reused
branch may have been in.

#### Scenario: First run on a clean repo
- **WHEN** `Watcher.work` runs against a repo on branch `main` with one
  commit and an active change `task-1`
- **THEN** a branch `see/task-1` exists in the repo before the agent runs
- **THEN** the working tree is checked out on `see/task-1` when the agent
  begins
- **THEN** the tip of `see/task-1` is the captured SHA

#### Scenario: Re-run reuses an existing branch
- **WHEN** `Watcher.work` runs against a repo that already has a
  `see/<change>` branch from a previous run
- **THEN** `Watcher.work` switches to the existing branch instead of
  erroring
- **THEN** the branch tip is reset to the captured SHA before the agent
  runs (any extra commits from prior runs are discarded)

#### Scenario: Reused branch with drifted tip
- **WHEN** `see/<change>` exists but its tip is not the captured SHA
  (descendant, or unrelated commit)
- **THEN** `Watcher.work` switches to the branch, resets it to the
  captured SHA, and proceeds as if the branch had been created fresh
  from that SHA

### Requirement: Watcher rolls back the branch on agent failure
When the agent returns a non-nil error, `Watcher.work` SHALL restore the
repo to its pre-run state: switch back to the original branch ref, reset
hard to the captured commit SHA, delete `see/<change>`, and return the
agent error. On a repo that started on a detached HEAD the rollback SHALL
use `git switch --detach <sha>` instead of switching to a branch.

#### Scenario: Agent fails on a branched repo
- **WHEN** `Watcher.work` started on `main` at SHA `A` and the agent
  errors after creating dirty edits and one commit on `see/<change>`
- **THEN** the working tree is on `main` after rollback
- **THEN** `main` is reset to SHA `A` (no merge, no extra commits)
- **THEN** `see/<change>` no longer exists
- **THEN** `Watcher.work` returns the agent error

#### Scenario: Agent fails on a detached-HEAD repo is unsupported
- **WHEN** `Watcher.work` runs against a repo on a detached HEAD
- **THEN** `Watcher.work` returns an error before any branch mutation
- **THEN** no `see/<change>` branch is created
- **THEN** the working tree is unchanged

### Requirement: Watcher merges the agent's commit back on success
When the agent returns nil and the change is archived, `Watcher.work`
SHALL run `git add -A` and `git commit` on `see/<change>`, then switch
back to the original branch ref and merge `see/<change>` back with
`git merge --no-ff -m "see: merge openspec change <change>"`. After the
merge, `Watcher.work` SHALL delete `see/<change>` via `git branch -d`
(safe-delete; the reflog keeps the branch recoverable if the merge was
not fully completed for any reason).

#### Scenario: Successful run produces a merge commit on the original branch
- **WHEN** `Watcher.work` started on `main`, the agent succeeded, and the
  change is archived
- **THEN** `main` contains a new merge commit with subject
  `see: merge openspec change <change>`
- **THEN** `see/<change>` is deleted
- **THEN** the working tree is checked out on `main`

#### Scenario: Merge conflict is treated as failure
- **WHEN** the original branch has moved such that `git merge --no-ff
  see/<change>` reports a conflict
- **THEN** `Watcher.work` aborts the merge (`git merge --abort`),
  switches back to the original branch, resets hard to the captured SHA,
  deletes `see/<change>`, and returns the merge error

### Requirement: Watcher refuses detached HEAD at run start
`Watcher.work` SHALL treat a detached HEAD as an unsupported configuration
for v1. When `git symbolic-ref --short HEAD` returns empty, the watcher
SHALL log a clear error message and return without creating any branch.

#### Scenario: Detached HEAD returns an error
- **WHEN** `Watcher.work` is invoked on a repo with HEAD pointing directly
  at a commit (no current branch)
- **THEN** `Watcher.work` returns an error and the repo state is
  unchanged