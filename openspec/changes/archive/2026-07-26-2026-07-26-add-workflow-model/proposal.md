## Why

A configured workflow pins the agent's prompt, condition, and commit message, but it does not pin the model the agent uses. Operators who want a cheap model for housekeeping work and a stronger model for substantive changes must currently pass `--model` through the surrounding shell or rely on the agent's defaults. `see` already mediates every `pi` invocation, so it is the natural place to carry one optional per-workflow model selector.

This change adds an optional `model` field to each named workflow entry. When set, `see` passes it to `pi` as `--model` for that workflow's runs. When unset, the behavior is unchanged.

## What Changes

- Add an optional `model` string to every `WorkflowConfig` entry. Blank or omitted is the zero-value default and is not passed to `pi`.
- Carry the workflow's `model` from the loaded config into the `Watcher` for the duration of that workflow's run.
- Extend the `Agent.Run` interface with a `model` parameter so the agent invocation receives it without an indirection through the agent's struct state.
- Have `PiAgent.Run` append `--model <model>` to the `pi` argv when the trimmed value is nonblank; otherwise the argv is unchanged.
- Pass an empty model from the OpenSpec-compatibility path, so the compat behavior is unchanged.
- Add regression tests for round-tripping the field, omitting it when blank, and passing it to `pi` when set.
- Add one requirement to the `workflow-condition` spec covering the optional model selector.

## Capabilities

### New Capabilities

None. The change extends an existing capability.

### Modified Capabilities

- `workflow-condition`: a workflow MAY carry an optional `model` selector that is passed to `pi` as `--model`; when blank, the agent's default model is used unchanged.

## Impact

- `config.go`: `WorkflowConfig` gains a `Model` field. The strict decoder already permits extra fields of the existing types, and `model` is a plain string, so the loader requires no new validation; a blank or absent value is a no-op.
- `main.go`: `Watcher` gains a `Model` field. `runOneWorkflow` copies `wf.Model` (trimmed) onto the child watcher. The three `agent.Run` call sites pass `w.Model`; the OpenSpec-compatibility site passes `""`. `Agent` interface and `PiAgent.Run` grow one positional `model string` parameter; `PiAgent` appends `--model <value>` to argv when the trimmed value is nonblank.
- `main_test.go`: `fakeAgent` records the model it received; new tests cover `PiAgent` passing and omitting `--model`, and a workflow with `model` flowing through to the agent.
- `config_test.go`: round-trip test for the new field plus a regression test proving a blank `model` decodes and is accepted.
- `AGENTS.md`: one bullet in the workflow schema describing the optional `model` field.
- `openspec/specs/workflow-condition/spec.md`: one new requirement with two scenarios.
- No new dependencies. No behavior change for configurations that omit `model`.
- The `Agent` interface grows by one parameter; only the two in-repo implementations change.
