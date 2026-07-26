## Context

`see` currently loads one global prompt, condition, and commit template and gives each repository at most one custom workflow decision per polling pass. The watcher also maintains persistent custom lanes, but its current lane rule assumes the selected lane is already checked out when it exists.

The new configuration model contains ordered named workflows. For each repository resolved from `root_dir`, `include`, and `exclude`, the watcher evaluates every workflow in configuration order. A condition can select work independently for each workflow, but the repository checkout is shared, so workflow execution must remain serialized and lane transitions must be explicit.

## Goals / Non-Goals

**Goals:**

- Decode and strictly validate a `workflows` sequence with unique names and per-workflow prompt, condition, and commit values.
- Process repositories and workflows deterministically in stable order.
- Run at most one agent session at a time.
- Give every workflow/change pair an independent persistent branch and log identity.
- Roll back a failed workflow attempt, then continue with later workflows when the checkout is safely clean.
- Leave the final usable workflow lane checked out after a repository pass.
- Preserve ignored files and prior commits during custom-lane rollback.

**Non-Goals:**

- Persisting scheduler position or adding round-robin scheduling.
- Running workflows concurrently.
- Merging workflow lanes into the operator's branch.
- Supporting both the old top-level custom fields and the new workflow configuration.
- Adding a new external dependency.

## Decisions

### Use `workflows` as the configuration field

Each item is a complete independent workflow:

```yaml
workflows:
  - name: openspec
    prompt: |-
      Apply the latest OpenSpec change.
    condition: "..."
    commit: "see: apply openspec {change}"
```

`name`, `prompt`, `condition`, and `commit` are required and trimmed for validation where appropriate. Names must be unique and are used for human-readable event context and stable identity input.

### Process repositories outside, workflows inside

Repository discovery remains responsible only for producing a stable, deduplicated list. The watcher then executes:

```text
for repository in repositories:
    capture starting branch
    for workflow in workflows:
        resolve condition
        if active:
            run and settle that workflow
```

Condition status `1` skips only the current workflow. A condition or agent failure is recorded for that workflow and does not prevent later workflows, provided cleanup restores a safe clean checkout.

### Namespace identity by workflow and change

The custom lane and per-agent log component use the full lowercase Secure Hash Algorithm 256-bit (SHA-256) digest of:

```text
workflow.name + "\x00" + normalizedChange
```

The separator prevents ambiguous concatenations. Human-readable workflow and change values remain in prompts, commit messages, events, and diagnostics; raw condition output is not placed in branch or log paths.

### Transition only through a clean checkout

Before creating or switching a workflow lane, the watcher verifies that tracked and non-ignored untracked changes are absent. A clean checkout may switch from the current branch or lane directly to the requested workflow lane. A dirty checkout is a workflow failure before mutation.

A newly created lane starts at the current commit. An existing lane resumes its tip without resetting prior successful commits. After each workflow settles, the next workflow may switch to another lane. After all workflows, the last usable active lane remains checked out; if none was active, the starting branch remains checked out.

### Isolate rollback and continuation

For an existing lane, capture its tip immediately before the agent, then on failure reset to that tip and run non-ignored cleanup. For a newly created lane, return to the pre-workflow branch, restore its captured commit, and delete only that new lane. Cleanup warnings do not replace the workflow error.

The watcher continues to later workflows after ordinary rollback. If cleanup itself leaves the checkout dirty, detached, or otherwise unsafe to switch, it stops processing that repository rather than risking another workflow's data.

### Keep compatibility behavior explicit

OpenSpec compatibility behavior remains available when no `workflows` configuration is supplied, so existing installations without custom workflow definitions continue to discover OpenSpec changes. The new multi-workflow configuration is the replacement for the old top-level custom fields; those fields are rejected by strict decoding.

## Risks / Trade-offs

- **A workflow can starve no later workflow because every workflow is evaluated, but an always-active workflow can cause repeated work on every polling pass.** → Preserve the existing level-triggered contract and make workflows responsible for returning idle after completion.
- **Switching lanes can overwrite operator edits.** → Require a clean working tree before every lane transition and never include ignored files in the dirty check.
- **A failed cleanup can strand the repository.** → Emit warnings, stop that repository when safety cannot be established, and do not start another agent in the unsafe checkout.
- **Removing top-level fields is a configuration migration break.** → Document the new schema and provide an example showing one workflow per former custom configuration.
- **Adding workflow identity to events changes consumers.** → Update event serialization, TUI messages, and tests together; retain human-readable change values.
