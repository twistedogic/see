## Why

`see` is currently structured as a release bot. The success path of
`Watcher.work` snapshots the user's starting branch ref, runs the agent
in a `see/<change>` workspace branch, and at the end switches back to
the captured ref and `git merge --no-ff see/<change>` into it. Every
successful run advances the user's branch of record, regardless of
whether they wanted that advancement.

The user wants `see` to stop acting as a release bot. A successful
agent run should leave the repository on the `see/<change>` branch with
the user's starting branch untouched. Promotion to the user's branch
of choice becomes a manual step (or a future hook). This keeps
`see`'s blast radius small and explicit: one branch per run, no
implicit `--no-ff` merges, no silent advancement of `main`.

## What Changes

- `Watcher.work` no longer switches back to the original branch ref
  after a successful agent run, no longer runs
  `git merge --no-ff see/<change>`, and no longer deletes
  `see/<change>` via `git branch -d` on success. The workspace branch
  is the user's working branch going forward; HEAD is left on it.
- The catch-up `git add -A && git commit -m "see: apply openspec
  change <change>"` after a successful agent run is preserved. It
  remains the boundary between "agent work" and "operator commits."
- The rollback path on agent failure is unchanged: switch back,
  `reset --hard <captured-SHA>`, `branch -D see/<change>`. The
  rollback continues to delete the workspace branch so a retry can
  rebuild it cleanly.
- The `see/<change>` branch name is preserved. Renaming it (to
  `applied/<change>`, no prefix, etc.) is out of scope; the existing
  tests and grep workflows depend on the prefix.
- Existing tests in `main_test.go` that pin the merge-back contract
  (merge commit on `main`, `see/<change>` deleted after merge, agent
  commit reachable from `main`) are rewritten to pin the new contract:
  `see/<change>` exists at run-end on the user's branch; `main` is
  untouched.
- `config.yaml`'s `shape_choices` block gets a new entry documenting
  that successful runs leave the user's starting branch untouched.

## Capabilities

### Modified Capabilities

- `watcher`: the success-path requirement that currently mandates
  `git merge --no-ff` into the original branch is replaced by a new
  requirement that pins the leave-HED-on-workspace-branch contract.
  The success-path and rollback-path portions of the existing
  Warning-events requirement are updated to remove the merge-related
  steps that no longer run.

## Impact

- `main.go`: in `Watcher.work`, the success path's
  `git switch ref`, `git merge --no-ff <branch>`, merge-failure
  cleanup block, and `git branch -d <branch>` are removed. The
  `ponytail:` comment that motivated `--no-ff` (visibility on a
  single-commit branch) is removed with the code it annotated.
- `main_test.go`: the "merge commit on `main`" assertions, the
  "`see/<change>` deleted after merge" assertions, the
  "agent's commit reachable from `main`" assertions, the
  "merge commit has 2 parents" assertion, and the
  "no `see/` branches remain" final-state assertion are removed or
  rewritten. The drifted-workspace regression test (reused branch
  pinned back to captured SHA) stays: that contract is unchanged.
- `tui/` tests: untouched. `ChangeDoneMsg` rendering is unaffected;
  the TUI never advertised the merge result and gains no new
  responsibilities.
- `eventlog.go`, `tui/`, `go.mod`, `go.sum`: untouched.
- `openspec/specs/watcher/spec.md`: under the `watcher` capability,
  the "Watcher merges the agent's commit back on success" Requirement
  and its `Merge conflict is treated as failure` Scenario are
  removed; a new Requirement with two Scenarios pins the new
  contract; the `Watcher emits Warning events for cleanup-step
  failures` Requirement is updated to drop the now-unreachable
  merge-related entries from its step list.
- `openspec/config.yaml`: one new bullet added to `shape_choices`.
