## ADDED Requirements

### Requirement: Global configuration uses a strict YAML schema

`see` SHALL read global configuration from the single mapping document at `filepath.Join(os.UserConfigDir(), "see", "config.yaml")`. The mapping SHALL accept only these optional top-level fields:

- `watches`: a sequence of string watch patterns.
- `prompt`: a string agent prompt template, including YAML Ain't Markup Language (YAML) literal block scalars for multiline text.

A missing or empty file SHALL produce a zero-value configuration without error. Malformed YAML, additional YAML documents, unknown fields, and values whose types do not match the schema SHALL be fatal startup errors. The error SHALL identify the configuration file and retain available line information, and the watcher SHALL NOT start.

The legacy `filepath.Join(os.UserConfigDir(), "see", "watches")` file SHALL NOT be read, merged, or used as a fallback.

#### Scenario: Valid configuration loads both fields

- **WHEN** `config.yaml` contains a `watches` string sequence and a multiline `prompt` string
- **THEN** `see` loads both values from the same document
- **AND** the prompt text preserves the block scalar's line breaks

#### Scenario: Missing configuration is empty configuration

- **WHEN** `config.yaml` does not exist
- **THEN** configuration loading succeeds with no watches and no configured prompt

#### Scenario: Unknown field is rejected

- **WHEN** `config.yaml` contains the misspelled field `promt`
- **THEN** `see` reports an error identifying the unknown field and configuration file
- **AND** exits with status `2` before the watcher starts

#### Scenario: Invalid field type is rejected

- **WHEN** `config.yaml` defines `watches` as a mapping instead of a sequence of strings
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

## MODIFIED Requirements

### Requirement: Discovery resolves the watch list from layered sources

`see` SHALL resolve the list of repositories to watch from three sources:

1. The repeatable `--watch <pattern>` flag.
2. The `watches` sequence in `os.UserConfigDir()/see/config.yaml`, unless `--ignore-config` is set.
3. The current working directory as a fallback when steps 1 and 2 produce no entries.

The flag entries and configured entries SHALL be unioned, not replaced: an operator may pass `--watch` to add one repository to whatever the configuration already lists.

#### Scenario: No flag and no configured watches falls back to current working directory

- **WHEN** `see` is invoked with no `--watch`, `config.yaml` is absent or has no watch entries, and the working directory contains two git repositories as immediate subdirectories
- **THEN** the resolved watch list contains both repositories
- **AND** neither the batch-level JavaScript Object Notation Lines (JSONL) stream nor the Terminal User Interface (TUI) reflects any new source

#### Scenario: Flag adds to configured watches

- **WHEN** `see --watch /extra/repo` is invoked with `config.yaml` containing `watches: ["~/work/*"]`
- **THEN** the resolved watch list contains every match of `~/work/*` plus `/extra/repo`
- **AND** duplicates between the two sources collapse to a single entry

#### Scenario: --ignore-config skips configured watches

- **WHEN** `see --ignore-config --watch ~/only/repo` is invoked with `config.yaml` listing `~/other/repo`
- **THEN** the resolved watch list contains only `~/only/repo`
- **AND** the configuration file is not consulted

#### Scenario: Missing config file is not an error

- **WHEN** `config.yaml` does not exist
- **THEN** resolution proceeds with command-line entries and the current-working-directory fallback as if the file were empty
- **AND** no error is returned

#### Scenario: Invalid config is fatal at startup

- **WHEN** `config.yaml` cannot be read or parsed according to the global configuration schema
- **THEN** `see` prints one actionable error identifying the configuration file and exits with status `2`
- **AND** the watcher does not start

### Requirement: Watcher renders the agent prompt from a configurable template

`see` SHALL select the process-wide agent prompt template in this order:

1. A `--prompt` value containing at least one non-whitespace character.
2. A `prompt` value from `config.yaml` containing at least one non-whitespace character, unless `--ignore-config` is set.
3. The default template embedded into the binary from the in-tree file `prompt.md` at the repository root.

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

#### Scenario: Ignored configuration cannot supply prompt

- **WHEN** `config.yaml` contains `prompt: "Configured {change}"` and `see` is invoked with `--ignore-config` and no nonblank `--prompt`
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

#### Scenario: Unknown tokens are preserved verbatim

- **WHEN** `SetPromptTemplate("Apply {change} in {repo} on {date}")` is called on a `Watcher` and `Watcher.work` invokes `Agent.Run` for any change
- **THEN** the prompt passed to `Agent.Run` substitutes only `{change}`
- **AND** `{repo}` and `{date}` appear unchanged in the output

#### Scenario: Empty change name still renders

- **WHEN** `SetPromptTemplate("prefix {change} suffix")` is set and `Watcher.work` invokes `Agent.Run` for an empty change name
- **THEN** the prompt passed to `Agent.Run` is `"prefix  suffix"` with a single space where the change name was

#### Scenario: prompt.md build-time embedding

- **WHEN** the binary is built without a `prompt.md` file at the repository root alongside `main.go`
- **THEN** the build fails at compile time with an error pointing at the `//go:embed` directive
- **AND** no binary is produced
