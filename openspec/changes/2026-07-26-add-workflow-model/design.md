## Context

`see` already mediates every `pi` invocation: the OpenSpec-compat path and every named workflow call `Watcher.agent.Run`, which in practice is `PiAgent.Run`. The argv is hard-coded to `--mode json --no-session <prompt>`, so operators cannot pin a model per workflow without wrapping `pi` in a shell script. The recently added multi-workflow support gave each workflow its own prompt, condition, and commit; adding `model` is the same shape — one more optional field that follows the workflow's value through the existing call chain.

The leading constraint is that the OpenSpec-compatibility path must keep its current behavior. That path does not consult any `WorkflowConfig`, so the new field is workflow-scoped by construction.

## Goals / Non-Goals

**Goals:**
- Let one workflow pin the model `pi` uses, without affecting other workflows.
- Keep the field optional; absent or blank means "use `pi`'s default", which is the same as today.
- Avoid an extra validation step at startup; `pi` is the source of truth for whether a model is acceptable.
- Keep the implementation within the Go standard library and existing in-repo patterns.

**Non-Goals:**
- A top-level `--model` flag for the OpenSpec-compatibility path. The compat path already inherits `pi`'s default and changing it is a separate ask.
- Validating `model` against `pi --list-models`. That would add a startup shell-out and a coupling to `pi`'s catalog format.
- Per-run model overrides (CLI flag > config). The existing `prompt` precedent has a CLI override; `model` does not, by deliberate scope.
- Carrying the model in commit subjects, branch names, log filenames, or event payloads. None of those need a model identity today.
- A `--models` (cycling) flag. `pi` exposes both `--model` and `--models`; this change is the singular, fixed selector.

## Decisions

### `model` is an optional, unvalidated workflow field

`WorkflowConfig` gains a `Model string` field with the `model` YAML tag. The strict decoder already accepts string fields with no extra plumbing, and `validateWorkflows` continues to require only `name`, `prompt`, `condition`, and `commit`; `model` is opt-in. A blank or absent value is treated as "unset" and is never sent to `pi`. The trim happens at the call site so the value stored in `WorkflowConfig.Model` matches what the operator wrote (no surprise rewriting at decode time) and so a whitespace-only value behaves identically to an absent one.

Validation is deliberately not performed. `pi --model X` exits non-zero when `X` is unknown, and that error is already caught by the existing retry path. Adding a startup check would mean shelling out to `pi --list-models` on every startup, which costs time and ties `see` to `pi`'s catalog shape.

### The field flows through `Watcher` to the existing `Agent.Run` call

`Watcher` gains a `Model string` field. `runOneWorkflow` populates it on the child watcher (the same pattern it already uses for `Condition`, `CommitTemplate`, `WorkflowName`, and `PromptTemplate`). All three `agent.Run` call sites pass `w.Model`; the OpenSpec-compatibility site passes `""` because no `WorkflowConfig` is in scope there.

The `Agent` interface signature grows by one positional parameter: `Run(ctx, path, change, prompt, model string)`. This is the smallest change that carries the value to the right place. An `RunOptions` struct was rejected because there is only one new field, and the existing parameter list (path, change, prompt) is short enough that adding a positional is more readable than introducing a struct now. If a second extra field appears, that decision should be revisited.

### `PiAgent` appends `--model` when the trimmed value is nonblank

`PiAgent.Run` builds its argv as `["--mode", "json", "--no-session"]`, then appends `"--model", trim(model)` when `strings.TrimSpace(model) != ""`, then appends the positional `prompt`. The order keeps all flags before the prompt positional, which matches the convention `pi` advertises in `--help`. `pi` parses flags anywhere in argv, so reordering is safe; the order chosen is the readable one.

The OpenSpec-compat path passes `""`, the trim turns it into a no-op, and the resulting argv is byte-identical to today. No behavior change there.

## Risks / Trade-offs

- **The `Agent` interface signature grows** → Only two implementations exist in this repo (`PiAgent` and `fakeAgent`); both are updated together. External implementers of the interface are not expected.
- **An invalid model string is not caught at startup** → A typo in `model: gpt-5-minii` surfaces as an agent exit error after the agent has already been launched, which the existing retry loop handles. The cost is one wasted `pi` invocation per startup per typo'd workflow; the alternative is a `pi --list-models` round-trip on every startup.
- **The model value is passed verbatim to `pi` with no escaping** → `exec.Command` does not interpret the string, so the only characters that can break the argv are NULs, which are rejected by `pi` already. Leading/trailing whitespace is trimmed at the boundary; interior whitespace is preserved.
- **A workflow that wants `pi`'s default cannot distinguish "no field" from "empty string"** → Both decode to `""`, both produce the same argv, both are equivalent in practice. This is the intended equivalence.

## Migration Plan

1. Extend the strict configuration decoding path with one new optional field and update the embedded example template.
2. Extend `Watcher`, the `Agent` interface, `PiAgent`, and the three call sites in a single change so the argv shape is consistent in every code path.
3. Add regression tests at the `PiAgent` boundary (argv shape with and without `model`) and the workflow boundary (the field flows from `WorkflowConfig` to `agent.Run`).
4. Update `AGENTS.md` and the `workflow-condition` spec.
5. Run `gofmt` and `go test -timeout 30s ./...`.

No configuration migration is required: existing configs without `model` decode unchanged and produce the same argv they do today.

## Open Questions

None. A top-level `--model` flag for the OpenSpec-compat path is a deliberate non-goal; revisit if operators ask.
