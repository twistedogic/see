## Why

`see` currently supports only one configured custom workflow per invocation: one prompt, one condition, and one commit template. This prevents a repository from being driven by independent automations such as OpenSpec work and dependency updates in the same watch pass. Supporting ordered, named workflows will let each repository evaluate multiple independent work sources while preserving serialized agent execution and safe branch isolation.

## What Changes

- Add a strict `workflows` configuration sequence; each workflow has a required unique `name`, `prompt`, `condition`, and `commit`.
- Evaluate every workflow in configuration order for every discovered repository.
- Run at most one `pi` agent session at a time, then continue to the next workflow and repository.
- Treat condition exit status `1` as an idle workflow and continue; isolate ordinary workflow failures so later workflows still run.
- Clean up failed workflow attempts before continuing, preserving prior persistent-lane history and stopping only when cleanup cannot restore a safe working tree.
- Namespace custom branch and per-agent log identities by workflow name plus normalized condition output.
- Include workflow identity in watcher events, logs, prompts, and diagnostics where needed to distinguish independent work.
- Allow clean switching between persistent workflow lanes during one repository pass and leave the final usable workflow lane checked out.
- Remove the top-level `prompt`, `condition`, and `commit` configuration fields. **BREAKING**
- Keep `exclude` as a sequence of glob patterns.
- Preserve the existing OpenSpec behavior as the default `openspec` workflow configuration or equivalent compatibility path, including its prompt and catch-up commit semantics.

## Capabilities

### New Capabilities

None. The change extends existing watcher, workflow-condition, event-log, and configuration behavior.

### Modified Capabilities

- `watcher`: process multiple ordered workflows per repository, isolate failures, switch between workflow lanes safely, and leave the final usable lane checked out.
- `workflow-condition`: replace one global custom workflow with independently validated named workflow definitions and namespace stable identities by workflow name and change.
- `event-log`: carry workflow identity in events and preserve an unambiguous serialized event stream for multiple workflows.

## Impact

- Configuration decoding and validation in `config.go`, including startup validation and the example configuration.
- Watcher orchestration, retry handling, branch lifecycle, rollback, prompt rendering, commit rendering, and event types in `main.go`.
- Per-run log naming and event serialization where workflow identity is added.
- Terminal User Interface (TUI) event messages and rendering so workflow names remain distinguishable.
- Existing configuration users must migrate top-level custom fields to named entries under `workflows`.
- Tests covering configuration, discovery, workflow conditions, branch isolation, retries, events, logs, and TUI behavior will need updates and regression coverage.
- No new external dependency is required.
