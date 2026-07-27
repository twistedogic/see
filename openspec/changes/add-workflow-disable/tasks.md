## 1. Configuration field

- [ ] 1.1 Add a `Disable bool` field to `WorkflowConfig` in `config.go`
      with the `yaml:"disable"` tag. No new validation: the strict
      decoder accepts the bool, and `validateWorkflows` continues to
      require only `name`, `prompt`, `condition`, and `commit`.
- [ ] 1.2 Add a `Disable bool` field to the frontmatter decode struct in
      `workflow_files.go` and copy the parsed value onto the produced
      `WorkflowConfig`, exactly as `model` is threaded today. The
      accepted-keys set for frontmatter is now `name`, `condition`,
      `commit`, `model`, `disable`; the strict decoder continues to
      reject any sixth key.
- [ ] 1.3 Add a round-trip test in `config_test.go` decoding a workflow
      with `disable: true`, asserting the value reaches
      `WorkflowConfig.Disable`, and asserting a workflow with no
      `disable` field decodes to `false`.
- [ ] 1.4 Add a frontmatter test in `workflow_files_test.go` decoding a
      `.md` body whose frontmatter sets `disable: true`, asserting the
      produced `WorkflowConfig.Disable` is `true`, and asserting a
      frontmatter with no `disable` key produces `false`.

## 2. Filter and validation ordering

- [ ] 2.1 Add a filter step to the startup path in `main.go`, placed
      immediately after `validateWorkflows(cfg)` succeeds. It SHALL drop
      every entry with `Disable == true` from the evaluated list and the
      resulting slice SHALL be what the watcher uses. Concretely, after
      the filter set `cfg.Workflows` to the enabled-only slice and
      ensure `w.Workflows` holds that filtered slice (today
      `w.Workflows = cfg.Workflows` runs before `validateWorkflows`, so
      the assignment must move after the filter or be re-applied).
- [ ] 2.2 Add a test: a `Workflows` slice with one disabled and one
      enabled entry, after load, yields an evaluated list containing
      only the enabled entry, in its original relative order.
- [ ] 2.3 Add a test asserting the ordering invariant: a disabled entry
      with a blank required field (e.g. blank `condition`) still fails
      startup, because `validateWorkflows` runs on the full list before
      the filter. The error identifies the workflow and the missing
      field.
- [ ] 2.4 Add a test asserting that a disabled duplicate name is still
      rejected: two entries named `openspec`, one with `disable: true`,
      fail startup at `validateWorkflows` before any filtering.
- [ ] 2.5 Add a test asserting that disabling every workflow yields an
      empty evaluated slice (the watcher's subsequent OpenSpec-mode
      behavior is unchanged and not asserted here).

## 3. Cross-source integration

- [ ] 3.1 Add a test covering the merged path: `workflows_dir` contains
      an `openspec.md` with frontmatter `disable: true` and
      `config.yaml` has one enabled entry. After load, the evaluated
      list contains only the enabled `config.yaml` entry; the disabled
      file workflow is absent.
- [ ] 3.2 Add a test that a disabled file workflow is still validated:
      an `openspec.md` with `disable: true` and a blank `commit` fails
      startup, naming the file path and the missing field.

## 4. Documentation

- [ ] 4.1 Update `AGENTS.md`: add `disable` to the workflow-entry
      schema, add `disable` to the frontmatter accepted-keys list, and
      add one sentence noting that disabling every workflow reverts the
      watcher to OpenSpec compatibility mode.
- [ ] 4.2 Update the embedded `config.example.yaml` template to include
      a commented `disable: false` line under one of the workflow
      examples so the field is discoverable on first run.
- [ ] 4.3 Run `gofmt` on the changed Go files and
      `go test -timeout 30s ./...`.

## 5. Spec sync and validation

- [ ] 5.1 Run `openspec validate --change add-workflow-disable` and
      confirm no issues.
- [ ] 5.2 Run `openspec sync-specs --change add-workflow-disable` to
      promote the delta specs into `openspec/specs/workflow-condition/`
      and `openspec/specs/workflow-files/`.
- [ ] 5.3 Re-run `openspec validate --change add-workflow-disable` after
      the sync.
