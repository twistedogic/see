## Context

`see` evaluates every entry in the merged workflow list (`.md` files from
`workflows_dir` first, then `config.yaml` `workflows:` in order) on every
polling pass for every watched repository. There is no in-place way to
keep a workflow configured but stop it running: the operator must delete
the entry or comment out the entire block. The merged list already passes
through `validateWorkflows` (nonblank `name`/`prompt`/`condition`/`commit`,
unique `name`) and the strict YAML decoder rejects unknown keys on both
sources (`KnownFields(true)` in `config.go` and `workflow_files.go`).

The field this change adds is the same shape as the optional `model`
field added to `WorkflowConfig` and frontmatter: one optional value that
travels with the workflow. The difference is behavioral — `disable`
removes the workflow from evaluation rather than feeding a value into the
agent call.

## Goals / Non-Goals

**Goals:**
- Let an operator park a fully-configured, known-good workflow in place
  and toggle it back on with a one-line, one-key diff, instead of
  deleting it or churning a comment block.
- Apply the switch uniformly to both workflow sources (`config.yaml`
  entries and `.md` frontmatter).
- Keep a parked workflow from rotting silently: it must still validate,
  so re-enabling it needs no repair.
- Keep the downstream run loop, Terminal User Interface (TUI), and event
  stream free of any new branch, marker, or event.

**Non-Goals:**
- Observability of the parked state. A disabled workflow produces no
  event, no TUI row, and no log line. This is the accepted trade-off of
  the chosen approach (see Decisions); a future change could add a
  "disabled" indicator if operators find the silence surprising.
- Partial disable semantics (`disable` only on weekdays, only for a
  named repository, etc.). The field is a static boolean.
- A "draft parker" that exempts disabled entries from validation. The
  operator chose strict validation: a disabled entry must still be
  complete and correct.

## Decisions

### Load-time filter, not a run-time skip

The disabled workflow is removed from `cfg.Workflows` as the last step
of configuration load, after the file/config merge and after
`validateWorkflows`. The run loop, which already iterates
`w.Workflows`, is unchanged.

**Alternative considered:** a run-time skip (`for _, wf := range w.Workflows
{ if wf.Disable { continue } }`) that keeps disabled entries in the list
and would let a future change emit a "skipped" event or TUI marker. This
was rejected for this change because the operator explicitly chose the
load-time variant: disabled means "not present," and the simplest
possible downstream — no skip branch, no event, no TUI change — is the
payoff. Run-time skip remains available as a future change if
observability of the parked state becomes important.

### Disabled entries are still fully validated (strict)

`validateWorkflows` runs over the **full** merged list, including
disabled entries, before any are filtered. A disabled entry must still
have nonblank `name`/`prompt`/`condition`/`commit` and a unique `name`.

**Alternative considered:** exempting disabled entries from validation so
a half-written draft can be parked. Rejected: it makes `disable` a
"draft parker," but the operator wants a "park a known-good workflow"
switch. Exemption would also let a disabled duplicate name silently
shadow an enabled one after filtering. Strict validation keeps parked
workflows from rotting and makes un-parking a no-cost operation.

### Validate-then-filter ordering is a fixed invariant

Validation must run on the full list *before* the filter. If the filter
ran first, two entries with the same name where one is disabled would
collapse to a single non-duplicate entry and slip through. Validate then
filter catches it. This ordering is the one rule that reconciles "strict
validation" with "load-time removal": you cannot strictly validate
entries you have already removed.

Concretely, the filter is a new step placed after `validateWorkflows`
returns nil in the existing configuration-load path, operating on the
already-merged `cfg.Workflows` slice.

### The field lives on `WorkflowConfig` and the frontmatter struct

`WorkflowConfig` gains `Disable bool \`yaml:"disable"\``. The frontmatter
decode struct in `workflow_files.go` gains the same field, and the
parsed value is copied onto the produced `WorkflowConfig` exactly as
`model` is today. The accepted-keys set for frontmatter grows from four
to five; the strict decoder continues to reject any sixth key. No new
type, helper, or dependency is introduced.

## Risks / Trade-offs

- **Disabling every workflow reverts to OpenSpec compatibility mode** →
  An empty evaluated list is, to the watcher, identical to having no
  `workflows:` block. A repository with an active `openspec/changes/`
  entry would then be processed by the OpenSpec resolver. This is the
  documented consequence of "disabled means not present" and is not
  special-cased; the same happens today if every workflow is commented
  out. Mitigation is documentation in `AGENTS.md` and the proposal.
- **A parked workflow is invisible** → No event, TUI row, or log line
  confirms the disable took effect. The operator's only confirmation is
  that the workflow stops running. The strict YAML decoder closes the
  typo hole (`diable: true` fails startup), leaving "set on the wrong
  entry" as the only silent failure, which is operator error. Mitigation
  is documentation; a future change can add observability if needed.
- **Strict validation can surprise an operator parking a draft** → An
  operator who sets `disable: true` on an entry with a blank condition
  gets a startup error rather than a parked workflow. This is the chosen
  contract; the proposal and spec call it out explicitly so it is not
  discovered at runtime.

## Migration Plan

1. Add the `Disable` field to `WorkflowConfig` and the frontmatter
   struct, threading the parsed frontmatter value onto the produced
   `WorkflowConfig`.
2. Add the filter step after `validateWorkflows` in the
   configuration-load path.
3. Add regression tests: a disabled workflow is filtered out; a disabled
   entry with a blank field still fails validation; a disabled duplicate
   name still fails; disabling all workflows yields an empty list;
   frontmatter `disable: true` parks the file workflow.
4. Update `config.example.yaml`, `AGENTS.md`, and the two specs.
5. Run `gofmt` and `go test -timeout 30s ./...`.

No configuration migration is required: existing configs with no
`disable` field decode to `false` everywhere and behave identically to
today.

## Open Questions

None. Observability of the parked state (events, TUI marker) is a
deliberate non-goal for this change; revisit if operators report the
silence as a problem.
