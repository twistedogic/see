## MODIFIED Requirements

### Requirement: Discovery resolves the watch list from the configured root

`see` SHALL resolve the list of repositories to watch from the
configuration file selected by `--config <path>` (default:
`os.UserConfigDir()/see/config.yaml`), with the current working
directory as a fallback when the configured `root_dir` is blank or
absent. `--config=-` is the escape hatch for the case the precedence
rule does not cover: when the configuration file is malformed and
must not be read at startup. An explicit `--config=<path>` selects a
non-default configuration file. The watch list SHALL be derived from
the configured `root_dir` (after tilde expansion), filtered through
the optional `include` and `exclude` sequences, then handed to the
classification layer.

The `see` binary SHALL NOT accept a `--watch` flag. The
current-working-directory fallback SHALL be the only source consulted
when the configured `root_dir` is blank or absent. There is no
CLI-side precedence layer because there is no CLI watch input.

#### Scenario: No config falls back to cwd

- **WHEN** `see` is invoked with no `--config`, `config.yaml` is
  absent or has a blank `root_dir`, and the working directory
  contains two git repositories as immediate subdirectories
- **THEN** the resolved watch list contains both repositories
- **AND** neither the batch-level JavaScript Object Notation Lines
  (JSONL) stream nor the Terminal User Interface (TUI) reflects any
  new source

#### Scenario: Configured root resolves its children

- **WHEN** `config.yaml` sets `root_dir: "~/Dev"` and `~/Dev`
  contains immediate subdirectories `playground-rust` and `notes`
- **THEN** the resolved watch list contains `~/Dev/playground-rust`
  and `~/Dev/notes` (subject to classification and exclude filtering)

#### Scenario: --config=- skips the config layer

- **WHEN** `see --config=-` is invoked with the default
  `config.yaml` listing `root_dir: "~/Dev"`
- **THEN** the configuration file is not consulted
- **AND** the watch list falls back to the current working directory

#### Scenario: --config=<path> loads the named file

- **WHEN** `see --config=~/team/see.yaml` is invoked with the
  default `config.yaml` listing `root_dir: "~/Dev"` and
  `~/team/see.yaml` listing `root_dir: "~/only"`
- **THEN** the resolved watch list is built from `~/only`
- **AND** the default `config.yaml` is not consulted

#### Scenario: Missing config file is not an error

- **WHEN** `config.yaml` does not exist
- **THEN** resolution proceeds with the current-working-directory
  fallback as if the configuration were empty
- **AND** no error is returned

#### Scenario: Malformed config line is fatal at startup

- **WHEN** the configuration file selected by `--config` cannot be
  read or parsed according to the global configuration schema
- **THEN** `see` prints one actionable error identifying the
  configuration file and exits with status `2`
- **AND** the watcher does not start

### Requirement: Discovery expands `root_dir`, `include`, and `exclude` patterns

`root_dir`, `include`, and `exclude` patterns SHALL expand in three
ways:

- A leading `~` in `root_dir` or any `include` / `exclude` entry
  SHALL be replaced with the user's home directory (`$HOME` when
  set, otherwise the result of `os.UserHomeDir()`).
- `include` entries SHALL be expanded as glob patterns via
  `filepath.Glob` joined to `root_dir`. An `include` sequence that
  is absent or empty SHALL be treated as "every immediate child of
  `root_dir`".
- `exclude` entries SHALL be matched against the basename of each
  candidate via `filepath.Match`. An `exclude` sequence that is
  absent or empty SHALL be treated as "exclude nothing".
- Environment-variable expansion (`$VAR`) SHALL NOT be performed.
- A pattern containing `**` SHALL be rejected at config-load time
  with a clear error message that names the offending field, because
  `filepath.Match` does not support recursive globs and the project
  chooses not to add a dependency for the feature.

#### Scenario: root_dir tilde expansion

- **WHEN** the home directory is `/home/alice` and the configuration
  sets `root_dir: "~/Dev"`
- **THEN** `root_dir` resolves to `/home/alice/Dev` before any
  classification runs

#### Scenario: include with literal name

- **WHEN** `root_dir: "~/Dev"` and `include: [playground-rust]` and
  `~/Dev/playground-rust` exists
- **THEN** the candidate set contains `~/Dev/playground-rust`

