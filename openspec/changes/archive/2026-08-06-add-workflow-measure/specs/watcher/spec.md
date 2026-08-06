# watcher delta — add-workflow-measure

## MODIFIED Requirements

### Requirement: Successful custom runs create a catch-up commit only for staged changes

After a custom agent run succeeds in branch mode, `Watcher` SHALL, if
the workflow defines a `check` and the working tree is dirty with
changes to land, run the check gate (see workflow-condition). On a
failed check, `Watcher` SHALL trigger the branch-mode custom rollback
(see the rollback requirement) and return the check-failure error
without staging or committing. On a passed check — or when no check is
defined — `Watcher` SHALL, if the workflow has a resolved measure gate,
run the candidate measure and require the candidate to be strictly
greater than the baseline (see workflow-condition). On a measure
failure, `Watcher` SHALL trigger the branch-mode custom rollback and
return the measure-failure error without staging or committing. On
improvement — or when no measure gate is resolved, or when the working
tree is clean — `Watcher` SHALL stage all working-tree changes. It SHALL
create a commit with the rendered custom commit message only when the
index differs from `HEAD`. When no staged changes remain, including when
the agent committed all work itself or made no changes, the watcher
SHALL return success without invoking `git commit` and without emitting a
no-changes `Warning`. Commits made by the agent SHALL remain intact. The
watcher SHALL leave the custom lane checked out in either case.

#### Scenario: Passing check and improvement precede the catch-up commit

- **WHEN** a custom agent succeeds in branch mode, the workflow defines a
  check and a measure, the working tree is dirty, the check exits `0`,
  and the candidate strictly exceeds the baseline
- **THEN** the check runs in the lane checkout before staging
- **AND** the candidate measure runs in the lane checkout before staging
- **AND** the watcher stages the changes and creates the catch-up commit
- **AND** leaves the custom lane checked out

#### Scenario: Failed check short-circuits the candidate measure

- **WHEN** a custom agent succeeds in branch mode, the workflow defines
  both a check and a measure, and the check exits nonzero
- **THEN** the candidate measure is not executed
- **AND** no `git add` or `git commit` runs for the attempt
- **AND** the lane is restored to its pre-attempt tip (or removed if
  newly created)
- **AND** the attempt returns the check-failure error

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

#### Scenario: Non-improvement triggers branch rollback without committing

- **WHEN** a custom agent succeeds in branch mode, the workflow defines a
  measure, the candidate does not strictly exceed the baseline
- **THEN** no `git add` or `git commit` runs for the attempt
- **AND** the lane is restored to its pre-attempt tip (or removed if
  newly created)
- **AND** the attempt's working-tree changes are discarded
- **AND** the attempt returns the measure-failure error

#### Scenario: Leftover changes receive custom commit

- **WHEN** a custom agent succeeds, no measure gate is resolved (and no
  check is defined or the check passes), and the agent leaves tracked or
  untracked changes
- **THEN** the watcher stages those changes
- **AND** commits them with the rendered custom commit message
- **AND** leaves the custom lane checked out

#### Scenario: Agent committed all changes

- **WHEN** a custom agent succeeds after committing all of its work
- **THEN** the watcher preserves the agent commits
- **AND** creates no additional commit
- **AND** emits no no-changes warning

#### Scenario: Idempotent run with no measure is a successful no-op

- **WHEN** a custom agent succeeds without changing the repository
- **AND** the workflow has no measure gate
- **THEN** the watcher creates no commit
- **AND** the check is not executed
- **AND** returns success
- **AND** the condition may trigger another run on the next polling pass

#### Scenario: Idempotent run is a successful no-op

- **WHEN** a custom agent succeeds without changing the repository
- **THEN** the watcher creates no commit
- **AND** the check is not executed
- **AND** returns success
- **AND** the condition may trigger another run on the next polling pass

### Requirement: Watcher rolls back only the failed custom attempt

*(Branch mode only — see lane-isolation for the worktree-mode contract.)*
Immediately before invoking an agent in branch mode, `Watcher` SHALL
capture the selected workflow lane tip. If the agent fails, the workflow
check fails, or the workflow measure gate fails (at the baseline or the
candidate), on an existing lane, the watcher SHALL reset tracked state to
that tip, remove non-ignored untracked files created by the attempt,
preserve ignored files and earlier lane commits, and leave the clean lane
available for subsequent workflows. If the lane was created by the
attempt, the watcher SHALL return to the branch that was checked out
before that workflow, restore its captured commit, and delete only the
new lane.

#### Scenario: Failure on an existing lane preserves history

- **WHEN** an existing custom lane has commits `A`, `B`, and `C`
- **AND** the agent creates edits or commits after `C` and then fails,
  or the workflow check fails after the agent succeeded, or the workflow
  measure gate fails after the agent succeeded
- **THEN** the lane tip is restored to `C`
- **AND** commits `A`, `B`, and `C` remain reachable from the lane
- **AND** the lane remains checked out
- **AND** the agent error, check-failure error, or measure-failure error
  is returned

