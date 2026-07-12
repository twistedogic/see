## Context

`Watcher.work` currently has three git interaction points: capture the
current commit SHA, run `git reset --hard <sha>` on agent failure, and
run `git add -A && git commit` on agent success. All three operate on
whatever branch the repo was checked out on when the watcher found it.
The agent process inherits the same working directory and branch.

The watcher is a tight poll loop with no human in the loop, so any
guarantee about repo state during an agent run must be enforced by the
watcher itself. Today the only such guarantee is the post-hoc `git reset
--hard` rollback.

## Goals / Non-Goals

**Goals:**
- During an agent run, the repo's original branch is unchanged from the
  watcher's perspective: no agent edits, no agent commits, no accidental
  dirty state.
- On success, the change is delivered to the original branch as a visible
  merge commit so the history reflects what the watcher did.
- On failure, the original branch is exactly as the watcher found it and
  no `see/<change>` branch is left behind.
- The branch-creation flow is idempotent so `retryN`'s re-runs do not
  fail on `git switch -c`.

**Non-Goals:**
- Supporting detached HEAD as a valid starting state (v1 returns an
  error; user must switch to a branch first).
- Cherry-picking the agent's commit when starting from a detached HEAD
  (out of scope; would add a parallel path with limited upside).
- Changing the merge strategy to `--ff-only` or rebase (the whole point
  of the branch is to leave a visible audit trail; `--no-ff` is honest).
- Adding merge-conflict resolution beyond abort + rollback (the watcher
  has no human in the loop; complex resolution is out of scope).
- Changing `retryN`'s signature or the `Watcher.work` return shape
  (covered by `fix-retry-n-silent-failure`; orthogonal change).
- Touching the agent prompt or how the agent is invoked.

## Decisions

**Branch naming: `see/<change>`.**

The OpenSpec change name is unique within a repo, already in hand, and
descriptive. A prefix prevents collisions with user-created branches and
makes the branch's provenance obvious in `git branch` listings.

Alternatives considered:
- `see/<change>-<timestamp>`: more uniqueness, harder to read, and
  `retryN` already gives us idempotency via existing-branch detection.
- `see/<change>-<short-sha>`: useful if multiple watcher processes hit
  the same repo, but the existing code is single-watcher per
  working directory; out of scope.
- A random suffix: same reasoning as timestamp; no benefit.

**Merge strategy: `git merge --no-ff`.**

`--no-ff` always produces a merge commit on the original branch, even
when `see/<change>` is a linear descendant. The resulting history shows
"this commit came from a see-foo branch" as a real graph node, which
matches the watcher's role as a batch driver.

Alternatives considered:
- Default `git merge <name>`: fast-forwards when possible, creating a
  linear history indistinguishable from a direct commit. Hides the
  watcher's involvement. Rejected.
- `git merge --ff-only`: refuses to merge when the original branch has
  moved. Would force a rollback whenever any external commit landed on
  the original branch during the run. Too aggressive.
- `git rebase see/<change> && git merge --ff-only`: linearizes first, then
  fast-forward merges. More moving parts; same result as a fast-forward
  merge without `--no-ff`; rejected for the same reason as default merge.

**Idempotent branch creation via `git branch --list`, then pin to the captured SHA.**

A single `git branch --list see/<change>` call distinguishes "create"
from "reuse." After the branch exists, `Watcher.work` runs `git reset
--hard <sha>` on the branch so its tip is always the captured SHA
regardless of what state the reused branch was in.

The reset is destructive on a reused branch that has drifted (extra
commits the user didn't ask for). This is acceptable because `see/<change>`
is a watcher-owned branch namespace: by convention users don't put their
own work there. If a user does and the watcher wipes it, that's on the
naming convention, not on the watcher being sneaky. The alternative —
detecting drift and erroring — punts the cleanup to the user with no
upside, since the watcher is going to overwrite the branch's contents
anyway.

Alternatives considered:
- `git switch -c` everywhere, ignoring the "already exists" case: any
  retry after a partial run would error and the watcher would loop
  forever on the same failure. Rejected.
- Delete-and-recreate: same end state as the reset, but two git calls
  instead of one. Rejected on cost.
- Validate tip before reuse (`if tip != sha { error }`): still doesn't
  fix the case, just punts to the user. Worse than the reset, which
  fixes it deterministically.
- Skip the reset and trust the branch state: a drifted branch would
  carry unrelated commits into the merge-back, polluting the original
  branch's history. Rejected.

**Detached HEAD: refuse with a clear error.**

The watcher is for tracked repos under active development; running it
against a detached HEAD is the exception, not the rule. Detecting this
case and bailing early avoids the entire "what does merge-back mean
when there's no branch to merge into?" question.

Alternatives considered:
- Cherry-pick the agent's commit onto the captured SHA: preserves the
  detached workflow but adds a parallel code path with limited upside
  (the watcher is run on purpose-built watcher-managed repos).
- Fast-forward the detached HEAD to `see/<change>` (`git reset --hard`):
  loses the detached-ness; arguably the same as just creating a branch
  for the user.

**Merge conflicts: abort + rollback.**

When `git merge --no-ff` reports conflicts, the watcher has no
human-in-the-loop resolver. The safest move is to abort the merge,
reset the original branch to its pre-run state, delete `see/<change>`,
and return the merge error so the retry layer can decide what to do.

Alternatives considered:
- Leave the merge-in-progress state and return error: user has to
  resolve manually before the watcher can continue. Too user-facing.
- Commit with conflict markers: never acceptable.

## Risks / Trade-offs

- **Original branch has moved during agent run → merge conflict →
  agent's work is discarded** → Acceptable: the change is still active
  in `openspec/changes/` and the next watcher cycle will retry from
  scratch. The original branch is untouched.
- **`see/<change>` lingers if rollback itself fails** (e.g., reflog
  deleted) → Use `git branch -D` (force-delete) on the rollback path
  instead of `-d`, and log loudly if the force-delete fails. The branch
  is recovered by `git reflog` if needed.
- **Watcher leaves the working tree on the original branch after a
  successful run, but the change is now a merge commit rather than a
  direct commit** → This is a visible history change for users who
  were relying on linear history. Document in the change proposal; the
  watcher has always been the committer of record, so adding a merge
  step is consistent with its role.
- **Branch create succeeds but agent errors before any commits land**
  → Rollback path deletes the empty branch. No state leak.
- **Concurrent watcher processes against the same repo** → Out of
  scope. The existing code is single-watcher per working directory.
  Global-lock concern is already acknowledged in `main.go`.
- **Test fixture assumes `git init -q` lands on a real branch** →
  Modern git creates an unborn `main` branch and the test's first
  `git commit` instantiates it, so the test ends up on `main`.
  Older git versions or configs with `init.defaultBranch=`
  different may land differently. Add explicit `git switch -c main`
  in the test fixture to be defensive.

## Migration Plan

No migration required. The watcher is the only consumer of its own git
state. Existing watcher runs that find `see/<change>` branches from this
change will reuse them on the next cycle (idempotent branch creation).

Rollback strategy: if the change needs to be reverted, removing the
branch-creation step from `Watcher.work` restores the prior behavior;
any `see/<change>` branches left behind from this version can be cleaned
up with `git branch -D see/<change>` (one per repo).