#### Scenario: include with wildcard glob

- **WHEN** `root_dir: "~/Dev"` and `include: [playground*]` and
  `~/Dev` contains immediate subdirectories `playground-rust`,
  `playground-go`, and `notes`
- **THEN** the candidate set contains `~/Dev/playground-rust` and
  `~/Dev/playground-go`
- **AND** `~/Dev/notes` is not in the candidate set

#### Scenario: empty include means every child

- **WHEN** `root_dir: "~/Dev"` and `include:` is absent or empty
- **THEN** the candidate set contains every immediate child of
  `~/Dev` that is a directory

#### Scenario: exclude drops basenames

- **WHEN** the candidate set is `{~/Dev/bin, ~/Dev/playground-rust,
  ~/Dev/notes}` and `exclude: [bin, playground*]`
- **THEN** the post-filter candidate set is `{~/Dev/notes}`
- **AND** the watch list contains only `~/Dev/notes` after
  classification

#### Scenario: empty exclude means exclude nothing

- **WHEN** `exclude:` is absent or empty
- **THEN** no candidate is dropped by the exclude step

#### Scenario: pattern with `**` is rejected at load

- **WHEN** `config.yaml` contains `include: ["playground/**"]` or
  `root_dir` containing `**`
- **THEN** `see` prints an error identifying the offending field
  (`include[0]` or `root_dir`), explaining that `**` is not
  supported, and exits with status `2`
- **AND** the watcher does not start

#### Scenario: malformed glob brackets are rejected at load

- **WHEN** `config.yaml` contains `include: ["[unclosed"]`
- **THEN** `see` prints an error identifying `include[0]` and the
  underlying `filepath.Match` error, and exits with status `2`
- **AND** the watcher does not start

### Requirement: Discovery classifies each entry as a repo or a parent-of-repos

Each candidate produced by the `include` / `exclude` step SHALL be
classified by stat'ing the candidate itself:

- A candidate whose root contains a `.git` file or directory SHALL be
  treated as a single repo.
- A candidate whose root is a directory and does not contain `.git`
  SHALL be treated as a parent-of-repos; each immediate child with
  `.git` SHALL be added to the resolved list.
- A candidate that does not exist, is a regular file, or is a
  parent-of-repos with no `.git` children SHALL emit a `Warning`
  event and be skipped.

#### Scenario: Candidate is a single repo

- **WHEN** a candidate resolves to a path with `.git/`
- **THEN** the resolved list contains that path
- **AND** `Watcher.work` runs against it directly

#### Scenario: Candidate is a parent-of-repos

- **WHEN** a candidate resolves to a directory with immediate
  `.git/` children `repoA` and `repoB`
- **THEN** the resolved list contains `<candidate>/repoA` and
  `<candidate>/repoB`
- **AND** `Watcher.work` runs against each

#### Scenario: Candidate is a parent with no repo children

- **WHEN** a candidate resolves to a directory with no `.git/`
  children
- **THEN** a `Warning` event is emitted naming the path
- **AND** the batch continues without that entry

#### Scenario: Candidate is missing

- **WHEN** a candidate resolves to a path that does not exist
- **THEN** a `Warning` event is emitted naming the path
- **AND** the batch continues without that entry

### Requirement: Discovery dedupes and sorts the watch list

After all candidates are resolved and classified, the final watch
list SHALL be deduplicated by absolute path and sorted in ascending
path order. Two candidates that resolve to the same absolute path
(for example a literal `include` entry and a glob match that
includes the same repo) SHALL appear exactly once.

#### Scenario: Overlapping sources collapse

- **WHEN** `include: [/abs/path/repo, "~/work/*"]` resolves to a
  list that contains `/abs/path/repo` more than once
- **THEN** the final watch list contains `/abs/path/repo` exactly
  once

#### Scenario: Stable ordering for the TUI

- **WHEN** the resolved list contains `/repos/zeta`, `/repos/alpha`,
  and `/repos/mu`
- **THEN** the final list is ordered `/repos/alpha`, `/repos/mu`,
  `/repos/zeta`
- **AND** the Terminal User Interface (TUI) renders the same order
  on every scan

### Requirement: Configuration validates fields at load time

