## Context

`see`'s current isolation strategy is single-mode: every agent run
operates on the operator's checkout directly. `Watcher.work` (in
`main.go`) creates `see/<change>` on that checkout, runs the agent
there, rolls back on failure, and on success leaves the lane checked
out with a catch-up commit. The operator's real branch is unreachable
during the run and requires `git checkout <branch>` afterward to
restore.

This is the established behavior. Operators running `see` for an
extended period have come to expect it. The change proposed here does
not alter branch mode for those who don't opt in; it introduces an
alternative isolation strategy that the operator enables per-`see`
invocation via `--worktree` or globally via the `worktree:` config
field.

The worktree strategy is enabled by `git worktree`, a mature git
feature that links a separate working directory back to the same
`.git` database. Two key properties make it suitable here:

1. The lane's branch (`see/<change>`) and the operator's branch
   (`main` or whatever they were on) can be checked out
   simultaneously in two different directories because they are
   different refs.
2. The two checkouts share git's object database and ref namespace,
   so `git log see/<change>` from the operator's checkout shows the
   same history as a `cd` into the worktree.

Git's worktree semantics around `git worktree add` and `git worktree
remove` are well-defined and stable across versions since worktrees
shipped in git 2.5 (2015). The implementation here leans on stable
behavior: `-B` to create-or-reset a branch, `--force` on `add` to
reuse a registered worktree path, `remove --force` to clean up after
a failure.

The watcher's existing primitives — `GetCurrentCommit`,
`originalRef`, the dirty-tree check, the per-change digest, the
catch-up commit — continue to be useful. Their roles shift: in
worktree mode, `originalRef` becomes the rebase + merge target
rather than a rollback anchor; the dirty-tree check defends against a
writeful condition command rather than a soon-to-be-overwritten
working tree.

## Goals / Non-Goals

**Goals:**
- Add a `--worktree` flag and `worktree:` config field that switch
  the isolation strategy to git-worktree-based.
- Make the operator's checkout completely untouched during an agent
  run when worktree mode is active.
- In worktree mode, deliver the agent's commits to the operator's
  branch via rebase + fast-forward merge by default.
- Provide a `--auto-merge=false` / `auto_merge: false` escape hatch
  that rebase-only's the lane onto the operator's branch and leaves
  the lane + worktree for manual review.
- Keep the existing branch mode fully functional and unchanged for
  operators who do not opt in.
- Add validation rules that fail fast at startup for invalid
  combinations (`auto_merge` without `worktree`; `worktree_root`
  without `worktree`).
- Surface the worktree location in a configurable top-level field
  with a sensible default that does not collide with the discovery
  layer.

**Non-Goals:**
- Changing branch mode's behavior. Branch mode's contract is what
  operators rely on today.
- Running multiple agent invocations concurrently against one repo.
  Worktree mode enables it but does not require it; the polling loop
  remains serial.
- Cherry-picking agent commits onto the operator's branch instead of
  rebasing. Rebase keeps linear history and matches the operator's
  mental model of "the agent did some work, here it is on my
  branch."
- Adding merge-conflict resolution. Rebase conflicts and merge
  failures are rollback signals, same as agent failures.
- Cross-platform worktree edge cases beyond what git itself handles
  (cross-filesystem worktrees, worktrees on network drives). Git
  rejects these; we surface git's error message.
- Adding a `--no-ff` flag to worktree mode to leave a merge commit
  visible. The design rationale for `--no-ff` in the archived
  `isolate-agent-runs-on-branch` change was "watcher as visible
  committer of record"; in worktree mode the operator IS the
  committer of record, so linear history is the honest choice.
- Changing the OpenSpec compatibility mode's merge strategy
  (`git add -A` + `git commit` catch-up on the lane). That mode's
  merge contract is governed by the `watcher` capability's existing
  requirements; the lane-isolation capability governs the worktree
  variant only.

## Decisions

**Three explicit isolation modes, selected by `(worktree, auto_merge)`.**

The pair `(worktree, auto_merge)` cleanly partitions the operator's
options into three modes plus one impossible state (branch + auto-merge
is meaningless because there's no worktree to auto-merge from). The
impossible state is rejected at validation time:

```
   worktree   auto_merge   mode
   ────────   ──────────   ──────────────────────────────
   false      (any)        branch                       (default)
   true       true         worktree + auto-merge        (default of --worktree)
   true       false        worktree + manual-merge
   true       true         (worktree_root also set)     worktree + auto-merge, custom root
