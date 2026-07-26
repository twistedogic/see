# watcher

## Purpose

Define how `Watcher` runs an agent against an active openspec change without
disturbing the original branch. The watcher pins every agent run to a
`see/<change>` branch, rolls back on agent failure, and leaves the working
tree on `see/<change>` at the end of a successful run so promotion to the
user's branch of choice is an explicit operator step.
## Requirements
### Requirement: Watcher selects custom work before the OpenSpec compatibility fallback
The strict configuration schema SHALL accept an optional `workflows` sequence. Each workflow SHALL be a mapping with nonblank, unique `name`, `prompt`, `condition`, and `commit` string fields. A configured workflow SHALL be evaluated independently for every watched repository. The former top-level `prompt`, `condition`, and `commit` fields SHALL be rejected as unknown fields.

When no `workflows` are configured, `see` SHALL retain OpenSpec compatibility behavior: active OpenSpec changes drive work, the embedded prompt is used when no configured prompt applies, archival determines completion, and the default OpenSpec catch-up commit subject is used.

#### Scenario: Configured condition selects custom mode
- **WHEN** configuration contains a nonblank condition
- **THEN** the watcher evaluates that condition in each watched repository
- **AND** it does not inspect `openspec/changes/` to select work

#### Scenario: Missing condition preserves OpenSpec behavior
- **WHEN** configuration omits `condition` or supplies a blank value
- **AND** a repository has an active OpenSpec change `add-dark-mode`
- **THEN** the watcher processes `add-dark-mode` using `see/add-dark-mode`
- **AND** it uses the existing OpenSpec prompt, archival completion check, rollback, and default commit message

#### Scenario: Missing condition with no OpenSpec change is idle
- **WHEN** configuration has no nonblank condition
- **AND** a repository has no active OpenSpec change
- **THEN** the agent is not invoked for that repository

#### Scenario: Complete workflow configuration loads
- **WHEN** configuration supplies two named workflows with nonblank prompt, condition, and commit values
- **THEN** startup succeeds
- **AND** both workflows are available in configuration order

#### Scenario: Duplicate workflow names are rejected
- **WHEN** two workflow entries have the same nonblank name
- **THEN** configuration loading fails
- **AND** the error identifies the duplicate workflow name

#### Scenario: Missing workflow field is rejected
- **WHEN** a workflow omits its prompt, condition, or commit
- **THEN** startup fails before watching
- **AND** the error identifies the workflow and missing field

#### Scenario: Legacy top-level custom fields are rejected
- **WHEN** configuration contains a top-level `condition`, `prompt`, or `commit` field
- **THEN** strict configuration loading fails
- **AND** the error identifies the unsupported field

#### Scenario: Empty workflow configuration preserves compatibility
- **WHEN** configuration omits `workflows`
- **THEN** OpenSpec compatibility discovery remains active
- **AND** no custom workflow condition is invoked

### Requirement: Watcher rejects dirty state before custom branch mutation
Before creating or resuming a custom automation branch, `Watcher` SHALL verify that the current working tree has no tracked or untracked changes. If it is dirty, the attempt SHALL fail before `git switch`, `git reset --hard`, branch creation, or agent invocation. Ignored files SHALL NOT make the working tree dirty for this check.

#### Scenario: Dirty source branch is preserved
- **WHEN** a custom condition resolves work while the repository has an unstaged tracked edit
- **THEN** the attempt fails with an actionable dirty-working-tree error
- **AND** the edit remains unchanged
- **AND** no custom branch is created or switched
- **AND** the agent is not invoked

#### Scenario: Untracked file prevents a custom run
- **WHEN** a custom condition resolves work while the repository has an untracked file
- **THEN** the attempt fails before branch mutation
- **AND** the untracked file remains unchanged

#### Scenario: Ignored file does not prevent a custom run
- **WHEN** a custom condition resolves work while the repository has only ignored files beyond committed state
- **THEN** the watcher may create or resume the custom automation branch

### Requirement: Watcher creates or resumes a persistent custom automation branch
*(Branch mode only — see lane-isolation for the worktree-mode contract.)* For an active workflow change in branch mode, `Watcher` SHALL use a branch named `see/<digest>`, where the digest is derived from the workflow name and normalized change. If the lane does not exist, it SHALL be created at the current commit. If it exists, the watcher SHALL switch to it only when the working tree is clean and SHALL resume its current tip without resetting prior commits. The watcher SHALL permit switching from another clean branch or workflow lane.

#### Scenario: First custom run creates its lane
- **WHEN** custom change `add-dark-mode` resolves to branch `see/<digest>` and that branch does not exist
- **THEN** the branch is created at the captured current commit before the agent runs
- **AND** the agent runs with `see/<digest>` checked out

#### Scenario: Repeated custom run resumes its lane
- **WHEN** `see/<digest>` is checked out with successful commits from an earlier pass
- **AND** the condition emits the same custom change again
- **THEN** the watcher runs the agent from the existing branch tip
- **AND** no earlier commit is reset or deleted

#### Scenario: Existing lane is not reset from another branch
- **WHEN** `see/<digest>` exists but the repository is checked out on `main`
- **AND** the condition resolves the change whose branch is `see/<digest>`
- **THEN** the watcher switches to the lane without resetting prior commits
- **AND** prior commits remain reachable
- **AND** the agent runs

#### Scenario: Distinct workflows have distinct lanes
- **WHEN** two workflows emit the same normalized change
- **THEN** they use different branch names
- **AND** work on one lane cannot reset or delete the other lane

