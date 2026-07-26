## 1. Configuration model and validation

- [ ] 1.1 Add a `WorkflowConfig` type and replace top-level custom workflow fields with `Config.Workflows []WorkflowConfig`, preserving strict YAML decoding and ordered entries.
- [ ] 1.2 Add configuration tests for workflow decoding, multiline prompts, required fields, duplicate names, wrong field types, legacy top-level field rejection, and empty-workflow OpenSpec compatibility.
- [x] 1.3 Update `config.example.yaml` and `AGENTS.md` with the `workflows` schema, required fields, ordering, and migration from top-level `prompt`, `condition`, and `commit`.

## 2. Workflow condition and identity tests

- [ ] 2.1 Add failing tests proving each workflow condition is evaluated independently in repository/workflow order and exit status `1` skips only the current workflow.
- [ ] 2.2 Add failing tests for workflow-scoped SHA-256 identity, proving equal changes from different workflow names use different branch and log identities while repeated pairs remain stable.
- [ ] 2.3 Add failing tests proving each workflow renders its own prompt and commit templates with `{change}` substitution.
- [ ] 2.4 Add failing tests proving condition, agent, and catch-up failures are isolated and later workflows run after successful rollback.

## 3. Sequential watcher orchestration

- [ ] 3.1 Extend watcher work state to carry workflow name, prompt, condition, commit template, and workflow-scoped change identity through resolution, retries, agent execution, events, and settlement.
- [ ] 3.2 Replace the single-condition resolver path with an ordered per-repository workflow pass while retaining the no-workflows OpenSpec compatibility resolver.
- [ ] 3.3 Scope retry handling to each workflow and preserve serialized execution so no two pi sessions run concurrently.
- [ ] 3.4 Add watcher tests for repository order, workflow order, one active agent session, idle workflows, multiple active workflows, and continuation after ordinary workflow failure.

## 4. Workflow lane lifecycle and rollback

- [ ] 4.1 Update workflow lane naming to use the workflow-name/change digest and add tests for independent persistent lanes.
- [ ] 4.2 Permit switching between workflow lanes from a clean checkout while preserving prior commits and reject dirty tracked or non-ignored untracked state before mutation.
- [ ] 4.3 Update rollback to clean a failed workflow attempt, preserve existing lane history, delete only newly created lanes, and return to a safe branch when required.
- [ ] 4.4 Continue to later workflows after successful rollback and stop processing a repository when cleanup cannot establish a safe clean checkout.
- [ ] 4.5 Add repository fixture tests for successful multiple-lane runs, failed existing lanes, failed new lanes, ignored files, dirty trees, cleanup warnings, and final-lane checkout behavior.

## 5. Events, logs, and terminal user interface

- [ ] 5.1 Add workflow identity to workflow-related event types while keeping `RepoSeen.HasChange` workflow-neutral and preserving human-readable change values.
- [ ] 5.2 Update JSONL event serialization and per-agent log naming to include workflow-scoped digest identities without placing raw condition output in paths.
- [ ] 5.3 Update terminal user interface messages and rendering to display workflow names wherever multiple workflow events can otherwise be ambiguous.
- [ ] 5.4 Add event-log and terminal user interface tests covering workflow identity, equal changes across workflows, retry/failure/warning events, and safe log paths.

## 6. Integration verification

- [ ] 6.1 Update existing custom-workflow and compatibility tests to use the new configuration model and preserve OpenSpec behavior when `workflows` is omitted.
- [ ] 6.2 Run `gofmt` on changed Go files and execute `go test -timeout 30s ./...`.
- [ ] 6.3 Run `go build ./...` and validate the completed OpenSpec artifacts with `openspec validate --change "support-multiple-workflows"`.
