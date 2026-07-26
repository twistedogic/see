## MODIFIED Requirements

### Requirement: Configuration selects a custom workflow
The strict configuration schema SHALL accept an optional sequence of named workflow mappings under `workflows`. Each workflow SHALL define nonblank `name`, `prompt`, `condition`, and `commit` strings. A workflow condition containing at least one non-whitespace character SHALL select that workflow for evaluation; the workflow's prompt and commit SHALL be used only for that workflow. The old top-level custom fields SHALL no longer be accepted.

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

#### Scenario: One workflow is idle while another is active
- **WHEN** the first workflow condition exits with status `1` and the second exits with status `0`
- **THEN** the first workflow is skipped
- **AND** the second workflow's change is processed

#### Scenario: Condition failure is workflow-scoped
- **WHEN** one workflow condition exits with status `2`
- **THEN** that workflow reports a condition error
- **AND** later workflows are still evaluated if the checkout remains safe

### Requirement: Normalized change determines workflow branch and log identity
`see` SHALL compute the full lowercase Secure Hash Algorithm 256-bit (SHA-256) digest of `workflow.name + "\x00" + normalizedChange` and use `see/<digest>` as the workflow lane. Per-agent log filenames SHALL use the same digest. The same workflow name and normalized change SHALL produce the same identity across polling passes and process restarts, while different workflow names SHALL produce different identities even for equal change values.

#### Scenario: Same change in different workflows is isolated
- **WHEN** `openspec` and `update` both emit `package-update`
- **THEN** their lane digests differ
- **AND** their per-agent log identities differ

#### Scenario: Repeated workflow change resumes its lane
- **WHEN** the same workflow emits the same normalized change on a later pass
- **THEN** it selects the same persistent lane
- **AND** prior commits on that lane remain available

### Requirement: Change token renders workflow templates
For an active workflow, `see` SHALL replace every literal `{change}` occurrence in that workflow's prompt and commit template with the normalized change. Unknown tokens SHALL remain literal, and rendered values SHALL be passed directly as process arguments without shell evaluation.

#### Scenario: Each workflow receives its own rendered templates
- **WHEN** the `openspec` workflow emits `add-dark-mode`
- **THEN** its agent prompt uses the `openspec` prompt with `{change}` replaced
- **AND** its catch-up commit uses the `openspec` commit template with `{change}` replaced
- **AND** the `update` templates are not used
