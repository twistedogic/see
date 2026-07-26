## 1. Add worktree fields to the configuration schema

- [x] 1.1 Add `Worktree bool`, `AutoMerge bool`, and `WorktreeRoot string`
      fields to the `Config` struct in `config.go`. Use `yaml` tags
      matching the new field names exactly. The default-zero values
      (`false`, `false`, `""`) are the schema defaults and match the
      proposed runtime defaults.
      **Note:** `AutoMerge` uses `*bool` (not plain `bool`) so the
      resolver can distinguish "unset, default true" from "explicitly
      false". The spec sets the runtime default to `true` while the
      zero value is `false`; plain `bool` cannot represent manual-merge
      (`worktree: true, auto_merge: false`) in config. Tasks 2.3(c)
      and the spec's manual-merge scenario require this.
- [x] 1.2 Add a `validateWorktreeSettings(cfg *Config) error` helper
      in `config.go` that enforces:
      - `cfg.AutoMerge` requires `cfg.Worktree`
        (reject if `cfg.Worktree == false && cfg.AutoMerge == true`).
      - `cfg.WorktreeRoot` requires `cfg.Worktree`
        (reject if `cfg.Worktree == false && cfg.WorktreeRoot != ""`).
      The function returns an error message that names the offending
      field; the caller surfaces it via `os.Stderr` and exits with
      status `2`, consistent with existing validation errors.
- [x] 1.3 Wire `validateWorktreeSettings` into the existing
      `validateWorkflows`-style startup validation flow in `main()`,
      after `loadStartupConfig` returns and before `resolveConfiguredTargets`
      runs. Reject unknown fields continues to come from the strict
      YAML decoder; the new validator is a separate, additive check.

## 2. Reproduce the missing-worktree-mode behavior with failing tests

- [x] 2.1 Add `TestConfigRejectsAutoMergeWithoutWorktree` to
      `config_test.go`. Configuration sets `worktree: false` and
      `auto_merge: true`. Assert that `validateWorktreeSettings`
      returns a non-nil error mentioning `auto_merge`.
- [x] 2.2 Add `TestConfigRejectsWorktreeRootWithoutWorktree`. Config
      sets `worktree: false` and `worktree_root: /somewhere`. Assert
      `validateWorktreeSettings` returns an error mentioning
      `worktree_root`.
- [x] 2.3 Add `TestConfigAcceptsValidCombinations`. Three sub-cases:
      (a) `worktree: false, auto_merge: false` (no error — branch
      mode with auto_merge explicitly off), (b) `worktree: true,
      auto_merge: true, worktree_root: ""` (default root), (c)
      `worktree: true, auto_merge: false, worktree_root: ~/custom`.
      Assert no error.
- [x] 2.4 Run `go test ./...`. Confirm the new tests fail because
      `Worktree`, `AutoMerge`, and `WorktreeRoot` don't exist yet
      and `validateWorktreeSettings` doesn't exist yet.

## 3. Add worktree-mode helpers

- [x] 3.1 Add `ensureWorktree(path, change string) (created bool,
      err error)` to `main.go`. Behavior:
      - Calls `git worktree prune` against `path` to clear stale
        metadata.
      - If `see/<digest>` branch already exists, uses its tip as
        the start point; otherwise uses `path`'s current `HEAD`.
      - Runs `git -C <path> worktree add -B see/<digest>
        <worktree_root>/<repo-basename>--<digest> <start>`.
      - Returns `created=true` when the branch did not exist before
        the call, `created=false` otherwise.
      - On any git error, returns wrapped error with the underlying
        command output.
      Wire `worktree_root` and the `<repo-basename>--<digest>` path
      computation via parameters or a small struct so the same
      helper serves custom mode, OpenSpec compat mode, and the
      `--worktree-root` override.
- [x] 3.2 Add `rollbackWorktree(path, change string, created bool,
      cause error) error` to `main.go`. Behavior:
      - Emits a `Warning` via `w.warn` for each git step that
        fails, mirroring the existing `rollbackCustomLane` pattern.
      - In order: `git rebase --abort` (no-op when not in a
        rebase), `git merge --abort` (no-op when not in a merge),
        `git worktree remove --force <worktree_path>`, and
        `git branch -D see/<digest>` (the `-D` is correct: the
        lane must not survive rollback regardless of whether it
        existed before this attempt, because in worktree mode the
        lane is per-attempt).
      - Returns the original `cause` error unchanged.
