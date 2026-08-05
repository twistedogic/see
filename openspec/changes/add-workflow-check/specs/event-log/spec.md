# event-log delta — add-workflow-check

## ADDED Requirements

### Requirement: CheckFailed event carries the check command and captured standard error

The `CheckFailed` event payload SHALL marshal into the batch-level JSONL
envelope under the same `{"ts": "<RFC3339Nano UTC>", "event":
<payload>}` wrapping as every other event. The payload SHALL carry
`Path`, `Workflow`, and `Change` (identical in meaning to the
`ChangeFailed` payload), plus the rendered check `Command` string, the
integer `ExitCode`, and the captured standard error as the `Err` string.
It SHALL NOT carry `HasChange`. Consumers SHALL distinguish a check
failure from an agent failure by the event payload type rather than by
parsing a message string.

The `CheckFailed` event SHALL appear in the JSONL stream exactly once
per repository per polling pass when the final attempt failed at the
check, in timestamp order relative to the surrounding events, and SHALL
NOT be accompanied by a `ChangeFailed` event for the same final failure.

#### Scenario: CheckFailed appears in the JSONL stream

- **WHEN** `Watcher.runOnce` runs against one repo whose final attempt
  fails because the workflow check exited nonzero
- **THEN** the JSONL contains a line whose `event` payload is the
  `CheckFailed` event
- **AND** that line carries the rendered check command, the integer
  exit code, and the captured standard error
- **AND** no `ChangeFailed` line is emitted for that final failure

#### Scenario: CheckFailed and ChangeFailed are mutually exclusive per pass

- **WHEN** the final attempt for a repo fails
- **THEN** exactly one of `CheckFailed` or `ChangeFailed` appears in the
  JSONL for that repo on that pass
- **AND** the choice is determined by whether the final error is a
  check-failure error

#### Scenario: CheckFailed carries workflow identity

- **WHEN** a `CheckFailed` event is emitted for the `openspec` workflow
  on repo `/x` with change `add-dark-mode`
- **THEN** the payload's `Path` is `/x`, `Workflow` is `openspec`, and
  `Change` is `add-dark-mode`
