# watcher delta — add-log-rotation

## ADDED Requirements

### Requirement: Per-invocation logs are bounded to a fixed retention per stem

After `PiAgent.Run` creates a per-invocation `.jsonl` file and the agent
command has finished, `see` SHALL bound the number of per-invocation
files that share that file's stem to a fixed retention count. The stem
SHALL be the filename component preceding the
`--<utc-timestamp>--<pid>` suffix: `<repo-basename>--<change>` in
OpenSpec compatibility mode and `<repo-basename>--<digest>` in custom
mode, identical to the identity the filename encodes. Rotation SHALL
run within `PiAgent.Run` after the per-invocation file has been closed,
so the just-written file is never deleted while open, and SHALL run
whether or not the agent run succeeded — any run that produced a file
is counted toward the stem's history.

To select a stem's files, `see` SHALL consider only files in the
configured log directory whose name begins with the exact prefix
`<stem>--` and ends with `.jsonl`. Files of any other shape — including
the batch-level `see--<utc-timestamp>--<pid>.jsonl` event log — SHALL
NOT be selected or deleted. `see` SHALL determine recency by sorting
the matched filenames in descending lexicographic order; the
fixed-width `YYYYMMDDTHHMMSS` timestamp makes lexicographic order
chronological at the filename's second granularity. `see` SHALL retain
the newest files up to the retention count and delete the remainder.
The retention count SHALL be a fixed implementation constant (currently
5) and SHALL NOT be operator-configurable.

Deletion SHALL be best-effort: a failure to remove an older file SHALL
NOT fail the agent run, emit an event, or alter the `logPath` or error
`PiAgent.Run` returns. Rotation is observability hygiene, not
correctness.

#### Scenario: Retention bounds the newest files per stem

- **WHEN** `PiAgent.Run` writes a per-invocation file for stem
  `myproj--add-dark-mode`, after which 7 files for that stem exist in
  the log directory
- **THEN** after `Run` returns, exactly the 5 newest files for that
  stem remain
- **AND** the oldest 2 are deleted
- **AND** the just-written file is among the 5 retained

#### Scenario: A stem with five or fewer files is a no-op

- **WHEN** `PiAgent.Run` writes a per-invocation file and that stem has
  5 or fewer files after the write
- **THEN** no file for that stem is deleted
- **AND** `PiAgent.Run` returns as it would without rotation

#### Scenario: Each stem is rotated independently

- **WHEN** a repository has two active changes producing two distinct
  stems, and each stem has accumulated more than the retention count
- **THEN** each stem is bounded to the retention count independently
- **AND** neither stem's files count against the other's retention

#### Scenario: Batch-level event logs are not rotated

- **WHEN** the log directory also contains batch-level
  `see--<utc-timestamp>--<pid>.jsonl` event-log files
- **THEN** those files are never selected or deleted by per-invocation
  rotation
- **AND** they remain on disk after the run

#### Scenario: Rotation runs after a failed agent run

- **WHEN** `PiAgent.Run` creates and closes a per-invocation file and
  the agent command then fails
- **THEN** rotation still bounds that stem's files to the retention
  count
- **AND** the failure is returned unchanged from `PiAgent.Run`

#### Scenario: A deletion failure does not fail the run

- **WHEN** removing an older file fails (permission denied, file busy)
- **THEN** `PiAgent.Run` returns the same `logPath` and error it would
  have returned without rotation
- **AND** no event is emitted for the deletion failure
- **AND** the files that could be deleted are deleted

#### Scenario: The just-written file is closed before rotation

- **WHEN** `PiAgent.Run` writes the newest file for a stem that exceeds
  the retention count
- **THEN** rotation runs only after that file is closed
- **AND** the just-written file is retained as the newest file for the
  stem