#### Scenario: Clean checkout switches to an existing lane
- **WHEN** a requested workflow lane exists and another branch is checked out with a clean working tree
- **THEN** the watcher switches to the requested lane
- **AND** it preserves the lane's existing commits

#### Scenario: Dirty checkout blocks lane switching
- **WHEN** a requested workflow lane exists and the current checkout has tracked or non-ignored untracked changes
- **THEN** the workflow fails before branch mutation
- **AND** the changes remain unchanged

### Requirement: Watcher rolls back only the failed custom attempt
*(Branch mode only — see lane-isolation for the worktree-mode contract.)* Immediately before invoking an agent in branch mode, `Watcher` SHALL capture the selected workflow lane tip. If the agent fails on an existing lane, the watcher SHALL reset tracked state to that tip, remove non-ignored untracked files created by the attempt, preserve ignored files and earlier lane commits, and leave the clean lane available for subsequent workflows. If the lane was created by the attempt, the watcher SHALL return to the branch that was checked out before that workflow, restore its captured commit, and delete only the new lane.

#### Scenario: Failure on an existing lane preserves history
- **WHEN** an existing custom lane has commits `A`, `B`, and `C`
- **AND** the agent creates edits or commits after `C` and then fails
- **THEN** the lane tip is restored to `C`
- **AND** commits `A`, `B`, and `C` remain reachable from the lane
- **AND** the lane remains checked out
- **AND** the agent error is returned

#### Scenario: Failure removes newly-created lane
- **WHEN** a custom lane is created for the first time and the agent fails
- **THEN** the original branch and captured commit are restored
- **AND** the new custom lane is deleted
- **AND** the agent error is returned

#### Scenario: Agent-created untracked files are removed
- **WHEN** the custom working tree was clean before the agent ran
- **AND** the failing agent creates an untracked non-ignored file
- **THEN** rollback removes that file

#### Scenario: Ignored files survive rollback
- **WHEN** a failing custom agent creates or modifies an ignored file
- **THEN** rollback does not delete that ignored file

#### Scenario: Existing workflow lane preserves history after failure
- **WHEN** an existing workflow lane has prior commits and its agent fails after making changes
- **THEN** the lane is reset to its pre-attempt tip
- **AND** prior commits remain reachable
- **AND** later workflows may run after cleanup

#### Scenario: New workflow lane is removed after failure
- **WHEN** an agent fails during the first attempt on a new workflow lane
- **THEN** the pre-workflow branch and commit are restored
- **AND** only the newly created lane is deleted
- **AND** later workflows may run from the restored clean checkout

### Requirement: Successful custom runs create a catch-up commit only for staged changes
After a custom agent run succeeds, `Watcher` SHALL stage all working-tree changes. It SHALL create a commit with the rendered custom commit message only when the index differs from `HEAD`. When no staged changes remain, including when the agent committed all work itself or made no changes, the watcher SHALL return success without invoking `git commit` and without emitting a no-changes `Warning`. Commits made by the agent SHALL remain intact. The watcher SHALL leave the custom lane checked out in either case.

#### Scenario: Leftover changes receive custom commit
- **WHEN** a custom agent succeeds and leaves tracked or untracked changes
- **THEN** the watcher stages those changes
- **AND** commits them with the rendered custom commit message
- **AND** leaves the custom lane checked out

#### Scenario: Agent committed all changes
- **WHEN** a custom agent succeeds after committing all of its work
- **THEN** the watcher preserves the agent commits
- **AND** creates no additional commit
- **AND** emits no no-changes warning

#### Scenario: Idempotent run is a successful no-op
- **WHEN** a custom agent succeeds without changing the repository
- **THEN** the watcher creates no commit
- **AND** returns success
- **AND** the condition may trigger another run on the next polling pass

### Requirement: Watcher creates a per-change branch before running the agent
*(Branch mode only — see lane-isolation for the worktree-mode contract.)* In OpenSpec compatibility mode running in branch mode, when `Watcher.work` begins processing an active change, it SHALL capture the current commit SHA and original branch ref, then create or reuse `see/<change>`. After the branch exists, compatibility mode SHALL pin its tip to the captured SHA via `git reset --hard <sha>` so the agent starts from known state regardless of prior branch drift.

In custom mode running in branch mode, the watcher SHALL instead follow the persistent custom automation branch requirement in this change: create `see/<digest>` once, resume it only when it is already checked out, and never reset its prior successful commits before an attempt.

#### Scenario: Compatibility first run creates a branch
- **WHEN** compatibility-mode work runs on branch `main` with active OpenSpec change `task-1`
- **THEN** `see/task-1` exists before the agent runs
- **THEN** the working tree is checked out on `see/task-1`
- **THEN** the tip of `see/task-1` is the captured SHA

#### Scenario: Compatibility re-run reuses and resets an existing branch
- **WHEN** compatibility-mode work finds an existing `see/<change>` branch
- **THEN** it switches to the existing branch instead of erroring
- **THEN** it resets the branch tip to the captured SHA before the agent runs

#### Scenario: Compatibility reused branch with drifted tip
- **WHEN** compatibility mode finds `see/<change>` at a descendant or unrelated commit
- **THEN** it switches to the branch, resets it to the captured SHA, and proceeds from that SHA

#### Scenario: Custom re-run preserves an existing lane
- **WHEN** custom mode resolves a change whose hashed branch is already checked out with prior successful commits
- **THEN** the watcher starts the next agent attempt from that branch tip
- **AND** it does not reset or delete the prior successful commits