- [x] 3.3 Add `mergeWorktreeLane(path, operatorRef, worktreePath
      string) error` to `main.go`. Behavior:
      - `cd <worktreePath>; git add -A; git diff --cached --quiet
        || git commit -m <rendered catch-up message>` (catch-up
        commit inside the worktree, same logic as today's
        `catchUpCustomCommit`).
      - `git -C <worktreePath> rebase <operatorRef>` (where
        `<operatorRef>` is the operator's current branch tip, not
        the captured commit from attempt start).
      - On rebase non-zero exit: run `git rebase --abort` and
        return the wrapped rebase error. Caller handles rollback.
      - Re-run the dirty-tree check against `path` (the operator's
        checkout). On dirty: return a wrapped
        `see: working tree on <path> is dirty; commit or stash
        before merge runs` error. Caller handles rollback.
      - `git -C <path> merge --ff-only see/<digest>`.
      - On merge non-zero exit: `git merge --abort`, return the
        wrapped merge error. Caller handles rollback.
      - On success: `git -C <path> worktree remove --force
        <worktreePath>`; `git -C <path> branch -d see/<digest>`.
      - Returns nil on success.

## 4. Add CLI flags and main() wiring

- [x] 4.1 Add three new flags to `main()` in `main.go`:
      `worktree = flag.Bool("worktree", false, "run agents in a git
      worktree so the operator's checkout is never switched")`,
      `autoMerge = flag.Bool("auto-merge", true, "in worktree mode,
      rebase and fast-forward merge the lane into the operator's
      branch on success; pass --auto-merge=false to leave the
      rebased lane for manual review")`, and `worktreeRoot =
      flag.String("worktree-root", "", "override the worktree
      location (default ~/.cache/see/worktrees); only meaningful
      with --worktree")`.
- [x] 4.2 Resolve the effective `(Worktree, AutoMerge,
      WorktreeRoot)` triple after `loadStartupConfig` returns.
      Precedence: CLI flag > config field > default. `WorktreeRoot`
      resolution: CLI flag if non-empty, else `cfg.WorktreeRoot`,
      else `~/.cache/see/worktrees`. Tilde-expand the resolved
      `WorktreeRoot` once with `expandTilde` (same helper used for
      `root_dir`).
- [x] 4.3 Reject `--auto-merge` or `--auto-merge=false` without
      `--worktree` (and without `worktree: true` in config) at
      startup with an actionable error and exit status `2`.
      Equivalent to the validation in step 1.2 but checked against
      the resolved values, not the raw config.
- [x] 4.4 Add the three new resolved values to the `Watcher`
      struct (`Worktree bool`, `AutoMerge bool`, `WorktreeRoot
      string`) so `Watcher.work` can read them.
- [x] 4.5 Pass the resolved values into the watcher. Either via
      the `Watcher` struct fields directly (simplest, matches the
      existing `PromptTemplate` / `Condition` / `CommitTemplate`
      field pattern) or via a small `LaneIsolationConfig` value
      carried alongside `Workflows`.

## 5. Add the mode dispatch to Watcher.work

- [x] 5.1 In `Watcher.work` (or its successor that owns the
      per-attempt orchestration), branch on `w.Worktree`. The
      default branch keeps the existing path (ensureBranch /
      ensureWorkflowLane / rollbackCustomLane / catchUpCustomCommit).
      The new branch calls `ensureWorktree`, runs the agent in
      `<worktree_root>/<repo-basename>--<digest>`, then calls
      `mergeWorktreeLane` (when `w.AutoMerge` is true) or stops
      with the rebased lane + worktree preserved (when
      `w.AutoMerge` is false).
- [x] 5.2 The `Agent.Run` invocation in worktree mode SHALL pass
      the worktree path as `path` (the second argument). The
      `digest` argument (third) SHALL be the lane digest used by
      `pathFor` to compute the log filename, same as today.
- [x] 5.3 Failure handling: every `ensureWorktree`,
      `mergeWorktreeLane`, and `git rebase`/`git merge` error
      routes through `rollbackWorktree`. `rollbackWorktree` always
      runs the full cleanup regardless of which step failed.
