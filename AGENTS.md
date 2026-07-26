# AGENTS.md

Guidelines for AI agents and contributors working on this project.

## Documentation

- Never use acronyms without explaining them in documents. The first time an acronym appears, spell out the full term followed by the acronym in parentheses (e.g., "Application Programming Interface (API)"). Subsequent uses may use the acronym alone.

## Commit Messages

- When writing a commit message, never add your agent name as author or co-author. Commits must reflect the human contributor as author and must not include agent names in the author or co-author trailers.

## Bug Fixes

- When doing a bug fix, always start by reproducing the bug and add a failing test case before changing production code. The failing test must demonstrate the bug, and the fix must turn it green. Never merge a bug fix without a regression test.

## Testing

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
  - Always run `go test -timeout 30s ./...` (or shorter). A wedged
    poll loop or goroutine should fail fast at 30 seconds rather than
    hitting the runner's default 10-minute ceiling and masking the
    real bug under a generic timeout.
  - Reserve spawning the binary for manual smoke checks and one-shot
    `see --once` runs against a fixture repo, never inside an
    automated test.

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
(Linux/macOS: `$XDG_CONFIG_HOME/see/config.yaml` or `~/.config/see/config.yaml`;
Windows: `%AppData%/see/config.yaml`). The file is YAML Ain't Markup Language
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

workflows:
  - name: openspec
    prompt: |-
      Apply the OpenSpec change "{change}".
    condition: "git rev-parse --abbrev-ref HEAD"
    commit: "see: apply {change}"
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
- `workflows` is an ordered sequence. Each entry requires a unique nonblank
  `name`, `prompt`, `condition`, and `commit`. Entries run in configuration
  order for every discovered repository; workflows are not run concurrently.
- A workflow `prompt` is a string. Literal-block scalars (`|`, `|-`, `|+`)
  preserve interior line breaks; use `|-` to strip the trailing newline. The
  single token `{change}` is replaced with that workflow's active change name;
  no other tokens are substituted.
- A workflow `condition` is a platform-shell command. Exit status `0` means
  work exists, exit status `1` means that workflow is idle, and any other
  nonzero status is a failure.
- A workflow `commit` is the catch-up commit message template. The same
  `{change}` substitution rule applies.

All fields are optional except the fields inside a configured workflow entry.
Omitting `workflows` preserves OpenSpec compatibility mode: OpenSpec changes
are discovered using the embedded prompt and the default commit subject.
The former top-level `prompt`, `condition`, and `commit` fields are not
accepted; migrate each old custom configuration into one named workflow.

The former `watches` field and `--watch` command-line flag are not accepted.

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

### Startup requirements

Custom mode is validated once at startup. Every configured workflow requires a
nonblank prompt, condition, and commit template, plus a unique nonblank name.
Missing or duplicate fields fail fast with an actionable message before the
watcher starts. The configured `commit` value is trimmed; whitespace-only is
treated as blank.

### Shell contract

The condition runs in the repository's working directory under the
platform shell:

- `/bin/sh -c <condition>` on Unix-like systems,
- `cmd.exe /C <condition>` on Windows.

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

Each unique change value owns one lane `see/<digest>`. The lane
exists for as long as the operator wants that change active; the
watcher does not delete it on success.

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