### Requirement: Watcher leaves HEAD on the per-change branch after a successful run
*(Branch mode only — see lane-isolation for the worktree-mode contract.)* In branch mode, when the agent returns nil and the change is archived, `Watcher.work`
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
*(Branch mode only — see lane-isolation for the worktree-mode contract.)* In OpenSpec compatibility mode running in branch mode, when the agent returns a non-nil error, `Watcher.work` SHALL restore the repository to its pre-run state by switching back to the original branch ref, resetting hard to the captured commit SHA, deleting `see/<change>`, and returning the agent error.

In custom mode running in branch mode, rollback SHALL follow the persistent-lane requirement in this change. A failed attempt on a pre-existing lane SHALL restore that lane to its pre-attempt tip without deleting it. A failed attempt that created a new lane SHALL restore the original branch and delete only that newly-created lane. In both modes, a detached HEAD SHALL remain unsupported and SHALL be rejected before branch mutation.

#### Scenario: Compatibility agent failure deletes the disposable branch
- **WHEN** compatibility-mode work starts on `main` at SHA `A` and the agent errors after editing or committing on `see/<change>`
- **THEN** the working tree is on `main` after rollback
- **THEN** `main` is reset to SHA `A`
- **THEN** `see/<change>` no longer exists
- **THEN** `Watcher.work` returns the agent error

#### Scenario: Detached HEAD remains unsupported
- **WHEN** either mode begins on a detached HEAD
- **THEN** `Watcher.work` returns an error before branch mutation
- **THEN** the working tree is unchanged

#### Scenario: Existing custom lane survives failure
- **WHEN** a custom agent fails while running on a lane that existed before the attempt
- **THEN** the lane is restored to its captured pre-attempt tip
- **AND** the lane is not deleted
- **AND** it remains checked out

#### Scenario: Newly-created custom lane is removed on failure
- **WHEN** a custom agent fails during the first attempt on a newly-created lane
- **THEN** the original branch and commit are restored
- **AND** only the newly-created lane is deleted

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
Each compatibility-mode log file SHALL retain the existing name `<repo-basename>--<change>--<utc-timestamp>--<pid>.jsonl`. Each custom-mode log file SHALL be named `<repo-basename>--<digest>--<utc-timestamp>--<pid>.jsonl`, where `<digest>` is the full lowercase hexadecimal Secure Hash Algorithm 256-bit (SHA-256) digest of the normalized change. In both forms:

- `<repo-basename>` is `filepath.Base(repo)`.
- `<utc-timestamp>` is the Coordinated Universal Time (UTC) invocation time in `YYYYMMDDTHHMMSS` format.
- `<pid>` is the current process identifier.

Raw custom condition output SHALL NOT appear in a log path.

#### Scenario: Compatibility filename retains change name
- **WHEN** compatibility mode invokes the agent at 2026-07-14T15:30:22 UTC for repo `/repos/myproj`, OpenSpec change `add-dark-mode`, and PID 12345
- **THEN** the file path is `<log-dir>/myproj--add-dark-mode--20260714T153022--12345.jsonl`

#### Scenario: Custom filename uses digest
- **WHEN** custom mode invokes the agent for normalized change `add-dark-mode`
- **THEN** the filename's change component is the full SHA-256 digest of `add-dark-mode`
- **AND** raw `add-dark-mode` does not appear as that component

#### Scenario: Unsafe custom change cannot escape log directory
- **WHEN** a normalized custom change contains spaces or path traversal characters
- **THEN** the log file remains directly under the configured log directory
- **AND** its change component contains only lowercase hexadecimal digest characters

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

### Requirement: Discovery resolves the watch list from the configured root

`see` SHALL resolve the list of repositories to watch from the
configuration file selected by `--config <path>` (default:
`os.UserConfigDir()/see/config.yaml`), with the current working
directory as a fallback when the configured `root_dir` is blank or
absent. `--config=-` is the escape hatch for the case the precedence
rule does not cover: when the configuration file is malformed and
must not be read at startup. An explicit `--config=<path>` selects a
non-default configuration file. The watch list SHALL be derived from
the configured `root_dir` (after tilde expansion), filtered through
the optional `include` and `exclude` sequences, then handed to the
classification layer.

The `see` binary SHALL NOT accept a `--watch` flag. The
current-working-directory fallback SHALL be the only source consulted
when the configured `root_dir` is blank or absent. There is no
CLI-side precedence layer because there is no CLI watch input.

#### Scenario: No config falls back to cwd

- **WHEN** `see` is invoked with no `--config`, `config.yaml` is
  absent or has a blank `root_dir`, and the working directory
  contains two git repositories as immediate subdirectories
- **THEN** the resolved watch list contains both repositories
- **AND** neither the batch-level JavaScript Object Notation Lines
  (JSONL) stream nor the Terminal User Interface (TUI) reflects any
  new source

#### Scenario: Configured root resolves its children

- **WHEN** `config.yaml` sets `root_dir: "~/Dev"` and `~/Dev`
  contains immediate subdirectories `playground-rust` and `notes`
- **THEN** the resolved watch list contains `~/Dev/playground-rust`
  and `~/Dev/notes` (subject to classification and exclude filtering)

#### Scenario: --config=- skips the config layer

- **WHEN** `see --config=-` is invoked with the default
  `config.yaml` listing `root_dir: "~/Dev"`
- **THEN** the configuration file is not consulted
- **AND** the watch list falls back to the current working directory

#### Scenario: --config=<path> loads the named file

