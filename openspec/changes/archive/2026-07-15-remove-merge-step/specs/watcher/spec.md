# watcher (delta)

## REMOVED Requirements

### Requirement: Watcher merges the agent's commit back on success

When the agent returns nil and the change is archived, `Watcher.work`
SHALL run `git add -A` and `git commit` on `see/<change>`, then switch
back to the original branch ref and merge `see/<change>` back with
`git merge --no-ff -m "see: merge openspec change <change>"`. After the
merge, `Watcher.work` SHALL delete `see/<change>` via `git branch -d`
(safe-delete; the reflog keeps the branch recoverable if the merge was
not fully completed for any reason).

#### Scenario: Successful run produces a merge commit on the original branch

- **WHEN** `Watcher.work` started on `main`, the agent succeeded, and
  the change is archived
- **THEN** `main` contains a new merge commit with subject
  `see: merge openspec change <change>`
- **THEN** `see/<change>` is deleted
- **THEN** the working tree is checked out on `main`

#### Scenario: Merge conflict is treated as failure

- **WHEN** the original branch has moved such that `git merge --no-ff
  see/<change>` reports a conflict
- **THEN** `Watcher.work` aborts the merge (`git merge --abort`),
  switches back to the original branch, resets hard to the captured
  SHA, deletes `see/<change>`, and returns the merge error

**Reason:** `see` is no longer structured as a release bot. Successful
agent runs leave `HEAD` on `see/<change>` with the user's starting
branch untouched; promotion to the user's branch of choice is an
operator decision, not a side effect of a successful run. A
replacement Requirement below pins the new contract.

## ADDED Requirements

### Requirement: Watcher leaves HEAD on the per-change branch after a successful run

When the agent returns nil and the change is archived, `Watcher.work`
SHALL run `git add -A` and `git commit -m "see: apply openspec change
<change>"` on `see/<change>` so that any files the agent left dirty
are absorbed into a single `see`-owned commit. `Watcher.work` SHALL
then emit `ChangeDone` and return nil. `Watcher.work` SHALL NOT switch
back to the original branch ref, SHALL NOT run
`git merge --no-ff see/<change>`, and SHALL NOT delete
`see/<change>` via `git branch -d` on this code path. The original
branch ref captured at the start of `work` is not modified by this
success path.

The rollback path on agent failure is unaffected and is governed by
the existing "Watcher rolls back the branch on agent failure"
Requirement; the workspace branch is still pinned to the pre-run SHA
by the existing "Watcher creates a per-change branch before running
the agent" Requirement.

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

## MODIFIED Requirements

### Requirement: Watcher emits Warning events for cleanup-step failures

When `Watcher.work` performs a rollback, completion, or pre-run
check step that fails but is not itself the reason `work` returns
an error, `Watcher.work` SHALL emit a `Warning` event with the
repo path, change name, and the step's failure message. The
warning SHALL be emitted in addition to whatever boundary event
(`ChangeFailed`, `ChangeDone`, or none for a no-op) the work
function emits; the warning SHALL NOT replace the boundary event
or alter the error returned by `work`.

The pre-run check that emits a Warning is the detached-HEAD check:
when `git symbolic-ref --short HEAD` returns empty,
`Watcher.work` SHALL emit a `Warning` event naming the repo and
SHALL return a `detached HEAD` error.

The rollback and completion steps that emit Warning when they fail
are:

- `git switch` back to the original branch ref
- `git reset --hard <captured-SHA>` after the switch
- `git branch -D <branch>` to clean up the per-change branch
- `git add -A` after a successful agent run
- `git commit` after `git add -A`

In v0 and earlier the list also included `git merge --no-ff
<branch>`, `git merge --abort`, and `git branch -d <branch>` after
a successful merge. Those steps were reachable only via the
success path's merge-back step, which is removed by the
"Watcher leaves HEAD on the per-change branch after a successful
run" Requirement. The rollback-only steps (`git switch`, `git reset
--hard`, `git branch -D`) are unchanged because the rollback path
is preserved. The catch-up steps (`git add -A`, `git commit`) are
unchanged because the catch-up commit is preserved.

#### Scenario: Rollback git switch failure emits a Warning

- **WHEN** the agent errors and the subsequent `git switch` back
  to the original ref fails
- **THEN** `Watcher.work` emits a `Warning` event naming the
  switch failure
- **AND** `Watcher.work` returns the original agent error

#### Scenario: Detached HEAD emits a Warning and returns an error

- **WHEN** `Watcher.work` is invoked on a repo with HEAD pointing
  directly at a commit (no current branch)
- **THEN** `Watcher.work` emits a `Warning` event naming the
  repo and the detached-HEAD condition
- **AND** `Watcher.work` returns a `detached HEAD` error before
  any branch mutation
