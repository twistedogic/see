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
For each configured workflow and watched repository, `see` SHALL execute that workflow's predicate in the repository through the platform shell with the watcher context. Unix-like systems SHALL use `/bin/sh -c`; Windows SHALL use `cmd.exe /C`. Exit status `0` SHALL mean work is available, exit status `1` SHALL mean that workflow is idle, and any other nonzero status SHALL be a workflow condition error containing captured standard error when available.

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

#### Scenario: Windows trailing newline is removed
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
For an active workflow, `see` SHALL replace every literal `{change}` occurrence in that workflow's prompt and commit template with the normalized change. Unknown tokens SHALL remain literal, and rendered values SHALL be passed directly as process arguments without shell evaluation.

#### Scenario: Prompt and commit receive the same change
- **WHEN** the normalized change is `add-dark-mode`
- **AND** the prompt is `Apply {change}`
- **AND** the commit template is `see: apply {change}`
- **THEN** the agent receives `Apply add-dark-mode`
- **AND** the catch-up commit message is `see: apply add-dark-mode`

#### Scenario: Multiple tokens are replaced
- **WHEN** a template contains `{change}` more than once
- **THEN** every occurrence is replaced with the normalized change

#### Scenario: Unknown tokens remain literal
- **WHEN** a template contains `{repo}` or another token not named `{change}`
- **THEN** the unknown token remains unchanged

#### Scenario: Each workflow receives its own rendered templates
- **WHEN** the `openspec` workflow emits `add-dark-mode`
- **THEN** its agent prompt uses the `openspec` prompt with `{change}` replaced
- **AND** its catch-up commit uses the `openspec` commit template with `{change}` replaced
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