- **WHEN** `see --config=~/team/see.yaml` is invoked with the
  default `config.yaml` listing `root_dir: "~/Dev"` and
  `~/team/see.yaml` listing `root_dir: "~/only"`
- **THEN** the resolved watch list is built from `~/only`
- **AND** the default `config.yaml` is not consulted

#### Scenario: Missing config file is not an error

- **WHEN** `config.yaml` does not exist
- **THEN** resolution proceeds with the current-working-directory
  fallback as if the configuration were empty
- **AND** no error is returned

#### Scenario: Malformed config line is fatal at startup

- **WHEN** the configuration file selected by `--config` cannot be
  read or parsed according to the global configuration schema
- **THEN** `see` prints one actionable error identifying the
  configuration file and exits with status `2`
- **AND** the watcher does not start

### Requirement: Discovery expands `root_dir`, `include`, and `exclude` patterns

`root_dir`, `include`, and `exclude` patterns SHALL expand in three
ways:

- A leading `~` in `root_dir` or any `include` / `exclude` entry
  SHALL be replaced with the user's home directory (`$HOME` when
  set, otherwise the result of `os.UserHomeDir()`).
- `include` entries SHALL be expanded as glob patterns via
  `filepath.Glob` joined to `root_dir`. An `include` sequence that
  is absent or empty SHALL be treated as "every immediate child of
  `root_dir`".
- `exclude` entries SHALL be matched against the basename of each
  candidate via `filepath.Match`. An `exclude` sequence that is
  absent or empty SHALL be treated as "exclude nothing".
- Environment-variable expansion (`$VAR`) SHALL NOT be performed.
- A pattern containing `**` SHALL be rejected at config-load time
  with a clear error message that names the offending field, because
  `filepath.Match` does not support recursive globs and the project
  chooses not to add a dependency for the feature.

#### Scenario: root_dir tilde expansion

- **WHEN** the home directory is `/home/alice` and the configuration
  sets `root_dir: "~/Dev"`
- **THEN** `root_dir` resolves to `/home/alice/Dev` before any
  classification runs

#### Scenario: include with literal name

- **WHEN** `root_dir: "~/Dev"` and `include: [playground-rust]` and
  `~/Dev/playground-rust` exists
- **THEN** the candidate set contains `~/Dev/playground-rust`

#### Scenario: include with wildcard glob

- **WHEN** `root_dir: "~/Dev"` and `include: [playground*]` and
  `~/Dev` contains immediate subdirectories `playground-rust`,
  `playground-go`, and `notes`
- **THEN** the candidate set contains `~/Dev/playground-rust` and
  `~/Dev/playground-go`
- **AND** `~/Dev/notes` is not in the candidate set

#### Scenario: empty include means every child

- **WHEN** `root_dir: "~/Dev"` and `include:` is absent or empty
- **THEN** the candidate set contains every immediate child of
  `~/Dev` that is a directory

#### Scenario: exclude drops basenames

- **WHEN** the candidate set is `{~/Dev/bin, ~/Dev/playground-rust,
  ~/Dev/notes}` and `exclude: [bin, playground*]`
- **THEN** the post-filter candidate set is `{~/Dev/notes}`
- **AND** the watch list contains only `~/Dev/notes` after
  classification

#### Scenario: empty exclude means exclude nothing

- **WHEN** `exclude:` is absent or empty
- **THEN** no candidate is dropped by the exclude step

#### Scenario: pattern with `**` is rejected at load

- **WHEN** `config.yaml` contains `include: ["playground/**"]` or
  `root_dir` containing `**`
- **THEN** `see` prints an error identifying the offending field
  (`include[0]` or `root_dir`), explaining that `**` is not
  supported, and exits with status `2`
- **AND** the watcher does not start

#### Scenario: malformed glob brackets are rejected at load

- **WHEN** `config.yaml` contains `include: ["[unclosed"]`
- **THEN** `see` prints an error identifying `include[0]` and the
  underlying `filepath.Match` error, and exits with status `2`
- **AND** the watcher does not start

### Requirement: Discovery classifies each entry as a repo or a parent-of-repos

Each candidate produced by the `include` / `exclude` step SHALL be
classified by stat'ing the candidate itself:

- A candidate whose root contains a `.git` file or directory SHALL be
  treated as a single repo.
- A candidate whose root is a directory and does not contain `.git`
  SHALL be treated as a parent-of-repos; each immediate child with
  `.git` SHALL be added to the resolved list.
- A candidate that does not exist, is a regular file, or is a
  parent-of-repos with no `.git` children SHALL emit a `Warning`
  event and be skipped.

#### Scenario: Candidate is a single repo

- **WHEN** a candidate resolves to a path with `.git/`
- **THEN** the resolved list contains that path
- **AND** `Watcher.work` runs against it directly

#### Scenario: Candidate is a parent-of-repos

- **WHEN** a candidate resolves to a directory with immediate
  `.git/` children `repoA` and `repoB`
- **THEN** the resolved list contains `<candidate>/repoA` and
  `<candidate>/repoB`
- **AND** `Watcher.work` runs against each

#### Scenario: Candidate is a parent with no repo children

- **WHEN** a candidate resolves to a directory with no `.git/`
  children
- **THEN** a `Warning` event is emitted naming the path
- **AND** the batch continues without that entry

#### Scenario: Candidate is missing

- **WHEN** a candidate resolves to a path that does not exist
- **THEN** a `Warning` event is emitted naming the path
- **AND** the batch continues without that entry

### Requirement: Discovery dedupes and sorts the watch list

