## Why

Today, every successful agent run leaves the operator's checkout on the
`see/<change>` branch. To get back to their real branch they must run
`git checkout <branch>` themselves — and while the agent is running, they
cannot use their checkout at all, because it is the lane. Operators who
want to keep working on their own branch between agent runs have no way to
do so: the watcher's own branch-creation mechanism takes the checkout
hostage for the duration of every run.

This change adds an opt-in `worktree` isolation mode that runs the agent
inside a `git worktree` linked to the operator's checkout. The
operator's checkout is never switched, so they can keep editing,
committing, and rebasing on their own branch while the agent thinks.
When the agent succeeds, the lane's commits are rebased onto the
operator's current branch tip and fast-forward merged into it, so the
agent's work lands on the operator's branch without any manual merge
step. A `--auto-merge=false` escape hatch leaves the rebased lane for
manual review.

## What Changes

- Add a top-level `--worktree` flag (default off) and a top-level
  `worktree:` config field (default `false`). When enabled, the agent
  runs in a `git worktree` linked to the operator's checkout rather than
  in the checkout itself.
- Add a top-level `--auto-merge` flag (default on) and a top-level
  `auto_merge:` config field (default `true`). Only meaningful in
  worktree mode; in worktree mode it controls whether the rebased lane
  is fast-forward merged into the operator's branch or left for manual
  review.
- Add a top-level `worktree_root:` config field (default
  `~/.cache/see/worktrees`). Where new worktrees are created when
  `worktree: true`. Tilde-expanded.
- Define three explicit isolation modes and their contracts:
  - **branch** (default): unchanged from today. Lane lives on the
    operator's checkout; success leaves HEAD on the lane.
  - **worktree + auto-merge** (default when `worktree: true`): lane
    lives in a worktree; operator's checkout never switches; on
    success the lane is rebased onto the operator's current branch tip
    and fast-forward merged into it; the lane and worktree are
    deleted afterward.
  - **worktree + manual-merge** (when `worktree: true` and
    `auto_merge: false`): lane lives in a worktree; operator's
    checkout never switches; on success the lane is rebased onto the
    operator's current branch tip and left for manual review; the
    lane and worktree are preserved.
- Add validation rules: `auto_merge` requires `worktree`;
  `worktree_root` requires `worktree`. Misconfigurations fail at
  startup with status `2`.
- CLI precedence: `--worktree` and `--auto-merge` flags override the
  corresponding config fields. Config fields override defaults. Same
  precedence pattern as `--prompt`.

**BREAKING**: none for default behavior. The default mode is still
branch mode with the same operational contract as today.

## Capabilities

### New Capabilities

- `lane-isolation`: defines the three isolation modes (branch,
  worktree + auto-merge, worktree + manual-merge), their selection via
  flag and config field, their merge-back semantics, their rollback
  semantics, and the validation rules for invalid combinations. This
  capability owns the worktree plumbing and the cross-mode contracts.

### Modified Capabilities

- `watcher`: scope the existing branch-mode requirements (per-change
  branch creation, leave-HEAD-on-lane after success, per-mode
  rollback) to the branch isolation mode. Add a thin dispatch
  requirement that delegates to `lane-isolation` when worktree mode is
  active. The branch-mode requirements themselves are unchanged;
  only their applicability is gated.

## Impact

- `main.go`:
  - New helpers: `ensureWorktree(path, change, digest) (created bool,
    err error)`, `rollbackWorktree(path, change, digest, attemptTip
    string, created bool, cause error)`, `mergeWorktreeLane(path,
    operatorRef, worktreePath string) error`.
  - `Watcher.workResolved` (or its successor): dispatch into branch
    mode or worktree mode based on the resolved config.
  - New flags: `--worktree` (bool, default false), `--auto-merge`
    (bool, default true). Wired into `main()`.
  - New `--worktree-root` flag (string, default empty). Wired into
    `main()`; falls back to config field, falls back to default.
  - `originalRef` and `GetCurrentCommit` continue to be used; their
    roles shift slightly in worktree mode (operator's ref becomes the
    rebase/merge target rather than a rollback anchor).
- `config.go`:
  - New fields on `Config`: `Worktree bool`, `AutoMerge bool`,
    `WorktreeRoot string`.
  - New validation function (or extension to existing): reject
    `auto_merge: true` without `worktree: true`; reject
    `worktree_root: <non-empty>` without `worktree: true`.
  - Strict schema: unknown fields still rejected.
- `main_test.go`:
  - New tests for `ensureWorktree` (fresh lane, existing lane with
    dirty tree, existing lane with clean tree, stale worktree dir,
    cross-filesystem root, etc.).
  - New tests for `rollbackWorktree` (created-this-attempt,
    reused-lane cases).
  - New tests for `mergeWorktreeLane` (clean rebase, rebase conflict,
    operator-dirty at merge time, fast-forward failure).
  - New tests for mode dispatch given `Worktree`/`AutoMerge`/
    `WorktreeRoot` config combinations.
  - New tests for the validation rules in `config.go`.
- `AGENTS.md`:
  - New section: "Lane isolation modes" describing the three modes
    and their contracts.
  - Rewrite the "Persistent per-change lanes" and "Lane lifecycle"
    sections to describe both branch-mode and worktree-mode behavior.
  - Configuration schema block: add the three new top-level fields.
  - "Custom Workflows" section: cross-reference the lane-isolation
    mode selection.
- No dependency changes. No external tool integration changes.
- No breaking changes to existing flag/field behavior. Operators who
  do not set `worktree: true` see no behavioral change.