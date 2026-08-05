## 1. Rotation helper

- [x] 1.1 Add a `maxInvocLogsPerStem = 5` constant in `eventlog.go`.
- [x] 1.2 Add `rotateLogs(dir, stem string, keep int)` in `eventlog.go` next to `logFilename`: list `dir`, select entries whose name has the exact prefix `stem+"--"` and the `.jsonl` suffix, sort the names descending, and delete every file past index `keep`. Each deletion error is swallowed (best-effort).
- [x] 1.3 Extract the stem computation (`filepath.Base(repo) + "--" + change`) so `pathFor` and `rotateLogs` share one source of truth and cannot drift.

## 2. Trigger in PiAgent.Run

- [x] 2.1 In `main.go` `PiAgent.Run`, restructure the two return paths so the per-invocation file is closed before rotation runs (close explicitly rather than relying on the deferred close for the rotation point), then call `rotateLogs(p.logDir, stem, maxInvocLogsPerStem)`, then return the captured `(logPath, runErr)`.
- [x] 2.2 Ensure rotation runs on both the nil-activity and the activity-parser code paths, and on both agent-success and agent-failure outcomes.
- [x] 2.3 Ensure rotation is skipped on the file-creation-failure path (no file was written, `logPath` is empty).

## 3. Tests

- [x] 3.1 `rotateLogs` unit test: a stem with 7 files is reduced to the 5 newest; the oldest 2 are deleted; a stem with ≤5 is untouched.
- [x] 3.2 Prefix selectivity: a stem `myproj--add` does not delete files for stem `myproj--add-dark-mode`; both groups rotate independently when each overflows.
- [x] 3.3 Batch-log exclusion: `see--<ts>--<pid>.jsonl` files in the same directory are never selected or deleted by rotation.
- [x] 3.4 Recency by filename: files with the same stem but descending timestamps are ordered chronologically by lexicographic name sort (no mtime dependence).
- [x] 3.5 Best-effort deletion: an unwritable/unremovable older file does not fail the run, does not emit an event, and the deletable files are still removed.
- [x] 3.6 `PiAgent.Run` integration: after a run (success and failure), the stem is bounded to 5; the just-written file is retained and is closed before rotation; the run's returned `(logPath, err)` is unchanged by rotation.
- [x] 3.7 Both modes: compatibility-mode `<repo>--<change>` and custom-mode `<repo>--<digest>` stems rotate independently.

## 4. Docs

- [x] 4.1 `AGENTS.md`: add a "Log retention" note under the `log_dir` section — per-invocation logs are bounded to the 5 newest per `<repo>--<change>` stem, rotation runs after each run and is best-effort, and the batch-level event log is not bounded.
- [x] 4.2 Run `openspec validate --change add-log-rotation` and resolve any reported delta issues.