After all candidates are resolved and classified, the final watch
list SHALL be deduplicated by absolute path and sorted in ascending
path order. Two candidates that resolve to the same absolute path
(for example a literal `include` entry and a glob match that
includes the same repo) SHALL appear exactly once.

#### Scenario: Overlapping sources collapse

- **WHEN** `include: [/abs/path/repo, "~/work/*"]` resolves to a
  list that contains `/abs/path/repo` more than once
- **THEN** the final watch list contains `/abs/path/repo` exactly
  once

#### Scenario: Stable ordering for the TUI

- **WHEN** the resolved list contains `/repos/zeta`, `/repos/alpha`,
  and `/repos/mu`
- **THEN** the final list is ordered `/repos/alpha`, `/repos/mu`,
  `/repos/zeta`
- **AND** the Terminal User Interface (TUI) renders the same order
  on every scan

### Requirement: Configuration validates fields at load time

`loadConfig` SHALL validate every watch-related field after the YAML
decode and before returning, so configuration errors exit with status
`2` before the watcher starts instead of producing warnings during
the first scan. The validation SHALL run in this order:

1. If `root_dir` is nonblank: reject `**`, tilde-expand via
   `expandTilde`, stat the result, and require it to be a directory.
   The expanded path SHALL be stashed back into `cfg.RootDir` so the
   resolver does not re-expand.
2. For each entry in `include`: reject `**`, probe
   `filepath.Match(entry, "test")` to catch `ErrBadPattern`, and
   tilde-expand. The expanded entry SHALL be stashed back into the
   slice.
3. For each entry in `exclude`: the same checks as `include`.

Errors SHALL name the offending field path (`root_dir`,
`include[2]`, `exclude[0]`) and the underlying cause. Validation
SHALL NOT consult the filesystem beyond `expandTilde` and the
single `os.Stat` on `root_dir`.

#### Scenario: Unknown watch field is rejected at load

- **WHEN** `config.yaml` contains the old `watches:` sequence
- **THEN** the strict YAML decoder reports `watches` as an unknown
  field, identifies the configuration file, and `see` exits with
  status `2`
- **AND** the watcher does not start

#### Scenario: root_dir does not exist

- **WHEN** `config.yaml` sets `root_dir: "/nope"` and `/nope` does
  not exist
- **THEN** `see` prints `root_dir "/nope": <stat error>` and exits
  with status `2`
- **AND** the watcher does not start

#### Scenario: root_dir is a file

- **WHEN** `config.yaml` sets `root_dir: "/etc/hosts"` and that
  path is a regular file
- **THEN** `see` prints `root_dir "/etc/hosts": not a directory`
  and exits with status `2`

#### Scenario: include entry contains `**`

- **WHEN** `config.yaml` sets `include: ["work/**"]`
- **THEN** `see` prints `include[0]: '**' is not supported` and
  exits with status `2`

#### Scenario: include entry has malformed brackets

- **WHEN** `config.yaml` sets `include: ["[unclosed"]`
- **THEN** `see` prints `include[0]: invalid glob pattern: <error>`
  and exits with status `2`

#### Scenario: tilde expansion fails

- **WHEN** `HOME` is unset and `os.UserHomeDir` fails, and
  `config.yaml` sets `root_dir: "~/Dev"`
- **THEN** `see` prints `root_dir: expand ~: <error>` and exits
  with status `2`

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

### Requirement: Watcher renders the agent prompt from a configurable template

`see` SHALL select the process-wide agent prompt template in this order:

1. A `--prompt` value containing at least one non-whitespace character.
2. A `prompt` value from the configuration file selected by
   `--config <path>` (default: `os.UserConfigDir()/see/config.yaml`)
   containing at least one non-whitespace character, unless
   `--config=-` is set.
3. The default template embedded into the binary from the in-tree file `prompt.md` at the repository root.

`Watcher.work` SHALL derive the prompt passed to `Agent.Run` by substituting the literal token `{change}` in the selected template string with the active change name. `Watcher.PromptTemplate` SHALL hold the selected command-line or configured template; if it is empty or contains only whitespace, the watcher SHALL use the embedded default.

A `Watcher.SetPromptTemplate(s string)` setter SHALL normalize the input by trimming surrounding whitespace and treating the empty result as "use the embedded default". The setter is the documented mutator on the field; assigning to `PromptTemplate` directly is permitted but bypasses normalization.

The renderer SHALL replace every occurrence of `{change}` in the template with the active change name. No other tokens are defined; any other `{name}` substring SHALL be preserved verbatim, including its literal `{` and `}` characters.

#### Scenario: Command-line prompt overrides configured prompt

- **WHEN** `config.yaml` contains `prompt: "Configured {change}"` and `see` is invoked with `--prompt "Command {change}"`
- **THEN** the selected template is `"Command {change}"`
- **AND** `Agent.Run` receives `"Command add-foo"` for change `add-foo`

#### Scenario: Configured prompt overrides embedded default

- **WHEN** `config.yaml` contains `prompt: "Configured {change}"` and `--prompt` is absent or blank
- **THEN** the selected template is `"Configured {change}"`
- **AND** `Agent.Run` receives `"Configured add-foo"` for change `add-foo`

#### Scenario: --config=- cannot supply prompt

- **WHEN** `config.yaml` contains `prompt: "Configured {change}"` and `see` is invoked with `--config=-` and no nonblank `--prompt`
- **THEN** the embedded `prompt.md` template is selected

#### Scenario: Empty PromptTemplate uses the embedded default

