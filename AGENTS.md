# AGENTS.md

Guidelines for AI agents and contributors working on this project.

## Documentation

- Never use acronyms without explaining them in documents. The first time an acronym appears, spell out the full term followed by the acronym in parentheses (e.g., "Application Programming Interface (API)"). Subsequent uses may use the acronym alone.

## Commit Messages

- When writing a commit message, never add your agent name as author or co-author. Commits must reflect the human contributor as author and must not include agent names in the author or co-author trailers.

## Bug Fixes

- When doing a bug fix, always start by reproducing the bug and add a failing test case before changing production code. The failing test must demonstrate the bug, and the fix must turn it green. Never merge a bug fix without a regression test.

## Testing

- Local Continuous Integration (CI) runs through `task` (see
  `Taskfile.yml`), which mirrors `.github/workflows/ci.yml`: `task` or
  `task default` runs format → lint → test → build, and `task fmt`,
  `task lint`, `task test`, and `task build` run each step alone.
- `main` is a watch loop: `Watcher.Watch` (`main.go`) runs one
  `runOnce` pass immediately, then waits `Watcher.PollInterval`
  (`DefaultPollInterval` = five minutes via `NewWatcher`) after every
  successful pass until `SIGINT` or `SIGTERM` cancels the context.
  `--interval=0` restores the pre-default tight-poll loop; negative
  intervals are rejected at startup. Tests that spawn the binary hang
  the test runner, so:
  - Prefer unit tests that drive `Watcher.runWithRetry` (or a single `runOnce`
    pass) directly with a `fakeAgent` and a `recordingObserver`
    (see `main_test.go`). Assert on the observed event sequence and
    the captured `Run` arguments — never on process exit codes or
    stdout.
  - Set `Watcher.PollInterval` to a short duration or zero in unit
    tests so the loop returns within a bounded deadline. A literal
    `Watcher{}` defaults to zero interval for this reason.
  - Always run `task test` (`go test -timeout 30s ./...`, or shorter). A wedged
    poll loop or goroutine should fail fast at 30 seconds rather than
    hitting the runner's default 10-minute ceiling and masking the
    real bug under a generic timeout.
  - Reserve spawning the binary for manual smoke checks and one-shot
    `see --once` runs against a fixture repo, never inside an
    automated test.
- Tests run concurrently: any test that does not call `t.Setenv`
  (directly or via a helper) and does not mutate package globals must
  call `t.Parallel()` as its first statement. The suite is dominated
  by git subprocess latency, so parallelism is the primary runtime
  lever. Wall-clock assertions in parallel tests must be load-robust:
  assert ordering / interval-gap properties rather than absolute
  latencies, cancel from callbacks or observers instead of racing a
  fixed `time.Sleep`, and use generous anti-hang deadlines (10s),
  since scheduler and fork/exec load from sibling tests can stall
  shell-kill paths far beyond nominal latency.

## Technical Decisions

- When making a technical decision, do not give much weight to development cost and time. Instead, prefer correctness, readability, simplicity, and long-term maintainability. Short-term effort is a secondary concern; the chosen approach should be one we are willing to live with for years.

## Observability

- Always consider observability of the application in development.
- Prefer structured logging (key/value fields, consistent log levels, machine-parseable format) over unstructured log strings.
- For servers, prefer Prometheus metrics (counters, gauges, histograms) exposed on a standard scrape endpoint, in addition to structured logs.

## Maintenance of AGENTS.md

- Keep AGENTS.md up to date on key design decisions and development workflows. When a decision is made or a workflow changes, update this file in the same change so it remains the source of truth for future contributors and agents.

## Configuration

`see` reads one global configuration file at `os.UserConfigDir()/see/config.yaml`
(Linux/macOS: `$XDG_CONFIG_HOME/see/config.yaml` or `~/.config/see/config.yaml`). The file is YAML Ain't Markup Language
(YAML), parsed strictly: unknown fields, wrong field types, malformed input, and
multi-document input are rejected at startup with an actionable error. A missing
or empty file is treated as "no configuration" and is not an error.

### Schema

