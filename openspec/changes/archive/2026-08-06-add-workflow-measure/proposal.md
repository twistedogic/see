## Why

`see` already drives *one iteration* of an experiment loop per polling
pass and owns every primitive an autoresearch-style workflow needs: a
level-triggered `condition` (is the loop still active?), an agent run
(propose a change), a `check` gate (did the tests still pass?), a
catch-up commit, rollback to a clean slate, and retry. What is missing
is an independent **metric** measurement that captures a baseline
*before* the agent runs, re-measures *after*, and gates landing on
strict improvement — while exposing the score to the agent but keeping
the measurement itself out of the agent's reach.

Operators running autoresearch loops (AI proposes a code/config change,
an independent harness scores it, keep-if-improved else revert) have no
in-process lever for "measure, compare, and keep only if better." The
`check` gate cannot express it: it runs once after the agent, has no
notion of a prior baseline, and the operator would have to smuggle
baseline state through a committed file the agent can read and game.

## What Changes

- Add an optional `measure` string field to each workflow entry, defined
  two ways (precedence mirrors `prompt`): a nonblank frontmatter `measure:`
  value wins; otherwise `see` falls back to a convention script at
  `~/.config/see/measure/<workflow-name>.sh`. An absent `measure` and no
  convention file mean the workflow has no measure gate, identical to
  behavior before this field existed. A present-but-blank `measure` is
  rejected at startup.
- `see` runs the resolved measure command through the platform shell
  under the same contract as `condition` and `check` (`/bin/sh -c` on
  Unix, `cmd.exe /C` on Windows; watcher context attached; own process
  group on Unix), with its working directory set to the agent's working
  directory (the lane checkout in branch mode, the worktree directory in
  worktree mode).
- The measure runs **twice** per attempt: once before the agent to
  capture the **baseline**, and once after a passing `check` (or when no
  `check` is configured) to capture the **candidate**. Both values are
  held in `see`'s memory and are never written where the agent can read
  them.
- A new `{metric}` token, parallel to `{change}`, is substituted in the
  `prompt` and `commit` templates with the baseline value, so the agent
  sees the score to beat. The measure command itself is not a consumer
  of `{metric}` (it is the producer); it does support `{change}` like
  `check`.
- The landing decision becomes: keep iff (no `check`, or `check` passed)
  **and** candidate is strictly greater than baseline. Measure standard
  output is normalized exactly like a condition value (trailing
  carriage-return/line-feed trimmed, single line, non-whitespace) and
  parsed as a 64-bit floating-point value; higher is better. Ties and
  non-improvement do not land.
- A measure that exits nonzero, emits no output, emits multiple lines,
  or emits a value that cannot be parsed as a number is a measure
  failure, as is a candidate that does not strictly exceed the baseline.
  Any measure failure SHALL NOT create a commit and SHALL trigger the
  active lane-isolation mode's rollback to a clean slate, returning a
  measure-failure error.
- A measure-failure error flows through the existing `runWithRetry`
  loop, giving the agent up to `RetryCount` fresh attempts per poll to
  improve the metric. When retries are exhausted, the terminal event is
  a new `MeasureFailed` event (distinct from `ChangeFailed` and
  `CheckFailed`, and mutually exclusive with each per pass) so the
  Terminal User Interface (TUI) and the JavaScript Object Notation Lines
  (JSONL) stream can report "no improvement" honestly.
- **Integrity model.** When the measure is supplied as the convention
  file or as an inline frontmatter value, the script lives outside the
  watched repository, so the agent — which runs in the repository
  working directory — cannot casually read or execute it. The agent
  receives the *value* to beat (`{metric}`) but not the *mechanism*.
  An operator who points `measure` at an in-repository path forfeits
  this tamper-resistance. The residual gap — a determined agent reading
  `~/.config/see/measure/<name>.sh` by absolute path on a shared
  filesystem — is accepted; closing it would require a sandbox `see`
  does not provide today.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `workflow-condition`: a new requirement defines the `measure` gate —
  its two definition forms and precedence, the platform-shell contract,
  the baseline-before-agent / candidate-after-check double run, the
  float64 strict-greater comparison, the `{metric}` substitution into
  prompt and commit, the no-commit / rollback-to-clean-slate on failure,
  and the skip when no measure is configured. The `{change}` token
  rendering requirement extends its template set to include `measure`.
- `workflow-files`: the `.md` frontmatter accepted-keys set gains
  `measure` as an optional key.
- `watcher`: the branch-mode custom success path captures a baseline
  measure before the agent and runs the candidate measure (after a
  passing check) before staging; the branch-mode custom rollback and the
  worktree rollback requirements add measure failure to their trigger
  sets; a new requirement defines the `MeasureFailed` terminal event and
  its selection (in place of `ChangeFailed` / `CheckFailed`) when the
  final attempt failed at the measure.
- `lane-isolation`: the worktree + auto-merge and worktree +
  manual-merge success sequences gain the measure gate (candidate
  measure after a passing check, before the catch-up commit / rebase),
  and the worktree rollback requirement adds measure failure to its
  triggers.
- `event-log`: the batch-level JSONL stream gains a `MeasureFailed`
  payload variant carrying the rendered command, exit code, baseline,
  candidate, and captured standard error; the success event records the
  baseline and candidate metric values when a measure gate is
  configured.

## Impact

- `config.go`: new `Measure string \`yaml:"measure"\`` field on
  `WorkflowConfig`; validation that a present `measure` is nonblank
  (whitespace-only rejected) alongside the existing per-field checks.
- `workflow_files.go`: new `Measure string` field on the frontmatter
  decode struct, threaded onto the produced `WorkflowConfig`, and added
  to the accepted-keys set; resolution of the convention fallback file
  at `~/.config/see/measure/<workflow-name>.sh` (existence check only; a
  missing file means "no measure" and is not an error).
- `main.go`: a `runMeasure` helper mirroring `resolveCustomCondition` /
  `runCheck` shell plumbing, returning the normalized metric string and
  a parse error; a `measureFailedError` sentinel carrying the rendered
  command, exit code, baseline, candidate, and stderr; a baseline
  capture step before `agent.Run` (skipped when no measure is resolved);
  a candidate measure + compare step inserted after a passing `check`
  and before staging in all three landing paths; routing of measure
  failure to each mode's existing rollback; `runOnce` terminal-event
  selection extended to prefer `MeasureFailed` (via `errors.As` on
  `measureFailedError`) over `CheckFailed` / `ChangeFailed`.
- `eventlog.go`: a `MeasureFailed` event type and optional `Baseline` /
  `Candidate` fields on the success event, populated only when a measure
  gate is configured.
- `tui/`: a new `MeasureFailed` case that renders like `ChangeFailed` /
  `CheckFailed` with a "measure failed" / "no improvement" label.
- `AGENTS.md`: the workflow-entry schema gains `measure`; the frontmatter
  keys list gains `measure`; a "Measure gate" subsection documents the
  two definition forms, the shell contract, the double run, the
  comparison rule, the `{metric}` token, the integrity model, rollback,
  and retry behavior.
- `config.example.yaml`: a commented `measure:` line in the example
  `workflows:` entry.
- No new dependencies. No breaking changes: a workflow with no `measure`
  field and no convention file behaves exactly as today.
