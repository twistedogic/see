## MODIFIED Requirements

### Requirement: Custom condition resolves one change through the platform shell
For each configured workflow and watched repository, `see` SHALL execute that
workflow's predicate in the repository through `/bin/sh -c` with the watcher
context. Exit status `0` SHALL mean work is available, exit status `1` SHALL
mean that workflow is idle, and any other nonzero status SHALL be a workflow
condition error containing captured standard error when available.

#### Scenario: Condition succeeds with work
- **WHEN** the condition exits with status `0` and writes `add-dark-mode` to standard output
- **THEN** the repository has an active custom change
- **AND** its change value is derived from `add-dark-mode`

#### Scenario: Condition reports no work
- **WHEN** the condition exits with status `1`
- **THEN** the repository is idle for that polling pass
- **AND** the agent is not invoked

#### Scenario: Condition reports a failure
- **WHEN** the condition exits with status `2` and writes `syntax error` to standard error
- **THEN** the polling attempt fails with a condition error containing `syntax error`
- **AND** no custom branch is created
- **AND** the agent is not invoked

#### Scenario: Cancellation stops the condition
- **WHEN** the watcher context is cancelled while a condition is running
- **THEN** the shell process is cancelled
- **AND** the watcher terminates according to its existing context-cancellation contract

#### Scenario: One workflow is idle while another is active
- **WHEN** the first workflow condition exits with status `1` and the second exits with status `0`
- **THEN** the first workflow is skipped
- **AND** the second workflow's change is processed

#### Scenario: Condition failure is workflow-scoped
- **WHEN** one workflow condition exits with status `2`
- **THEN** that workflow reports a condition error
- **AND** later workflows are still evaluated if the checkout remains safe

### Requirement: Condition stdout is normalized into one change value
After a condition exits with status `0`, `see` SHALL remove trailing carriage-return and line-feed characters from its standard output. The remaining value SHALL contain at least one non-whitespace character and SHALL NOT contain another carriage return or line feed. The resulting single-line value, including any non-newline leading or trailing whitespace, SHALL be the normalized change used by every downstream custom-workflow operation.

#### Scenario: Conventional trailing newline is removed
- **WHEN** a successful condition writes `add-dark-mode\n`
- **THEN** the normalized change is exactly `add-dark-mode`

#### Scenario: Trailing carriage return is removed
- **WHEN** a successful condition writes `add-dark-mode\r\n`
- **THEN** the normalized change is exactly `add-dark-mode`

#### Scenario: Empty or whitespace-only successful output is rejected
- **WHEN** a condition exits with status `0` and writes no standard output or only whitespace
- **THEN** the attempt fails with an actionable empty-change error
- **AND** no branch is created
- **AND** the agent is not invoked

#### Scenario: Multiline output is rejected
- **WHEN** a successful condition writes two nonempty lines
- **THEN** the attempt fails with an actionable single-line requirement error
- **AND** no branch is created
- **AND** the agent is not invoked

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
execute the rendered `check` command through `/bin/sh -c` under the
same contract as the workflow `condition`, with the watcher context
attached, in its own process group so cancellation does not strand
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

### Requirement: A workflow MAY define an optional measure gate

Each workflow entry, whether supplied through `config.yaml` `workflows:`
or through the frontmatter of a `.md` file in `workflows_dir`, SHALL
accept an optional `measure` string field. An absent `measure` with no
resolvable convention script SHALL mean the workflow has no measure
gate, identical to behavior before this field existed. A present
`measure` SHALL, after trimming whitespace, contain at least one
non-whitespace character; an empty or whitespace-only `measure` SHALL
fail startup with an actionable error naming the workflow and the field.

The resolved measure command SHALL be selected in this order:

1. The workflow's nonblank `measure` field (frontmatter or `config.yaml`
   value).
2. Otherwise, the regular file at
   `~/.config/see/measure/<workflow-name>.sh`, if it exists; `see` SHALL
   read its contents and execute them as the measure command. A missing
   convention directory or a missing `<workflow-name>.sh` SHALL mean the
   workflow has no measure gate and SHALL NOT be an error.

The measure command SHALL run through `/bin/sh -c` under the same
contract as the workflow `condition` and `check`, with the watcher
context attached, in its own process group so cancellation does not
strand descendants. The command SHALL run with its working directory set
to the agent's working directory (the lane checkout in branch mode, the
worktree directory in worktree mode). The literal token `{change}` SHALL
be substituted in the `measure` template under the same rule as
`prompt`, `condition`, `commit`, and `check`; the token `{metric}` SHALL
NOT be substituted in `measure` (it is the producer of `{metric}`, not a
consumer) and SHALL remain literal if present.

For an attempt where a measure gate is resolved, `see` SHALL run the
measure command once **before** the agent to capture the baseline and
once **after** a passing `check` — or after the agent when no `check` is
defined — to capture the candidate. Both the baseline and candidate
values SHALL be held in `see`'s memory and SHALL NOT be written to any
path reachable from the agent's working directory. A baseline-measure
failure SHALL abort the attempt before the agent is invoked.

