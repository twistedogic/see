# watcher delta — make-cli-flags-override-config

## MODIFIED Requirements

### Requirement: Discovery resolves the watch list from layered sources

`see` SHALL resolve the list of repositories to watch from three
sources, applied in this precedence order:

1. The repeatable `--watch <pattern>` flag, if any flag is present.
2. The `watches` sequence in `os.UserConfigDir()/see/config.yaml`,
   unless `--ignore-config` is set.
3. The current working directory as a fallback when steps 1 and 2
   produce no entries.

The first source that contributes at least one entry SHALL be the
only source consulted for the watch list: flag entries replace
configured entries entirely rather than union with them, mirroring
the precedence rule already used for the prompt template
(`--prompt` > configured `prompt` > embedded default). This gives
every layered configuration knob one consistent rule. `--ignore-config`
remains the escape hatch for the case the precedence rule does not
cover: when the configuration file is malformed and must not be
read at startup.

#### Scenario: No flag, no config falls back to cwd

- **WHEN** `see` is invoked with no `--watch`, `config.yaml` is
  absent or has no watch entries, and the working directory contains
  two git repositories as immediate subdirectories
- **THEN** the resolved watch list contains both repositories
- **AND** neither the batch-level JavaScript Object Notation Lines
  (JSONL) stream nor the Terminal User Interface (TUI) reflects any
  new source

#### Scenario: Flag replaces config

- **WHEN** `see --watch /extra/repo` is invoked with `config.yaml`
  containing `watches: ["~/work/*"]`
- **THEN** the resolved watch list contains only `/extra/repo`
- **AND** the configured `~/work/*` entries are not consulted

#### Scenario: --ignore-config skips the config layer

- **WHEN** `see --ignore-config --watch ~/only/repo` is invoked
  with `config.yaml` listing `~/other/repo`
- **THEN** the resolved watch list contains only `~/only/repo`
- **AND** the configuration file is not consulted

#### Scenario: Missing config file is not an error

- **WHEN** `config.yaml` does not exist
- **THEN** resolution proceeds with command-line entries and the
  current-working-directory fallback as if the file were empty
- **AND** no error is returned

#### Scenario: Malformed config line is fatal at startup

- **WHEN** `config.yaml` cannot be read or parsed according to the
  global configuration schema
- **THEN** `see` prints one actionable error identifying the
  configuration file and exits with status `2`
- **AND** the watcher does not start
