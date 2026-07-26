## MODIFIED Requirements

### Requirement: Watcher selects custom work before the OpenSpec compatibility fallback
The strict configuration schema SHALL accept an optional `workflows` sequence. Each workflow SHALL be a mapping with nonblank, unique `name`, `prompt`, `condition`, and `commit` string fields. A configured workflow SHALL be evaluated independently for every watched repository. The former top-level `prompt`, `condition`, and `commit` fields SHALL be rejected as unknown fields.

When no `workflows` are configured, `see` SHALL retain OpenSpec compatibility behavior: active OpenSpec changes drive work, the embedded prompt is used when no configured prompt applies, archival determines completion, and the default OpenSpec catch-up commit subject is used.

#### Scenario: Configured condition selects custom mode
- **WHEN** configuration contains a nonblank condition
- **THEN** the watcher evaluates that condition in each watched repository
- **AND** it does not inspect `openspec/changes/` to select work

#### Scenario: Missing condition preserves OpenSpec behavior
- **WHEN** configuration omits `condition` or supplies a blank value
- **AND** a repository has an active OpenSpec change `add-dark-mode`
- **THEN** the watcher processes `add-dark-mode` using `see/add-dark-mode`
- **AND** it uses the existing OpenSpec prompt, archival completion check, rollback, and default commit message

#### Scenario: Missing condition with no OpenSpec change is idle
- **WHEN** configuration has no nonblank condition
- **AND** a repository has no active OpenSpec change
- **THEN** the agent is not invoked for that repository

#### Scenario: Complete workflow configuration loads
- **WHEN** configuration supplies two named workflows with nonblank prompt, condition, and commit values
- **THEN** startup succeeds
- **AND** both workflows are available in configuration order

#### Scenario: Duplicate workflow names are rejected
- **WHEN** two workflow entries have the same nonblank name
- **THEN** configuration loading fails
- **AND** the error identifies the duplicate workflow name

#### Scenario: Missing workflow field is rejected
- **WHEN** a workflow omits its prompt, condition, or commit
- **THEN** startup fails before watching
- **AND** the error identifies the workflow and missing field

#### Scenario: Legacy top-level custom fields are rejected
- **WHEN** configuration contains a top-level `condition`, `prompt`, or `commit` field
- **THEN** strict configuration loading fails
- **AND** the error identifies the unsupported field

#### Scenario: Empty workflow configuration preserves compatibility
- **WHEN** configuration omits `workflows`
- **THEN** OpenSpec compatibility discovery remains active
- **AND** no custom workflow condition is invoked

### Requirement: Watcher creates or resumes a persistent custom automation branch
For an active workflow change, `Watcher` SHALL use a branch named `see/<digest>`, where the digest is derived from the workflow name and normalized change. If the lane does not exist, it SHALL be created at the current commit. If it exists, the watcher SHALL switch to it only when the working tree is clean and SHALL resume its current tip without resetting prior commits. The watcher SHALL permit switching from another clean branch or workflow lane.

#### Scenario: First custom run creates its lane
- **WHEN** custom change `add-dark-mode` resolves to branch `see/<digest>` and that branch does not exist
- **THEN** the branch is created at the captured current commit before the agent runs
- **AND** the agent runs with `see/<digest>` checked out

#### Scenario: Repeated custom run resumes its lane
- **WHEN** `see/<digest>` is checked out with successful commits from an earlier pass
- **AND** the condition emits the same custom change again
- **THEN** the watcher runs the agent from the existing branch tip
- **AND** no earlier commit is reset or deleted

#### Scenario: Existing lane is not reset from another branch
- **WHEN** `see/<digest>` exists but the repository is checked out on `main`
- **AND** the condition resolves the change whose branch is `see/<digest>`
- **THEN** the watcher switches to the lane without resetting prior commits
- **AND** prior commits remain reachable
- **AND** the agent runs

#### Scenario: Distinct workflows have distinct lanes
- **WHEN** two workflows emit the same normalized change
- **THEN** they use different branch names
- **AND** work on one lane cannot reset or delete the other lane

#### Scenario: Clean checkout switches to an existing lane
- **WHEN** a requested workflow lane exists and another branch is checked out with a clean working tree
- **THEN** the watcher switches to the requested lane
- **AND** it preserves the lane's existing commits

#### Scenario: Dirty checkout blocks lane switching
- **WHEN** a requested workflow lane exists and the current checkout has tracked or non-ignored untracked changes
- **THEN** the workflow fails before branch mutation
- **AND** the changes remain unchanged