```

Two boolean fields rather than an enum string keep the YAML simple
and the precedence rules in `main()` straightforward. A string enum
(`mode: branch|worktree`) would require parse-and-validate logic;
two booleans are read-and-compare.

Alternatives considered:
- One boolean `worktree: true|false` and a separate `merge_strategy:
  ff|manual` enum: same surface area, less natural mapping from CLI
  flag (`--auto-merge`) to config.
- A single `mode: branch|worktree|worktree-no-merge` string: more
  explicit but expands the schema for a three-way choice; adds a
  parse step.
- A `merge: never|always|auto` string: introduces a new term; the
  `auto_merge` name already maps cleanly to "auto-merge after
  successful rebase."

**Default location: `~/.cache/see/worktrees`.**

The default lives outside any plausible `root_dir` for the discovery
layer. Discovery's `include` globs would not match
`~/.cache/see/worktrees/<repo>--<digest>/` even if `root_dir` were
`~/.cache`, because the include patterns are anchored to immediate
children of `root_dir`, not arbitrary paths.

The default also matches the operator's mental model of "things I
shouldn't poke around in" — git's own cache and user-level cache
directories. Operators who want to `cd` into the lane directory can
override with `worktree_root: ~/Dev/.see-worktrees` (a sibling
configuration would then require an `exclude` glob to keep discovery
from picking it up; we document this).

Alternatives considered:
- Hidden sibling (`~/Dev/.see-worktrees/`): discoverable by `cd`,
  invisible to most glob patterns because of the leading dot. Strong
  alternative. Rejected as the default because it ties the default
  to a particular `root_dir` value, which we don't know at
  startup.
- Visible sibling (`~/Dev/<repo>--see-<digest>/`): high discovery
  collision risk; rejected.

**Lane naming: keep `see/<digest>`.**

The branch ref `see/<digest>` is preserved across modes. Reasons:
- Existing operators with `see/<change>` branches on disk from prior
  branch-mode runs transition cleanly: `--worktree` activates with
  `-B`, reusing the existing branch.
- `git log see/<digest>` works identically from either the
  operator's checkout or the worktree, because the ref is shared.
- Event payloads, log filenames, and TUI labels that key off the
  branch name continue to work.

The digest is still computed from `workflow.name + "\x00" +
normalizedChange` (when a workflow is configured) or `normalizedChange`
alone (in OpenSpec compat mode). Per-attempt identity is preserved
through the digest, so multiple polling passes on the same change
land on the same lane.

Alternatives considered:
- A per-attempt branch name (`see/<digest>-<timestamp>`): eliminates
  reuse ambiguity but breaks the cross-mode identity and forces
  operators to relearn the lane structure.
- A per-attempt branch in worktree mode only: introduces a
  mode-conditional naming rule; more code; no benefit.

**Operator's checkout never switches in worktree mode.**

The `git switch` dance that branch mode uses to enter and exit the
lane is unnecessary in worktree mode. `originalRef` is still
captured — it becomes the rebase + merge target. The dirty-tree
check at attempt start is still performed as defense in depth
against writeful condition commands; the operator's checkout is no
longer at risk of being overwritten by `git switch`, but a
condition command that writes to the operator's tree is still a
problem.

Alternatives considered:
- Keep the dirty-tree check but drop the `originalRef` capture: a
  rebase needs a target ref, so `originalRef` is unavoidable.
- Drop both: the rebase would target a hard-coded ref like `main`,
  which would be wrong when the operator is on a feature branch.
  Rejected.

**Auto-merge: rebase onto operator's current branch tip, then
fast-forward merge.**

The rebase target is the *current* tip of the operator's branch at
rebase time, not the original commit captured at attempt start. If
the operator committed during the agent's run, the agent's commits
replay on top of the operator's new commits. Linear history; no
merge commit visible on the operator's branch.

The merge step uses `--ff-only` because the rebase just produced a
lane tip that is a fast-forward of the operator's branch. If the
operator commits between rebase and merge (a narrow window), the
fast-forward fails and we rollback. The lane + worktree are removed
on rollback; the operator's commits are preserved (they live on
their own branch).

After merge succeeds, `git worktree remove --force` cleans up the
worktree directory and `git branch -d see/<digest>` deletes the now-
merged lane. `git worktree remove` does not delete the branch; we
do it explicitly so the operator's `git branch` listing stays
clean.

Alternatives considered:
- `git merge --no-ff` to leave a merge commit visible: rejected. In
  branch mode the watcher was the visible committer of record and
  `--no-ff` made the watcher's role honest. In worktree mode the
  operator IS the committer of record and a `--no-ff` commit would
  clutter their history with a redundant node.
- Re-then-merge into the original captured commit, ignoring operator
  commits during the run: discards operator work. Rejected.
- Skip rebase, merge with a real merge commit: linear history is
  cleaner; rebase onto the operator's tip handles the
  operator-commits-mid-run case for free.

**Rebase conflicts and merge failures trigger full rollback.**

A rebase conflict means the operator and the agent touched the same
files in incompatible ways. There is no human-in-the-loop resolver;
the safest move is to abort the rebase (`git rebase --abort`), remove
the worktree, delete the lane, and return the error. The operator's
checkout is untouched.

A `--ff-only` failure (operator committed between rebase and merge)
is similar. `git merge --abort` cleans any half-applied state; the
worktree and lane are removed; the operator's checkout is
untouched.

The end-of-run dirty-tree check catches the case where the operator
has uncommitted edits at merge time. Without it, the fast-forward
would either fail (because the working tree is dirty) or succeed
but leave the operator's uncommitted edits in an inconsistent state
relative to the merged index. Reject before merging.

Alternatives considered:
- Skip the end-of-run dirty-tree check, rely on merge to fail
  loudly: tested and surprising — git's behavior on a dirty tree
  with `--ff-only` is to refuse with a confusing error. Better to
  check explicitly.
- Leave the merge conflict state on disk for operator resolution:
  too user-facing, too coupled to `see`'s lifecycle.

**Validation rejects impossible states at startup.**

```
   config state                                       result
   ────────────                                       ──────
   worktree: false, auto_merge: (anything)            branch mode (auto_merge ignored)
   worktree: true, auto_merge: true                   worktree + auto-merge
   worktree: true, auto_merge: false                  worktree + manual-merge
   worktree: false, auto_merge: true                  ERROR: auto_merge requires worktree
   worktree: false, auto_merge: false                 ERROR: auto_merge requires worktree
   worktree: true, auto_merge: true,                  worktree + auto-merge at custom root
     worktree_root: /path
   worktree: false, worktree_root: /path              ERROR: worktree_root requires worktree
