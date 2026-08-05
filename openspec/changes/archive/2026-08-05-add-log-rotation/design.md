# Design — add-log-rotation

## The seam

Rotation is a post-write lifecycle step on the per-invocation log file,
inserted into the single `PiAgent.Run` implementation after the agent
command has finished and the file is closed. There is exactly one
implementation of `Agent.Run` (`PiAgent`) and one funnel that calls it
(`Watcher.runAgent`, a one-line pass-through), so the trigger is a
single site with no branching across modes:

```
PiAgent.Run:
   create <logDir>/<stem>--<ts>--<pid>.jsonl
   run agent → file gets stdout+stderr   (success or failure)
   close the file
   rotateLogs(logDir, stem, maxInvocLogsPerStem)   ← new
   return (logPath, runErr)
```

Rotation does **not** belong in `Watcher.runAgent` even though that is
the single caller, because `runAgent` is also the path the test
`fakeAgent` takes, and fakes do not write real log files. Putting the
trigger on the concrete `PiAgent.Run` ensures rotation only ever touches
real on-disk logs.

## Why rotation must fire per run, not per startup

The watch loop never restarts: `Watcher.Watch` runs one `runOnce`
immediately, then sleeps `PollInterval` (default five minutes) until the
context is cancelled. Within a single long-lived process, every polling
pass that finds work creates a new per-invocation file, forever:

```
   poll 1 ─▶ run ─▶ file
   poll 2 ─▶ run ─▶ file      same process, same stem,
   poll 3 ─▶ run ─▶ file      no restart to trigger cleanup ─▶
   ...                        unbounded growth
   poll N ─▶ run ─▶ file
```

Startup-only rotation (the cheap option most tools reach for) is
therefore insufficient: it bounds nothing for a process that lives for
days. Rotation must run after each write. The cost is one directory read
per agent run, which is negligible against a per-agent-run that already
spawns a subprocess.

## The grouping key

The stem is the part of the filename before the
`--<utc-timestamp>--<pid>` suffix. Because `logFilename` joins every
field with `--`, the full name is always
`<repo-basename>--<change-or-digest>--<ts>--<pid>.jsonl`, so the stem is
unambiguously `<repo-basename>--<change-or-digest>` in both modes —
`<change>` in compatibility mode, the SHA-256 `<digest>` in custom mode.
The digest is just the custom-mode spelling of the change identity, so
grouping by stem rotates one logical log stream per `(repository,
change)` pair, independently.

Selection for a stem uses the exact prefix `<stem>--` (note the trailing
`--`) plus the `.jsonl` suffix. The trailing `--` is what makes
grouping robust: a stem `myproj--add` does **not** match files for stem
`myproj--add-dark-mode`, because after `myproj--add` comes `-dark-mode`,
not `--`. The existing `--`-joined naming guarantees this; rotation must
match `stem+"--"`, never bare `stem`, or it would collapse overlapping
stems.

The batch-level event log `see--<ts>--<pid>.jsonl` is structurally
excluded: it has a three-component shape, never matches any
per-invocation `<stem>--` prefix, and is out of scope regardless.

## Recency without stat

The timestamp embedded in the name is fixed-width
`YYYYMMDDTHHMMSS`, so sorting matched filenames in descending
lexicographic order is a chronological sort at the filename's second
granularity — no `os.Stat`/`mtime` calls. Ties within the same second
(by a different process identifier) resolve arbitrarily among
themselves, which is irrelevant at a retention window of five.
Filesystem `mtime` would add stat calls for no gain and can be disturbed
by external tooling; the embedded creation timestamp is authoritative.

## Failure handling

Rotation is best-effort, matching the rest of the log machinery
("JSONL is observability, not data"). A failure to remove an older file
is swallowed: it does not fail the agent run, does not emit a `Warning`
event, and does not change what `PiAgent.Run` returns. The newest files
up to the retention count are retained even if a deletion fails; a
stranded older file simply waits for the next rotation pass to try
again.

## Concurrency

`see` runs as a singleton: one process per machine, one agent run at a
time per repo. The just-written file is the newest for its stem and is
never a deletion candidate, so rotating before or after `f.Close()` is
equivalent in the common case. The implementation closes the file
before rotating anyway, so even the theoretical cross-process case
(two `see` processes, same stem) cannot delete a file that is still
being written. No locking is added; a per-stem lock would be complexity
for a race the singleton model already excludes.

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Grouping unit | per stem `<repo>--<change/digest>` | the natural unit the filename already encodes; different changes do not compete for each other's slots |
| When to rotate | after each `PiAgent.Run`, file closed | only placement that bounds a never-restarting loop |
| Which population | per-invocation only | matches "per repo"; the batch event log is a separate stream |
| Retention count | hardcoded `5` | add a `log_keep` knob only when a real need appears |
| Recency | lexicographic filename sort, descending | fixed-width timestamp → chronological, zero stat |
| Failures | best-effort, swallow | observability, not correctness |

## Alternatives considered

- **Per-repo basename grouping (rejected).** Keep the newest 5 across
  *all* changes/workflows for a repository, so a repo juggling three
  active changes competes for five total slots. Rejected because the
  per-change stream is the logical log unit the filename already
  encodes; per-stem rotation preserves each change's recent history
  independently and needs no naming change.
- **Startup-only rotation (rejected).** Rotate once when `see` starts.
  Cheapest, but the watch loop never restarts, so it bounds nothing for
  a long-lived process — the exact growth this change exists to stop.
- **Filesystem mtime for recency (rejected).** `mtime` is more precise
  than the filename's second granularity but adds a stat per file and
  is mutable by external tooling. The embedded creation timestamp is
  authoritative and stat-free.
- **Configurable retention count (rejected, deferred).** A `log_keep`
  field now is speculative; the constant is fine until an environment
  needs a different value.
- **Bounding the batch-level event log too (rejected, deferred).** It
  grows on process restart, a different cadence from the per-run files.
  Bundling it would couple two unrelated growth problems; a future
  change can address it on its own terms.
