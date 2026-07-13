# watcher

## Purpose

Define how `Watcher` runs an agent against an active openspec change without
disturbing the original branch. The watcher pins every agent run to a
`see/<change>` branch, rolls back on agent failure, and merges the result
back via a `--no-ff` merge commit so the audit trail reflects the watcher's
role.
## Requirements
### Requirement: Watcher creates a per-change branch before running the agent
When `Watcher.work` begins processing an active change, it SHALL capture
the current commit SHA and the original branch ref (the symbolic-ref
short name) at the same moment, then create or reuse a branch named
`see/<change>`. After the branch exists, `Watcher.work` SHALL pin the
branch tip to the captured SHA via `git reset --hard <sha>` so the agent
always starts from a known state, regardless of any state the reused
branch may have been in.

#### Scenario: First run on a clean repo
- **WHEN** `Watcher.work` runs against a repo on branch `main` with one
  commit and an active change `task-1`
- **THEN** a branch `see/task-1` exists in the repo before the agent runs
- **THEN** the working tree is checked out on `see/task-1` when the agent
  begins
- **THEN** the tip of `see/task-1` is the captured SHA

#### Scenario: Re-run reuses an existing branch
- **WHEN** `Watcher.work` runs against a repo that already has a
  `see/<change>` branch from a previous run
- **THEN** `Watcher.work` switches to the existing branch instead of
  erroring
- **THEN** the branch tip is reset to the captured SHA before the agent
  runs (any extra commits from prior runs are discarded)

#### Scenario: Reused branch with drifted tip
- **WHEN** `see/<change>` exists but its tip is not the captured SHA
  (descendant, or unrelated commit)
- **THEN** `Watcher.work` switches to the branch, resets it to the
  captured SHA, and proceeds as if the branch had been created fresh
  from that SHA

### Requirement: Watcher rolls back the branch on agent failure
When the agent returns a non-nil error, `Watcher.work` SHALL restore the
repo to its pre-run state: switch back to the original branch ref, reset
hard to the captured commit SHA, delete `see/<change>`, and return the
agent error. On a repo that started on a detached HEAD the rollback SHALL
use `git switch --detach <sha>` instead of switching to a branch.

#### Scenario: Agent fails on a branched repo
- **WHEN** `Watcher.work` started on `main` at SHA `A` and the agent
  errors after creating dirty edits and one commit on `see/<change>`
- **THEN** the working tree is on `main` after rollback
- **THEN** `main` is reset to SHA `A` (no merge, no extra commits)
- **THEN** `see/<change>` no longer exists
- **THEN** `Watcher.work` returns the agent error

#### Scenario: Agent fails on a detached-HEAD repo is unsupported
- **WHEN** `Watcher.work` runs against a repo on a detached HEAD
- **THEN** `Watcher.work` returns an error before any branch mutation
- **THEN** no `see/<change>` branch is created
- **THEN** the working tree is unchanged

### Requirement: Watcher merges the agent's commit back on success
When the agent returns nil and the change is archived, `Watcher.work`
SHALL run `git add -A` and `git commit` on `see/<change>`, then switch
back to the original branch ref and merge `see/<change>` back with
`git merge --no-ff -m "see: merge openspec change <change>"`. After the
merge, `Watcher.work` SHALL delete `see/<change>` via `git branch -d`
(safe-delete; the reflog keeps the branch recoverable if the merge was
not fully completed for any reason).

#### Scenario: Successful run produces a merge commit on the original branch
- **WHEN** `Watcher.work` started on `main`, the agent succeeded, and the
  change is archived
- **THEN** `main` contains a new merge commit with subject
  `see: merge openspec change <change>`
- **THEN** `see/<change>` is deleted
- **THEN** the working tree is checked out on `main`

#### Scenario: Merge conflict is treated as failure
- **WHEN** the original branch has moved such that `git merge --no-ff
  see/<change>` reports a conflict
- **THEN** `Watcher.work` aborts the merge (`git merge --abort`),
  switches back to the original branch, resets hard to the captured SHA,
  deletes `see/<change>`, and returns the merge error

### Requirement: Watcher refuses detached HEAD at run start
`Watcher.work` SHALL treat a detached HEAD as an unsupported configuration
for v1. When `git symbolic-ref --short HEAD` returns empty, the watcher
SHALL log a clear error message and return without creating any branch.

#### Scenario: Detached HEAD returns an error
- **WHEN** `Watcher.work` is invoked on a repo with HEAD pointing directly
  at a commit (no current branch)
- **THEN** `Watcher.work` returns an error and the repo state is
  unchanged

### Requirement: Watcher runs only one pass when Once is true
When `Watcher.Once` is true, `Watcher.Watch` SHALL return after the first
call to `runOnce` completes, whether that call returned an error or nil.
When `Watcher.Once` is false (the default), `Watcher.Watch` SHALL keep
calling `runOnce` in a loop until the supplied `context.Context` is
cancelled or `runOnce` returns a non-nil error; in either termination
case `Watcher.Watch` SHALL return.

#### Scenario: Once mode returns after a successful pass
- **WHEN** `Watcher.Once` is true and `runOnce` returns nil
- **THEN** `Watcher.Watch` returns nil after that single pass
- **THEN** `Watcher.Watch` does not call `runOnce` again

#### Scenario: Once mode returns after a failed pass
- **WHEN** `Watcher.Once` is true and `runOnce` returns a non-nil error
- **THEN** `Watcher.Watch` returns that error
- **THEN** `Watcher.Watch` does not call `runOnce` again