### Requirement: Watcher rolls back only the failed custom attempt
Immediately before invoking an agent, `Watcher` SHALL capture the selected workflow lane tip. If the agent fails on an existing lane, the watcher SHALL reset tracked state to that tip, remove non-ignored untracked files created by the attempt, preserve ignored files and earlier lane commits, and leave the clean lane available for subsequent workflows. If the lane was created by the attempt, the watcher SHALL return to the branch that was checked out before that workflow, restore its captured commit, and delete only the new lane.

#### Scenario: Failure on an existing lane preserves history
- **WHEN** an existing custom lane has commits `A`, `B`, and `C`
- **AND** the agent creates edits or commits after `C` and then fails
- **THEN** the lane tip is restored to `C`
- **AND** commits `A`, `B`, and `C` remain reachable from the lane
- **AND** the lane remains checked out
- **AND** the agent error is returned

#### Scenario: Failure removes newly-created lane
- **WHEN** a custom lane is created for the first time and the agent fails
- **THEN** the original branch and captured commit are restored
- **AND** the new custom lane is deleted
- **AND** the agent error is returned

#### Scenario: Agent-created untracked files are removed
- **WHEN** the custom working tree was clean before the agent ran
- **AND** the failing agent creates an untracked non-ignored file
- **THEN** rollback removes that file

#### Scenario: Ignored files survive rollback
- **WHEN** a failing custom agent creates or modifies an ignored file
- **THEN** rollback does not delete that ignored file

#### Scenario: Existing workflow lane preserves history after failure
- **WHEN** an existing workflow lane has prior commits and its agent fails after making changes
- **THEN** the lane is reset to its pre-attempt tip
- **AND** prior commits remain reachable
- **AND** later workflows may run after cleanup

#### Scenario: New workflow lane is removed after failure
- **WHEN** an agent fails during the first attempt on a new workflow lane
- **THEN** the pre-workflow branch and commit are restored
- **AND** only the newly created lane is deleted
- **AND** later workflows may run from the restored clean checkout

## ADDED Requirements

### Requirement: Watcher processes workflows independently in repository order
For each repository resolved by discovery, `Watcher` SHALL evaluate every configured workflow in configuration order. It SHALL process repositories in the stable order supplied by discovery. At most one agent session SHALL run at a time. A workflow condition that exits with status `1` SHALL skip only that workflow and processing SHALL continue with the next workflow.

A condition failure, agent failure, or catch-up failure SHALL be associated with the current workflow. After ordinary rollback, the watcher SHALL continue with later workflows for the same repository. If cleanup cannot restore a safe clean checkout, the watcher SHALL stop processing that repository and SHALL NOT invoke another agent in it.

#### Scenario: Every workflow is evaluated for one repository
- **WHEN** a repository has two configured workflows
- **THEN** the first workflow is evaluated before the second
- **AND** the second is evaluated even when the first workflow is idle

#### Scenario: Active workflows run sequentially
- **WHEN** both workflow conditions report active work
- **THEN** the first workflow's agent session completes before the second workflow's agent session starts
- **AND** no concurrent agent sessions are created

#### Scenario: Failed workflow does not block later workflow
- **WHEN** the first active workflow's agent fails and rollback restores a clean checkout
- **THEN** the failure is reported for the first workflow
- **AND** the next workflow is evaluated and may run

#### Scenario: Unsafe cleanup stops the repository
- **WHEN** a failed workflow cannot restore a clean safe checkout
- **THEN** the watcher reports the cleanup failure
- **AND** it does not invoke another workflow for that repository

### Requirement: Watcher leaves the final usable workflow lane checked out
After all workflows for a repository are processed, `Watcher` SHALL leave the most recently usable active workflow lane checked out. If no workflow was active, it SHALL leave the branch that was checked out when repository processing began. A successful workflow SHALL not merge its lane into the starting branch.

#### Scenario: Final active workflow lane remains checked out
- **WHEN** two workflows run successfully for one repository
- **THEN** the second workflow's lane is checked out when repository processing ends
- **AND** both workflow lanes retain their commits

#### Scenario: No active workflow preserves the starting branch
- **WHEN** all workflow conditions exit with status `1`
- **THEN** no workflow lane is created or switched to
- **AND** the starting branch remains checked out

#### Scenario: A failed final workflow leaves a safe usable checkout
- **WHEN** the final active workflow fails and rollback succeeds
- **THEN** the cleaned workflow lane remains available as the final lane when it existed before the attempt
- **AND** no later workflow remains to run
