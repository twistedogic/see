## ADDED Requirements

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
