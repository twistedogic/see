## Context

`see` currently combines two concerns inside `Watcher.work`: OpenSpec work discovery and the reusable mechanics that isolate an agent run on a Git branch. A repository is considered runnable only when `openspec/changes/` contains an active directory; that directory name supplies the prompt token, branch suffix, log filename, completion check, and commit message.

The reusable mechanics already cover repository discovery, retries, Git rollback, agent invocation, event logging, and the Terminal User Interface (TUI). The new custom mode must feed those mechanics from a shell condition without weakening the existing zero-configuration OpenSpec workflow.

Custom conditions are trusted local configuration. They run before the agent and therefore must behave as side-effect-free predicates.

## Goals / Non-Goals

**Goals:**
- Let one configured shell command decide whether each watched repository has work.
- Treat successful condition stdout as a stable, human-meaningful change value.
- Use that value consistently for prompt rendering, commit rendering, branch identity, events, and display.
- Preserve a custom change branch across successful polling passes and roll back only the latest failed attempt.
- Keep configurations without a custom condition behaviorally compatible with the current OpenSpec workflow.
- Keep the implementation in the Go standard library except for dependencies already present.

**Non-Goals:**
- Multiple conditions or workflow profiles in one `see` process. Operators can run multiple processes with different configuration files.
- Edge-trigger persistence, deduplication databases, or run history outside Git and the existing logs.
- A provider interface, plugin system, or built-in integrations for issue trackers.
- Running custom conditions concurrently across repositories.
- Isolating the watched checkout in a separate Git worktree.
- Guaranteeing one `see`-authored commit per successful run when the agent already committed everything or made no changes.

## Decisions

### Custom mode is selected by a nonblank `condition`

The strict configuration schema gains `condition` and `commit` strings. A nonblank condition selects custom mode for every watched repository. Custom mode requires a nonblank effective prompt and a nonblank commit template; startup fails before watching if either is missing. The effective prompt keeps the existing command-line-over-configuration precedence.

When `condition` is blank, `see` follows the existing OpenSpec resolver, default prompt, `see/<change>` branch, archival completion check, and default commit message. The `commit` field belongs to custom mode and is not consulted by compatibility mode.

This keeps first-run and existing configurations working while making OpenSpec a fallback rather than the watcher's only protocol. A provider abstraction was rejected because there are only two resolution paths and both can feed the existing work function directly.

### The condition is a platform-shell predicate whose stdout names one change

The condition runs in the repository directory under the platform shell (`/bin/sh -c` on Unix-like systems and `cmd.exe /C` on Windows) with the watcher's context. Its result contract is:

- exit `0`: work exists; consume stdout as the change value;
- exit `1`: no work; the repository is idle;
- shell launch failure or any other nonzero exit: condition failure, including captured stderr in the diagnostic.

`see` removes trailing carriage-return and line-feed characters from stdout. The remaining value must contain at least one non-whitespace character and be single-line; otherwise the condition fails. Non-newline leading or trailing whitespace remains part of the value, hash, and templates. A single-line identifier keeps event records, commit subjects, filenames, and the TUI well-formed. Conditions that need rich data can emit an identifier and let the prompt tell the agent how to retrieve details.

The condition string itself is not interpolated or parsed by `see`; shell quoting and expansion belong to the selected shell. Normal condition stdout is consumed rather than copied to `see` stdout. Conditions are documented as side-effect-free because they execute before branch isolation.

Alternatives rejected:
- Argument-vector configuration is more portable but does not match the requested shell-command ergonomics.
- Treating every nonzero exit as idle would hide syntax errors and missing commands.
- Allowing multiline values would require escaping or sanitizing every event and display surface for little benefit.

### The normalized change value is the sole custom work identity

Custom mode computes the lowercase hexadecimal Secure Hash Algorithm 256-bit (SHA-256) digest of the normalized change value and uses `see/<digest>` as the branch name. The same normalized bytes are used for hashing and template substitution. Full digests avoid introducing collision handling, while the TUI continues to show the human-readable change rather than the branch.

The per-agent log filename uses the digest instead of raw condition output. This prevents slashes, traversal sequences, or other shell output from affecting filesystem paths. Events may carry the human-readable change because they are structured and the value is constrained to one line.

Hashing the configured command was rejected because one condition may emit different work items. Hashing both command and output was rejected because changing how an unchanged item is discovered should not fork its branch.

### `{change}` is one shared template token

Every literal `{change}` occurrence is replaced in both the selected prompt and selected catch-up commit template. Unknown tokens remain unchanged. The existing renderer can remain deliberately small and be reused for both strings.

The commit command receives the rendered message as an argument rather than through a shell, so condition output cannot inject Git command options or commands.