- **WHEN** a `Watcher` is constructed with no call to `SetPromptTemplate` or `Watcher.PromptTemplate == ""`, and `Watcher.work` invokes `Agent.Run` for change `add-foo`
- **THEN** the prompt passed as the fourth argument to `Agent.Run` contains the substring `add-foo`
- **AND** the prompt body equals the embedded `prompt.md` contents with `{change}` substituted by `add-foo`

#### Scenario: Whitespace-only template normalizes to the default

- **WHEN** `SetPromptTemplate("   ")` is called on a `Watcher`
- **THEN** `Watcher.PromptTemplate` equals the embedded default, not the whitespace string

#### Scenario: User-supplied template renders with the change name

- **WHEN** `SetPromptTemplate("Apply the change {change} now")` is called on a `Watcher` and `Watcher.work` invokes `Agent.Run` for change `add-foo`
- **THEN** the prompt passed to `Agent.Run` is `"Apply the change add-foo now"`

#### Scenario: Multiple substitutions are all replaced

- **WHEN** `SetPromptTemplate("first {change} second {change}")` is called on a `Watcher` and `Watcher.work` invokes `Agent.Run` for change `add-foo`
- **THEN** the prompt passed to `Agent.Run` is `"first add-foo second add-foo"`

#### Scenario: Unknown tokens are preserved verbatim

- **WHEN** `SetPromptTemplate("Apply {change} in {repo} on {date}")` is called on a `Watcher` and `Watcher.work` invokes `Agent.Run` for any change
- **THEN** the prompt passed to `Agent.Run` substitutes only `{change}`
- **AND** `{repo}` and `{date}` appear unchanged in the output

#### Scenario: Empty change name still renders

- **WHEN** `SetPromptTemplate("prefix {change} suffix")` is set and `Watcher.work` invokes `Agent.Run` for an empty change name
- **THEN** the prompt passed to `Agent.Run` is `"prefix  suffix"` with a single space where the change name was

#### Scenario: prompt.md build-time embedding

- **WHEN** the binary is built without a `prompt.md` file at the repository root alongside `main.go`
- **THEN** the build fails at compile time with an error pointing at the `//go:embed` directive
- **AND** no binary is produced

### Requirement: Command-line interval configures continuous polling

`see` SHALL expose a top-level `--interval` flag parsed with Go duration syntax. The flag SHALL default to `5m` and SHALL set the process-wide delay between successful completed scans. A value of `0` SHALL disable the delay. A negative value SHALL produce an actionable command-line error and exit status 2 before the watcher starts.

The interval SHALL remain a command-line-only setting. It SHALL NOT add a field to global configuration.

#### Scenario: Default interval is five minutes

- **WHEN** `see` is invoked without `--interval` in continuous mode
- **THEN** the watcher uses a five-minute interval between successful completed scans

#### Scenario: Operator selects a shorter interval

- **WHEN** `see --interval=30s` is invoked in continuous mode
- **THEN** the watcher uses a 30-second interval between successful completed scans

#### Scenario: Zero disables the delay

- **WHEN** `see --interval=0` is invoked in continuous mode
- **THEN** the watcher starts another pass after a successful pass without an intentional delay

#### Scenario: Negative interval is rejected

- **WHEN** `see --interval=-1s` is invoked
- **THEN** `see` reports that `--interval` must be non-negative
- **AND** exits with status 2 before the watcher starts

### Requirement: Continuous watcher waits after successful passes

When `Watcher.Once` is false, `Watcher.Watch` SHALL start its first `runOnce` pass immediately. When that pass succeeds and `Watcher.PollInterval` is greater than zero, `Watcher.Watch` SHALL wait for the full interval measured from completion of that pass before starting the next pass. Scans SHALL NOT overlap.

The wait SHALL be interruptible by the supplied `context.Context`. Cancellation before a pass or during the wait SHALL make `Watcher.Watch` return `nil` without waiting for the interval to elapse. A non-nil `runOnce` error SHALL still return immediately without waiting or starting another pass.

#### Scenario: First pass starts immediately

- **WHEN** continuous `Watcher.Watch` starts with a five-minute interval and a live context
- **THEN** it invokes `runOnce` without an initial five-minute delay

#### Scenario: Delay begins after pass completion

- **WHEN** a continuous watcher has a five-minute interval and a successful pass completes at time T
- **THEN** the next pass does not start before T plus five minutes
- **AND** no second pass overlaps the completed pass

#### Scenario: Cancellation interrupts the delay

- **WHEN** a successful pass completes and the context is cancelled while `Watcher.Watch` is waiting for the next pass
- **THEN** `Watcher.Watch` returns `nil` promptly
- **AND** does not start another pass

#### Scenario: Pass error bypasses the delay

- **WHEN** `runOnce` returns a non-nil error in continuous mode
- **THEN** `Watcher.Watch` returns that error immediately
- **AND** does not wait for the interval or invoke `runOnce` again

#### Scenario: Once mode never waits

- **WHEN** `Watcher.Once` is true and the single `runOnce` pass succeeds
- **THEN** `Watcher.Watch` returns `nil` immediately after that pass
- **AND** does not wait for `Watcher.PollInterval`

#### Scenario: Retry timing is unchanged

- **WHEN** a repository attempt fails and `Watcher.RetryCount` permits another attempt within the same `runOnce` pass
- **THEN** the retry begins according to the existing retry contract without waiting for `Watcher.PollInterval`

### Requirement: Global configuration uses a strict YAML schema

`see` SHALL read global configuration from the single mapping document at `filepath.Join(os.UserConfigDir(), "see", "config.yaml")`. The mapping SHALL accept only these optional top-level fields:

