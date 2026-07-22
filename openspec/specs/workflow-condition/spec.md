# workflow-condition

## Purpose

Define how custom workflow configuration resolves one work item, derives its stable identity, renders templates, and repeats work while its condition remains true.

## Requirements

### Requirement: Configuration selects a custom workflow
The strict configuration schema SHALL accept optional `condition` and `commit` string fields. A `condition` containing at least one non-whitespace character SHALL select custom workflow mode for every watched repository. In custom mode, `see` SHALL require both a nonblank effective prompt and a nonblank `commit` template before the watcher starts. The effective prompt SHALL retain the existing precedence of nonblank `--prompt`, then nonblank configured `prompt`.

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

### Requirement: Custom condition resolves one change through the platform shell
For each watched repository in custom mode, `see` SHALL execute the configured condition in that repository through the platform shell with the watcher's context. Unix-like systems SHALL use `/bin/sh -c`; Windows SHALL use `cmd.exe /C`. Exit status `0` SHALL mean work is available, exit status `1` SHALL mean no work is available, and a shell launch failure or any other nonzero exit status SHALL be a condition error. A condition error SHALL retain captured standard error in its diagnostic when standard error is nonempty.

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
`see` SHALL compute the Secure Hash Algorithm 256-bit (SHA-256) digest of the normalized change bytes, encode the full digest as lowercase hexadecimal, and use `see/<digest>` as the custom automation branch. The per-agent log filename SHALL use the same digest instead of raw condition output. The same normalized change SHALL always produce the same identity, including across polling passes and process restarts.

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

### Requirement: Change token renders prompt and commit templates
For a custom change, `see` SHALL replace every literal `{change}` occurrence in both the selected prompt template and configured commit template with the normalized change. Unknown tokens SHALL remain literal. Rendering SHALL pass the resulting strings directly as process arguments rather than evaluating either rendered template through a shell.

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