```yaml
root_dir: "~/Dev"
include:
  - "playground-*"
exclude:
  - "playground-old"

# Lane isolation: "branch" (default) runs the agent in the
# operator's checkout; "worktree" runs it in a git worktree so the
# operator's checkout is never switched.
worktree: false
# Only meaningful with worktree: true. true (default) rebases the
# lane onto the operator's branch tip and fast-forward merges it on
# success; false leaves the rebased lane for manual review.
auto_merge: true
# Only meaningful with worktree: true. Parent directory for new
# worktrees; tilde-expanded. Defaults to ~/.cache/see/worktrees.
worktree_root: "~/.cache/see/worktrees"

# Log directory for the batch-level event log and per-invocation
# agent logs. SEE_LOG_DIR (when non-empty) overrides this field,
# which overrides the default below. ~ is expanded like root_dir.
log_dir: "~/.cache/see/logs"

# Markdown workflow files are loaded before workflows below.
# Defaults to ~/.config/see/workflows/ when omitted.
workflows_dir: "~/.config/see/workflows/"

workflows:
  - name: openspec
    # disable: false        # optional; true parks this workflow (still validated)
    prompt: |-
      Apply the OpenSpec change "{change}".
    condition: "git rev-parse --abbrev-ref HEAD"
    commit: "see: apply {change}"
    # check: "openspec validate {change}"   # optional quality gate; absent or blank = no gate
    # measure: "./bench.sh {change}"        # optional improvement gate; absent falls back to ~/.config/see/measure/<name>.sh
  - name: dependencies
    prompt: |-
      Update the dependency identified by "{change}".
    condition: "./scripts/next-dependency-update"
    commit: "see: update dependency {change}"
```

- `root_dir` is the base directory. `~` and `~/path` are expanded;
  environment variables are not. When blank or omitted, `see` falls back to
  the current working directory. A configured root must exist and be a
  directory when the configuration loads.
- `include` is a sequence of glob patterns relative to `root_dir`. An omitted
  or empty sequence includes every immediate child directory. Patterns use
  `filepath.Match` syntax (`*`, `?`, `[abc]`); recursive `**` is rejected.
- `exclude` is a sequence of glob patterns matched against each included
  candidate's basename. An omitted or empty sequence excludes nothing. It
  follows the same tilde-expansion and pattern-validation rules as `include`.
- `workflows_dir` is an optional directory containing custom workflows as
  Markdown (`.md`) files. `~` and `~/path` are expanded, recursive `**` is
  rejected, and the default is `~/.config/see/workflows/`. A missing directory
  contributes no file workflows; a path that exists but is not a directory
  fails startup.
- `workflows` is an ordered sequence. Each entry requires a unique nonblank
  `name`, `prompt`, `condition`, and `commit`. The optional `model` string is
  passed to `pi` as `--model` for that workflow's runs when nonblank;
  otherwise the agent's default model is used. Entries run in configuration
  order for every discovered repository; workflows are not run concurrently.
- Each workflow entry accepts an optional `disable` boolean (default
  `false`). An entry with `disable: true` is still fully validated but
  removed from the evaluated list after validation, so the run loop,
  TUI, and event stream never see it. This parks a fully-configured,
  known-good workflow in place for a one-key toggle. Disabling every
  workflow empties the list, which reverts the watcher to OpenSpec
  compatibility mode (identical to commenting out the whole `workflows`
  block).
- A workflow `prompt` is a string. Literal-block scalars (`|`, `|-`, `|+`)
  preserve interior line breaks; use `|-` to strip the trailing newline. The
  single token `{change}` is replaced with that workflow's active change name;
  no other tokens are substituted.
- A workflow `condition` is a platform-shell command. Exit status `0` means
  work exists, exit status `1` means that workflow is idle, and any other
  nonzero status is a failure.
- A workflow `commit` is the catch-up commit message template. The same
  `{change}` substitution rule applies.
