# workflow-condition delta — add-workflow-measure

## MODIFIED Requirements

### Requirement: Change token renders prompt and commit templates

For an active workflow, `see` SHALL replace every literal `{change}`
occurrence in that workflow's `prompt`, `commit`, `check`, and
`measure` (when present) templates with the normalized change. Unknown
tokens SHALL remain literal, and rendered values SHALL be passed
directly as process arguments without shell evaluation.

#### Scenario: Prompt and commit receive the same change

- **WHEN** the normalized change is `add-dark-mode`
- **AND** the prompt is `Apply {change}`
- **AND** the commit template is `see: apply {change}`
- **AND** the check is `openspec validate {change}`
- **AND** the measure is `./bench.sh {change}`
- **THEN** the agent receives `Apply add-dark-mode`
- **AND** the catch-up commit message is `see: apply add-dark-mode`
- **AND** the check command run is `openspec validate add-dark-mode`
- **AND** the measure command run is `./bench.sh add-dark-mode`

#### Scenario: Multiple tokens are replaced

- **WHEN** a template contains `{change}` more than once
- **THEN** every occurrence is replaced with the normalized change

#### Scenario: Unknown tokens remain literal

- **WHEN** a template contains `{repo}` or another token not named
  `{change}` or `{metric}`
- **THEN** the unknown token remains unchanged

#### Scenario: Each workflow receives its own rendered templates

- **WHEN** the `openspec` workflow emits `add-dark-mode`
- **THEN** its agent prompt uses the `openspec` prompt with `{change}`
  replaced
- **AND** its catch-up commit uses the `openspec` commit template with
  `{change}` replaced
- **AND** its check (if any) uses the `openspec` check template with
  `{change}` replaced
- **AND** its measure (if any) uses the `openspec` measure template with
  `{change}` replaced
- **AND** the `update` templates are not used

## ADDED Requirements

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

The measure command SHALL run through the platform shell under the same
contract as the workflow `condition` and `check`: `/bin/sh -c` on
Unix-like systems and `cmd.exe /C` on Windows, with the watcher context
attached, in its own process group on Unix so cancellation does not
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

### Requirement: `{metric}` token renders the baseline metric into prompt and commit

When a workflow has a resolved measure gate for an attempt, `see` SHALL
replace every literal `{metric}` occurrence in that workflow's `prompt`
and `commit` templates with the normalized baseline value — the same
string that was parsed as a 64-bit floating-point number for the
comparison. Rendered values SHALL be passed directly as process
arguments or commit-message text without shell evaluation. The
`measure` command template SHALL NOT receive `{metric}` substitution.

When no measure gate is resolved for a workflow, the `{metric}` token
SHALL remain literal in that workflow's `prompt` and `commit` templates,
identical to any other unknown token. `{metric}` SHALL NOT be
substituted in `condition`, `check`, or `measure`.

#### Scenario: The prompt receives the baseline metric

- **WHEN** a workflow's prompt is `Beat {metric}. Current change: {change}.`
- **AND** the resolved baseline is `0.73` and the normalized change is
  `tune-bench`
- **THEN** the agent receives `Beat 0.73. Current change: tune-bench.`

#### Scenario: The commit template receives the baseline metric

- **WHEN** a workflow's commit template is `autoresearch: improve {metric}`
- **AND** the resolved baseline is `0.73`
- **THEN** the catch-up commit message is `autoresearch: improve 0.73`

#### Scenario: {metric} is literal when no measure gate is resolved

- **WHEN** a workflow has no `measure` field and no convention script
- **AND** the workflow's prompt is `Beat {metric}.`
- **THEN** the agent receives `Beat {metric}.` unchanged

#### Scenario: The measure command does not consume {metric}

- **WHEN** a workflow's measure is `./bench.sh {metric}`
- **AND** the baseline is `0.73`
- **THEN** the measure command run is `./bench.sh {metric}` (literal)