`loadConfig` SHALL validate every watch-related field after the YAML
decode and before returning, so configuration errors exit with status
`2` before the watcher starts instead of producing warnings during
the first scan. The validation SHALL run in this order:

1. If `root_dir` is nonblank: reject `**`, tilde-expand via
   `expandTilde`, stat the result, and require it to be a directory.
   The expanded path SHALL be stashed back into `cfg.RootDir` so the
   resolver does not re-expand.
2. For each entry in `include`: reject `**`, probe
   `filepath.Match(entry, "test")` to catch `ErrBadPattern`, and
   tilde-expand. The expanded entry SHALL be stashed back into the
   slice.
3. For each entry in `exclude`: the same checks as `include`.

Errors SHALL name the offending field path (`root_dir`,
`include[2]`, `exclude[0]`) and the underlying cause. Validation
SHALL NOT consult the filesystem beyond `expandTilde` and the
single `os.Stat` on `root_dir`.

#### Scenario: Unknown watch field is rejected at load

- **WHEN** `config.yaml` contains the old `watches:` sequence
- **THEN** the strict YAML decoder reports `watches` as an unknown
  field, identifies the configuration file, and `see` exits with
  status `2`
- **AND** the watcher does not start

#### Scenario: root_dir does not exist

- **WHEN** `config.yaml` sets `root_dir: "/nope"` and `/nope` does
  not exist
- **THEN** `see` prints `root_dir "/nope": <stat error>` and exits
  with status `2`
- **AND** the watcher does not start

#### Scenario: root_dir is a file

- **WHEN** `config.yaml` sets `root_dir: "/etc/hosts"` and that
  path is a regular file
- **THEN** `see` prints `root_dir "/etc/hosts": not a directory`
  and exits with status `2`

#### Scenario: include entry contains `**`

- **WHEN** `config.yaml` sets `include: ["work/**"]`
- **THEN** `see` prints `include[0]: '**' is not supported` and
  exits with status `2`

#### Scenario: include entry has malformed brackets

- **WHEN** `config.yaml` sets `include: ["[unclosed"]`
- **THEN** `see` prints `include[0]: invalid glob pattern: <error>`
  and exits with status `2`

#### Scenario: tilde expansion fails

- **WHEN** `HOME` is unset and `os.UserHomeDir` fails, and
  `config.yaml` sets `root_dir: "~/Dev"`
- **THEN** `see` prints `root_dir: expand ~: <error>` and exits
  with status `2`

### Requirement: Global configuration uses a strict YAML schema

`see` SHALL read global configuration from the single mapping document at `filepath.Join(os.UserConfigDir(), "see", "config.yaml")`. The mapping SHALL accept only these optional top-level fields:

- `root_dir`: a string base directory path. Tilde expansion is
  performed; environment-variable expansion is not.
- `include`: a sequence of glob patterns relative to `root_dir`.
- `exclude`: a sequence of glob patterns matched against the basename
  of each candidate from the `include` step.
- `prompt`: a string agent prompt template, including YAML Ain't
  Markup Language (YAML) literal block scalars for multiline text.
- `condition`: a platform-shell command string that selects custom
  workflow mode when nonblank.
- `commit`: a custom catch-up commit message template used only in
  custom workflow mode.

The legacy `watches` field SHALL NOT be accepted. The legacy
`filepath.Join(os.UserConfigDir(), "see", "watches")` file SHALL
NOT be read, merged, or used as a fallback. A missing or empty file
SHALL produce a zero-value configuration without error. Malformed
YAML, additional YAML documents, unknown fields, and values whose
types do not match the schema SHALL be fatal startup errors. The
error SHALL identify the configuration file and retain available
line information, and the watcher SHALL NOT start.

#### Scenario: Valid configuration loads every field

- **WHEN** `config.yaml` contains a `root_dir` string, `include` and
  `exclude` string sequences, and multiline `prompt`, `condition`,
  and `commit` strings
- **THEN** `see` loads all six values from the same document
- **AND** each literal block scalar preserves its line breaks

#### Scenario: Missing configuration is empty configuration

- **WHEN** `config.yaml` does not exist
- **THEN** configuration loading succeeds with no `root_dir`,
  `include`, `exclude`, `prompt`, `condition`, or `commit`

#### Scenario: Unknown field is rejected

