## ADDED Requirements

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
command-line entries and the current-working-directory (cwd)
fallback still produce a working watch list. The watcher SHALL start
regardless of bootstrap outcome.

#### Scenario: First run writes the template

- **WHEN** `see` is invoked with no `--config` flag and the default
  configuration file does not exist
- **THEN** the parent directory `filepath.Join(<base>, "see")` exists
  with mode `0o755` (created if absent)
- **AND** the default configuration file exists with mode `0o644`
- **AND** the file's contents equal the embedded template byte-for-byte
- **AND** `loadConfig` on that file returns a zero-value configuration
  with no error
- **AND** the watcher starts and proceeds with the cwd fallback or
  command-line entries

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
- **AND** the decoded configuration has `Watches == nil` and
  `Prompt == ""`
- **AND** no unknown-field, type, or multi-document error is raised