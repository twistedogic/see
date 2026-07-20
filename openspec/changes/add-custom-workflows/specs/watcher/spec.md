## ADDED Requirements

### Requirement: Watcher selects custom work before the OpenSpec compatibility fallback
When the loaded configuration contains a nonblank custom condition, `Watcher` SHALL use only the custom condition resolver for every watched repository. When the condition is blank or absent, `Watcher` SHALL retain the current OpenSpec resolver and SHALL NOT invoke a shell condition. Compatibility mode SHALL preserve active-change discovery, `see/<change>` branch naming, prompt defaults, archive-based completion, rollback, and the `see: apply openspec change <change>` catch-up commit message.

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
For a resolved custom change, `Watcher` SHALL use the hashed branch supplied by the custom workflow resolver. If the branch does not exist, the watcher SHALL create it at the current commit. If the branch exists and is already checked out, the watcher SHALL continue from its current tip without resetting or deleting prior commits. If the branch exists but another branch is checked out, the watcher SHALL fail without switching or mutating either branch and SHALL instruct the operator to return to the automation lane or resolve the branch intentionally.

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
- **THEN** the attempt fails without switching branches
- **AND** neither `main` nor `see/<digest>` moves
- **AND** the agent is not invoked

### Requirement: Watcher rolls back only the failed custom attempt
Immediately before invoking the agent on a custom lane, `Watcher` SHALL capture the lane tip. If the agent fails on a lane that existed before the attempt, the watcher SHALL reset tracked state to that captured tip, remove untracked files created during the attempt, preserve ignored files, preserve all commits that predate the attempt, leave the lane checked out, and return the agent error. If the failed attempt created the lane, the watcher SHALL switch back to the original branch, restore its captured commit, delete the newly-created lane, and return the agent error. Cleanup failures SHALL emit `Warning` events and SHALL NOT replace the original agent error.

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

## MODIFIED Requirements

### Requirement: Watcher creates a per-change branch before running the agent
In OpenSpec compatibility mode, when `Watcher.work` begins processing an active change, it SHALL capture the current commit SHA and original branch ref, then create or reuse `see/<change>`. After the branch exists, compatibility mode SHALL pin its tip to the captured SHA via `git reset --hard <sha>` so the agent starts from known state regardless of prior branch drift.

In custom mode, the watcher SHALL instead follow the persistent custom automation branch requirement in this change: create `see/<digest>` once, resume it only when it is already checked out, and never reset its prior successful commits before an attempt.

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

### Requirement: Watcher rolls back the branch on agent failure
In OpenSpec compatibility mode, when the agent returns a non-nil error, `Watcher.work` SHALL restore the repository to its pre-run state by switching back to the original branch ref, resetting hard to the captured commit SHA, deleting `see/<change>`, and returning the agent error.

In custom mode, rollback SHALL follow the persistent-lane requirement in this change. A failed attempt on a pre-existing lane SHALL restore that lane to its pre-attempt tip without deleting it. A failed attempt that created a new lane SHALL restore the original branch and delete only that newly-created lane. In both modes, a detached HEAD SHALL remain unsupported and SHALL be rejected before branch mutation.

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

### Requirement: Global configuration uses a strict YAML schema
`see` SHALL read global configuration from the single mapping document at `filepath.Join(os.UserConfigDir(), "see", "config.yaml")`. The mapping SHALL accept only these optional top-level fields:

- `watches`: a sequence of string watch patterns.
- `prompt`: a string agent prompt template, including YAML Ain't Markup Language (YAML) literal block scalars for multiline text.
- `condition`: a platform-shell command string that selects custom workflow mode when nonblank.
- `commit`: a custom catch-up commit message template used only in custom workflow mode.

A missing or empty file SHALL produce a zero-value configuration without error. Malformed YAML, additional YAML documents, unknown fields, and values whose types do not match the schema SHALL be fatal startup errors. The error SHALL identify the configuration file and retain available line information, and the watcher SHALL NOT start.

The legacy `filepath.Join(os.UserConfigDir(), "see", "watches")` file SHALL NOT be read, merged, or used as a fallback.

#### Scenario: Valid configuration loads every field
- **WHEN** `config.yaml` contains a `watches` string sequence and multiline `prompt`, `condition`, and `commit` strings
- **THEN** `see` loads all four values from the same document
- **AND** each literal block scalar preserves its line breaks

#### Scenario: Missing configuration is empty configuration
- **WHEN** `config.yaml` does not exist
- **THEN** configuration loading succeeds with no watches, prompt, condition, or commit template

#### Scenario: Unknown field is rejected
- **WHEN** `config.yaml` contains the misspelled field `conditon`
- **THEN** `see` reports an error identifying the unknown field and configuration file
- **AND** exits with status `2` before the watcher starts

#### Scenario: Invalid field type is rejected
- **WHEN** `config.yaml` defines `condition` as a mapping instead of a string
- **THEN** `see` reports a type error identifying the configuration file
- **AND** exits with status `2` before the watcher starts

#### Scenario: Malformed or multiple-document YAML is rejected
- **WHEN** `config.yaml` is malformed or contains more than one YAML document
- **THEN** `see` reports a configuration error
- **AND** exits with status `2` before the watcher starts

#### Scenario: Legacy watches file is ignored
- **WHEN** `config.yaml` is absent and the legacy `watches` file exists
- **THEN** configuration loading returns no configured watches
- **AND** discovery proceeds to command-line entries or the current-working-directory fallback
