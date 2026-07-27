# watcher delta — add-log-dir-config

## RENAMED Requirements

- FROM: `### Requirement: Default log location is the OS cache directory`
- TO: `### Requirement: Log directory resolves from env, config, or a default`

## MODIFIED Requirements

### Requirement: Log directory resolves from env, config, or a default

`see` SHALL resolve the log directory for per-invocation agent logs
and the batch-level event log from three sources, applied in this
precedence order:

1. The `SEE_LOG_DIR` environment variable, when set to a non-empty
   string.
2. The top-level `log_dir` configuration field, when set to a value
   containing at least one non-whitespace character.
3. The default `~/.cache/see/logs`.

The first source that supplies a value SHALL be the only source
consulted. `SEE_LOG_DIR` SHALL win over a configured `log_dir` so an
operator with the environment variable set keeps getting the
directory they expect; the config field is the common path and the
environment variable stays the override. A whitespace-only `log_dir`
configuration value SHALL be treated as unset and SHALL fall through
to the default.

Every source SHALL be tilde-expanded using the same rule as
`root_dir`: a leading `~` or `~/` is replaced with the user's home
directory (`$HOME` when set, otherwise the result of
`os.UserHomeDir()`), and `~foo` (without a slash) is treated as a
literal path. Environment-variable expansion (`$VAR`) SHALL NOT be
performed. This applies uniformly to the environment variable, the
configured field, and the default, so `SEE_LOG_DIR=~/logs` resolves
to `<home>/logs` rather than creating a literal directory named `~`.

The resolved directory SHALL be created with `MkdirAll` (mode
`0o755`) if it does not exist before the event log is opened. A
failure to create the directory (permission denied, a path component
that is a file, a read-only parent) SHALL be a fatal startup error
that prints one actionable message naming the resolved path and the
underlying cause and exits with status `2` before the watcher starts.

The resolved directory SHALL be guaranteed to exist and be writable
by the time `PiAgent.Run` is called, and `Run` SHALL NOT attempt to
create the directory and SHALL NOT fall back to running the agent
without capture (unchanged from the prior contract; only the
resolution source and default location change).

#### Scenario: No env var uses default

- **WHEN** `SEE_LOG_DIR` is unset or empty
- **AND** `config.yaml` omits `log_dir` or sets it to a blank value
- **THEN** the log directory is `<home>/.cache/see/logs/`
- **AND** no `see/logs/` segment is appended beneath it

#### Scenario: Config log_dir overrides the default

- **WHEN** `SEE_LOG_DIR` is unset or empty
- **AND** `config.yaml` sets `log_dir: "~/Dev/.see-logs"`
- **AND** the home directory is `/home/alice`
- **THEN** the log directory is `/home/alice/Dev/.see-logs`
- **AND** no `see/logs/` segment is appended

#### Scenario: Env var overrides default

- **WHEN** `SEE_LOG_DIR` is set to `/var/log/see`
- **AND** `config.yaml` sets `log_dir: "~/Dev/.see-logs"`
- **THEN** the log directory is `/var/log/see`
- **AND** the configured `log_dir` is not consulted

#### Scenario: Whitespace-only log_dir falls through to the default

- **WHEN** `SEE_LOG_DIR` is unset or empty
- **AND** `config.yaml` sets `log_dir: "   "`
- **THEN** the log directory is `<home>/.cache/see/logs/`

#### Scenario: Tilde in SEE_LOG_DIR is expanded

- **WHEN** `SEE_LOG_DIR` is set to `~/logs`
- **AND** the home directory is `/home/alice`
- **THEN** the log directory is `/home/alice/logs`
- **AND** no literal directory named `~` is created

#### Scenario: Tilde in a configured log_dir is expanded

- **WHEN** the home directory is `/home/alice`
- **AND** `config.yaml` sets `log_dir: "~/see-logs"`
- **THEN** the resolved log directory is `/home/alice/see-logs`

#### Scenario: --config=- skips the config layer for log_dir

- **WHEN** `see --config=-` is invoked
- **AND** the default `config.yaml` sets `log_dir: "~/Dev/.see-logs"`
- **AND** `SEE_LOG_DIR` is unset
- **THEN** the log directory is `<home>/.cache/see/logs/`
- **AND** the configured `log_dir` is not consulted

#### Scenario: Absent directory is created

- **WHEN** the resolved log directory does not exist and its parent
  is writable
- **THEN** `see` creates the directory (and any missing parents) with
  mode `0o755`
- **AND** startup continues

#### Scenario: Uncreatable directory is a fatal startup error

- **WHEN** the resolved log directory cannot be created because a path
  component is a regular file, or the parent is read-only, or
  permission is denied
- **THEN** `see` prints one line naming the resolved path and the
  underlying cause
- **AND** exits with status `2` before the watcher starts
- **AND** the event log is not opened