#### Scenario: Default mode loops until context is cancelled
- **WHEN** `Watcher.Once` is false and the supplied context is cancelled
  before any pass returns an error
- **THEN** `Watcher.Watch` returns nil

#### Scenario: Default mode stops on first pass error
- **WHEN** `Watcher.Once` is false and `runOnce` returns a non-nil error
- **THEN** `Watcher.Watch` returns that error without calling `runOnce`
  again

#### Scenario: Once mode preserves the per-pass contract
- **WHEN** `Watcher.Once` is true and the first pass runs against a repo
  with an active change
- **THEN** the existing per-pass requirements (branch creation, rollback,
  merge) still apply unchanged to that pass

### Requirement: PiAgent writes agent output to a JSONL file per run
When `PiAgent.Run` is invoked, it SHALL create a `.jsonl` file in the
configured log directory and redirect the agent's combined stdout and
stderr to that file for the duration of the call. The file SHALL be
closed before `Run` returns.

#### Scenario: Successful run produces a populated file
- **WHEN** `PiAgent.Run` completes successfully against an agent that
  writes to both stdout and stderr
- **THEN** a `.jsonl` file exists at the computed path
- **AND** the file contains the agent's combined stdout and stderr
  output, byte-for-byte

#### Scenario: Run proceeds when log directory cannot be created
- **WHEN** `os.MkdirAll` of the log directory fails (read-only
  filesystem, invalid `SEE_LOG_DIR`, etc.)
- **THEN** `PiAgent.Run` logs a warning to stderr
- **AND** the agent is invoked without output redirection
- **AND** `PiAgent.Run` returns based solely on the agent's exit
  status (capture failure does not affect the run's outcome)

#### Scenario: Run proceeds when log file cannot be opened
- **WHEN** the log directory exists but the per-run file cannot be
  created (permission denied, disk full)
- **THEN** `PiAgent.Run` logs a warning to stderr
- **AND** the agent is invoked without output redirection
- **AND** the run proceeds normally

### Requirement: Default log location is the OS cache directory
When no `SEE_LOG_DIR` environment variable is set, `PiAgent.Run` SHALL
write log files to `os.UserCacheDir()/see/logs/`. When `SEE_LOG_DIR` is
set to a non-empty string, that string SHALL be used as the log
directory in place of the default.

#### Scenario: No env var uses default
- **WHEN** `SEE_LOG_DIR` is unset or empty
- **THEN** the log directory is `os.UserCacheDir()/see/logs/`

#### Scenario: Env var overrides default
- **WHEN** `SEE_LOG_DIR` is set to `/var/log/see`
- **THEN** the log directory is `/var/log/see`
- **AND** no `see/logs/` subdirectory is appended

### Requirement: Each agent invocation produces a distinct file
For each invocation of `PiAgent.Run`, the computed log file path SHALL
uniquely identify that invocation within the same process. Within a
single process, two invocations within the same wall-clock second SHALL
still produce distinct filenames (the PID is part of the filename).

#### Scenario: Retries produce separate files
- **WHEN** `Watcher.work` invokes `agent.Run` twice for the same
  `(repo, change)` pair (a retry after a failure)
- **THEN** two distinct log files exist after both invocations
- **AND** the files differ by timestamp (or PID + attempt counter)

### Requirement: TUI mode surfaces the log path via LogPath event
When `Watcher.work` invokes `agent.Run` and capture succeeds, and an
observer is wired (TUI mode), `Watcher.work` SHALL emit a `LogPath`
event with the file path before returning.

#### Scenario: Successful capture emits LogPath
- **WHEN** `PiAgent.Run` writes a log file successfully in TUI mode
- **THEN** the observer receives a `LogPath` event whose `Path` field
  equals the file path

#### Scenario: Failed capture emits no LogPath
- **WHEN** `PiAgent.Run` fails to create the log file
- **THEN** no `LogPath` event is emitted
- **AND** no other observer event compensates (capture is observability,
  not correctness)

### Requirement: Log mode prints the log path to stderr
When `Watcher.work` invokes `agent.Run` and capture succeeds in log
mode (`--mode=log`), `Watcher.work` SHALL print the file path to
stderr via `log.Printf` so non-interactive runs make the path
discoverable.

#### Scenario: Successful capture prints the path
- **WHEN** `PiAgent.Run` writes a log file successfully in log mode
- **THEN** `log.Printf("see: log → %s", path)` is called with the path

#### Scenario: Failed capture prints nothing
- **WHEN** `PiAgent.Run` fails to create the log file in log mode
- **THEN** no path is printed (the warning from the capture-failure
  scenario already went to stderr)

### Requirement: Log filenames encode repo, change, timestamp, and PID
Each log file SHALL be named
`<repo-basename>--<change>--<utc-timestamp>--<pid>.jsonl` where:

- `<repo-basename>` is `filepath.Base(repo)`
- `<change>` is the active change name passed to `Agent.Run`
- `<utc-timestamp>` is the UTC time of the invocation in
  `YYYYMMDDTHHMMSS` format
- `<pid>` is the current process ID

#### Scenario: Filename follows the documented format
- **WHEN** `PiAgent.Run` is invoked at 2026-07-14T15:30:22 UTC for
  repo `/repos/myproj` with change `add-dark-mode` and PID 12345
- **THEN** the file path is
  `<log-dir>/myproj--add-dark-mode--20260714T153022--12345.jsonl`