- [x] 5.4 The pre-attempt `originalRef` capture remains; in
      worktree mode it becomes the rebase + merge target rather
      than a rollback anchor.
- [x] 5.5 The pre-attempt dirty-tree check (`hasUntrackedOrModified`)
      remains; in worktree mode it runs against the operator's
      checkout at attempt start (defense in depth against
      writeful conditions).

## 6. Add worktree-mode unit tests

- [x] 6.1 Add `TestEnsureWorktreeCreatesFreshLane`. Temp repo,
      `git init` + commit + `git switch -c main`, no existing
      `see/<digest>` branch. Call `ensureWorktree`. Assert:
      `see/<digest>` branch exists; worktree directory exists;
      worktree's `.git` file points at the main repo's
      `.git/worktrees/<digest>/`; `created=true`.
- [x] 6.2 Add `TestEnsureWorktreeReusesExistingLane`. Pre-create
      the lane at a non-`HEAD` commit by switching to it,
      committing a sentinel file, then switching back. Call
      `ensureWorktree`. Assert: lane tip is preserved (sentinel
      file reachable from `see/<digest>`); worktree path matches
      `created=false`.
- [x] 6.3 Add `TestEnsureWorktreeRecoversStaleDirectory`. Manually
      create a directory at the expected worktree path that is not
      a registered worktree. Call `ensureWorktree`. Assert:
      succeeds; `git worktree list` shows the new worktree.
- [x] 6.4 Add `TestRollbackWorktreeRemovesLaneAndWorktree`.
      Trigger `rollbackWorktree` after a successful
      `ensureWorktree`. Assert: worktree directory is gone;
      `see/<digest>` branch no longer exists; operator's checkout
      is unchanged.
- [x] 6.5 Add `TestMergeWorktreeLaneRebasesAndMerges`. Pre-create
      a worktree with two agent commits on `see/<digest>`. Run
      `mergeWorktreeLane` with the operator's `main` as the
      target. Assert: `main` tip equals the rebased lane tip; lane
      branch deleted; worktree directory gone.
- [x] 6.6 Add `TestMergeWorktreeLaneOnOperatorCommitDuringRun`.
      Capture the original `main` tip before the attempt. Have
      the operator's checkout advance one commit during the
      agent's run (simulate via a separate goroutine or test
      harness setup). Run `mergeWorktreeLane`. Assert: the
      rebase target was the new `main` tip; both the operator's
      commit and the rebased agent commits are reachable from
      `main`.
- [x] 6.7 Add `TestMergeWorktreeLaneRebaseConflictTriggersRollback`.
      Set up a rebase conflict between the agent's commits and a
      divergent operator commit. Run `mergeWorktreeLane`. Assert:
      returns an error; lane branch and worktree directory are
      gone; operator's checkout is unchanged.
- [x] 6.8 Add `TestMergeWorktreeLaneOperatorDirtyTriggersRollback`.
      Make the operator's checkout dirty before `mergeWorktreeLane`
      reaches the merge step. Assert: returns a dirty-merge-time
      error; lane and worktree cleaned up; operator's dirty edits
      preserved.
- [x] 6.9 Add `TestMergeWorktreeLaneFastForwardFailureTriggersRollback`.
      Force a `--ff-only` failure by advancing `main` between
      rebase and merge. Assert: rollback completes; operator's
      late commit is preserved on `main`.
- [x] 6.10 Add `TestWorktreeModeEndToEnd`. Full pass through
      `Watcher.work` with worktree mode enabled and a fake agent.
      Assert: agent was invoked with `cwd` in the worktree;
      operator's checkout stayed on `main`; `main` received the
      rebased agent commits; worktree directory and lane branch
      were cleaned up; `ChangeDone` event was emitted.

## 7. Add dispatch and CLI tests

