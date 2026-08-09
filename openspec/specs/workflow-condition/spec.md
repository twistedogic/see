# workflow-condition

## Purpose

Define how custom workflow configuration resolves one work item, derives its stable identity, renders templates, and repeats work while its condition remains true.
## Requirements
### Requirement: Configuration selects a custom workflow
The strict configuration schema SHALL accept an optional sequence of named workflow mappings under `workflows`. Each workflow SHALL define nonblank `name`, `prompt`, `condition`, and `commit` strings. A workflow condition containing at least one non-whitespace character SHALL select that workflow for evaluation; the workflow's prompt and commit SHALL be used only for that workflow. The old top-level custom fields SHALL no longer be accepted.

#### Scenario: Complete custom workflow configuration starts
- **WHEN** configuration supplies nonblank `condition`, `prompt`, and `commit` values
- **THEN** startup succeeds
- **AND** watched repositories use custom workflow mode

#### Scenario: Command-line prompt completes custom configuration
- **WHEN** configuration supplies nonblank `condition` and `commit` values but no prompt
- **AND** `--prompt` supplies a nonblank value
- **THEN** startup succeeds
- **AND** the command-line prompt is the custom prompt template

#### Scenario: Custom mode rejects a missing prompt
- **WHEN** configuration supplies a nonblank `condition` and `commit` but neither `--prompt` nor configuration supplies a nonblank prompt
- **THEN** `see` exits with status `2` before watching
- **AND** the error identifies the missing custom prompt

#### Scenario: Custom mode rejects a missing commit template
- **WHEN** configuration supplies a nonblank `condition` and an effective custom prompt but `commit` is blank or absent
- **THEN** `see` exits with status `2` before watching
- **AND** the error identifies the missing custom commit template

#### Scenario: Workflow fields are independent
- **WHEN** configuration contains `openspec` and `update` workflows with different prompt, condition, and commit values
- **THEN** each condition is evaluated with its own workflow settings
- **AND** one workflow's template does not affect the other

#### Scenario: Blank required workflow field is rejected
- **WHEN** a workflow has a blank prompt, condition, or commit
- **THEN** startup fails
- **AND** the error identifies the workflow name and field

### Requirement: Custom condition resolves one change through the platform shell
For each configured workflow and watched repository, `see` SHALL execute that workflow's predicate in the repository through `/bin/sh -c` with the watcher context. Exit status `0` SHALL mean work is available, exit status `1` SHALL mean that workflow is idle, and any other nonzero status SHALL be a workflow condition error containing captured standard error when available.

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

### Requirement: Normalized change determines custom branch and log identity
`see` SHALL compute the full lowercase Secure Hash Algorithm 256-bit (SHA-256) digest of `workflow.name + "\x00" + normalizedChange` and use `see/<digest>` as the workflow lane. Per-agent log filenames SHALL use the same digest. The same workflow name and normalized change SHALL produce the same identity across polling passes and process restarts, while different workflow names SHALL produce different identities even for equal change values.

#### Scenario: Repeated change selects the same identity
- **WHEN** a condition emits `add-dark-mode` on two polling passes
- **THEN** both passes select the same `see/<digest>` branch
- **AND** both per-agent log filenames use the same digest component

#### Scenario: Repeated change selects the same identity
- **WHEN** a condition emits `add-dark-mode` on two polling passes
- **THEN** both passes select the same `see/<digest>` branch
- **AND** both per-agent log filenames use the same digest component

#### Scenario: Different change selects a different identity
- **WHEN** one pass emits `add-dark-mode` and a later pass emits `fix-cache`
- **THEN** the two changes select different automation branches

#### Scenario: Unsafe filename characters remain data
- **WHEN** a successful condition emits a value containing spaces or path traversal characters but no line break
- **THEN** the value is passed unchanged to template rendering
- **AND** branch and log paths contain only the hexadecimal digest rather than the raw value

#### Scenario: Same change in different workflows is isolated
- **WHEN** `openspec` and `update` both emit `package-update`
- **THEN** their lane digests differ
- **AND** their per-agent log identities differ

#### Scenario: Repeated workflow change resumes its lane
- **WHEN** the same workflow emits the same normalized change on a later pass
- **THEN** it selects the same persistent lane
- **AND** prior commits on that lane remain available

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

