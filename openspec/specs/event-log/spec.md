# event-log

## Purpose

Define the batch-level JavaScript Object Notation Lines (JSONL)
event log that `see` writes for every invocation, the
`ensureLogDir` startup check that guarantees the log directory
exists before the watcher begins, and the `Warning` /
`InfraError` event shapes the watcher emits when a per-repo
cleanup step hiccups or a process-level failure surfaces. The
JSONL is the source of truth for the event timeline; the Terminal
User Interface (TUI) is a pure view of the same stream.
## Requirements
### Requirement: see writes a batch-level JSONL event stream
When `see` starts, after the run mode has been resolved but before the watcher begins, `see` SHALL create one JavaScript Object Notation Lines (JSONL) file at `$SEE_LOG_DIR/see--<utc-timestamp>--<pid>.jsonl`, or `os.UserCacheDir()/see/logs/see--<utc-timestamp>--<pid>.jsonl` if `SEE_LOG_DIR` is unset. Each `Event` value the watcher emits SHALL be wrapped in an envelope `{"ts": "<RFC3339Nano UTC>", "event": <event-payload>}` and written as one line to that file, in the order the events fire, before the file is flushed for that event. The `ts` field SHALL be the observed-at time the `eventLogger` marshalled the event, not the wall-clock time the watcher created the event struct, expressed as the Go layout `time.RFC3339Nano` formatted on a value in Coordinated Universal Time (UTC). Under concurrent producers this keeps line ordering monotonic against the writer's wall clock rather than a producer's wall clock.

The envelope SHALL wrap without otherwise modifying the underlying event payload. `RepoSeen` SHALL carry `Path` and the workflow-neutral `HasChange` boolean. The previous `HasOpenspec` field SHALL NOT be emitted. For example, `RepoSeen{Path: "/x", HasChange: true}` SHALL marshal as `{"ts":"<rfc3339nano>","event":{"Path":"/x","HasChange":true}}`.

#### Scenario: One file per batch
- **WHEN** `see` is invoked twice in sequence from the same process identifier (PID)
- **THEN** two distinct JSONL files exist under the log directory
- **AND** each file's name encodes a unique UTC timestamp

#### Scenario: Every watcher event lands in the JSONL
- **WHEN** `Watcher.runOnce` runs against one repo with one resolved change and the agent succeeds
- **THEN** the JSONL contains, in order, `RepoSeen`, `ChangeStarted`, `LogPath`, and `ChangeDone`
- **THEN** every line is valid JSON encoding of `{"ts": <rfc3339nano-string>, "event": <event-payload>}`
- **AND** the inner `event` payload round-trips the original `Event` field set without renaming or reordering

#### Scenario: Repository availability is workflow-neutral
- **WHEN** either a custom condition or the OpenSpec compatibility resolver produces a change
- **THEN** the `RepoSeen` event payload contains `HasChange: true`
- **AND** it does not contain a `HasOpenspec` field

#### Scenario: Repository without work reports no change
- **WHEN** the selected resolver produces no change
- **THEN** the `RepoSeen` event payload contains `HasChange: false`
- **AND** it does not contain a `HasOpenspec` field

#### Scenario: Each line carries an RFC3339Nano UTC timestamp
- **WHEN** the JSONL is read with `time.Parse(time.RFC3339Nano, ...)` on every `ts` value
- **THEN** the parse succeeds for every line
- **THEN** the parsed time lies within one second of the wall clock at process exit

#### Scenario: File sink and stdout mirror carry the same envelope
- **WHEN** `see --mode=log` runs with stdout piped and the mirror sink is wired
- **THEN** the JSONL file and stdout stream carry byte-identical lines
- **AND** each line decodes to the same `{ts, event}` envelope on both sinks

#### Scenario: Envelope marshalling failure does not crash the watcher
- **WHEN** the inner payload fails to marshal
- **THEN** the `Observe` call is a no-op for that event
- **THEN** no panic, log line, or process exit occurs
- **AND** the watcher continues to the next event

### Requirement: ensureLogDir exits before the watcher starts
`see` SHALL call `ensureLogDir()` after `selectRunMode` has
resolved but before any watcher goroutine is launched. If the log
directory cannot be created (`MkdirAll` fails), `see` SHALL write
one line to stderr naming the directory it tried to create and
SHALL exit with status `2`. No watcher work SHALL run; no JSONL
SHALL be created.

#### Scenario: Bad SEE_LOG_DIR aborts before watching
- **WHEN** `SEE_LOG_DIR` is set to a path whose parent is a
  regular file (MkdirAll will fail)
