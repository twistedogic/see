# event-log delta — add-workflow-measure

## ADDED Requirements

### Requirement: MeasureFailed event carries the measure command, baseline, candidate, and captured standard error

The `MeasureFailed` event payload SHALL marshal into the batch-level
JSONL envelope under the same `{"ts": "<RFC3339Nano UTC>", "event":
<payload>}` wrapping as every other event. The payload SHALL carry
`Path`, `Workflow`, and `Change` (identical in meaning to the
`ChangeFailed` and `CheckFailed` payloads), plus the rendered measure
`Command` string, the integer `ExitCode` when the command ran, the
`Baseline` string (the normalized baseline value when the baseline was
captured, otherwise empty), the `Candidate` string (the normalized
candidate value when the candidate was captured, otherwise empty), and
the captured standard error as the `Err` string. It SHALL NOT carry
`HasChange`. Consumers SHALL distinguish a measure failure from a check
or agent failure by the event payload type rather than by parsing a
message string.

The `MeasureFailed` event SHALL appear in the JSONL stream exactly once
per repository per polling pass when the final attempt failed at the
measure gate, in timestamp order relative to the surrounding events, and
SHALL NOT be accompanied by a `CheckFailed` or `ChangeFailed` event for
the same final failure.

#### Scenario: MeasureFailed appears in the JSONL stream

- **WHEN** `Watcher.runOnce` runs against one repo whose final attempt
  fails because the candidate did not strictly exceed the baseline
- **THEN** the JSONL contains a line whose `event` payload is the
  `MeasureFailed` event
- **AND** that line carries the rendered measure command, the baseline,
  the candidate, and any captured standard error
- **AND** no `CheckFailed` or `ChangeFailed` line is emitted for that
  final failure

#### Scenario: MeasureFailed, CheckFailed, and ChangeFailed are mutually exclusive per pass

- **WHEN** the final attempt for a repo fails
- **THEN** exactly one of `MeasureFailed`, `CheckFailed`, or
  `ChangeFailed` appears in the JSONL for that repo on that pass
- **AND** the choice is determined by whether the final error is a
  measure-failure, check-failure, or agent error

#### Scenario: MeasureFailed carries workflow identity and metrics

- **WHEN** a `MeasureFailed` event is emitted for the `autoresearch`
  workflow on repo `/x` with change `tune-bench`, baseline `0.73`, and
  candidate `0.71`
- **THEN** the payload's `Path` is `/x`, `Workflow` is `autoresearch`,
  `Change` is `tune-bench`, `Baseline` is `0.73`, and `Candidate` is
  `0.71`

#### Scenario: A baseline-measure failure carries no candidate

- **WHEN** the final attempt failed at the baseline measure before the
  agent ran
- **THEN** the `MeasureFailed` payload's `Baseline` and `Candidate` are
  empty
- **AND** the payload carries the rendered command, the exit code, and
  the captured standard error

### Requirement: Successful landings record the baseline and candidate metric

When a workflow has a resolved measure gate and an attempt lands the
agent's changes (a catch-up commit in branch mode, a rebase plus
fast-forward merge in worktree + auto-merge), the success event `see`
emits for that attempt SHALL carry the `Baseline` and `Candidate`
normalized metric strings, so the JSONL stream records the improvement
that justified the landing. When no measure gate is resolved for the
workflow, the success event SHALL carry no metric fields and SHALL be
identical to the pre-change success event.

#### Scenario: A measure-gated success records both metrics

- **WHEN** an attempt lands with baseline `0.73` and candidate `0.79`
- **THEN** the success event payload for that attempt carries `Baseline`
  set to `0.73` and `Candidate` set to `0.79`

#### Scenario: A non-measure success carries no metric fields

- **WHEN** an attempt lands for a workflow with no measure gate
- **THEN** the success event payload carries no `Baseline` or
  `Candidate` fields
- **AND** is identical to the success event before this change