### Requirement: Workflows may be supplied as Markdown files
A workflow MAY be supplied as a single `.md` file under `workflows_dir` instead of as an entry in `config.yaml` `workflows:`. The workflow's `name` SHALL be the filename without its `.md` extension. The file SHALL consist of YAML frontmatter (between two `---` lines) plus a Markdown body; the frontmatter SHALL carry `condition`, `commit`, and an optional `model`, and the body SHALL be the workflow's prompt. Workflows supplied as files SHALL participate in the same validation, ordering, and identity rules as workflows supplied through `config.yaml`. See [workflow-files](../workflow-files/spec.md) for the complete discovery and parsing contract.

#### Scenario: A single workflow file is equivalent to a config.yaml entry
- **WHEN** `workflows_dir` contains `openspec.md` with a valid frontmatter and body
- **THEN** startup succeeds
- **AND** `openspec` is one workflow in the merged set
- **AND** its `prompt`, `condition`, `commit`, and `model` come from the file
- **AND** its evaluation order matches its filename's alphabetical position

#### Scenario: A workflow file with a conflicting config.yaml entry is rejected
- **WHEN** `workflows_dir` contains `openspec.md`
- **AND** `config.yaml` `workflows:` contains an entry with `name: openspec`
- **THEN** `see` exits with status `2` before watching
- **AND** the error names the file path and the `workflows:` index

#### Scenario: A frontmatter name does not rename the workflow
- **WHEN** `workflows_dir` contains `openspec.md` with frontmatter `name: apply-openspec`
- **THEN** the workflow's effective name is `openspec`, the filename
- **AND** the workflow's lane digest is derived from `openspec`, not from `apply-openspec`

#### Scenario: A workflow file's body becomes its prompt
- **WHEN** `workflows_dir` contains `openspec.md` whose body is `Apply {change} and verify.`
- **AND** the normalized change is `add-dark-mode`
- **THEN** the agent receives `Apply add-dark-mode and verify.` for that workflow

#### Scenario: A workflow file with a blank body is rejected
- **WHEN** `workflows_dir` contains `openspec.md` whose body is empty or whitespace-only
- **THEN** `see` exits with status `2` before watching
- **AND** the error names the file path and identifies the missing prompt body

#### Scenario: A workflow file with an unknown frontmatter key is rejected
- **WHEN** `workflows_dir` contains `openspec.md` whose frontmatter has a key other than `name`, `condition`, `commit`, or `model`
- **THEN** `see` exits with status `2` before watching
- **AND** the error names the file path and the unknown key

#### Scenario: The config.yaml workflows block remains accepted
- **WHEN** configuration contains `workflows:` with one entry
- **AND** `workflows_dir` is absent, missing, or empty
- **THEN** startup succeeds
- **AND** the `workflows:` entry runs as the only workflow
- **AND** no `.md` workflow files are required

### Requirement: Custom conditions are level-triggered
`see` SHALL evaluate the custom condition again on every polling pass. Every pass on which it exits with status `0` and emits a valid change SHALL invoke the agent, including when the normalized change is identical to the previous pass. `see` SHALL NOT persist false-to-true edge state or treat a prior successful run as completion while the condition remains true.

#### Scenario: True condition repeats work
- **WHEN** the condition emits `add-dark-mode` on two consecutive polling passes
- **THEN** the agent is invoked once on each pass
- **AND** both invocations use the same persistent automation branch

#### Scenario: Condition becomes false
- **WHEN** a condition emits a valid change on one pass and exits with status `1` on the next
- **THEN** the first pass invokes the agent
- **AND** the second pass leaves the repository idle without invoking the agent

#### Scenario: Retry re-resolves the condition
- **WHEN** a custom attempt fails and the retry count permits another attempt
- **THEN** the watcher executes the condition again before the retry
- **AND** exit status `1` makes that retry a successful idle no-op
- **AND** a different valid stdout value selects the branch, prompt, and commit message for that newly resolved change

### Requirement: Workflow MAY select a model passed to `pi` as `--model`
A workflow entry SHALL accept an optional `model` string. When the trimmed value is nonblank, `see` SHALL pass `--model <model>` to the `pi` invocation for that workflow's runs. When the value is absent, blank, or whitespace-only, `see` SHALL NOT pass `--model` to `pi` and the agent's default model SHALL apply unchanged. The OpenSpec-compatibility path, which does not use a configured workflow, SHALL continue to invoke `pi` without `--model` and is unaffected by the new field.

