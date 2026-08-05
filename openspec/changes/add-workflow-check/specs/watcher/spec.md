# watcher delta — add-workflow-check

## MODIFIED Requirements

### Requirement: Successful custom runs run the check gate, then create a catch-up commit only for staged changes

After a custom agent run succeeds in branch mode, `Watcher` SHALL, if
the workflow defines a `check` and the working tree is dirty with
changes to land, run the check gate (see workflow-condition). On a
failed check, `Watcher` SHALL trigger the branch-mode custom rollback
(see the rollback requirement) and return the check-failure error
without staging or committing. On a passed check — or when no check is
defined, or when the working tree is clean — `Watcher` SHALL stage all
working-tree changes. It SHALL create a commit with the rendered custom
commit message only when the index differs from `HEAD`. When no staged
changes remain, including when the agent committed all work itself or
made no changes, the watcher SHALL return success without invoking
`git commit` and without emitting a no-changes `Warning`. Commits made
by the agent SHALL remain intact. The watcher SHALL leave the custom
lane checked out in either case.

#### Scenario: Passing check precedes the catch-up commit

- **WHEN** a custom agent succeeds in branch mode, the workflow defines
  a check, the working tree is dirty, and the check exits `0`
- **THEN** the check runs in the lane checkout before staging
- **AND** the watcher stages the changes and creates the catch-up commit
- **AND** leaves the custom lane checked out

#### Scenario: Failed check triggers branch rollback without committing

- **WHEN** a custom agent succeeds in branch mode, the workflow defines
  a check, and the check exits nonzero
- **THEN** no `git add` or `git commit` runs for the attempt
- **AND** the lane is restored to its pre-attempt tip (or removed if
  newly created)
- **AND** the attempt's working-tree changes are discarded
- **AND** the attempt returns the check-failure error

#### Scenario: Leftover changes receive custom commit

- **WHEN** a custom agent succeeds, no check is defined (or the check
  passes), and the agent leaves tracked or untracked changes
- **THEN** the watcher stages those changes
- **AND** commits them with the rendered custom commit message
- **AND** leaves the custom lane checked out

#### Scenario: Agent committed all changes

- **WHEN** a custom agent succeeds after committing all of its work
- **THEN** the watcher preserves the agent commits
- **AND** creates no additional commit
- **AND** emits no no-changes warning

#### Scenario: Idempotent run is a successful no-op

- **WHEN** a custom agent succeeds without changing the repository
- **THEN** the watcher creates no commit
- **AND** the check is not executed
- **AND** returns success
- **AND** the condition may trigger another run on the next polling pass

### Requirement: Watcher rolls back only the failed custom attempt

*(Branch mode only — see lane-isolation for the worktree-mode contract.)*
Immediately before invoking an agent in branch mode, `Watcher` SHALL
capture the selected workflow lane tip. If the agent fails, or the
workflow check fails, on an existing lane, the watcher SHALL reset
tracked state to that tip, remove non-ignored untracked files created by
the attempt, preserve ignored files and earlier lane commits, and leave
the clean lane available for subsequent workflows. If the lane was
created by the attempt, the watcher SHALL return to the branch that was
checked out before that workflow, restore its captured commit, and
delete only the new lane.

#### Scenario: Failure on an existing lane preserves history

- **WHEN** an existing custom lane has commits `A`, `B`, and `C`
- **AND** the agent creates edits or commits after `C` and then fails,
  or the workflow check fails after the agent succeeded
- **THEN** the lane tip is restored to `C`
- **AND** commits `A`, `B`, and `C` remain reachable from the lane
- **AND** the lane remains checked out
- **AND** the agent error or check-failure error is returned

#### Scenario: Failure removes newly-created lane

- **WHEN** a custom lane is created for the first time and the agent
  fails, or the workflow check fails after the agent succeeded
- **THEN** the original branch and captured commit are restored
- **AND** the new custom lane is deleted
- **AND** the agent error or check-failure error is returned

#### Scenario: Agent-created untracked files are removed

- **WHEN** the custom working tree was clean before the agent ran
- **AND** the failing agent, or a failing check after a successful
  agent, leaves non-ignored untracked files
- **THEN** rollback removes those files

#### Scenario: Ignored files survive rollback

- **WHEN** a failing custom agent, or a failing check, creates or
  modifies an ignored file
- **THEN** rollback does not delete that ignored file

#### Scenario: Existing workflow lane preserves history after failure

- **WHEN** an existing workflow lane has prior commits and its agent
  fails, or its check fails after the agent succeeded
- **THEN** the lane is reset to its pre-attempt tip
- **AND** prior commits remain reachable
- **AND** later workflows may run after cleanup

#### Scenario: New workflow lane is removed after failure

- **WHEN** an agent fails, or a check fails after the agent succeeded,
  during the first attempt on a new workflow lane
- **THEN** the pre-workflow branch and commit are restored
- **AND** only the newly created lane is deleted
- **AND** later workflows may run from the restored clean checkout

## ADDED Requirements

### Requirement: A failed check emits CheckFailed as the terminal event

`Watcher` SHALL define a `CheckFailed` event carrying the repository
path, workflow name, change, the rendered check command, the integer
exit code, and the captured standard error string. The event SHALL
implement the `Event` interface and SHALL carry a short summary suitable
for the `RetryAttempt` and Terminal User Interface (TUI) rendering.

When the final attempt for a repository fails and the returned error is
a check-failure error (the workflow check exited nonzero), `runOnce`
SHALL emit a `CheckFailed` event instead of a `ChangeFailed` event. When
the final error is any other error, `runOnce` SHALL emit `ChangeFailed`
unchanged. A single terminal event SHALL be emitted per repository per
polling pass: `CheckFailed` and `ChangeFailed` SHALL NOT both be emitted
for the same final failure.

`RetryAttempt` events between attempts SHALL continue to carry the prior
attempt's error summary, including check-failure summaries, unchanged.

The `CheckFailed` event SHALL be forwarded to the TUI observer in TUI
mode and written to the batch-level JSONL stream in log mode, identically
to `ChangeFailed`.

#### Scenario: Final check failure emits CheckFailed

- **WHEN** the final attempt for a repository fails because the workflow
  check exited nonzero
- **THEN** `runOnce` emits a `CheckFailed` event carrying the rendered
  command, exit code, and captured standard error
- **AND** does not emit a `ChangeFailed` event for that failure

#### Scenario: Final agent failure emits ChangeFailed

- **WHEN** the final attempt for a repository fails for any reason other
  than the workflow check
- **THEN** `runOnce` emits a `ChangeFailed` event unchanged
- **AND** does not emit a `CheckFailed` event

#### Scenario: Retry between check failures carries the summary

- **WHEN** a check fails on attempt 1 and another attempt is permitted
- **THEN** the `RetryAttempt` event before attempt 2 carries a summary
  of the check failure
- **AND** only one terminal event is emitted after the final attempt
