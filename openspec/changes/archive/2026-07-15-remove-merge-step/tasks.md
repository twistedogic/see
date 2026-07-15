## 1. Failing tests (`main_test.go`)

- [x] 1.1 Add a test that runs `Watcher.work` against a repo on
  `main` with one active change, an agent stub returning
  `(path, nil)`, and asserts that after `work` returns, `HEAD`
  is checked out on `see/<change>` (not on `main`).
- [x] 1.2 Add a test that pins `main`'s SHA before and after the
  run and asserts they are equal (no merge happened, no commit
  added on `main`).
- [x] 1.3 Add a test that asserts `see/<change>` exists after the
  run and is NOT a candidate for `git branch -d` (its tip is not
  fully reachable from `main`). The existing branch-cleanup
  ergonomics (`-d` would succeed because `main` is unchanged)
  is intentional — the test just pins that `see/<change>` is
  not deleted by `work`.
- [x] 1.4 Add a test that asserts the catch-up commit's subject
  is reachable from `see/<change>`'s tip and contains any file
  the agent left dirty (use a sentinel file the agent stub does
  not commit).
- [x] 1.5 Update the existing test at line ~671 ("expected merge
  commit on main, got") to assert the opposite: no merge commit
  subject on `main`.
- [x] 1.6 Update the existing test at line ~675 ("expected
  `see/task-1` to be deleted") to assert the opposite:
  `see/task-1` exists at run end.
- [x] 1.7 Update the existing test at line ~693 ("agent's apply
  commit reachable from `main`") to assert the agent's commit
  is reachable from `see/<change>` but NOT from `main`.
- [x] 1.8 Update the existing test at line ~909 ("merge commit
  contains sentinel.txt") to assert the sentinel is committed
  on `see/<change>` instead.
- [x] 1.9 Update the existing test at line ~929 ("expected merge
  commit with 2 parents") to assert the workspace branch has
  exactly one parent on a single-commit agent run (the
  catch-up commit's parent is the captured pre-run SHA).
- [x] 1.10 Update the existing test at line ~980 ("expected no
  `see/` branches") to assert `see/<change>` DOES exist at run
  end and is HEAD.
- [x] 1.11 Update the existing test at line ~840 (drifted
  `see/<change>` from a prior partial run must not leak) so
  the assertion is: the drifted branch is pinned back to the
  captured pre-run SHA before the agent runs (same contract),
  AND the branch is left in place at run end (new contract).
- [x] 1.12 Confirm the failure-path tests still pass without
  modification: agent error → `see/<change>` deleted, `main`
  reset to captured SHA, HEAD on `main`. The rollback contract
  is unchanged.

## 2. Code change (`main.go`)

- [x] 2.1 In `Watcher.work`'s success path, remove the
  `git switch ref` step that returns to the user's starting
  branch. The `git add -A && git commit` catch-up step above
  it stays.
- [x] 2.2 Remove the `git merge --no-ff <branch> -m "see: merge
  openspec change <change>"` step.
- [x] 2.3 Remove the merge-failure cleanup block (the `git
  merge --abort`, second `git reset --hard`, second
  `git branch -D`, and the `fmt.Errorf("merge %s: %w", ...)`
  return).
- [x] 2.4 Remove the `git branch -d <branch>` cleanup step
  that ran after a successful merge.
- [x] 2.5 Remove the `ponytail:` comment on the merge line
  ("merge --no-ff so the watcher's involvement shows up as a
  graph node..."). It described the now-removed step.
- [x] 2.6 Confirm the rollback block (agent-error path) still
  runs `git switch ref`, `git reset --hard <captured-SHA>`,
  `git branch -D <branch>` in that order. The
  `ponytail:` comment on that block ("rollback runs every
  cleanup step regardless of the previous failure") stays.
- [x] 2.7 Confirm the catch-up `git add -A && git commit
  -m "see: apply openspec change <change>"` step still runs
  after `done := true`. Its `ponytail:` comment ("same inline
  commit pattern as before — runs even when archive or commit
  fails so partial progress isn't lost") stays.
- [x] 2.8 Confirm `ChangeDone` is still emitted after the
  catch-up commit on the success path. Order:
  catch-up commit → `ChangeDone` → return.

## 3. Spec deltas (`openspec/specs/watcher/spec.md`)

- [x] 3.1 Add a `## REMOVED Requirements` block under the
  `watcher` capability: remove the existing "Watcher merges
  the agent's commit back on success" Requirement and its
  two Scenarios ("Successful run produces a merge commit on
  the original branch" and "Merge conflict is treated as
  failure"). Provide a one-line `Reason:` that names the new
  contract.
- [x] 3.2 Add a `## ADDED Requirements` block: a new
  Requirement that names the new contract — `Watcher` leaves
  HEAD on `see/<change>` after a successful run, `main` is
  not modified — plus two Scenarios: "Successful run leaves
  HEAD on workspace branch" and "Original branch tip is
  unchanged on success."
- [x] 3.3 Add a `## MODIFIED Requirements` entry: the
  existing "Watcher emits Warning events for cleanup-step
  failures" Requirement is updated to drop the now-unreachable
  merge-related entries (`git merge --no-ff <branch>`, `git
  merge --abort`, `git branch -d <branch>` after a successful
  merge) from its step list. The rollback-path entries
  (`git switch`, `git reset --hard`, `git branch -D`) and the
  catch-up entries (`git add -A`, `git commit`) stay.

## 4. Config note (`openspec/config.yaml`)

- [x] 4.1 Add a new bullet to `shape_choices` documenting that
  successful runs leave the user's starting branch untouched
  and leave HEAD on `see/<change>`. One line, in the voice
  of the existing entries (e.g. "no merge back to ref; HEAD
  inherits on see/<change> at run end").

## 5. Validation

- [x] 5.1 Run `go build ./...` and confirm it compiles.
- [x] 5.2 Run `go test ./...` and confirm all tests pass,
  including the rewritten merge-related tests (tasks 1.5–1.10)
  and the unchanged failure-path tests (task 1.12).
- [x] 5.3 Run `openspec validate remove-merge-step` and
  confirm green.
- [ ] 5.4 Manual smoke: in `see`'s own repo (or a scratch
  repo with one openspec change), run `see --mode=log --once`
  against the change. After the run, confirm `git branch
  --show-current` reports `see/<change>`, confirm
  `git rev-parse main` matches the SHA captured before
  `see` started, and confirm `git branch --list "see/*"`
  shows the workspace branch still in place.
- [ ] 5.5 Manual smoke: introduce a failing agent (stub
  returning an error), run `see --mode=log --once`. After
  the run, confirm HEAD is back on `main`, `main` is at the
  pre-run SHA, and `git branch --list "see/*"` is empty.
  Verifies the unchanged rollback path is still wired
  correctly alongside the new success path.
- [ ] 5.6 Manual smoke: run `see` against two repos in
  sequence (`--watch` pointing at both). After both runs,
  confirm each repo's HEAD is on its own `see/<change>`
  branch, neither repo's starting branch has moved, and
  the batch-level JSONL carries one `RepoSeen` and one
  `ChangeDone` per repo.