- [x] 7.1 Add `TestWatcherDispatchesToWorktreeMode`. Construct a
      `Watcher` with `Worktree: true, AutoMerge: true` and a
      fake agent. Assert `Watcher.work` calls the worktree
      helpers and not the branch-mode helpers (check via the
      fake agent's recorded `path` argument — should be the
      worktree directory, not the operator's checkout).
- [x] 7.2 Add `TestWatcherDispatchesToBranchMode`. Same setup
      with `Worktree: false`. Assert the existing branch-mode
      path is taken.
- [x] 7.3 Add `TestCLIRaisesAutoMergeWithoutWorktree`. Invoke
      `main()` indirectly (or extract the flag-validation
      function and unit-test it). Pass `--auto-merge=false`
      without `--worktree` and without `worktree: true` in
      config. Assert the program exits with status `2` and an
      actionable error message.
- [x] 7.4 Add `TestCLIFlagsOverrideConfig`. Config sets
      `worktree: false`. CLI passes `--worktree`. Assert the
      watcher is constructed with `Worktree: true`. Same for
      `--auto-merge=false` overriding `auto_merge: true`.
- [x] 7.5 Add `TestWorktreeRootDefaultAndOverride`. With no
      `--worktree-root` flag and no `worktree_root:` config:
      assert the resolved root is `~/.cache/see/worktrees`.
      With `--worktree-root ~/custom`: assert the resolved
      root is `<home>/custom`.

## 8. Update AGENTS.md

- [x] 8.1 Add a new section "Lane isolation modes" describing the
      three modes (branch, worktree + auto-merge, worktree +
      manual-merge) with their contracts. Include the operator
      experience during and after each mode.
- [x] 8.2 Update the "Persistent per-change lanes" section (or
      replace it with a new "Lane lifecycle" section) to describe
      both branch-mode and worktree-mode behavior. Highlight the
      operator-checkout-stays-on-real-branch property of worktree
      mode.
- [x] 8.3 Update the configuration schema block to add the three
      new top-level fields with their defaults.
- [x] 8.4 Add a "Configuration" subsection under "Lane isolation
      modes" documenting validation rules and the CLI/config
      precedence.
- [x] 8.5 Update the "Custom Workflows" section to cross-reference
      "Lane isolation modes" so workflow operators discover the
      worktree option without reading both sections in full.
- [x] 8.6 Add a "Migration from branch mode" note explaining how
      operators transition: set `worktree: true`, restart, observe
      the first run create a worktree under the default (or
      configured) root, the lane ref is reused via `git worktree
      add -B`.

## 9. Update TUI surface (follow-up)

- [x] 9.1 Decide the TUI representation for worktree-mode repos.
      Three options surfaced during exploration: (a) hide the lane
      and rely on `git log`; (b) show the lane as a separate row
      with its worktree path; (c) inline the lane as a secondary
      state on the repo row. Implement the chosen option.
      **Decision: (a).** The TUI is purely event-driven from the
      observer and references no branch/worktree state directly; the
      existing `ChangeStarted`/`ChangeDone`/`ChangeFailed`/`LogPath`
      events are mode-agnostic and flow through unchanged, so the
      operator sees the same per-repo status in worktree mode as in
      branch mode. `git log`/`git worktree list` remain the inspection
      surface for the lane. No TUI code change needed. Options (b)/(c)
      are deferred as future enhancement.
- [x] 9.2 If option (b) or (c), surface the rebased-and-merged
      **N/A under decision (a)** — no distinct merged/manual-review
      row state is rendered. The mode-agnostic events already convey
      the run outcome.
      outcome (`applied X → merged into <branch>`) and the
      manual-review outcome (`applied X → rebased onto <branch>,
      awaiting merge`) in the row state.

## 10. Verify

- [x] 10.1 `go vet ./...` clean.
- [x] 10.2 `go build ./...` clean.
- [x] 10.3 `go test -race -timeout 30s ./...` green. The 30-second
      ceiling is mandatory per `AGENTS.md` testing rules; tests
      that wedge the poll loop should fail fast.
- [x] 10.4 `openspec validate worktree-mode` green.
- [x] 10.5 Manual smoke check: run `see --worktree --once` against
      a fixture repo with an active OpenSpec change. Confirm the
      worktree directory appears under
      `~/.cache/see/worktrees/<repo>--<digest>/`, the agent runs
      inside it, and the rebased commits land on the operator's
      branch. Then run `see --worktree --auto-merge=false --once`
      against the same repo. Confirm the lane is preserved and the
      operator's branch is unchanged.
- [x] 10.6 Read-through of `main.go`: confirm the worktree-mode
      rollback runs in the right order (`rebase --abort`,
      `merge --abort`, `worktree remove`, `branch -D`), confirm
      no stray references to deleted branch-mode code paths, and
      confirm the mode dispatch's branch-mode branch is
      byte-for-byte the existing behavior.
