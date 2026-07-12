## 1. Add helpers

- [x] 1.1 Add `originalRef(path string) (string, error)` to `main.go`.
      Runs `git -C <path> symbolic-ref --short HEAD`. Returns the trimmed
      output on success, or an empty string and nil error when the ref
      is empty (detached HEAD). Use `CombinedOutput` and check the exit
      code separately so an empty ref doesn't masquerade as an error
      from missing output.
- [x] 1.2 Add `ensureBranch(path, sha, name string) error` to `main.go`.
      Runs `git -C <path> branch --list <name>`; if the output contains
      `name`, run `git -C <path> switch <name>`. Otherwise run
      `git -C <path> switch -c <name>`. After the branch exists, run
      `git -C <path> reset --hard <sha>` to pin the tip to the captured
      SHA. Return the first non-nil error.

## 2. Reproduce the missing-isolation behavior with a failing test

- [x] 2.1 Add `TestWorkIsolatesAgentRunOnBranch` to `main_test.go`.
      Spin up a temp repo, init + commit + switch -c main, set up an
      active change, run `Watcher.work` with a fake agent that succeeds
      and archives the change. Assert that after the run:
      (a) HEAD on `main` has a merge commit whose subject contains
      `"see: merge openspec change"`,
      (b) `git branch --list see/<change>` is empty,
      (c) the working tree is on `main`,
      (d) `git log --all --oneline` shows the agent's `see: apply`
      commit on the `see/<change>` ref before deletion.
- [x] 2.2 Run `go test ./...`. Confirm the test fails (current
      `Watcher.work` commits on `main` directly, so the merge-commit
      assertion fails).

## 3. Refactor Watcher.work

- [x] 3.1 Capture `current, err := GetCurrentCommit(path)` and
      `ref, err := originalRef(path)` at the top of `work()`. If
      `originalRef` reports a detached HEAD, log a clear error and
      return `false, err`.
- [x] 3.2 After selecting the active change, call
      `ensureBranch(path, current, "see/"+change)`. The function pins
      the branch tip to `current` so a reused or drifted branch always
      starts the agent from the captured SHA. If it returns an error,
      return `false, err`.
- [x] 3.3 On agent error: replace the existing `git reset --hard
      current` block with the full rollback: switch back to `ref` (or
      `git switch --detach current` if `ref == ""`), `git reset --hard
      current`, `git branch -D see/<change>`. Return the agent error.
- [x] 3.4 On agent success and change-done, after the existing
      `git add -A` and `git commit`, add the merge-back: if `ref != ""`,
      `git switch <ref>`, then `git merge --no-ff see/<change> -m
      "see: merge openspec change <change>"`. On merge error, run
      `git merge --abort`, `git reset --hard current`, `git branch -D
      see/<change>`, return `false, mergeErr`. On merge success, run
      `git branch -d see/<change>` (safe-delete; reflog keeps it).

## 4. Update existing tests

- [x] 4.1 In `TestWorkCommitsOnSuccess`, add `run("switch", "-c", "main")`
      after the initial `git init -q` to guarantee a real branch state
      regardless of git version / `init.defaultBranch`.
- [x] 4.2 Widen the `git log --oneline` assertion in
      `TestWorkCommitsOnSuccess` to `git log --all --oneline` and check
      for the `see: apply openspec change task-1` subject on the
      `see/task-1` ref before it's deleted. The merge-commit message on
      `main` can be asserted separately or skipped here since it's
      covered by `TestWorkIsolatesAgentRunOnBranch`.

## 5. Add failure-mode tests

- [x] 5.1 Add `TestWorkRollsBackBranchOnAgentFailure`. Fake agent
      returns an error mid-run. After `Watcher.work`, assert: HEAD on
      `main` is at the pre-run SHA, `see/<change>` does not exist,
      `Watcher.work` returned the agent error.
- [x] 5.2 Add `TestWorkReusesExistingBranchAndResetsToOriginalSHA`.
      In the fixture: init + commit + switch -c main, then
      `git switch -c see/<change>`, write a sentinel file, commit it
      on `see/<change>` (so its tip is one commit past the original
      SHA), then `git switch main`. Run `Watcher.work` with a
      succeeding fake agent that archives the change. Assert: the
      sentinel file is gone from the working tree after the agent runs
      (the `git reset --hard current` in `ensureBranch` wiped it),
      the final merge commit on `main` does NOT contain the sentinel
      file (verified via `git show --stat <merge-sha>`), and
      `see/<change>` is deleted after the run.
- [x] 5.3 Add `TestWorkRejectsDetachedHead`. Detach HEAD in the
      fixture (`git checkout <sha>` after init), then run
      `Watcher.work`. Assert it returns an error and no `see/<change>`
      branch is created.

## 6. Verify

- [x] 6.1 `go vet ./...` clean.
- [x] 6.2 `go build ./...` clean.
- [x] 6.3 `go test -race ./...` green.
- [x] 6.4 Manual read-through of `main.go`: confirm the rollback path
      runs in the right order (switch away before reset, delete branch
      last), confirm the merge-back path produces a merge commit even
      on a single-commit `see/<change>`, confirm no stray references
      to deleted code paths.