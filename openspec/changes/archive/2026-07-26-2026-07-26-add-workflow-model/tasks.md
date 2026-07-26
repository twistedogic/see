## 1. Configuration

- [x] 1.1 Add a `Model string` field to `WorkflowConfig` in `config.go` with the
      `yaml:"model"` tag. No new validation: the strict decoder accepts the
      string, blank is allowed, and `validateWorkflows` continues to require
      only `name`, `prompt`, `condition`, and `commit`.
- [x] 1.2 Add a failing round-trip test in `config_test.go` that decodes a
      workflow with `model: openai/gpt-5-mini` and asserts the value reaches
      `WorkflowConfig.Model`.
- [x] 1.3 Add a regression test asserting that a workflow with no `model`
      field decodes to the zero value and that a workflow with `model: "  "`
      also decodes to the zero value (whitespace-only is treated as unset at
      the call site, not the decode site).

## 2. Watcher and Agent plumbing

- [x] 2.1 Add a `Model string` field to the `Watcher` struct in `main.go`,
      with a comment that it is populated by `runOneWorkflow` for the
      duration of one workflow's run and is empty in OpenSpec-compat mode.
- [x] 2.2 Update the `Agent` interface to
      `Run(ctx, path, change, prompt, model string) (string, error)`.
- [x] 2.3 Update `PiAgent.Run` to accept the new parameter. When
      `strings.TrimSpace(model) != ""`, append `"--model", trimmed` to the
      argv after the existing flags and before the positional prompt.
      Otherwise the argv is byte-identical to today.
- [x] 2.4 Update `fakeAgent.Run` in `main_test.go` to accept and record the
      new parameter (add a `models []string` slice mirroring `prompts`).
- [x] 2.5 Update the three `agent.Run` call sites in `main.go`
      (`workResolvedWorktree`, the custom-mode branch of `workResolved`, and
      the OpenSpec-compat branch of `workResolved`) to pass `w.Model`; the
      compat site passes `""`.
- [x] 2.6 In `runOneWorkflow`, set `child.Model = strings.TrimSpace(wf.Model)`
      on the child watcher copy, alongside the existing
      `Condition` / `CommitTemplate` / `WorkflowName` / prompt assignments.

## 3. Tests

- [x] 3.1 Add `TestPiAgentPassesModelFlag`: drive `PiAgent.Run` with a
      recorded-argv fake binary and assert that the argv contains
      `--model openai/gpt-5-mini` and the trailing positional prompt.
- [x] 3.2 Add `TestPiAgentOmitsModelFlagWhenBlank`: same harness, pass
      `""` (and once `"  "`) and assert the argv contains no `--model`
      occurrence and is otherwise byte-identical to the pre-change argv.
- [x] 3.3 Add `TestWorkflowModelFlowsToAgent`: configure a watcher with one
      workflow whose `model` is set, drive a successful run with a
      `fakeAgent`, and assert the recorded `models` slice contains the
      expected value.
- [x] 3.4 Add `TestWorkflowBlankModelDoesNotPropagate`: same harness with a
      blank `model`, assert the recorded `models` slice contains `""` and
      the resulting argv would have no `--model` flag.
- [x] 3.5 Update every existing `PiAgent{...}.Run(...)` and
      `fakeAgent.Run(...)` call in `main_test.go` to pass the new `""`
      argument so the suite compiles.
- [x] 3.6 Run `gofmt` on the changed Go files and `go test -timeout 30s ./...`.

## 4. Documentation

- [x] 4.1 Update `AGENTS.md` workflow schema with one bullet: "`model` is
      an optional string. When nonblank, it is passed to `pi` as
      `--model` for that workflow's runs; otherwise the agent's default
      model is used."
- [x] 4.2 Update the embedded `config.example.yaml` template to include a
      commented `model:` line under one of the workflow examples so the
      field is discoverable on first run.
- [x] 4.3 Run `openspec validate --change 2026-07-26-add-workflow-model`
      and confirm no issues.

## 5. Spec sync

- [x] 5.1 Add a new requirement to
      `openspec/changes/2026-07-26-add-workflow-model/specs/workflow-condition/spec.md`
      covering the optional per-workflow `model` selector, with two
      scenarios: a workflow with `model` passes `--model` to `pi`; a
      workflow without `model` (or with a blank one) does not.
- [x] 5.2 Run `openspec sync-specs --change 2026-07-26-add-workflow-model`
      to promote the delta spec into
      `openspec/specs/workflow-condition/spec.md`.
- [x] 5.3 Re-run `openspec validate --change 2026-07-26-add-workflow-model`
      after the sync.
