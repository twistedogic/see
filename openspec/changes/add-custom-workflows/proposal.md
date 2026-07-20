## Why

`see` currently hard-codes OpenSpec as its only source of work: it discovers `openspec/changes/`, derives the branch and prompt from an OpenSpec change name, and treats archival as completion. This prevents the watcher, Git isolation, retries, logging, and Terminal User Interface (TUI) from driving other repository workflows.

## What Changes

- Add optional `condition` and `commit` fields to `config.yaml` for defining a custom workflow.
- When `condition` is configured, run it through the platform shell in each watched repository. Exit status `1` means no work, exit status `0` means work is available, and any other nonzero status is a condition failure.
- Use normalized condition stdout as the active `{change}` value. Reject successful conditions that emit an empty, whitespace-only, or multiline value.
- Replace every `{change}` token in both the agent prompt and catch-up commit message.
- Run custom workflows on a persistent `see/<hash-of-change>` branch. Repeated condition output resumes the same branch; different output selects a different branch.
- Re-run the agent on every polling pass while the condition continues to succeed, without persisting edge-trigger state.
- Preserve commits made by the agent and create the custom-message commit only for remaining staged changes. A successful run with no remaining changes is a successful no-op.
- Preserve an existing automation branch and its history when a run fails; roll back only the failed run.
- Keep the current OpenSpec discovery, prompt, branch, completion, and commit behavior as the compatibility fallback when no custom condition is configured.
- Generalize repository availability events and TUI state so custom workflows are represented without claiming that every repository has OpenSpec.
- **BREAKING**: rename the JSONL `RepoSeen.HasOpenspec` payload field to a workflow-neutral availability field.

## Capabilities

### New Capabilities
- `workflow-condition`: Configuration, shell execution, stdout normalization, change hashing, and template rendering for custom workflows.

### Modified Capabilities
- `watcher`: Select between custom and OpenSpec-compatible work, run level-triggered custom changes on persistent branches, and apply the custom rollback and catch-up commit contracts.
- `tui`: Represent custom-workflow availability and changes using workflow-neutral repository state.
- `event-log`: Record workflow-neutral repository availability instead of the OpenSpec-specific `HasOpenspec` field.

## Impact

- `config.go`, `config.example.yaml`, and `AGENTS.md`: strict configuration schema and documentation gain custom workflow fields and fallback rules.
- `main.go`: work resolution, shell condition execution, prompt and commit rendering, branch identity, rollback, and events change.
- `tui/`: repository availability messages and model fields become workflow-neutral; the existing CHANGE column continues to display the resolved change value.
- Tests and OpenSpec specifications require custom-condition, compatibility-fallback, persistent-branch, event-schema, and TUI coverage.
- Existing configurations without `condition` remain behaviorally compatible with the OpenSpec workflow.
- No new runtime dependency is required; hashing, shell execution, and template replacement use the Go standard library and platform facilities.