- **WHEN** `config.yaml` contains the misspelled field `conditon`
- **THEN** `see` reports an error identifying the unknown field and
  configuration file
- **AND** exits with status `2` before the watcher starts

#### Scenario: Invalid field type is rejected

- **WHEN** `config.yaml` defines `condition` as a mapping instead
  of a string
- **THEN** `see` reports a type error identifying the configuration
  file
- **AND** exits with status `2` before the watcher starts

#### Scenario: Malformed or multiple-document YAML is rejected

- **WHEN** `config.yaml` is malformed or contains more than one YAML
  document
- **THEN** `see` reports a configuration error
- **AND** exits with status `2` before the watcher starts

### Requirement: First-run bootstrap materializes a default config file

When `see` is invoked with no `--config` flag and the resolved default
configuration path does not exist, `loadStartupConfig` SHALL create the
parent directory `filepath.Join(<base>, "see")` with mode `0o755`
if it does not exist, then write a template configuration document to
the default path with mode `0o644`. The template SHALL be embedded into
the binary at build time from a sibling file at the repository root
(consistent with `prompt.md`) via `//go:embed`, SHALL contain only YAML
comments and a YAML header, and SHALL decode under the strict schema
to a zero-value configuration identical to the one the loader returns
for the missing-file case.

Bootstrap SHALL NOT fire when `--config` is set to a non-empty value
(including `--config=-` and `--config=<path>`). Bootstrap SHALL NOT
overwrite an existing configuration file at the default path; an
empty file, a comments-only file, a valid configuration, and a
malformed configuration all bypass bootstrap and proceed through the
existing load branches.

If the write fails (permission denied, read-only filesystem, parent
directory unwritable), `see` SHALL emit a one-line notice to standard
error (stderr) identifying the target path and the failure reason,
and SHALL continue startup with a zero-value configuration so the
current-working-directory (cwd) fallback still produces a working
watch list. The watcher SHALL start regardless of bootstrap outcome.

#### Scenario: First run writes the template

- **WHEN** `see` is invoked with no `--config` flag and the default
  configuration file does not exist
- **THEN** the parent directory `filepath.Join(<base>, "see")` exists
  with mode `0o755` (created if absent)
- **AND** the default configuration file exists with mode `0o644`
- **AND** the file's contents equal the embedded template byte-for-byte
- **AND** `loadConfig` on that file returns a zero-value configuration
  with no error
- **AND** the watcher starts and proceeds with the cwd fallback

#### Scenario: Existing file is not overwritten

- **WHEN** `see` is invoked with no `--config` flag and the default
  configuration file already exists with arbitrary content (empty,
  comments-only, valid, or malformed)
- **THEN** the file's contents and mode are unchanged after startup
- **AND** bootstrap does not write to the path

#### Scenario: --config=- skips bootstrap

- **WHEN** `see --config=-` is invoked and the default configuration
  file does not exist
- **THEN** the default configuration file is not created
- **AND** the parent directory `filepath.Join(<base>, "see")` is not
  created
- **AND** the loader returns a zero-value configuration without
  reading or writing any file

#### Scenario: --config=<path> skips bootstrap

- **WHEN** `see --config=<other>` is invoked and the default
  configuration file does not exist
- **THEN** the default configuration file is not created
- **AND** the named file is read (or a "missing file" error path is
  taken) without writing any file at the default path

#### Scenario: Unwritable target is non-fatal

- **WHEN** `see` is invoked with no `--config` flag, the default
  configuration file does not exist, and the parent directory is not
  writable (permission denied)
- **THEN** `see` prints one line to stderr identifying the target
  path and the write failure
- **AND** `see` exits with status `0` if no other startup error
  applies
- **AND** the watcher starts with a zero-value configuration

#### Scenario: Bootstrap template decodes under strict schema

- **WHEN** the embedded template is written to the default path and
  immediately read back by `loadConfig`
- **THEN** decoding succeeds without error
- **AND** the decoded configuration has `RootDir == ""`,
  `Include == nil`, `Exclude == nil`, and `Prompt == ""`
- **AND** no unknown-field, type, or multi-document error is raised

## ADDED Requirements

(none beyond the MODIFIED Requirements above)

## REMOVED Requirements

(none — the changes reshape existing requirements rather than
deleting capability)