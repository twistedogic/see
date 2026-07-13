## ADDED Requirements

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