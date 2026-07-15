# watcher

## Purpose

Define how `Watcher` runs an agent against an active openspec change without
disturbing the original branch. The watcher pins every agent run to a
`see/<change>` branch, rolls back on agent failure, and leaves the working
tree on `see/<change>` at the end of a successful run so promotion to the
user's branch of choice is an explicit operator step.
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

### Requirement: Watcher leaves HEAD on the per-change branch after a successful run
When the agent returns nil and the change is archived, `Watcher.work`
SHALL run `git add -A` and `git commit -m "see: apply openspec change
<change>"` on `see/<change>` so that any files the agent left dirty are
absorbed into a single `see`-owned commit. `Watcher.work` SHALL then emit
`ChangeDone` and return nil. `Watcher.work` SHALL NOT switch back to the
original branch ref, SHALL NOT run `git merge --no-ff see/<change>`, and
SHALL NOT delete `see/<change>` via `git branch -d` on this code path.
The original branch ref captured at the start of `work` is not modified
by this success path. The rollback path on agent failure is unaffected
and is governed by the existing "Watcher rolls back the branch on agent
failure" Requirement.

#### Scenario: Successful run leaves HEAD on the workspace branch
- **WHEN** `Watcher.work` started on `main` with SHA `A`, the agent
  succeeded, and the change is archived
- **THEN** the working tree is checked out on `see/<change>` when
  `Watcher.work` returns
- **THEN** `see/<change>` is not deleted by `work`
- **THEN** the catch-up commit's subject
  `see: apply openspec change <change>` is reachable from
  `see/<change>`'s tip

#### Scenario: Original branch tip is unchanged on success
- **WHEN** `Watcher.work` started on `main` with SHA `A`, the agent
  succeeded, and the change is archived
- **THEN** `main` is at SHA `A` after `work` returns (no merge, no
  commit on `main`)
- **THEN** the agent's commits are reachable from `see/<change>` but
  not from `main`

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
- **THEN** the existing per-pass requirements (branch creation,
  rollback, leave-HEAD-on-workspace-branch) still apply unchanged to
  that pass

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

### Requirement: Watcher's retry loop returns the error from the final attempt
`Watcher.runOnce` SHALL retry `Watcher.work` for a given repo up to
the count passed to the `Watcher` constructor (formerly known as
`RetyCount`, renamed to `RetryCount`). If any attempt returns a nil
error, the loop SHALL stop and `runOnce` SHALL move to the next
repo. If every attempt returns a non-nil error, the loop SHALL
return the error from the final attempt. The loop SHALL emit a
`RetryAttempt` event before each retry after the first attempt.

#### Scenario: Succeeds on the first attempt
- **WHEN** `Watcher.work` returns nil on the first call
- **THEN** the retry loop does not invoke `work` again
- **THEN** the loop returns nil

#### Scenario: Succeeds on a later attempt
- **WHEN** `Watcher.work` returns `err1` then `err2` then nil
- **THEN** the loop returns nil after the third call

#### Scenario: Exhausts retries with errors
- **WHEN** `Watcher.work` returns `err1`, `err2`, `err3` over
  three calls
- **THEN** the loop returns `err3` after the third call

#### Scenario: Zero retries is a no-op
- **WHEN** the watcher is constructed with a retry count of `0`
- **THEN** `work` is not invoked
- **THEN** the loop returns nil
  *(ponytail: documented ceiling — a `-retry 0` misconfiguration
  silently succeeds. If this becomes load-bearing, add an
  explicit guard.)*


### Requirement: Discovery resolves the watch list from layered sources

`see` SHALL resolve the list of repos to watch from three
sources, in order of decreasing precedence:

1. The repeatable `--watch <pattern>` flag.
2. The contents of the config file at
   `$XDG_CONFIG_HOME/see/watches` (defaulting to
   `~/.config/see/watches` when `XDG_CONFIG_HOME` is
   unset), unless `--ignore-config` is set.
3. The current working directory as a fallback when
   steps 1 and 2 produce no entries.

The flag entries and the config entries are unioned, not
replaced: an operator may pass `--watch` to add a single
repo to whatever the config already lists.

#### Scenario: No flag, no config falls back to cwd

- **WHEN** `see` is invoked with no `--watch`, the config
  file is not present, and the working directory contains
  two git repos as immediate subdirectories
- **THEN** the resolved watch list contains both repos
- **AND** neither the batch-level JavaScript Object
  Notation Lines (JSONL) stream nor the Terminal User
  Interface (TUI) reflects any new source

#### Scenario: Flag adds to config

- **WHEN** `see --watch /extra/repo` is invoked with a
  config file listing `~/work/*`
- **THEN** the resolved watch list contains every match
  of `~/work/*` plus `/extra/repo`
- **AND** duplicates between the two sources collapse to
  a single entry

#### Scenario: --ignore-config skips the config layer

- **WHEN** `see --ignore-config --watch ~/only/repo` is
  invoked with a config file listing `~/other/repo`
- **THEN** the resolved watch list contains only
  `~/only/repo`
- **AND** the config file's contents are not consulted

#### Scenario: Missing config file is not an error

- **WHEN** the config file does not exist
- **THEN** the resolution proceeds with the flag entries
  and the cwd fallback as if the file were empty
- **AND** no error is returned

#### Scenario: Malformed config line is fatal at startup

- **WHEN** the config file contains a line that fails
  read, parse, or tilde expansion
- **THEN** `see` prints a single message naming the
  offending line number and exits with status `2`
- **AND** the watcher does not start

### Requirement: Discovery patterns expand paths and shell-globs

