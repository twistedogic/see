## Context

`see` is an autonomous batch driver: for each repo under the watch
list, it picks the first active openspec change, runs the configured
agent against it, and reports success or failure. Today, "success"
means `Watcher.work` reaches the end with the user's starting branch
(`main`, `develop`, whatever `git symbolic-ref --short HEAD` named)
moved forward by an `see`-owned `--no-ff` merge commit.

The merge-back step serves three purposes in the current design:

1. It puts the agent's work onto the user's working branch so that
   every subsequent tool that reads `main` (CI, a human, another
   bot) sees the change.
2. It marks the graph with a visible `see: merge openspec change X`
   commit even on a single-commit workspace branch, so a future
   reader can tell that `see` was involved. The `ponytail:` comment
   on the merge call names this as the rationale.
3. It deletes the `see/<change>` workspace branch via
   `git branch -d`, so each run starts from a clean slate.

The user wants the first purpose reversed. Promotion to the user's
branch of choice should be the user's decision; `see`'s job ends
when the change is archived and the workspace branch is clean. The
graph-visibility rationale (purpose 2) is moot without the merge.
The clean-slate argument (purpose 3) is replaced by the workspace
branch simply being the branch the user inherits and may or may not
rename.

The failure path is independent of this change: agent failure still
triggers `switch ref` + `reset --hard <captured-SHA>` + `branch -D`,
because a half-finished agent run on the user's working tree is
worse than a clean reset. That code is preserved as-is.

## Goals / Non-Goals

**Goals:**

- On successful agent run + successful archive, `Watcher.work`
  returns with HEAD left on `see/<change>` and the user's starting
  branch untouched.
- The `git add -A && git commit` catch-up step still runs after a
  successful agent invocation; it captures any files the agent left
  dirty so the user's first commit on the inherited branch is clean
  of agent leftovers.
- The existing pre-run, rollback, and retry contracts are
  unchanged: the workspace branch is still pinned to the captured
  pre-run SHA before the agent runs; agent failure still rolls back
  fully; retries still behave per the existing requirement.
- Tests pin the new contract, not the old one. Any test that
  asserted merge happened now asserts merge did *not* happen and
  the workspace branch is the post-run branch.

**Non-Goals:**

- Renaming `see/<change>` to something else. Existing tests and
  operator workflows (grepping for `see/`) assume the prefix; a
  rename is a separate change.
- An opt-in or flag for "merge back" behavior. The old behavior is
  gone. Operators who want a merge can `git merge --no-ff
  see/<change>` themselves.
- Touching the rollback path on agent failure. Rollback still
  nukes the workspace branch; the user does not inherit a branch
  when the run failed.
- Touching the retry loop. The retry semantics — work → on err
  reset & retry up to `-retry` — are unchanged; only the
  post-success step at the end of a successful pass changes.
- Touching the TUI's `ChangeDone` rendering beyond what's strictly
  necessary (none expected).
- Adding `git push` automation. Promotion is the user's job.

## Decisions

### Decision 1 — leave HEAD on `see/<change>`, no switch, no merge

**Choice.** After the catch-up commit on `see/<change>` and the
`ChangeDone` event emission, `Watcher.work` returns. There is no
`git switch ref`, no `git merge --no-ff <branch>`, and no
`git branch -d <branch>` on the success path.

**Rationale.** This is the user's stated requirement. It also
preserves a useful invariant: the workspace branch is always the
branch the user takes away from a run, regardless of whether the
run succeeded. The merge-related steps are unconditional removes;
the new code is `git add -A && git commit && ChangeDone`, and that
is it.

**Alternatives considered.**

- **Flag-gated merge-back** (e.g. `--merge-back` defaulted to
  off). Rejected by YAGNI / "the simpler version" rule: a second
  behavior adds surface area, tests, and help text for a feature
  the user is explicitly removing. Operators who want the merge
  can run `git merge --no-ff see/<change>` themselves.
- **Switch back to ref, do not merge.** Rejected: "stay on the
  change branch" reads literally; switching away contradicts the
  requirement and leaves the workspace branch orphaned against
  the user's working tree. It also adds the question of how the
  user gets back to it (`git switch see/<change>`), which is a
  tiny unforced ergonomic loss.

