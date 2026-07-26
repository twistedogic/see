## ADDED Requirements

### Requirement: Events identify the workflow that produced them
Watcher events associated with workflow execution SHALL include a nonblank workflow name in addition to the repository path and human-readable change where those fields apply. The batch-level JavaScript Object Notation Lines (JSONL) event stream and Terminal User Interface (TUI) projection SHALL preserve that workflow identity without replacing the existing human-readable change value.

#### Scenario: Workflow events are distinguishable
- **WHEN** two workflows run for the same repository
- **THEN** their started, log-path, completed, failed, retry, and warning events identify the corresponding workflow name
- **AND** consumers can distinguish equal change values from different workflows

#### Scenario: Repository availability remains workflow-neutral
- **WHEN** a repository has one or more active workflows
- **THEN** `RepoSeen` continues to report `HasChange: true`
- **AND** workflow-specific details are not required on `RepoSeen`

### Requirement: Workflow log paths remain namespaced and safe
Each per-agent log path SHALL use the workflow-specific digest identity rather than raw workflow name or condition output. The path SHALL remain directly under the configured log directory and SHALL contain no untrusted condition output as a path component.

#### Scenario: Equal changes produce separate log identities
- **WHEN** two workflows emit the same normalized change
- **THEN** their log paths differ by the workflow-scoped digest
- **AND** neither path contains the raw condition output
