## 1. Configuration

- [ ] 1.1 Add `Measure string \`yaml:"measure"\`` to `WorkflowConfig` in `config.go`.
- [ ] 1.2 Extend the per-workflow field validation to reject a present-but-blank/whitespace-only `measure`, naming the workflow and the field (treat `measure` as optional, so absent stays valid).
- [ ] 1.3 Add a commented `measure:` line to the example `workflows:` entry in `config.example.yaml`.

## 2. Workflow files and convention resolution

- [ ] 2.1 Add `Measure string` to the frontmatter decode struct in `workflow_files.go`; thread it onto the produced `WorkflowConfig`.
- [ ] 2.2 Add `measure` to the accepted frontmatter-keys set so an unknown key is still rejected.
- [ ] 2.3 Add a resolver that selects the effective measure command: nonblank `WorkflowConfig.Measure` wins; otherwise read `~/.config/see/measure/<workflow-name>.sh` if it exists; otherwise "no measure". A missing convention directory/file is not an error. Return both the command string and a "resolved?" boolean.

## 3. Measure execution

- [ ] 3.1 Add `runMeasure(ctx, cwd, command string) (value string, err error)` mirroring `resolveCustomCondition`/`runCheck` shell/context/process-group plumbing, with stdout captured and normalized exactly like a condition value (trim trailing CR/LF, require single-line non-whitespace).
- [ ] 3.2 Parse the normalized value with `strconv.ParseFloat(value, 64)`; on failure return a measure-failure error identifying the unparseable value.
- [ ] 3.3 Add a `measureFailedError` sentinel carrying the rendered command, exit code, baseline, candidate, and stderr; implement `Error()` so `errors.As` and the summary helper work (distinguish "no improvement" from "command errored" in the message, using whichever fields are populated).
- [ ] 3.4 Render `{change}` into the measure command via `renderTemplate` before execution; do NOT render `{metric}` into the measure command.

## 4. Landing-path integration (baseline + candidate)

- [ ] 4.1 Branch + custom: after the lane is ensured and before `agent.Run`, if a measure is resolved, run `runMeasure` for the baseline; on failure route to `rollbackWorkflowLane` with the `measureFailedError` and return it without invoking the agent; on success hold the baseline value and render `{metric}` into the prompt and commit templates.
- [ ] 4.2 Branch + custom: after a passing `check` (or when no check), if a measure is resolved, run `runMeasure` for the candidate; compare candidate strictly `>` baseline; on failure route to `rollbackWorkflowLane` and return the `measureFailedError` before staging; on improvement proceed to `catchUpCustomCommit`.
- [ ] 4.3 Worktree + auto-merge: capture the baseline before `agent.Run` (worktree cwd); run the candidate measure inside `mergeWorktreeLane` after the check gate and before the catch-up commit; route failure to `rollbackWorktree`.
- [ ] 4.4 Worktree + manual-merge: capture the baseline before `agent.Run`; run the candidate measure after the check gate and before `rebaseWorktreeLane`; route failure to `rollbackWorktree`.
- [ ] 4.5 Confirm the candidate measure runs even when the working tree is clean (non-deterministic metrics); confirm a workflow without a resolved measure invokes the agent unchanged and performs no `{metric}` substitution.

## 5. Terminal event and observability

- [ ] 5.1 Add a `MeasureFailed` event type (fields: `Path`, `Workflow`, `Change`, `Command`, `ExitCode`, `Baseline`, `Candidate`, `Err`, plus a `summary`) implementing `isEvent()`.
- [ ] 5.2 In `runOnce`, after `runWithRetry` returns the final error, select the terminal event by type in priority: `measureFailedError` → `MeasureFailed`; else `checkFailedError` → `CheckFailed`; else `ChangeFailed`. Never emit more than one per repo per pass.
- [ ] 5.3 Ensure `RetryAttempt` between attempts carries the measure-failure summary unchanged.
- [ ] 5.4 Add optional `Baseline` and `Candidate` string fields to the success event; populate them only when a measure gate is resolved, and record baseline→candidate on a successful landing.

## 6. TUI

- [ ] 6.1 Add a `MeasureFailed` case to the TUI event type-switch; render it like `ChangeFailed`/`CheckFailed` (error column / failed state) with a "measure failed" / "no improvement" label.

## 7. Tests

- [ ] 7.1 Config: a present-but-blank `measure` is rejected at startup; an absent `measure` is accepted and behaves as before.
- [ ] 7.2 Workflow files: `measure` is accepted in frontmatter; an unknown frontmatter key is still rejected.
- [ ] 7.3 Convention resolution: frontmatter `measure` overrides `~/.config/see/measure/<name>.sh`; absent frontmatter falls back to the convention file; neither present means no measure; a missing convention dir is not an error.
- [ ] 7.4 Branch mode: baseline is captured before the agent; `{metric}` reaches the prompt and commit; improvement precedes the catch-up commit; non-improvement rolls back to the pre-attempt tip, creates no commit, and yields a `MeasureFailed` terminal event.
- [ ] 7.5 Branch mode: an unparseable / nonzero-exit / empty / multiline measure output is a measure failure with rollback and `MeasureFailed`.
- [ ] 7.6 Branch mode: a baseline-measure failure skips the agent and yields `MeasureFailed`.
- [ ] 7.7 Worktree + auto-merge: improvement precedes rebase + ff-merge; non-improvement removes the worktree, deletes the lane, leaves the operator untouched, and yields `MeasureFailed`.
- [ ] 7.8 Worktree + manual-merge: non-improvement rolls back and yields `MeasureFailed`.
- [ ] 7.9 Interaction with `check`: a failed check short-circuits the candidate measure and yields `CheckFailed` (not `MeasureFailed`); a passing check followed by improvement lands.
- [ ] 7.10 Retry: a measure failure retries up to `RetryCount`; the final failure emits `MeasureFailed`; `RetryAttempt` summaries are present between attempts.
- [ ] 7.11 Token substitution: `{change}` renders in `measure`; `{metric}` renders in `prompt` and `commit` only when a measure is resolved and stays literal otherwise and in `measure`.
- [ ] 7.12 Integrity: the baseline value is never written to a path under the agent's working directory.
- [ ] 7.13 Cancellation stops a running measure.

## 8. Docs

- [ ] 8.1 `AGENTS.md`: add `measure` to the workflow-entry schema, to the frontmatter keys list, document the `~/.config/see/measure/<workflow-name>.sh` convention fallback, and add a "Measure gate" subsection (two definition forms, shell contract, double run, comparison rule, `{metric}` token, integrity model and accepted residual gap, rollback-to-clean-slate, retry behavior).
- [ ] 8.2 Run `openspec validate --change add-workflow-measure` and resolve any reported delta issues.
