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
  - Prefer unit tests that drive `Watcher.work` (or a single `runOnce`
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
watches:
  - "~/Dev/*"
  - "/var/repos"

prompt: |-
  Apply the OpenSpec change "{change}".
```

- `watches` is a sequence of strings. Each entry follows the same path,
  tilde expansion, and shell-glob rules as `--watch` (`~`, `~/path`, `*`,
  `?`, `[abc]`; `**` is rejected). Tilde is the only expansion performed;
  environment variables are not expanded.
- `prompt` is a string. Literal-block scalars (`|`, `|-`, `|+`) preserve
  interior line breaks; use `|-` to strip the trailing newline. The single
  token `{change}` is replaced with the active change name at runtime; no
  other tokens are substituted.

Both fields are optional. Omitting `watches` preserves the
current-working-directory fallback. Omitting `prompt` falls through to the
embedded `prompt.md` default.

### Migration from the legacy plain-text `watches` file

`see` no longer reads the old `os.UserConfigDir()/see/watches` plain-text
file. To migrate, replace each non-comment line with an entry under
`watches` in `config.yaml`:

```text
~/Dev/*
/var/repos
```

becomes:

```yaml
watches:
  - "~/Dev/*"
  - "/var/repos"
```

Quote globs so YAML does not parse brackets or asterisks unexpectedly.
Remove the old file after verifying startup.

### Prompt precedence

The effective prompt template is selected in this order:

1. Nonblank `--prompt` value (a flag overrides everything).
2. Nonblank `config.yaml` `prompt` value (a user-default).
3. Embedded `prompt.md` default (the build-time fallback).

"Blank" means whitespace-only. `--config=-` skips the global file
entirely: configured watches and the configured prompt both fall through
to CLI values and the embedded default respectively.

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
returns a zero-value `Config` so the command-line entries and the
current-working-directory fallback still produce a working watch
list. The watcher starts regardless of bootstrap outcome.
