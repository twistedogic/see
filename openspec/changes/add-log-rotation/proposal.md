## Why

`see` writes one per-invocation agent log per agent run into its log
directory, and the watch loop runs forever: every polling pass (default
five minutes) that finds work creates another file for that
`(repository, change)` identity. Over hours, days, and weeks the
per-invocation files accumulate without bound — dozens to hundreds per
stem — while the batch-level event log only grows on process restart.
There is no lever to bound this growth, so the log directory becomes a
slow leak that an operator discovers only when the disk fills.

## What Changes

- After each per-invocation agent log is written and closed,
  `see` bounds the number of per-invocation files that share that file's
  *stem* to a fixed retention count of 5, deleting the oldest beyond it.
- The stem is the filename component preceding the
  `--<utc-timestamp>--<pid>` suffix: `<repo-basename>--<change>` in
  OpenSpec compatibility mode and `<repo-basename>--<digest>` in custom
  mode — identical to the identity the filename already encodes. Each
  stem (each repository/change stream) is rotated independently.
- Rotation runs inside `PiAgent.Run` after the per-invocation file is
  closed, whether or not the agent run succeeded — any run that produced
  a file is counted toward the stem's history.
- Files are matched for a stem by the exact prefix `<stem>--` plus the
  `.jsonl` suffix, so the batch-level `see--<ts>--<pid>.jsonl` event
  log and any unrelated file are never selected or deleted.
- Recency is determined by sorting matched filenames in descending
  lexicographic order: the fixed-width `YYYYMMDDTHHMMSS` timestamp makes
  lexicographic order chronological at the filename's second
  granularity, with no file-stat needed.
- Deletion is best-effort. A failure to remove an older file does not
  fail the agent run, emit an event, or alter the `logPath` or error
  `PiAgent.Run` returns. Rotation is observability hygiene, not
  correctness — consistent with the existing "JSONL is observability,
  not data" stance.
- The retention count is a fixed implementation constant (5) and is not
  operator-configurable.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `watcher`: the per-invocation log lifecycle (create, distinct-name,
  log-path event) gains a bounded-retention requirement that caps the
  per-stem file count after each run. The batch-level event log is out
  of scope.

## Impact

- `eventlog.go`: a `rotateLogs(dir, stem string, keep int)` helper next
  to `logFilename`, plus a `maxInvocLogsPerStem = 5` constant. It lists
  the log directory, selects names matching the exact prefix
  `<stem>--` and the `.jsonl` suffix, sorts them descending, and
  deletes every file past `keep`. Each deletion error is swallowed.
- `main.go`: `PiAgent.Run` is restructured so the per-invocation file is
  closed before rotation runs (the just-written file is never deleted
  while open), then returns the captured `(logPath, err)`. The stem is
  computed once and shared by `pathFor` and `rotateLogs` so the two
  cannot drift.
- `AGENTS.md`: a "Log retention" note under the `log_dir` section
  documents the per-stem cap of 5, the per-stem grouping, the
  best-effort deletion, and that the batch-level event log is not
  bounded.
- No configuration changes (the count is a hardcoded constant). No new
  dependencies. No breaking changes: a stem with five or fewer files is
  a no-op, and existing on-disk logs are never migrated or deleted en
  masse — only the oldest excess of a stem that just grew past the cap.

### Non-goals

- Bounding the batch-level `see--<ts>--<pid>.jsonl` event log. That is
  a separate stream with different growth characteristics (one per
  process restart) and is left for a future change if it becomes a
  problem.
- Making the retention count operator-configurable. Hardcoded now; add a
  `log_keep` field only when a real need appears.
