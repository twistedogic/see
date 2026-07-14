## MODIFIED Requirements

### Requirement: PiAgent writes agent output to a JSONL file per run
When `PiAgent.Run` is invoked, it SHALL create a `.jsonl` file in
the configured log directory and redirect the agent's combined
stdout and stderr to that file for the duration of the call. The
file SHALL be closed before `Run` returns. The log directory is
guaranteed to exist and be writable by the time `Run` is called
(`ensureLogDir` in `main()`); `Run` SHALL NOT attempt to create
the directory and SHALL NOT fall back to running the agent
without capture. If the per-run file cannot be created,
`PiAgent.Run` SHALL return a non-empty `logPath` and a non-nil
error describing the file-creation failure; the watcher SHALL
surface this as a `ChangeFailed` event for the same invocation.

#### Scenario: Successful run produces a populated file
- **WHEN** `PiAgent.Run` completes successfully against an agent that
  writes to both stdout and stderr
- **THEN** a `.jsonl` file exists at the computed path
- **AND** the file contains the agent's combined stdout and stderr
  output, byte-for-byte
- **AND** `PiAgent.Run` returns a non-empty `logPath` and a nil
  error

#### Scenario: Per-run file creation failure surfaces as a Run error
- **WHEN** the log directory exists but the per-run file cannot be
  created (permission denied, disk full)
- **THEN** `PiAgent.Run` returns a non-nil error describing the
  file-creation failure
- **AND** `PiAgent.Run` does not invoke the agent
- **AND** the returned `logPath` is empty

### Requirement: Watcher emits Warning events for cleanup-step failures
When `Watcher.work` performs a rollback, completion, or pre-run
check step that fails but is not itself the reason `work` returns
an error, `Watcher.work` SHALL emit a `Warning` event with the
repo path, change name, and the step's failure message. The
warning SHALL be emitted in addition to whatever boundary event
(`ChangeFailed`, `ChangeDone`, or none for a no-op) the work
function emits; the warning SHALL NOT replace the boundary event
or alter the error returned by `work`.

The pre-run check that emits a Warning is the detached-HEAD check:
when `git symbolic-ref --short HEAD` returns empty,
`Watcher.work` SHALL emit a `Warning` event naming the repo and
SHALL return a `detached HEAD` error.

The rollback and completion steps that emit Warning when they fail
are:

- `git switch` back to the original branch ref
- `git reset --hard <captured-SHA>` after the switch
- `git branch -D <branch>` to clean up the per-change branch
- `git add -A` after a successful agent run
- `git commit` after `git add -A`
- `git merge --no-ff <branch>` to merge the per-change branch back
- `git merge --abort` when a merge conflict is detected
- `git branch -d <branch>` after a successful merge

#### Scenario: Rollback git switch failure emits a Warning
- **WHEN** the agent errors and the subsequent `git switch` back
  to the original ref fails
- **THEN** `Watcher.work` emits a `Warning` event naming the
  switch failure
- **AND** `Watcher.work` returns the original agent error

#### Scenario: Detached HEAD emits a Warning and returns an error
- **WHEN** `Watcher.work` is invoked on a repo with HEAD pointing
  directly at a commit (no current branch)
- **THEN** `Watcher.work` emits a `Warning` event naming the
  repo and the detached-HEAD condition
- **AND** `Watcher.work` returns a `detached HEAD` error before
  any branch mutation

### Requirement: Watcher surfaces the log path via LogPath event in both modes
When `Watcher.work` invokes `agent.Run` and `agent.Run` returns a
non-empty `logPath`, `Watcher.work` SHALL emit a `LogPath` event
with the file path before returning, regardless of whether an
observer was wired at construction time. In TUI mode the event is
forwarded to the bubbletea observer; in log mode it is written to
the batch-level JSONL. If `agent.Run` returns an empty `logPath`
(capture failure, which now propagates as a `ChangeFailed`), no
`LogPath` event is emitted for that invocation.

#### Scenario: Successful capture emits LogPath in both modes
- **WHEN** `PiAgent.Run` returns a non-empty `logPath` in either
  mode
- **THEN** the observer receives a `LogPath` event whose `Path`
  field equals the file path
- **AND** in log mode the event is written to the JSONL even
  though no TUI observer is wired

#### Scenario: Capture failure emits no LogPath and propagates as ChangeFailed
- **WHEN** `PiAgent.Run` returns an empty `logPath` and a non-nil
  error
- **THEN** `Watcher.work` does not emit a `LogPath` event for
  that invocation
- **AND** `Watcher.work` emits a `ChangeFailed` event carrying
  the file-creation error

## REMOVED Requirements

### Requirement: Run proceeds when log directory cannot be created
**Reason**: The graceful-degradation path (log warning + run
without capture) is removed. The log directory is now validated
up front by `ensureLogDir` in `main()`; if `MkdirAll` fails the
process exits `2` before the watcher starts. Per-run code in
`PiAgent.Run` assumes the directory exists and writes its
warning-free error path through `ChangeFailed` instead.

**Migration**: None required for callers — operators who hit a
broken `SEE_LOG_DIR` now see a single stderr line and exit `2`
instead of a per-run warning. The behaviour of a successful run
is unchanged.

### Requirement: Run proceeds when log file cannot be opened
**Reason**: Same as above. With the directory guaranteed to
exist, file-creation failures are rare and indicate a real
problem; running the agent without capture loses the agent's
output silently. The new behaviour propagates the failure via
`ChangeFailed`.

**Migration**: Same as above.

### Requirement: Log mode prints the log path to stderr
**Reason**: `--mode=log` is silent. The path is discoverable via
the `LogPath` event in the batch-level JSONL.

**Migration**: Operators who parsed this stderr line must switch
to reading `LogPath` events from the JSONL.