- **THEN** `see` exits with status `2` before any goroutine runs
- **THEN** stderr contains a single line naming the directory
- **AND** no JSONL file is created

#### Scenario: Default cache directory is created if missing
- **WHEN** `SEE_LOG_DIR` is unset and `os.UserCacheDir()/see/`
  does not yet exist
- **THEN** `ensureLogDir` creates it
- **THEN** the watcher starts

### Requirement: eventLogger fans out to the TUI observer in TUI mode
The `eventLogger` SHALL accept an optional second observer at
construction. When the second observer is non-nil, every `Observe`
call SHALL write to the JSONL first and THEN forward the same
event to the second observer. When the second observer is nil
(log mode), the JSONL is the only sink.

#### Scenario: TUI mode observes every event the JSONL captures
- **WHEN** `see --mode=tui` runs and the watcher emits
  `ChangeStarted` for a repo
- **THEN** the JSONL contains a `ChangeStarted` line for that
  repo
- **AND** the TUI receives a `ChangeStartedMsg` for that repo on
  the bubbletea program channel

#### Scenario: Log mode does not invoke a TUI observer
- **WHEN** `see --mode=log` runs and the watcher emits
  `ChangeStarted`
- **THEN** the JSONL contains a `ChangeStarted` line
- **AND** no bubbletea program is constructed

### Requirement: Warning event reports per-repo cleanup hiccups
`see` SHALL define a `Warning` event with `Path`, `Change`, and
`Msg` string fields. `Watcher.work` SHALL emit a `Warning` event
whenever a rollback or completion step that is not itself the
reason the work function returns an error reports a failure
(e.g. `git switch` back to the original branch fails, `git reset
--hard` to the captured commit fails, `git branch -D` of the
per-change branch fails, `git add -A` fails, `git commit` fails,
`git merge --no-ff` fails, `git merge --abort` fails, `git branch
-d` after a successful merge fails).

The watcher SHALL still return the original work error in those
cases; the `Warning` event carries the cleanup detail without
replacing the error.

#### Scenario: Rollback git switch failure emits a Warning
- **WHEN** the agent errors and the subsequent `git switch` back
  to the original ref fails
- **THEN** the observer receives a `Warning` event with the
  switch failure message
- **AND** `Watcher.work` returns the original agent error, not
  the switch error

#### Scenario: Detached HEAD emits a Warning before returning
- **WHEN** `Watcher.work` runs against a repo with a detached
  HEAD
- **THEN** the observer receives a `Warning` event with the
  detached-HEAD message
- **AND** `Watcher.work` returns the detached-HEAD error

### Requirement: InfraError event reports process-level failures
`see` SHALL define an `InfraError` event with `Where` and `Err`
string fields. `runTUI` SHALL emit an `InfraError` event when:

- `Watcher.Watch` returns a non-nil error (the watcher goroutine
  surfaced a failure).
- The bubbletea `Program.Run()` returns a non-nil error.

The `Where` field SHALL be one of `watcher` or `tui` to identify
the source.

#### Scenario: Watcher error becomes an InfraError event
- **WHEN** the watcher goroutine returns a non-nil error in TUI
  mode
- **THEN** the JSONL contains an `InfraError` line with
  `Where: "watcher"`
- **AND** the TUI's bubbletea model receives an `InfraErrorMsg`
  before quitting

#### Scenario: Bubbletea error becomes an InfraError event
- **WHEN** `prog.Run()` returns a non-nil error
- **THEN** the JSONL contains an `InfraError` line with
  `Where: "tui"`

### Requirement: No output to stdout or stderr while a mode is active
After the run mode has been resolved and before the process exits,
`see` SHALL NOT write to stdout or stderr in either mode. The
JSONL file and the TUI alternate screen are the only output
surfaces. The pre-mode-resolution startup path (flag parsing, run
mode resolution, `os.Getwd` failure, `ensureLogDir` failure)
remains permitted to write to stderr; those writes happen before
any mode is active.

#### Scenario: TUI mode writes nothing to stdout or stderr
- **WHEN** `see --mode=tui` runs against a working directory
  with one repo that has one active change and the agent succeeds
- **THEN** stdout is empty for the lifetime of the process
- **AND** stderr is empty after `ensureLogDir` returns

#### Scenario: Log mode writes nothing to stdout or stderr
- **WHEN** `see --mode=log` runs against the same fixture
- **THEN** stdout is empty
- **AND** stderr is empty after `ensureLogDir` returns
- **AND** the JSONL contains the full event timeline