The value SHALL be passed verbatim to `pi`; `see` SHALL NOT validate it against the agent's model catalog or otherwise interpret it. The model SHALL NOT be incorporated into the workflow's stable identity, branch name, log filename, commit subject, or event payload.

#### Scenario: Configured model reaches `pi`
- **WHEN** a workflow entry defines `model: openai/gpt-5-mini`
- **AND** that workflow's condition exits with status `0`
- **THEN** `see` invokes `pi` with `--mode json --no-session --model openai/gpt-5-mini <prompt>` for that run
- **AND** the workflow's persistent lane, log filename, and commit subject are unchanged

#### Scenario: Blank model does not reach `pi`
- **WHEN** a workflow entry has no `model` field, an empty `model`, or a whitespace-only `model`
- **AND** that workflow's condition exits with status `0`
- **THEN** `see` invokes `pi` with `--mode json --no-session <prompt>` (no `--model` argument)
- **AND** the run is behaviorally identical to the pre-change invocation

#### Scenario: OpenSpec-compatibility is unaffected
- **WHEN** configuration defines no `workflows` sequence
- **AND** a repository has an active OpenSpec change
- **THEN** `see` invokes `pi` without `--model` regardless of any `model` field elsewhere in the configuration

### Requirement: A workflow may be disabled and is filtered out after validation

Each workflow entry, whether supplied through `config.yaml` `workflows:` or through the frontmatter of a `.md` file in `workflows_dir`, SHALL accept an optional `disable` boolean field. A workflow whose `disable` field is `true` SHALL be removed from the evaluated workflow list during configuration load. An absent or `false` `disable` field SHALL leave the workflow enabled, identical to the behavior before this field existed.

Removal SHALL happen as the last step of configuration load, after the file/config merge and after the existing `workflows:` validation. A disabled entry SHALL still be subject to the full validation contract: nonblank `name`, `prompt`, `condition`, and `commit`, and a unique `name` within the merged list, evaluated over the full list before any entry is removed. A disabled entry SHALL NOT reach the run loop, the Terminal User Interface (TUI), or the event stream; no skip branch, event, or rendering change SHALL be introduced downstream.

The disabled entry's fields SHALL NOT be interpreted beyond loading and validation: its `condition`, `prompt`, `commit`, and `model` SHALL NOT be evaluated or passed to the agent while `disable` is `true`.

#### Scenario: A disabled workflow does not run

- **WHEN** configuration contains one workflow with `disable: true` and a second enabled workflow
- **AND** a watched repository makes the enabled workflow's condition exit with status `0`
- **THEN** the disabled workflow's condition is never executed
- **AND** only the enabled workflow is evaluated for that repository

#### Scenario: A disabled workflow is still fully validated

- **WHEN** configuration contains a workflow with `disable: true` and a blank `condition`
- **THEN** `see` exits with status `2` before watching
- **AND** the error identifies the workflow name and the missing field

#### Scenario: A disabled duplicate name is still rejected

- **WHEN** the merged workflow list contains two entries named `openspec`, one of which sets `disable: true`
- **THEN** `see` exits with status `2` before watching
- **AND** the error reports the duplicate workflow name
- **AND** no filtering occurs because validation failed first

#### Scenario: An absent disable field means enabled

- **WHEN** configuration contains a workflow entry with no `disable` field
- **THEN** that workflow is enabled and is evaluated normally

#### Scenario: Disabling every workflow reverts to OpenSpec compatibility mode

- **WHEN** the merged workflow list is non-empty and every entry sets `disable: true`
- **THEN** the evaluated workflow list is empty after filtering
- **AND** the watcher runs in OpenSpec compatibility mode
- **AND** a repository with an active `openspec/changes/` entry is processed by the OpenSpec resolver

#### Scenario: A disabled workflow is invisible to the run loop and events

- **WHEN** configuration contains a workflow with `disable: true`
- **AND** a watched repository is processed
- **THEN** the disabled workflow is absent from the evaluated list
- **AND** no event names the disabled workflow
- **AND** no branch named `see/<digest>` is created for the disabled workflow

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
execute the rendered `check` command through `/bin/sh -c` under
the same contract as the workflow `condition`, with the watcher context
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