- `root_dir`: a string base directory path. Tilde expansion is
  performed; environment-variable expansion is not.
- `include`: a sequence of glob patterns relative to `root_dir`.
- `exclude`: a sequence of glob patterns matched against the basename
  of each candidate from the `include` step.
- `prompt`: a string agent prompt template, including YAML Ain't
  Markup Language (YAML) literal block scalars for multiline text.
- `condition`: a platform-shell command string that selects custom
  workflow mode when nonblank.
- `commit`: a custom catch-up commit message template used only in
  custom workflow mode.

The legacy `watches` field SHALL NOT be accepted. The legacy
`filepath.Join(os.UserConfigDir(), "see", "watches")` file SHALL
NOT be read, merged, or used as a fallback. A missing or empty file
SHALL produce a zero-value configuration without error. Malformed
YAML, additional YAML documents, unknown fields, and values whose
types do not match the schema SHALL be fatal startup errors. The
error SHALL identify the configuration file and retain available
line information, and the watcher SHALL NOT start.

#### Scenario: Valid configuration loads every field

- **WHEN** `config.yaml` contains a `root_dir` string, `include` and
  `exclude` string sequences, and multiline `prompt`, `condition`,
  and `commit` strings
- **THEN** `see` loads all six values from the same document
- **AND** each literal block scalar preserves its line breaks

#### Scenario: Missing configuration is empty configuration

- **WHEN** `config.yaml` does not exist
- **THEN** configuration loading succeeds with no `root_dir`,
  `include`, `exclude`, `prompt`, `condition`, or `commit`

#### Scenario: Unknown field is rejected

- **WHEN** `config.yaml` contains the misspelled field `conditon`
- **THEN** `see` reports an error identifying the unknown field and
  configuration file
- **AND** exits with status `2` before the watcher starts

#### Scenario: Invalid field type is rejected

- **WHEN** `config.yaml` defines `condition` as a mapping instead
  of a string
- **THEN** `see` reports a type error identifying the configuration
  file
- **AND** exits with status `2` before the watcher starts

#### Scenario: Malformed or multiple-document YAML is rejected

- **WHEN** `config.yaml` is malformed or contains more than one YAML
  document
- **THEN** `see` reports a configuration error
- **AND** exits with status `2` before the watcher starts

### Requirement: First-run bootstrap materializes a default config file

When `see` is invoked with no `--config` flag and the resolved default
configuration path does not exist, `loadStartupConfig` SHALL create the
parent directory `filepath.Join(<base>, "see")` with mode `0o755`
if it does not exist, then write a template configuration document to
the default path with mode `0o644`. The template SHALL be embedded into
the binary at build time from a sibling file at the repository root
(consistent with `prompt.md`) via `//go:embed`, SHALL contain only YAML
comments and a YAML header, and SHALL decode under the strict schema
to a zero-value configuration identical to the one the loader returns
for the missing-file case.

Bootstrap SHALL NOT fire when `--config` is set to a non-empty value
(including `--config=-` and `--config=<path>`). Bootstrap SHALL NOT
overwrite an existing configuration file at the default path; an
empty file, a comments-only file, a valid configuration, and a
malformed configuration all bypass bootstrap and proceed through the
existing load branches.

If the write fails (permission denied, read-only filesystem, parent
directory unwritable), `see` SHALL emit a one-line notice to standard
error (stderr) identifying the target path and the failure reason,
and SHALL continue startup with a zero-value configuration so the
current-working-directory (cwd) fallback still produces a working
watch list. The watcher SHALL start regardless of bootstrap outcome.

#### Scenario: First run writes the template

- **WHEN** `see` is invoked with no `--config` flag and the default
  configuration file does not exist
- **THEN** the parent directory `filepath.Join(<base>, "see")` exists
  with mode `0o755` (created if absent)
- **AND** the default configuration file exists with mode `0o644`
- **AND** the file's contents equal the embedded template byte-for-byte
- **AND** `loadConfig` on that file returns a zero-value configuration
  with no error
- **AND** the watcher starts and proceeds with the cwd fallback

#### Scenario: Existing file is not overwritten

- **WHEN** `see` is invoked with no `--config` flag and the default
  configuration file already exists with arbitrary content (empty,
  comments-only, valid, or malformed)
- **THEN** the file's contents and mode are unchanged after startup
- **AND** bootstrap does not write to the path

#### Scenario: --config=- skips bootstrap

- **WHEN** `see --config=-` is invoked and the default configuration
  file does not exist
- **THEN** the default configuration file is not created
- **AND** the parent directory `filepath.Join(<base>, "see")` is not
  created
- **AND** the loader returns a zero-value configuration without
  reading or writing any file

#### Scenario: --config=<path> skips bootstrap

- **WHEN** `see --config=<other>` is invoked and the default
  configuration file does not exist
- **THEN** the default configuration file is not created
- **AND** the named file is read (or a "missing file" error path is
  taken) without writing any file at the default path

#### Scenario: Unwritable target is non-fatal

- **WHEN** `see` is invoked with no `--config` flag, the default
  configuration file does not exist, and the parent directory is not
  writable (permission denied)
- **THEN** `see` prints one line to stderr identifying the target
  path and the write failure
- **AND** `see` exits with status `0` if no other startup error
  applies
- **AND** the watcher starts with a zero-value configuration

#### Scenario: Bootstrap template decodes under strict schema

- **WHEN** the embedded template is written to the default path and
  immediately read back by `loadConfig`
- **THEN** decoding succeeds without error
- **AND** the decoded configuration has `RootDir == ""`,
  `Include == nil`, `Exclude == nil`, and `Prompt == ""`