```

Silent fallbacks hide bugs. The AGENTS.md ethos of fail-fast with
actionable errors applies; same validation pattern as the existing
workflow-field rejection.

**CLI flag → config field → default precedence.**

Identical to today's `--prompt` precedence. The CLI flag wins over
the config field which wins over the default. The bool config field
needs the pointer-or-omit pattern so we can distinguish "unset, use
default" from "explicitly false"; Go's `*bool` is the natural fit.

```
   --worktree              → worktree = true          (overrides config)
   --worktree=false        → worktree = false         (overrides config)
   worktree: true          → worktree = true          (default false)
   worktree: false         → worktree = false         (explicit; same as unset)

   --auto-merge=false      → auto_merge = false       (overrides config)
   (omitted)               → auto_merge = config or true
   auto_merge: false       → auto_merge = false       (explicit; works in worktree mode)
   auto_merge: true        → auto_merge = true        (default; works in both modes — but
                                                        validated to require worktree: true)
```

**`worktree_root` defaults to `~/.cache/see/worktrees`, tilde-expanded.**

Same tilde-expansion rule as `root_dir`. The discovery layer's
exclude-glob facility protects against `worktree_root` collisions
when an operator configures it as a sibling of watched repos; this
is operator responsibility, documented in AGENTS.md.

**`see` never spawns the worktree as a child process during
discovery or startup.**

The worktree is only created at attempt start, by `ensureWorktree`.
`git worktree prune` is called at the top of `ensureWorktree` to
clean up stale metadata (e.g., from a `rm -rf` operator action
between runs).

## Risks / Trade-offs

- **Rebase conflicts discard the agent's work for that attempt.**
  → The agent's commits are reachable from the deleted lane ref for
  the brief window before `git branch -D`; once deleted, they
  remain reachable from the reflog for the configured retention
  window. After that, they may be garbage collected. Operators
  who want to preserve a failed attempt can pull the lane ref
  from reflog. Mitigation: document.

- **Stale worktree directory from a crash blocks the next run.**
  → `git worktree prune` at the top of `ensureWorktree` clears
  the metadata; `git worktree add --force` reuses a registered
  worktree path; explicit `git worktree remove --force` plus
  `git worktree add -B` handles a path that exists with junk in
  it.

- **`worktree_root` inside `root_dir` causes discovery to
  double-watch.** → The default avoids this. Operators who override
  need an `exclude:` glob. Document in AGENTS.md.

- **Operator's branch must be committable for the rebase target to
  exist.** → Detached HEAD is rejected before the attempt, same as
  branch mode. Branch deletion during the run is a narrow window;
  if it happens, the rebase fails and we rollback.

- **Two `see` processes on the same repo with `--worktree` race on
  worktree creation.** → `git worktree add` is atomic in git's
  object store; one process succeeds, the other errors with a
  clear git message. The loser's attempt fails; the next polling
  pass retries. Eventually consistent. Document.

- **Auto-merge removes the lane branch even when the operator wants
  to inspect.** → Mitigated by `--auto-merge=false` escape hatch.

- **Rebase-and-merge changes the agent's commit hashes relative to
  what the agent sees during its run.** → The agent's hashes are
  the rebased hashes by the time they reach the operator's
  branch. The log file references the digest, not commit hashes,
  so logs continue to match.

- **Default worktree root is on a different filesystem from the
  watched repo on some operator setups.** → `git worktree add`
  errors clearly when the target is cross-filesystem; we surface
  the error. Document that the operator may need to set
  `worktree_root` to the same filesystem.

- **Worktree mode is significantly more code than branch mode.**
  → The added code lives in three new helpers and a dispatcher.
  Total expected delta: ~150–250 lines including tests. The
  branch-mode code path is unchanged.

## Migration Plan

No migration is required for existing operators. Operators who do
not set `worktree: true` see no behavioral change. The default mode
remains branch mode.

Operators who wish to migrate to worktree mode can do so by adding
`worktree: true` to their config and restarting `see`. The first
run after migration uses `git worktree add -B see/<digest>` which
reuses any existing `see/<digest>` branch from prior branch-mode
runs; the operator's checkout transitions from "checked out on
`see/<digest>`" to "checked out on `<their real branch>`" because
worktree mode never switches the checkout.

Rollback strategy: if the change needs to be reverted, removing the
worktree-mode dispatch from `Watcher.work` restores the prior
behavior. Any leftover worktree directories under
`~/.cache/see/worktrees/` can be removed with
`git worktree remove --force <path>` or simply `rm -rf` if the
`.git/worktrees/` metadata has already been pruned. Any
`see/<digest>` branches that were created and then merged-and-
deleted under worktree mode are gone; any left behind by a
manually-merged attempt can be cleaned with `git branch -D
see/<digest>`.

## Open Questions

- **TUI surface.** What does the lane look like in the TUI when it
  lives in a separate directory? Three options surfaced during
  exploration: hide the lane entirely and rely on `git log` for
  status; show the lane as a separate row with its worktree path;
  show the lane inline as a secondary state on the repo row.
  Capture as follow-up work in tasks.md.

- **Per-workflow override.** Current design is global flag + global
  config field. Operators may want different workflows to use
  different modes (e.g., production workflows on branch mode,
  experimental workflows on worktree mode). Capture as future
  enhancement, not in this change.

- **`see worktree list` subcommand.** Operators may want a manual
  inspection surface for the worktree set. Capture as future
  enhancement, not in this change.

- **Conflict surfacing.** When a rebase conflict causes rollback,
  the operator only sees "agent failed" with the rebase error in
  the wrapped message. Whether to surface this more prominently
  in the TUI / events stream is a UX question for follow-up.