#### Scenario: Failure removes newly-created lane

- **WHEN** a custom lane is created for the first time and the agent
  fails, or the check fails, or the measure gate fails (at the baseline
  or the candidate) after the agent succeeded
- **THEN** the original branch and captured commit are restored
- **AND** the new custom lane is deleted
- **AND** the agent error, check-failure error, or measure-failure error
  is returned

#### Scenario: Agent-created untracked files are removed

- **WHEN** the custom working tree was clean before the agent ran
- **AND** the failing agent, a failing check, or a failing measure after
  a successful agent leaves non-ignored untracked files
- **THEN** rollback removes those files

#### Scenario: Ignored files survive rollback

- **WHEN** a failing custom agent, a failing check, or a failing measure
  creates or modifies an ignored file
- **THEN** rollback does not delete that ignored file

#### Scenario: Existing workflow lane preserves history after failure

- **WHEN** an existing workflow lane has prior commits and its agent
  fails, or its check fails, or its measure gate fails after the agent
  succeeded
- **THEN** the lane is reset to its pre-attempt tip
- **AND** prior commits remain reachable
- **AND** later workflows may run after cleanup

#### Scenario: New workflow lane is removed after failure

- **WHEN** an agent fails, or a check fails, or a measure gate fails,
  during the first attempt on a new workflow lane
- **THEN** the pre-workflow branch and commit are restored
- **AND** only the newly created lane is deleted
- **AND** later workflows may run from the restored clean checkout

## ADDED Requirements

### Requirement: A measure gate captures a baseline before the agent and gates landing on improvement

For a custom attempt in branch mode where the workflow has a resolved
measure gate (see workflow-condition), `Watcher` SHALL, after the lane is
ensured and before invoking `agent.Run`, execute the measure command to
capture the baseline. On a baseline-measure failure, `Watcher` SHALL
trigger the branch-mode custom rollback, SHALL NOT invoke the agent, and
SHALL return the measure-failure error. On a successful baseline
capture, `Watcher` SHALL render `{metric}` into the prompt and commit
templates with the baseline value before invoking the agent. The
baseline value SHALL NOT be written to any path reachable from the
agent's working directory.

#### Scenario: Baseline is captured before the agent is invoked

- **WHEN** a workflow has a resolved measure gate and a polling pass
  reaches a successful condition
- **AND** the baseline measure writes `0.73`
- **THEN** `see` invokes the agent only after parsing `0.73` as the
  baseline
- **AND** the agent's prompt has `{metric}` replaced with `0.73`

#### Scenario: Baseline-measure failure skips the agent

- **WHEN** a workflow has a resolved measure gate and the baseline
  measure exits nonzero
- **THEN** the agent is not invoked
- **AND** the attempt returns a measure-failure error
- **AND** the branch-mode rollback runs

#### Scenario: A workflow without a measure invokes the agent unchanged

- **WHEN** a workflow has no resolved measure gate
- **THEN** no baseline is captured
- **AND** `{metric}` is not substituted
- **AND** the agent is invoked as before this requirement existed

### Requirement: MeasureFailed is the terminal event when the final attempt failed at the measure gate

When `Watcher.runOnce` retries a custom attempt and the final attempt's
error is a measure-failure error, `Watcher.runOnce` SHALL emit a
`MeasureFailed` terminal event for that repository on that pass instead
of a `ChangeFailed` or `CheckFailed` event. The selection SHALL be
determined by `errors.As` on the final error in this priority order:
`measureFailedError` → `MeasureFailed`; else `checkFailedError` →
`CheckFailed`; else `ChangeFailed`. Exactly one of `MeasureFailed`,
`CheckFailed`, or `ChangeFailed` SHALL appear per repository per polling
pass for the final failure, because a single attempt fails at exactly
one gate.

A measure failure SHALL be retried by `runWithRetry` like an agent or
check error: the agent receives up to `RetryCount` fresh attempts per
poll, each re-resolving the change, re-capturing the baseline, and
re-running from a clean slate. `RetryAttempt` events between attempts
SHALL carry the prior measure-failure summary unchanged.

#### Scenario: Final measure failure emits MeasureFailed

- **WHEN** the final attempt for a repo fails because the measure gate
  failed (non-improvement, a nonzero exit, or an unparseable value)
- **THEN** `runOnce` emits a `MeasureFailed` event for that repo
- **AND** does not emit `ChangeFailed` or `CheckFailed` for that final
  failure

#### Scenario: Measure, check, and agent failures are mutually exclusive per pass

- **WHEN** the final attempt for a repo fails
- **THEN** exactly one of `MeasureFailed`, `CheckFailed`, or
  `ChangeFailed` appears in the events for that repo on that pass
- **AND** the choice is determined by the type of the final error

#### Scenario: A measure failure is retried within the poll

- **WHEN** an attempt fails at the measure gate and `RetryCount` permits
  another attempt
- **THEN** the watcher re-resolves the change and re-runs the agent from
  a clean slate
- **AND** a `RetryAttempt` event carries the measure-failure summary