- **AND** no unknown-field, type, or multi-document error is raised

### Requirement: Watcher processes workflows independently in repository order
For each repository resolved by discovery, `Watcher` SHALL evaluate every configured workflow in configuration order. It SHALL process repositories in the stable order supplied by discovery. At most one agent session SHALL run at a time. A workflow condition that exits with status `1` SHALL skip only that workflow and processing SHALL continue with the next workflow.

A condition failure, agent failure, or catch-up failure SHALL be associated with the current workflow. After ordinary rollback, the watcher SHALL continue with later workflows for the same repository. If cleanup cannot restore a safe clean checkout, the watcher SHALL stop processing that repository and SHALL NOT invoke another agent in it.

#### Scenario: Every workflow is evaluated for one repository
- **WHEN** a repository has two configured workflows
- **THEN** the first workflow is evaluated before the second
- **AND** the second is evaluated even when the first workflow is idle

#### Scenario: Active workflows run sequentially
- **WHEN** both workflow conditions report active work
- **THEN** the first workflow's agent session completes before the second workflow's agent session starts
- **AND** no concurrent agent sessions are created

#### Scenario: Failed workflow does not block later workflow
- **WHEN** the first active workflow's agent fails and rollback restores a clean checkout
- **THEN** the failure is reported for the first workflow
- **AND** the next workflow is evaluated and may run

#### Scenario: Unsafe cleanup stops the repository
- **WHEN** a failed workflow cannot restore a clean safe checkout
- **THEN** the watcher reports the cleanup failure
- **AND** it does not invoke another workflow for that repository

### Requirement: Watcher leaves the final usable workflow lane checked out
*(Branch mode only — see lane-isolation for the worktree-mode contract.)* After all workflows for a repository are processed in branch mode, `Watcher` SHALL leave the most recently usable active workflow lane checked out. If no workflow was active, it SHALL leave the branch that was checked out when repository processing began. A successful workflow SHALL not merge its lane into the starting branch.

#### Scenario: Final active workflow lane remains checked out
- **WHEN** two workflows run successfully for one repository
- **THEN** the second workflow's lane is checked out when repository processing ends
- **AND** both workflow lanes retain their commits

#### Scenario: No active workflow preserves the starting branch
- **WHEN** all workflow conditions exit with status `1`
- **THEN** no workflow lane is created or switched to
- **AND** the starting branch remains checked out

#### Scenario: A failed final workflow leaves a safe usable checkout
- **WHEN** the final active workflow fails and rollback succeeds
- **THEN** the cleaned workflow lane remains available as the final lane when it existed before the attempt
- **AND** no later workflow remains to run

### Requirement: Watcher dispatches into the configured isolation mode
`Watcher.work` (or its successor) SHALL select an isolation mode based
on the resolved `(worktree, auto_merge, worktree_root)` configuration
that `main()` constructs from CLI flags and config fields. The
selection SHALL follow the lane-isolation capability's "Three
isolation modes are explicit and named" requirement:

- When `worktree` is false (the default), the watcher SHALL use
  branch mode and SHALL follow the existing branch-mode requirements
  (per-change branch creation, leave-HEAD-on-lane after success,
  per-mode rollback, final-lane-checked-out). The `auto_merge` and
  `worktree_root` values SHALL be ignored in branch mode.

- When `worktree` is true, the watcher SHALL use worktree mode and
  SHALL follow the lane-isolation capability's worktree-mode
  requirements (worktree creation, lane rebased onto operator's
  branch tip, fast-forward merge when `auto_merge` is true, rollback
  with worktree removed and lane deleted on failure). The existing
  branch-mode requirements SHALL NOT apply.

The dispatch SHALL be evaluated once per `Watcher.work` invocation
against the resolved configuration; runtime mode switching within a
single `Watcher.work` call is not supported.

#### Scenario: Default config selects branch mode
- **WHEN** `see` is invoked with no `--worktree` flag and the
  configuration has no `worktree:` field
- **THEN** every `Watcher.work` call uses branch mode
- **AND** the existing branch-mode requirements apply unchanged

#### Scenario: --worktree flag selects worktree mode
- **WHEN** `see` is invoked with `--worktree`
- **THEN** every `Watcher.work` call uses worktree mode
- **AND** the lane-isolation worktree-mode requirements apply
- **AND** the existing branch-mode requirements do not apply

#### Scenario: --worktree overrides worktree: false
- **WHEN** the configuration sets `worktree: false` and `see` is
  invoked with `--worktree`
- **THEN** worktree mode is selected for every `Watcher.work` call
- **AND** the lane-isolation worktree-mode requirements apply

#### Scenario: Configuration worktree: true without flag
- **WHEN** the configuration sets `worktree: true` and `--worktree` is
  not passed
- **THEN** worktree mode is selected
- **AND** the lane-isolation worktree-mode requirements apply

#### Scenario: auto_merge is ignored in branch mode
- **WHEN** branch mode is active and `auto_merge: true` is set in the
  configuration or `--auto-merge=true` is passed
- **THEN** the watcher proceeds with branch mode
- **AND** the `auto_merge` value is not consulted for behavior
- **AND** no error is emitted for the unused value

#### Scenario: worktree_root is ignored in branch mode
- **WHEN** branch mode is active and `worktree_root: <path>` is set in
  the configuration
- **THEN** the watcher proceeds with branch mode
- **AND** the `worktree_root` value is not consulted for behavior
- **AND** no error is emitted for the unused value