Patterns SHALL expand in two ways:

- A leading `~` in any pattern SHALL be replaced with
  the user's home directory (`$HOME` when set, otherwise
  the result of `os.UserHomeDir()`).
- Shell-glob metacharacters in any pattern (`*`, `?`,
  `[abc]`) SHALL be expanded via `filepath.Match`
  against the filesystem. Patterns with no
  metacharacters SHALL be treated as literal paths.
- Environment-variable expansion (`$VAR`) SHALL NOT be
  performed.
- A pattern containing `**` SHALL be rejected at
  startup with a clear error message, because
  `filepath.Match` does not support recursive globs and
  the project chooses not to add a dependency for the
  feature.

#### Scenario: Literal path

- **WHEN** a pattern is `--watch /repos/myrepo` and the
  path exists
- **THEN** the pattern contributes `/repos/myrepo` to
  the resolved list (subject to dedupe)

#### Scenario: Tilde expansion

- **WHEN** the home directory is `/home/alice` and a
  pattern is `--watch ~/work`
- **THEN** the pattern contributes `/home/alice/work`
  to the resolved list

#### Scenario: Shell-glob expansion

- **WHEN** the home directory is `/home/alice` and a
  pattern is `--watch "~/work/*"` and `~/work` contains
  immediate subdirectories `repoA` and `repoB`
- **THEN** the pattern contributes
  `/home/alice/work/repoA` and `/home/alice/work/repoB`
  to the resolved list

#### Scenario: Glob with no matches emits a Warning

- **WHEN** a pattern is `--watch "~/work/*"` and
  `~/work` contains no immediate subdirectories
- **THEN** a `Warning` event is emitted naming the
  pattern
- **AND** the batch continues with the remaining entries

#### Scenario: **-containing pattern is rejected

- **WHEN** a pattern contains `**`
- **THEN** `see` prints an error explaining that `**`
  is not supported, points at alternatives (multiple
  patterns or `find … -path`), and exits with status
  `2`

### Requirement: Discovery classifies each entry as a repo or a parent-of-repos

Each resolved entry SHALL be classified by stat'ing the
entry itself:

- An entry whose root contains a `.git` file or
  directory SHALL be treated as a single repo.
- An entry whose root is a directory and does not
  contain `.git` SHALL be treated as a parent-of-repos;
  each immediate child with `.git` SHALL be added to
  the resolved list.
- An entry that does not exist, is a regular file, or
  is a parent-of-repos with no `.git` children SHALL
  emit a `Warning` event and be skipped.

#### Scenario: Entry is a single repo

- **WHEN** `--watch /repos/myrepo` resolves to a path
  with `.git/`
- **THEN** the resolved list contains `/repos/myrepo`
- **AND** `Watcher.work` runs against it directly

#### Scenario: Entry is a parent-of-repos

- **WHEN** `--watch /work` resolves to a directory with
  immediate `.git/` children `repoA` and `repoB`
- **THEN** the resolved list contains `/work/repoA` and
  `/work/repoB`
- **AND** `Watcher.work` runs against each

#### Scenario: Entry is a parent with no repo children

- **WHEN** `--watch /work` resolves to a directory with
  no `.git/` children
- **THEN** a `Warning` event is emitted naming the path
- **AND** the batch continues without that entry

#### Scenario: Entry is missing

- **WHEN** `--watch /nonexistent` resolves to a path
  that does not exist
- **THEN** a `Warning` event is emitted naming the path
- **AND** the batch continues without that entry

### Requirement: Discovery dedupes and sorts the watch list

After all entries are resolved and classified, the final
watch list SHALL be deduplicated by absolute path and
sorted in ascending path order. Two entries that resolve
to the same absolute path (for example a literal config
entry and a glob match that includes the same repo)
SHALL appear exactly once.

#### Scenario: Overlapping sources collapse

- **WHEN** the flag list contains `/abs/path/repo` and
  the config contains `~/work/*` where `~/work/repo`
  resolves to the same absolute path
- **THEN** the final watch list contains
  `/abs/path/repo` exactly once

#### Scenario: Stable ordering for the TUI

- **WHEN** the resolved list contains
  `/repos/zeta`, `/repos/alpha`, and `/repos/mu`
- **THEN** the final list is ordered
  `/repos/alpha`, `/repos/mu`, `/repos/zeta`
- **AND** the Terminal User Interface (TUI) renders the
  same order on every scan

### Requirement: Watcher iterates the resolved watch list

`Watcher.Watch` SHALL accept a `repos []string`
argument instead of a `wd string` argument.
`Watcher.runOnce` SHALL iterate `repos` directly,
applying the per-repo processing (retry, agent
invocation, event emission) that it applies today. The
watcher's discovery responsibilities
(`os.ReadDir`, repo classification, deduplication)
SHALL move to the new discovery layer in `main`.

#### Scenario: Watcher sees only what the discovery layer resolved

- **WHEN** `Watcher.Watch(ctx, repos)` is invoked with
  a slice of three absolute paths
- **THEN** `Watcher.runOnce` runs against exactly those
  three paths
- **AND** `Watcher.runOnce` does not call `os.ReadDir`
  on any directory

#### Scenario: Watcher's per-repo contract is unchanged

- **WHEN** `Watcher.Watch(ctx, repos)` is invoked with
  a slice of paths
- **THEN** the existing per-repo requirements (branch
  creation, rollback, merge, retry, agent invocation,
  event emission) still apply to each entry
- **AND** the Terminal User Interface (TUI) and the
  batch-level JSONL event stream observe the same
  events per repo as before the change
