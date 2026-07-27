## Why

A configured workflow can only be removed from evaluation by deleting it
from `config.yaml` or the `workflows_dir`, or by commenting out the whole
block. There is no in-place switch to park a workflow that an operator
wants to keep configured but stop running for a while — a complete,
known-good workflow they intend to toggle back on. The only options today
(delete, or comment/uncomment a multi-line block) either lose the
configuration or churn the diff every time it is toggled.

## What Changes

- Add an optional `disable` boolean field to each workflow entry. A
  workflow with `disable: true` SHALL be removed from the evaluated
  workflow list during configuration load, after validation, so the run
  loop, the Terminal User Interface (TUI), and the event stream never see
  it. The field defaults to false; an omitted field means the workflow is
  enabled, identical to today.
- The field applies uniformly to both workflow sources: an entry in the
  `config.yaml` `workflows:` block, and the YAML frontmatter of a `.md`
  file in `workflows_dir`. Frontmatter gains `disable` as a fifth
  accepted key alongside `name`, `condition`, `commit`, and `model`.
- A disabled workflow is still subject to the full startup validation
  contract: nonblank `name`, `prompt`, `condition`, and `commit`, and a
  unique `name` within the merged list. Validation runs on the full
  merged list, including disabled entries, before any are filtered out.
  This keeps a parked workflow from rotting silently: the operator who
  flips it back on gets a workflow that already validates, and a disabled
  duplicate name cannot shadow an enabled one.
- The filter is the last step of configuration load, running after the
  file/config merge and after `validateWorkflows`. The downstream run
  loop is unchanged: it already iterates `w.Workflows`, which after this
  change contains only enabled entries.

### Consequence operators should know

Disabling every workflow reduces the evaluated list to empty, which is
indistinguishable to the watcher from having no `workflows:` block at
all. The watcher then runs in OpenSpec compatibility mode and applies
`openspec/changes/` in watched repositories. This is the natural tail of
"disabled means not present" — the same behavior as commenting out every
workflow — and is documented rather than special-cased.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `workflow-condition`: add a requirement that a workflow entry accepts
  an optional `disable` boolean, that disabled entries are still fully
  validated, and that disabled entries are removed from the evaluated
  list after validation so they never reach the run loop, the TUI, or
  events. Add the disable-all-reverts-to-OpenSpec-mode consequence as a
  scenario.
- `workflow-files`: modify the frontmatter-keys requirement to accept
  `disable` as a fifth optional key, with a scenario covering a `.md`
  workflow whose frontmatter sets `disable: true`.

## Impact

- `config.go`: new `Disable bool \`yaml:"disable"\`` field on
  `WorkflowConfig`. A filter step that drops `Disable == true` entries
  from the merged `cfg.Workflows` slice, placed after `validateWorkflows`
  succeeds (the merge already happens before validation). `validateWorkflows`
  is unchanged — it already iterates the full slice.
- `workflow_files.go`: new `Disable bool` field on the frontmatter decode
  struct, threaded onto the produced `WorkflowConfig`, and added to the
  accepted-keys set so an unknown key is still rejected.
- `main.go`: no change. The run loop reads `w.Workflows`, which after
  the filter contains only enabled entries.
- `config.example.yaml`: a commented `disable: false` line in the
  example `workflows:` entry.
- `AGENTS.md`: the workflow-entry schema gains the `disable` field; the
  frontmatter keys list gains `disable`; one sentence on the
  disable-all-reverts-to-OpenSpec consequence.
- `openspec/specs/workflow-condition/spec.md`: one new requirement plus
  scenarios.
- `openspec/specs/workflow-files/spec.md`: the frontmatter-keys
  requirement and its scenarios are modified.
- No new dependencies. No breaking changes: an operator with no
  `disable` field anywhere sees no behavioral change.