Each measure value SHALL be normalized exactly like a condition value:
trailing carriage-return and line-feed characters removed, the remaining
value required to be single-line and to contain at least one
non-whitespace character. The normalized value SHALL be parsed as a
64-bit floating-point number (`strconv.ParseFloat` with bit size 64);
higher values are better.

The landing decision for the attempt SHALL be: stage and land the
agent's working-tree changes (subject to the existing catch-up commit
rules) when the candidate is strictly greater than the baseline. A
candidate equal to or less than the baseline, a measure command that
exits with a nonzero status, an empty or whitespace-only output, a
multi-line output, or an output that cannot be parsed as a number SHALL
constitute a measure failure. A measure failure SHALL NOT create any
commit and SHALL trigger the active lane-isolation mode's rollback to a
clean slate (see lane-isolation and watcher), discarding the attempt's
working-tree changes, and SHALL return a measure-failure error carrying
the rendered command, the exit code when available, the baseline and
candidate values when available, and the captured standard error when
available.

A workflow without a measure gate SHALL run exactly as before this field
existed: no baseline is captured, `{metric}` is not substituted, and the
landing decision ignores any metric.

#### Scenario: A workflow without a measure behaves as before

- **WHEN** a workflow entry omits `measure` and no
  `~/.config/see/measure/<workflow-name>.sh` exists
- **THEN** the workflow runs and lands work exactly as before this field
  existed
- **AND** no measure command is ever executed for that workflow
- **AND** `{metric}` is not substituted in that workflow's prompt or
  commit

#### Scenario: A present blank measure is rejected at startup

- **WHEN** a workflow entry sets `measure` to an empty string or
  whitespace only
- **THEN** `see` exits with status `2` before watching
- **AND** the error identifies the workflow name and the `measure` field

#### Scenario: A frontmatter measure overrides the convention file

- **WHEN** a workflow's frontmatter sets `measure: ./bench.sh`
- **AND** `~/.config/see/measure/<workflow-name>.sh` also exists
- **THEN** the resolved measure command is `./bench.sh`
- **AND** the convention file is not read

#### Scenario: A missing frontmatter measure falls back to the convention file

- **WHEN** a workflow's frontmatter omits `measure`
- **AND** `~/.config/see/measure/<workflow-name>.sh` exists
- **THEN** the contents of that file are executed as the measure command
- **AND** the command runs with its working directory set to the agent's
  working directory

#### Scenario: No frontmatter measure and no convention file means no gate

- **WHEN** a workflow omits `measure`
- **AND** no `~/.config/see/measure/<workflow-name>.sh` exists
- **THEN** the workflow has no measure gate
- **AND** startup succeeds

#### Scenario: Baseline is captured before the agent and held in memory

- **WHEN** a workflow defines a measure and the baseline measure writes
  `0.73` to standard output
- **THEN** `see` parses `0.73` as the baseline before invoking the agent
- **AND** the value is not written to any path under the agent's working
  directory

#### Scenario: A baseline-measure failure aborts before the agent

- **WHEN** a workflow defines a measure and the baseline measure exits
  nonzero
- **THEN** the agent is not invoked for that attempt
- **AND** the attempt returns a measure-failure error
- **AND** the active mode rolls back to a clean slate

#### Scenario: Improvement lands the agent's changes

- **WHEN** the baseline is `0.73`, the candidate is `0.79`, and the
  working tree is dirty after a passing check
- **THEN** the active mode stages and commits the agent's changes
- **AND** the catch-up commit message is rendered with `{metric}` set to
  the baseline

#### Scenario: Non-improvement rolls back and creates no commit

- **WHEN** the candidate is not strictly greater than the baseline
- **THEN** no catch-up commit is created for the attempt
- **AND** the attempt's working-tree changes are discarded by the mode's
  rollback
- **AND** the attempt returns a measure-failure error

#### Scenario: An unparseable measure output is a measure failure

- **WHEN** the measure exits with status `0` and writes `ok` to standard
  output
- **THEN** the attempt fails with a measure-failure error identifying
  the unparseable value
- **AND** no commit is created

#### Scenario: Measure runs in the agent's working directory

- **WHEN** worktree mode is active and the agent ran in the worktree
  directory
- **AND** the workflow defines a measure
- **THEN** both the baseline and candidate measure run with their working
  directory set to the worktree directory

#### Scenario: Measure is not run when the agent fails

- **WHEN** the agent run returns a non-nil error
- **THEN** the candidate measure is not executed
- **AND** the existing agent-failure rollback applies

#### Scenario: A failed check short-circuits the candidate measure

- **WHEN** a workflow defines both `check` and `measure`
- **AND** the check exits nonzero after a successful agent run
- **THEN** the candidate measure is not executed
- **AND** the attempt returns a check-failure error

#### Scenario: Cancellation stops the measure

- **WHEN** the watcher context is cancelled while a measure is running
- **THEN** the shell process is cancelled
- **AND** the watcher terminates according to its existing
  context-cancellation contract

#### Scenario: Measure stderr is captured into the failure

- **WHEN** the measure exits nonzero and writes `benchmark crashed` to
  standard error
- **THEN** the measure-failure error and the emitted event carry
  `benchmark crashed`
