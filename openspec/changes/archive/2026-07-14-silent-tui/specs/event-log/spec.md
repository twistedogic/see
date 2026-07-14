## ADDED Requirements

### Requirement: see writes a batch-level JSONL event stream
When `see` starts, after the run mode has been resolved but before
the watcher begins, `see` SHALL create one JavaScript Object
Notation Lines (JSONL) file at
`$SEE_LOG_DIR/see--<utc-timestamp>--<pid>.jsonl` (or
`os.UserCacheDir()/see/logs/see--<utc-timestamp>--<pid>.jsonl` if
`SEE_LOG_DIR` is unset). Every `Event` value the watcher emits
SHALL be JSON-encoded and written as one line to that file, in
the order the events fire, before the file is flushed for that
event.

#### Scenario: One file per batch
- **WHEN** `see` is invoked twice in sequence from the same
  process identifier (PID)
- **THEN** two distinct JSONL files exist under the log directory
- **AND** each file's name encodes a unique UTC timestamp

#### Scenario: Every watcher event lands in the JSONL
- **WHEN** `Watcher.runOnce` runs against one repo with one active
  change and the agent succeeds
- **THEN** the JSONL contains, in order, `RepoSeen`,
  `ChangeStarted`, `LogPath`, `ChangeDone`

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
