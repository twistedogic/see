# tui delta — add-jsonl-stdout-mirror

## MODIFIED Requirements

### Requirement: `see` exposes a `--mode` flag selecting the output mode
`see` SHALL accept a `-mode` string flag. The flag SHALL accept the
values `tui` and `log`. The flag SHALL default to `tui`. When
`-mode` is `tui`, `see` SHALL render the live status grid via the
`tui` package and SHALL wire the `Watcher.observer` to an
`eventLogger` that fans events out to both the JSONL file and the
TUI's `ChanObserver`. When `-mode` is `log`, `see` SHALL wire the
`Watcher.observer` to an `eventLogger` with no secondary observer;
the JSONL file SHALL be the primary sink, and `see` SHALL also
mirror the JSONL line-for-line onto stdout when stdout is not a
terminal (a pipe or a redirect). When stdout IS a terminal,
`--mode=log` SHALL stay silent — the on-disk JSONL remains the
operator's only view in that case. The mirror SHALL receive the
same encoded bytes the JSONL file receives (one event per line,
in emission order) so `see --mode=log | jq` parses identically
to `cat <jsonl-file>`. The flag SHALL have no effect on agent
invocation, retry policy, or git rollback semantics.

#### Scenario: Default invocation renders the TUI
- **WHEN** `see` is invoked with no flags
- **THEN** mode resolves to `tui`
- **THEN** an observer IS wired to the `Watcher`
- **THEN** the live status grid renders via the TUI package
- **AND** a batch-level JSONL file is created under the log
  directory

#### Scenario: `--mode=log` is silent and writes only to JSONL
- **WHEN** `see --mode=log` is invoked and stdout IS a terminal
- **THEN** an observer IS wired to the `Watcher`
- **THEN** no `log.Printf` output is written to stderr
- **THEN** stdout is empty for the lifetime of the process
- **THEN** every watcher event lands in the JSONL file in
  emission order
- **THEN** the exit status on first-repo failure is non-zero,
  identical to a pre-TUI invocation

#### Scenario: `--mode=log` mirrors JSONL to stdout when stdout is not a TTY
- **WHEN** `see --mode=log` is invoked and stdout IS NOT a terminal
  (piped output, redirected to a file, captured by a CI runner)
- **THEN** an observer IS wired to the `Watcher`
- **THEN** every watcher event lands in the JSONL file in
  emission order
- **THEN** every watcher event ALSO lands on stdout, encoded as
  one line per event, byte-identical to the on-disk JSONL
- **THEN** no `log.Printf` output is written to stderr
- **THEN** the exit status on first-repo failure is non-zero,
  identical to a pre-TUI invocation

#### Scenario: `--mode=log` JSONL stream is pipe-parsable as line-delimited JSON
- **WHEN** `see --mode=log | jq` runs against any fixture
- **THEN** `jq` decodes one record per line on stdout
- **AND** each decoded record's fields match the underlying
  `Event` payload (e.g., `RepoSeen.Path`, `ChangeStarted.Change`)