- A workflow `check` is an optional platform-shell command run after a
  successful agent attempt and before any git landing operation. The same
  shell contract and `{change}` substitution rule apply as `condition`.
  Absent or whitespace-only means the workflow has no gate, identical to
  behavior before the field existed. See [Check gate](#check-gate) below.
- A workflow `measure` is an optional platform-shell command that gates
  landing on strict improvement. It runs once before the agent to
  capture the **baseline** and once after a passing check (or after the
  agent when no check is defined) to capture the **candidate**. The
  candidate must strictly exceed the baseline for the attempt to land;
  ties, non-improvement, nonzero exits, or unparseable values are all
  measure failures that roll back to a clean slate and are retried
  like an agent failure. Two definition forms: a nonblank
  `measure:` field wins; otherwise the regular file at
  `~/.config/see/measure/<workflow-name>.sh` is consulted. A blank
  `measure` field fails startup (so absent and present-blank are
  distinguishable). The `{change}` token substitutes into the measure
  command; the `{metric}` token (the baseline value) substitutes into
  `prompt` and `commit` only when a measure gate is resolved and
  remains literal in `measure`. See
  [Measure gate](#measure-gate) below.
- `worktree` (boolean, default `false`) selects lane isolation. `false`
  (the default) is **branch** mode: the agent runs in the operator's
  checkout and success leaves the checkout on the lane. `true` is
  **worktree** mode: the agent runs in a git worktree linked to the
  operator's checkout and the operator's checkout is never switched. See
  [Lane isolation modes](#lane-isolation-modes).
- `auto_merge` (boolean, default `true`) is only meaningful with
  `worktree: true`. `true` rebases the lane onto the operator's current
  branch tip and fast-forward merges it on success (the lane and worktree
  are removed). `false` rebases the lane and leaves it for manual review
  (the lane and worktree are preserved). An explicitly set `auto_merge`
  without `worktree: true` is rejected at startup.
- `worktree_root` (string, default `~/.cache/see/worktrees`) is only
  meaningful with `worktree: true`. It is the parent directory for new
  worktrees; `~` and `~/path` are expanded using the same rule as
  `root_dir`. A non-empty `worktree_root` without `worktree: true` is
  rejected at startup. New worktrees are created at
  `<worktree_root>/<repo-basename>--<digest>/`; if you place
  `worktree_root` inside `root_dir`, add an `exclude` glob so discovery
  does not double-watch the worktree directory.
- `log_dir` (string, default `~/.cache/see/logs`) is the directory `see`
  writes its batch-level event log and per-invocation agent logs to.
  It resolves with the precedence `SEE_LOG_DIR` (non-empty) > `log_dir`
  (non-blank) > `~/.cache/see/logs`: the environment variable stays the
  hard override, the config field is the common path. A whitespace-only
  value is treated as unset. `~` and `~/path` are expanded using the same
  rule as `root_dir` (so `SEE_LOG_DIR=~/logs` and `log_dir: "~/logs"`
  both resolve to `<home>/logs` rather than a literal directory named
  `~`); environment-variable expansion (`$VAR`) is not performed. The
  resolved directory is created (`MkdirAll` mode `0o755`) if absent; a
  failure to create it (permission denied, a path component that is a
  file, a read-only parent) is a fatal startup error that exits with
  status `2` before the watcher starts. Old logs are not migrated.

All fields are optional except the fields inside a configured workflow entry.
Omitting `workflows` when `workflows_dir` contributes no files preserves
OpenSpec compatibility mode: OpenSpec changes are discovered using the embedded
prompt and the default commit subject. The former top-level `prompt`,
`condition`, and `commit` fields are not accepted; migrate each old custom
configuration into one named workflow.

The former `watches` field and `--watch` command-line flag are not accepted.

### Migration of the default log directory (macOS)

The default log directory moved from `os.UserCacheDir()/see/logs/`
(`~/Library/Caches/see/logs/` on macOS, `~/.cache/see/logs` on Linux)
to the home-relative constant `~/.cache/see/logs`, matching
`worktree_root`'s shape so `~/.cache/see/` is the single root for
`see`'s ephemeral artifacts. Linux is unchanged. Old logs are not
touched (JSONL is observability, not data); copy or symlink them by
hand if you want to keep history. Operators who set `SEE_LOG_DIR` or
configure `log_dir` are unaffected.

### Log retention

`see` bounds the per-invocation agent logs to the 5 newest files per
`<repo-basename>--<change-or-digest>` stem. The grouping is the
filename component preceding the `--<utc-timestamp>--<pid>` suffix,
identical to the identity the filename already encodes:
`<repo>--<change>` in OpenSpec compatibility mode and
`<repo>--<digest>` in custom mode. Each stem rotates independently,
so a repository juggling several active changes keeps 5 files per
stream rather than competing for 5 across all of them. Rotation
runs after each `PiAgent.Run` once the just-written file is closed
(so the newest file is never a deletion candidate while open) and
applies whether or not the agent run succeeded — any run that
produced a file is counted toward the stem's history. Deletion is
best-effort: a failure to remove an older file does not fail the
run, does not emit an event, and does not alter the `logPath` or
error `PiAgent.Run` returns. The batch-level
`see--<utc-timestamp>--<pid>.jsonl` event log is structurally
excluded (different filename shape, never matches a
`<stem>--` prefix) and is not bounded by rotation; it grows on
process restart, a separate cadence that is left to a future
change if it becomes a problem. The retention count is a fixed
implementation constant (`maxInvocLogsPerStem = 5` in `eventlog.go`)
and is not operator-configurable — add a `log_keep` knob only when
a real need appears.

### Migration from legacy custom workflow configuration

A former single custom workflow:

```yaml
prompt: "Apply {change}"
condition: "echo add-dark-mode"
commit: "see: apply {change}"
```

becomes a named entry in `workflows`:

```yaml
workflows:
  - name: add-dark-mode
    prompt: "Apply {change}"
    condition: "echo add-dark-mode"
    commit: "see: apply {change}"
```

Choose a distinct name for each independent automation. Workflow names are
used to distinguish events and to isolate persistent branches and logs.

### Migration from legacy watch configuration

`see` no longer reads the old `os.UserConfigDir()/see/watches` plain-text file
or accepts the old `watches` field in `config.yaml`. Choose one common root,
move the relative portions of the old entries into `include`, and add basename
filters to `exclude` when needed:

```text
~/Dev/*
```

or:

```yaml
watches:
  - "~/Dev/*"
```

becomes:

```yaml
root_dir: "~/Dev"
include: [] # every immediate child
exclude:
  - "bin"
```

Quote globs so YAML does not parse brackets or asterisks unexpectedly. Remove
the old file and remove `--watch` from scripts or aliases before restarting
`see`.

### Prompt precedence

For OpenSpec compatibility mode, the effective prompt template is selected in
this order:

1. Nonblank `--prompt` value (a flag overrides everything).
2. Embedded `prompt.md` default (the build-time fallback).

Configured workflow entries always provide their own prompt template. The
`{change}` token is rendered separately for each workflow.

### Configuration loading and `--config`

`main()` calls `loadStartupConfig(configFlag)` once after parsing flags.
The flag accepts three values:

- **Unset or empty**: load the default `os.UserConfigDir()/see/config.yaml`.
- **An explicit path**: tilde-expand and load that file (for example, a
  project-local or shared dotfiles configuration).
- **`-`**: skip the configuration entirely — return a zero-value `Config`
  without resolving or reading the file, so a malformed configuration
  never blocks startup when the operator has explicitly opted out.

The loader applies known-field checking and rejects multi-document input
for any file it does read.

### First-run bootstrap of the default config file

When the loader runs against the default path (`--config` unset or
empty) and the path does not exist, `ensureDefaultConfig` writes the
embedded `config.example.yaml` template to it before `loadConfig`
reads. The parent directory `os.UserConfigDir()/see/` is created with
mode `0o755` if absent; the file is created with mode `0o644`. The
template contains only YAML comments and a header, so `loadConfig`
decodes it to a zero-value configuration — bootstrap has zero
behavioral effect, only a discoverable file on disk.

Bootstrap fires only on the default-path branch. `--config=-` and
`--config=<path>` never write. An existing configuration file is
never overwritten, regardless of its contents (empty,
comments-only, valid, or malformed). A bootstrap write failure
(permission denied, read-only filesystem, parent unwritable) is
non-fatal: `loadStartupConfig` writes one line to standard error
(stderr) naming the target path and the underlying error, then
returns a zero-value `Config` so the current-working-directory fallback still
produces a working watch list. The watcher starts regardless of bootstrap
outcome.

## Custom Workflows

A configured `workflows` sequence switches `see` into custom workflow mode for
every watched repository. Each workflow's shell command becomes its work-existence
predicate; the OpenSpec resolver, archival-completion check, and default commit
subject are replaced for that workflow. Workflows are evaluated in their
configuration order. Omitting `workflows` keeps the OpenSpec contract:
`openspec/changes/` directories drive work, archival counts as completion, and
the catch-up commit subject is `see: apply openspec change <change>`.

Custom workflows may also be direct, non-hidden `.md` children of
`workflows_dir`. The filename without `.md` is the workflow name and determines
alphabetical execution order; a frontmatter `name` key is accepted but ignored.
YAML frontmatter supplies required `condition` and `commit` values and an
optional `model`, `disable` (default `false`, parks the workflow), `check`
(absent or blank means no gate), and `measure` (absent falls back to the
convention script at `~/.config/see/measure/<workflow-name>.sh`; blank
fails startup), while the Markdown body is the prompt. File
workflows run before `config.yaml` workflows, and a name collision between the
sources fails startup.

Custom workflows run under whichever [lane isolation mode](#lane-isolation-modes)
the operator selected (`branch` by default, `worktree` opt-in); the mode applies
uniformly to every workflow.

### Startup requirements

Custom mode is validated once at startup. Every configured workflow requires a
nonblank prompt, condition, and commit template, plus a unique nonblank name.
Missing or duplicate fields fail fast with an actionable message before the
watcher starts. The configured `commit` value is trimmed; whitespace-only is
treated as blank.

### Shell contract

The condition runs in the repository's working directory under the
platform shell:

- `/bin/sh -c <condition>`.

The watcher's context is attached so SIGINT/SIGTERM cancels an
in-flight condition. On Unix the child is placed in its own process
group so canceling the watcher does not strand descendants. The
shell string is not interpolated or parsed by `see`; quoting and
expansion belong to the shell. Configure conditions as
side-effect-free predicates; they execute before branch isolation,
so anything they write to the working tree will be picked up by the
agent.

Exit semantics:

- **exit `0`** — work exists. Standard output (after a trailing
  carriage-return/line-feed trim) becomes the change value for the
  rest of the run.
- **exit `1`** — no work. The repository is idle; the watcher
  emits `RepoSeen{HasChange: false}` and moves on.
- **any other exit** — condition failure. `see` captures standard
  error, surfaces it in the `see:` error message and the `Warning`
  event (when one applies), and treats the run as failed.

Output rules:

- Trailing `\r` and `\n` bytes are stripped.
- The remaining value must contain at least one non-whitespace
  character; an empty or whitespace-only stdout is a failure.
- The remaining value must be single-line; embedded `\r` or `\n`
  is a failure. Multi-line payloads can emit an identifier and let
  the prompt tell the agent how to retrieve details.

### Stable identity from a single-line value

Custom mode hashes the normalized change value with the Secure Hash
Algorithm 256-bit (SHA-256) digest, lowercased to hexadecimal, and
uses it as the persistent branch suffix. The branch checked out
during a custom run is therefore `see/<digest>`. The same digest
is used as the per-invocation log filename component so raw
condition output never reaches a filesystem path.

Events, prompts, commit messages, and the Terminal User Interface
(TUI) keep the human-readable change value; only the branch and
log filename are opaque. Any non-newline byte change in the
condition's stdout selects a different branch and a different log
file.

### Templates: `{change}` in prompt and commit

The single token `{change}` is replaced in both the selected prompt
template and the configured `commit` template before either is
handed to the agent or `git commit`. Unknown tokens are preserved
verbatim. The substitution uses the normalized change value (the
same value that was hashed), not the digest.

### Persistent per-change lanes

*(Branch mode — see [Lane isolation modes](#lane-isolation-modes)
for the worktree-mode contract.)* Each unique change value owns one
lane `see/<digest>`. The lane exists for as long as the operator wants
that change active; the watcher does not delete it on success.

`see` rejects a dirty working tree (tracked or untracked changes;
ignored files do not count) before touching any branch, so a
later rollback cannot delete operator edits. The lane lifecycle is
then:

- **Lane does not exist** — created at the current commit.
- **Lane exists and HEAD is on it** — preserved as-is; no reset,
  prior commits stay in place.
- **Lane exists and HEAD is on a clean branch or another lane** —
  switched to the lane via `git switch`, leaving its commits
  intact. The dirty-tree guard above is the only safety check on
  the transition.

Rollback is mode-aware:

- **Existing lane** — reset to the commit captured immediately
  before the attempt; untracked files created by the attempt are
  removed (`git clean -fd`, no `-x`, so ignored caches survive);
  the lane and its earlier commits stay.
- **Lane created by the failed attempt** — return to the original
  branch, reset to the captured commit, delete the new lane.

Cleanup steps are best-effort: each failure emits a `Warning` event
without replacing the agent error. Ignored files (per the
`.gitignore`) are outside the rollback guarantee to avoid deleting
caches or local configuration that predated the run.

After all workflows for a repository are processed, the final
usable active workflow lane stays checked out so the next
polling pass resumes from there. A run where every condition
exits with status `1` leaves the branch that was originally
checked out untouched.

### Catch-up commit

After a successful custom run, `see` stages all working-tree
changes (`git add -A`) and inspects the staged diff:

- **staged changes exist** — commit them with the rendered `commit`
  template.
- **no staged changes** — return success without a commit and
  without a `Warning`. An idempotent run whose agent already
  committed everything, or made no changes, stays warning-free.

The lane stays checked out after the catch-up commit so the next
polling pass resumes the same persistent branch.

### Level-triggered polling and retries

Continuous polling (`Watcher.PollInterval`, default five minutes)
waits between successful passes. A condition that remains true
will trigger another run each interval; a condition that turns
false becomes an idle no-op. Each retry attempt re-evaluates the
condition, so a retry can:

- exit `0` and continue work,
- exit `1` and become idle (no `ChangeFailed`),
- select a different change value and switch to the corresponding
  lane.

A check failure behaves like an agent failure for retry purposes: it
returns an error, `runWithRetry` retries up to `RetryCount` times,
each retry re-resolves the change and re-runs the agent from a
clean slate, and the terminal event for the final failure is
`CheckFailed` (not `ChangeFailed`). `RetryAttempt` events between
attempts carry the concise "check failed" summary unchanged.

### Check gate

A configured workflow may carry an optional `check` shell command
that runs after a successful agent attempt and before any git
landing operation. The command uses the same platform-shell contract
as `condition` (`/bin/sh -c`), the
watcher context is attached, and the shell runs in its own
process group so cancellation does not strand descendants. The
literal token `{change}` is substituted under the same rule as
`prompt`, `condition`, and `commit`.

- **No-op skip.** The check runs only when the working tree is dirty
  with changes to land (the same dirtiness check that gates the
  catch-up commit). An idempotent successful run whose agent committed
  everything itself or changed nothing stays warning-free and skips
  the check, so a routine poll never spins up a test suite.
- **Pass.** Exit status `0` lets the existing landing path run
  unchanged: branch-mode catch-up commit, or worktree-mode catch-up
  commit + rebase (+ fast-forward merge when `auto_merge: true`).
- **Fail.** Any nonzero exit produces a `*checkFailedError` carrying
  the rendered command, the integer exit code, and the captured
  stderr. The active mode rolls back to a clean slate (branch mode:
  reset to the pre-attempt lane tip + `git clean -fd`; worktree
  mode: remove the worktree + delete the lane with `-D`). No commit
  is ever created for the attempt, and the operator's checkout is
  untouched in worktree mode.
- **Retry.** A check failure flows through `runWithRetry` like an
  agent failure: the agent gets up to `RetryCount` fresh attempts per
  poll to satisfy the gate.
- **Terminal event.** When the final attempt for a repository fails
  at the check, `runOnce` emits a `CheckFailed` event (carrying the
  rendered command, exit code, and stderr) instead of `ChangeFailed`.
  The two are mutually exclusive for the same final failure; the
  selection is driven by `errors.As` on `*checkFailedError` so a
  rollback wrapper around the sentinel still selects the check
  variant.

### Measure gate

A configured workflow may carry an optional `measure` shell command
that gates landing on strict improvement. It runs **twice** per
attempt: once before the agent to capture the **baseline**, and once
after a passing check (or after the agent when no check is defined) to
capture the **candidate**. Both values are held in `see`'s memory and
are never written where the agent can read them. The measure command
uses the same platform-shell contract as `condition` and `check`
(`/bin/sh -c`), the watcher context
is attached, and the shell runs in its own process group so
cancellation does not strand descendants. The command runs with its
working directory set to the agent's working directory (the lane
checkout in branch mode, the worktree directory in worktree mode).
The literal token `{change}` is substituted into the `measure` command
under the same rule as `prompt`, `condition`, `commit`, and `check`;
the token `{metric}` is **not** substituted into `measure` (it is the
producer of `{metric}`, not a consumer) and remains literal if present.

- **Two definition forms.** A nonblank `measure:` field (frontmatter
  or `config.yaml` value) wins. Otherwise, the regular file at
  `~/.config/see/measure/<workflow-name>.sh` is read and its contents
  executed as the measure command. A missing convention directory or
  a missing `<name>.sh` means "no measure gate" and is not an error.
  A present-but-blank `measure:` is a startup error.
- **Baseline before the agent.** For an attempt where a measure gate
  is resolved, `see` runs the measure command once before the agent to
  capture the baseline and parses it as a 64-bit floating-point
  number. A baseline-measure failure aborts the attempt before the
  agent runs, rolls back to a clean slate, and emits `MeasureFailed`.
  The baseline value is held in memory and rendered as the `{metric}`
  substitution in `prompt` and `commit`.
- **Candidate after the agent.** After a passing check (or after the
  agent when no check is defined), `see` runs the candidate measure
  regardless of whether the working tree is dirty (a non-deterministic
  metric may differ on an unchanged tree). The candidate is parsed as
  a 64-bit floating-point number.
- **Strict improvement lands.** Candidate strictly greater than baseline
  → land the agent's changes via the existing catch-up commit /
  rebase / ff-merge path. Candidate equal to or less than baseline →
  `measureFailedError`. The candidate measure is also a failure on
  nonzero exit, empty / multi-line output, or a value that cannot be
  parsed as a number.
- **Rollback to clean slate.** A measure failure does not create a
  commit. The active mode's rollback runs as if it were a check
  failure: branch mode resets the lane tip + `git clean -fd`; worktree
  mode removes the worktree + deletes the lane with `-D`. The
  operator's checkout is untouched in worktree mode.
- **Retry.** A measure failure flows through `runWithRetry` like an
  agent or check error: the agent gets up to `RetryCount` fresh
  attempts per poll, each re-resolving the change and re-capturing
  the baseline. `RetryAttempt` events between attempts carry the
  measure-failure summary unchanged.
- **Terminal event.** When the final attempt for a repository fails at
  the measure, `runOnce` emits a `MeasureFailed` event (carrying the
  rendered command, exit code, baseline, candidate, and stderr)
  instead of `ChangeFailed` or `CheckFailed`. The three are mutually
  exclusive for the same final failure; the selection is driven by
  `errors.As` on `*measureFailedError` so a rollback wrapper around
  the sentinel still selects the measure variant.
- **`{metric}` token.** When a measure gate is resolved, `{metric}` is
  substituted with the normalized baseline value (the same string that
  was parsed as a float64) into `prompt` and `commit`. When no measure
  gate is resolved, `{metric}` remains literal in those templates.
  `{metric}` is never substituted into `condition`, `check`, or
  `measure`.
- **Integrity model.** When the measure is supplied as the convention
  file or as an inline frontmatter value, the script lives outside the
  watched repository, so the agent — which runs in the repository
  working directory — cannot casually read or execute it. The agent
  receives the *value* to beat (`{metric}` in the prompt) but not the
  *mechanism*. An operator who points `measure` at an in-repository
  path forfeits this tamper-resistance. The residual gap — a
  determined agent reading `~/.config/see/measure/<name>.sh` by
  absolute path on a shared filesystem — is accepted; closing it would
  require a sandbox `see` does not provide today.

### JSONL event payload migration

The repository-availability event field has been renamed from
`HasOpenspec` to `HasChange`. Custom and OpenSpec runs now use the
same field; a custom condition that exits `0` and an OpenSpec
resolver that finds an active change both set `HasChange: true`,
and a custom condition that exits `1` and an OpenSpec repo with
no active change both set `HasChange: false`. The previous field
name is no longer emitted. Consumers reading the batch-level
JavaScript Object Notation Lines (JSONL) stream need to read
`HasChange` instead of `HasOpenspec`; no dual-write period.

## Lane isolation modes

`see` supports three explicit isolation modes, selected by the pair
`(worktree, auto_merge)`. The mode applies uniformly to every watched
repository for the lifetime of the process; it is resolved once at
startup from CLI flags and config fields and cannot switch mid-run.

- **branch** (`worktree: false`, the default) — the agent runs in the
  operator's checkout. The lane `see/<digest>` is created on that
  checkout, success leaves the checkout on the lane, and the lane lives
  until the operator removes it. This is the historical contract and is
  unchanged.
- **worktree + auto-merge** (`worktree: true, auto_merge: true`, the
  default when `worktree: true`) — the agent runs in a `git worktree`
  linked to the operator's checkout. The operator's checkout is never
  switched. On success the lane is rebased onto the operator's *current*
  branch tip (so commits the operator made during the run are preserved
  and the agent's commits replay on top) and fast-forward merged into
  it; the lane and worktree are removed after the merge.
- **worktree + manual-merge** (`worktree: true, auto_merge: false`) — as
  above, but on success the rebased lane is left in place (with the
  worktree directory) for manual review instead of being merged.

The operator experience: in branch mode the operator's checkout is on
the lane during and after a run, so they must `git checkout` back to
their branch. In both worktree modes the operator's checkout stays on
their own branch for the whole run; with auto-merge the agent's commits
land on their branch automatically, and with manual-merge the operator
inspects and merges the rebased lane themselves.

### Worktree lifecycle and rollback

Before each attempt in worktree mode, `see` prunes stale worktree
metadata (`git worktree prune`), reuses an existing worktree path or
clears an orphan directory at it, and runs `git worktree add --force -B
see/<digest> <worktree_root>/<repo-basename>--<digest> <start>`. The
start point is the existing lane tip when the lane already exists
(preserving prior commits) and the operator's `HEAD` otherwise. The
operator's checkout is never switched, reset, or checked out by `see`
while worktree mode is active.

A dirty operator checkout (tracked or non-ignored untracked changes;
ignored files do not count) blocks an attempt before the worktree is
created, as defense in depth against condition commands that write to
the operator's tree. On any failure (dirty tree, worktree creation,
agent error, rebase conflict, or fast-forward failure) `see` runs the
full cleanup in order and ignores individual step failures, emitting a
`Warning` for each that fails: `git rebase --abort` (state lives in the
worktree), `git merge --abort` (state lives in the operator's checkout),
`git worktree remove --force <path>`, and `git branch -D see/<digest>`.
The lane is always deleted on rollback in worktree mode (`-D`); the
operator's checkout is untouched.

### Configuration

Mode selection uses CLI-flag > config-field > default precedence,
identical to `--prompt`. The flags are `--worktree` (boolean, default
`false`), `--auto-merge` (boolean, default `true`), and
`--worktree-root` (string, default `~/.cache/see/worktrees`); the
corresponding config fields are `worktree`, `auto_merge`, and
`worktree_root`. `--worktree` overrides `worktree`, `--auto-merge`
overrides `auto_merge`, and `--worktree-root` overrides `worktree_root`.
`worktree_root` is tilde-expanded like `root_dir`.

Validation rejects invalid combinations at startup with exit status `2`:

- `auto_merge: true` (explicit) with `worktree: false`.
- an explicit `--auto-merge` flag (either form) without `--worktree`
  (and without `worktree: true` in config).
- a non-empty `worktree_root` with `worktree: false`.

An explicit `auto_merge: false` in branch mode is accepted as a harmless
no-op (the field is simply not consulted), and a plain default
branch-mode run is never rejected despite `auto_merge`'s runtime default
of `true`.

### Migration from branch mode

No migration is required: operators who do not set `worktree: true` see
no behavioral change. To opt in, add `worktree: true` (and optionally
`auto_merge: false` / `worktree_root:`) to the config and restart `see`,
or pass `--worktree` for a single run.

The first worktree-mode run uses `git worktree add -B see/<digest>`,
which reuses any existing `see/<digest>` branch from prior branch-mode
runs. The operator's checkout transitions from "checked out on the
lane" to "checked out on their own branch" because worktree mode never
switches the checkout. Any leftover worktree directories under
`~/.cache/see/worktrees/` (or a configured `worktree_root`) can be
removed with `git worktree remove --force <path>` or `rm -rf` once the
`.git/worktrees/` metadata is pruned; leftover `see/<digest>` branches
from a manually-merged attempt can be removed with
`git branch -D see/<digest>`.
