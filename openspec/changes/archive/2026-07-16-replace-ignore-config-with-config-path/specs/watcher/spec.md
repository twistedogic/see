## MODIFIED Requirements

### Requirement: Discovery resolves the watch list from layered sources

`see` SHALL resolve the list of repositories to watch from three
sources, applied in this precedence order:

1. The repeatable `--watch <pattern>` flag, if any flag is present.
2. The `watches` sequence in the configuration file selected by
   `--config <path>` (default: `os.UserConfigDir()/see/config.yaml`),
   unless `--config=-` is set.
3. The current working directory as a fallback when steps 1 and 2
   produce no entries.

The first source that contributes at least one entry SHALL be the
only source consulted for the watch list: flag entries replace
configured entries entirely rather than union with them, mirroring
the precedence rule already used for the prompt template
(`--prompt` > configured `prompt` > embedded default). This gives
every layered configuration knob one consistent rule. `--config=-`
is the escape hatch for the case the precedence rule does not
cover: when the configuration file is malformed and must not be
read at startup. An explicit `--config=<path>` selects a non-default
configuration file.

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

#### Scenario: --config=- skips the config layer

- **WHEN** `see --config=- --watch ~/only/repo` is invoked with the
  default `config.yaml` listing `~/other/repo`
- **THEN** the resolved watch list contains only `~/only/repo`
- **AND** the configuration file is not consulted

#### Scenario: --config=<path> loads the named file

- **WHEN** `see --config=~/team/see.yaml` is invoked with the
  default `config.yaml` listing `~/work/*` and `~/team/see.yaml`
  listing `~/only/repo`
- **THEN** the resolved watch list contains only `~/only/repo`
- **AND** the default `config.yaml` is not consulted

#### Scenario: Missing config file is not an error

- **WHEN** `config.yaml` does not exist
- **THEN** resolution proceeds with command-line entries and the
  current-working-directory fallback as if the file were empty
- **AND** no error is returned

#### Scenario: Malformed config line is fatal at startup

- **WHEN** the configuration file selected by `--config` cannot be
  read or parsed according to the global configuration schema
- **THEN** `see` prints one actionable error identifying the
  configuration file and exits with status `2`
- **AND** the watcher does not start

### Requirement: Watcher renders the agent prompt from a configurable template

`see` SHALL select the process-wide agent prompt template in this order:

1. A `--prompt` value containing at least one non-whitespace character.
2. A `prompt` value from the configuration file selected by
   `--config <path>` (default: `os.UserConfigDir()/see/config.yaml`)
   containing at least one non-whitespace character, unless
   `--config=-` is set.
3. The default template embedded into the binary from the in-tree file
   `prompt.md` at the repository root.

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