## Why

Today the watcher runs the agent in the repo's working directory and commits
on whatever branch the repo is checked out on. While the agent is running,
its in-progress edits and any commits it makes land directly on that
branch. The `git reset --hard` rollback only fires after the agent errors,
so a partially-completed agent run leaves cruft on the original branch
until either rollback or the final commit lands.

By routing every agent run through a dedicated `see/<change>` branch and
merging the result back on success, the original branch stays untouched
for the entire agent run. The change is delivered via a `--no-ff` merge
commit so the audit trail clearly shows what the watcher did.

## What Changes

- Before running the agent, `Watcher.work` captures the current commit SHA
  and the original branch ref (empty if detached), then creates or reuses
  a branch named `see/<change>`. After the branch exists, `Watcher.work`
  pins the branch to the captured SHA via `git reset --hard <sha>` so the
  agent always starts from a known state, regardless of any prior state
  the reused branch may have been in (e.g., a leftover from a previous
  rollback that didn't complete).
- The agent runs on `see/<change>`. If the agent errors, `Watcher.work`
  switches back to the original ref (or `--detach` to the captured SHA if
  the repo started on a detached HEAD), runs `git reset --hard <sha>`,
  deletes `see/<change>`, and returns the error.
- On success and change-archived, `Watcher.work` runs `git add -A` and
  `git commit` on `see/<change>` as today. It then switches back to the
  original ref and runs `git merge --no-ff see/<change> -m
  "see: merge openspec change <change>"`, followed by
  `git branch -d see/<change>`.
- Detached HEAD at the start of `Watcher.work` is an unsupported case for
  v1: the watcher logs and returns an error before any branch mutation.
  User must `git switch` to a real branch first.
- Branch creation is idempotent: if `see/<change>` already exists from a
  prior run (success or failure path), `Watcher.work` switches to it
  instead of erroring. The subsequent `git reset --hard <sha>` makes this
  reuse safe even when the existing branch has drifted (descendant of the
  original SHA, or pointing at an unrelated commit).
- A merge conflict on the `--no-ff` merge is treated as failure:
  `git merge --abort`, switch back to the original ref, `git reset --hard`
  to the captured SHA, delete `see/<change>`, return the error.

**BREAKING**: `Watcher.work`'s `git` interaction changes. No public API or
flag changes. Existing test `TestWorkCommitsOnSuccess` needs an explicit
`git switch -c main` after `git init` to guarantee a real branch, and its
commit-assertion may need to widen to `git log --oneline --all` or check
the merge commit message.

## Capabilities

### New Capabilities

- `watcher`: behavior of `Watcher` regarding branch isolation, including
  pre-run branch creation, failure rollback, success merge-back, and the
  detached-HEAD error.

### Modified Capabilities

(none — this change introduces a new capability rather than modifying an
existing one. The current `openspec/specs/` is empty.)

## Impact

- `main.go`:
  - `Watcher.work`: rewritten with branch-create / rollback / merge-back
    phases.
  - New helpers: `originalRef(path)` (returns symbolic-ref short name,
    empty string if detached) and `ensureBranch(path, sha, name)` (create
    or switch, then `git reset --hard <sha>` to pin the branch tip to the
    captured SHA — idempotent and drift-safe).
- `main_test.go`:
  - `TestWorkCommitsOnSuccess`: add `git switch -c main` after init;
    assertion may widen to look across refs.
  - New tests covering: branch created on first run, branch reused on
    second run, rollback on agent failure cleans up branch, success
    produces merge commit, detached HEAD returns error.
- No dependency changes.
- No CLI flag changes.
- No change to OpenSpec or `pi` integration.