## ADDED Requirements

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