### Custom branches are persistent lanes

On a true condition, custom mode captures the current commit and branch, then handles `see/<digest>` as follows:

- If the branch does not exist, create it at the captured commit and mark it as created for this attempt.
- If it exists and is already checked out, continue from its current tip without resetting prior commits.
- If it exists but another branch is checked out, fail with an actionable message rather than resetting, switching based on a condition evaluated against stale contents, or overwriting either branch.

The watched checkout therefore belongs to the automation lane while custom mode is active. This matches the chosen persistent-lane model and avoids adding Git worktree management.

Before creating or resuming a custom lane, `see` rejects a dirty working tree. This prevents the existing hard-reset rollback strategy from deleting operator edits and gives failed-run cleanup a known baseline.

The legacy OpenSpec branch reuse and reset behavior remains unchanged in compatibility mode.

### Custom rollback preserves the lane and removes only failed-attempt state

For an existing custom lane, an agent failure resets tracked state to the commit captured immediately before that attempt and removes untracked files created by the attempt; it does not delete the lane or its earlier commits. For a lane created by the failed attempt, rollback switches to the original branch, restores the captured commit, and deletes the new lane.

Cleanup steps remain best-effort and emit `Warning` events without replacing the original agent error. Ignored files are outside the rollback guarantee because deleting them could remove caches or local configuration that predated the run.

The legacy OpenSpec rollback remains unchanged.

### Successful custom runs use a catch-up commit only when needed

After an agent exits successfully, custom mode stages all changes. It checks the staged diff before committing:

- staged changes exist: commit them with the rendered custom message;
- no staged changes: return success without a commit or warning.

Commits created by the agent remain in history. Squashing them was rejected because it rewrites valid agent work; empty audit commits were rejected because a continuously true, idempotent condition would create one every polling interval.

Custom mode does not require the condition to turn false before committing. The next polling pass evaluates it again, and another run begins whenever it still exits `0`. Each retry attempt also re-evaluates the condition so a false result becomes an idle no-op and changed stdout selects the newly reported work item.

### Availability state becomes workflow-neutral

`RepoSeen.HasOpenspec` becomes `RepoSeen.HasChange`, with the corresponding TUI message and model field renamed. It is true when the selected resolver found a change and false when the custom condition exits `1` or compatibility mode finds no active OpenSpec change. Existing `ChangeStarted`, `ChangeDone`, `ChangeFailed`, `RetryAttempt`, `LogPath`, and `Warning` names remain valid because both modes produce a change value.

This intentionally changes the JavaScript Object Notation Lines (JSONL) event payload. Carrying both old and new booleans was rejected because it creates two sources of truth and preserves OpenSpec vocabulary indefinitely.

## Risks / Trade-offs

- **A condition has side effects before isolation** → Document conditions as predicates and include their stderr on failure; do not pretend the shell can be sandboxed.
- **A condition remains true indefinitely** → This is the selected level-triggered behavior; `--interval`, `--once`, and idempotent prompts bound cost operationally.
- **An operator switches away from an existing automation lane** → Fail without mutation and instruct the operator to switch back or remove/rename the lane intentionally.
- **Condition output changes slightly** → The normalized value intentionally defines identity; any non-newline byte change selects another branch.
- **The event payload rename breaks JSONL consumers** → Document the field migration from `HasOpenspec` to `HasChange`; no dual-write compatibility period.
- **Platform shells interpret the same string differently** → Configuration is user-local and explicitly shell-based; portable workflows must use syntax supported on their target platforms.
- **Agent-created ignored files survive rollback** → The rollback guarantee excludes ignored files to avoid deleting pre-existing caches and secrets.
- **A full hash makes branch names opaque** → Events, logs, prompts, and the TUI retain the readable change value; the opaque branch is a safe internal identity.

## Migration Plan

1. Extend strict configuration decoding and bootstrap documentation with optional `condition` and `commit` fields.
2. Add custom condition resolution and validation while retaining the existing resolver as the blank-condition fallback.
3. Add persistent custom branch and catch-up commit behavior behind custom mode.
4. Rename the repository availability event field and update the TUI and JSONL tests together.
5. Update `AGENTS.md` and user-facing event documentation with custom-mode configuration, shell exit semantics, template behavior, branch ownership, OpenSpec fallback, and the `HasOpenspec` to `HasChange` JSONL migration.
6. Verify the existing OpenSpec tests still pass and add custom-mode regression tests.

Rollback is a code revert. Existing OpenSpec users require no configuration migration. Custom `see/<digest>` branches are ordinary Git branches and remain available if the feature is rolled back.

## Open Questions

None. Multiple workflow profiles and separate Git worktrees are deferred until concrete use cases require them.
