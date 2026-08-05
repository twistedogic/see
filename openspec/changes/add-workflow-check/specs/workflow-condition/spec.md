# workflow-condition delta — add-workflow-check

## ADDED Requirements

### Requirement: A workflow MAY define an optional check gate

Each workflow entry, whether supplied through `config.yaml` `workflows:`
or through the frontmatter of a `.md` file in `workflows_dir`, SHALL
accept an optional `check` string field. An absent or blank `check`
SHALL mean the workflow has no check gate, identical to behavior before
this field existed. A present `check` SHALL, after trimming whitespace,
contain at least one non-whitespace character; an empty or
whitespace-only `check` SHALL fail startup with an actionable error
naming the workflow and the field.

When a workflow defines a `check` and a polling pass reaches a
successful agent run that left working-tree changes to land, `see` SHALL
execute the rendered `check` command through the platform shell under
the same contract as the workflow `condition`: `/bin/sh -c` on Unix-like
systems and `cmd.exe /C` on Windows, with the watcher context attached,
in its own process group on Unix so cancellation does not strand
descendants. The command SHALL run with its working directory set to the
agent's working directory (the lane checkout in branch mode, the
worktree directory in worktree mode) so it observes the agent's
uncommitted output.

Exit status `0` SHALL mean the check passed and landing SHALL proceed.
Any nonzero exit status SHALL mean the check failed; `see` SHALL capture
standard error when available. A failed check SHALL NOT create any
commit and SHALL trigger the active lane-isolation mode's rollback to a
clean slate (see lane-isolation and watcher), discarding the attempt's
working-tree changes, and SHALL return a check-failure error.

The check SHALL be skipped when the successful agent run left no
working-tree changes to land (the agent committed all work itself or
changed nothing), preserving the existing warning-free idempotent
no-op. The check SHALL NOT run when the agent run itself failed; the
existing agent-failure rollback applies unchanged.

The literal token `{change}` SHALL be substituted in the `check`
template under the same rule as `prompt`, `condition`, and `commit`;
unknown tokens SHALL remain literal.

#### Scenario: A workflow without a check behaves as before

- **WHEN** a workflow entry omits `check` or supplies a blank value
- **THEN** the workflow runs and lands work exactly as before this field
  existed
- **AND** no check command is ever executed for that workflow

#### Scenario: A present blank check is rejected at startup

- **WHEN** a workflow entry sets `check` to an empty string or
  whitespace only
- **THEN** `see` exits with status `2` before watching
- **AND** the error identifies the workflow name and the `check` field

#### Scenario: Passing check proceeds to landing

- **WHEN** a workflow defines `check: go test ./...`
- **AND** the agent succeeds and leaves working-tree changes
- **AND** the check exits with status `0`
- **THEN** the active mode lands the work (catch-up commit / rebase +
  merge) exactly as without a check

#### Scenario: Failing check rolls back to a clean slate and creates no commit

- **WHEN** a workflow defines `check` and the check exits nonzero after
  a successful agent run
- **THEN** no catch-up commit is created for the attempt
- **AND** the attempt's working-tree changes are discarded by the
  mode's rollback
- **AND** the attempt returns a check-failure error

#### Scenario: Check runs in the agent's working directory

- **WHEN** worktree mode is active and the agent ran in the worktree
  directory
- **AND** the workflow defines a check
- **THEN** the check runs with its working directory set to the worktree
  directory
- **AND** it observes the agent's uncommitted output there

#### Scenario: Check is skipped on an idempotent no-op run

- **WHEN** a workflow defines `check`
- **AND** the agent succeeds without leaving working-tree changes
- **AND** no catch-up commit would be created
- **THEN** the check is not executed
- **AND** the run is a warning-free no-op as before

#### Scenario: Check is not run when the agent fails

- **WHEN** the agent run returns a non-nil error
- **THEN** the check is not executed
- **AND** the existing agent-failure rollback applies

#### Scenario: Cancellation stops the check

- **WHEN** the watcher context is cancelled while a check is running
- **THEN** the shell process is cancelled
- **AND** the watcher terminates according to its existing
  context-cancellation contract

#### Scenario: Check stderr is captured into the failure

- **WHEN** the check exits nonzero and writes `build failed` to standard
  error
- **THEN** the check-failure error and the emitted event carry
  `build failed`

## MODIFIED Requirements

### Requirement: Change token renders prompt, commit, and check templates

For an active workflow, `see` SHALL replace every literal `{change}`
occurrence in that workflow's `prompt`, `commit`, and `check` (when
present) templates with the normalized change. Unknown tokens SHALL
remain literal, and rendered values SHALL be passed directly as process
arguments without shell evaluation.

#### Scenario: Prompt, commit, and check receive the same change

- **WHEN** the normalized change is `add-dark-mode`
- **AND** the prompt is `Apply {change}`
- **AND** the commit template is `see: apply {change}`
- **AND** the check is `openspec validate {change}`
- **THEN** the agent receives `Apply add-dark-mode`
- **AND** the catch-up commit message is `see: apply add-dark-mode`
- **AND** the check command run is `openspec validate add-dark-mode`

#### Scenario: Multiple tokens are replaced

- **WHEN** a template contains `{change}` more than once
- **THEN** every occurrence is replaced with the normalized change

#### Scenario: Unknown tokens remain literal

- **WHEN** a template contains `{repo}` or another token not named
  `{change}`
- **THEN** the unknown token remains unchanged

#### Scenario: Each workflow receives its own rendered templates

- **WHEN** the `openspec` workflow emits `add-dark-mode`
- **THEN** its agent prompt uses the `openspec` prompt with `{change}`
  replaced
- **AND** its catch-up commit uses the `openspec` commit template with
  `{change}` replaced
- **AND** its check (if any) uses the `openspec` check template with
  `{change}` replaced
- **AND** the `update` templates are not used