### Decision 2 — keep the catch-up `git add -A && git commit`

**Choice.** The two-line catch-up after a successful agent run is
preserved. It still emits a `Warning` event when either
`git add -A` or `git commit` fails, and `Watcher.work` still
returns nil (the warning does not change the success signal).

**Rationale.** Today the catch-up commit sits on `see/<change>`
just before the `see: merge` commit, and operators see it as one
of the merge's parents. After the change, the catch-up commit is
the only `see`-owned commit on the branch the user inherits. Its
purpose — boundary between agent work and operator commits —
becomes more important, not less. Without it, the user's first
commit on the inherited branch gets contaminated with files the
agent left dirty.

**Alternatives considered.**

- **Drop the catch-up commit entirely.** Rejected: it changes the
  semantics from "one `see:` commit marks the boundary" to "no
  boundary at all." A future operator has no way to tell where
  agent work ends.
- **Make `git add` selective (only files the agent touched).**
  Rejected: detecting "files the agent touched" requires either
  parsing the agent's stdout/journal (fragile) or wrapping the
  agent call in a worktree copy (substantial new test
  surface). Not worth v1.
- **Move `git add -A` into the agent's prompt so the agent
  always commits its own work.** Rejected: the agent prompt is
  out of `see`'s hands once written; `see` provides defense in
  depth here.

### Decision 3 — keep the `see/<change>` name; rename later if ever

**Choice.** The branch name stays `see/<change>`. No new constant
or rename is introduced.

**Rationale.** The name carries tool-flavor that no longer matches
its role strictly, but renaming has wide reach:
`main_test.go` greps for `see/task-1` and `see/` in several
places; operators may have external hooks that grep the same;
the rename itself is a low-value, broad-churn change. If the
prefix becomes actively misleading in practice, a follow-up
change can rename it.

**Alternatives considered.**

- **Rename to `applied/<change>`.** Rejected: requires a separate
  decision on collision rules with human-named branches; not what
  this change asked for.
- **Rename to no prefix (`<change>`).** Rejected for the same
  reason, plus the rename would collide with the change name
  itself in `git branch --list` output and would be confusing.

## Risks / Trade-offs

- **[Risk]** Operators who relied on `see` to advance `main` after
  each successful run will see their `main` fall behind their
  active changes indefinitely. **Mitigation:** the
  `shape_choices` block in `config.yaml` documents this so a
  future operator reading the source learns it; the
  spec's `--no-ff merge` requirement is removed and replaced
  with prose that names the new contract explicitly.

- **[Risk]** If a failure-path rollback's `git switch ref` fails,
  the user is left on `see/<change>` with dirty state. Today,
  that already triggers a `Warning` event but the warning can be
  silent in batch mode. After this change, the same scenario
  applies — HEAD stays on `see/<change>` if the rollback switch
  fails — and the risk profile is the same. **Mitigation:** the
  rollback Warning contract is preserved.

- **[Risk]** Multiple `see` runs in a row will create
  `see/<change-A>`, `see/<change-B>`, etc., with HEAD stacking
  on the most recent. `git symbolic-ref --short HEAD` will return
  one of those names on the next run, and that becomes the
  `originalRef` for the run after. **Mitigation:** this is
  intended. A run picks up exactly where the previous run left
  off; the user's branch of record is whichever run's workspace
  is current. Spec language states this explicitly.

- **[Trade-off]** The `ponytail:` comment on the merge call
  (graph visibility for `see`'s involvement) is removed because
  the merge itself is removed. Future readers will not see the
  tradeoff the comment captured. **Mitigation:** the new
  spec Requirement names the new contract, including the
  no-merge rule, so a future reader can grep for "merge" and
  learn the answer.

- **[Trade-off]** Without the merge, `main` no longer serves as a
  unified ledger of `see`'s work. Operators running `see`
  against many repos must check each repo's `see/<change>`
  branches separately to find what was applied. **Mitigation:**
  the batch-level JSONL already carries the `ChangeDone` event
  per repo; that's the ledger now.
