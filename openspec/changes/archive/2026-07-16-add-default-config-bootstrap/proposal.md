## Why

`see` reads configuration from `os.UserConfigDir()/see/config.yaml`, but the
file is never created. A first-time user has no way to discover the
configuration surface — the schema, the available fields, or the path to
the file itself — short of reading the source or the project documentation.
Materializing a commented template on first run (when the default path is
absent) gives the user a discoverable starting point without changing any
runtime behavior, since an all-comments YAML document decodes to the same
zero-value configuration that the missing-file branch already returns.

## What Changes

- Add a bootstrap step that writes a commented template to the default
  configuration path when the path is absent and `--config` is unset.
  The template is a sibling file at the repository root, embedded into
  the binary via `//go:embed` (consistent with `prompt.md`).
- Bootstrap runs only on the default-path branch of `loadStartupConfig`.
  `--config=-` skips the file entirely (no bootstrap, no read). A
  non-empty `--config=<path>` resolves and loads the named file (no
  bootstrap, regardless of whether that file exists).
- An existing configuration file is never overwritten. Bootstrap fires
  only on the absent-file case; an empty file, a comments-only file, a
  valid configuration, and a malformed configuration all leave the
  loader on the existing branches.
- A write failure (permission denied, read-only filesystem) is
  non-fatal. `see` logs a one-line notice to standard error (stderr)
  identifying the path and the failure reason, and proceeds with a
  zero-value configuration. The command-line and current-working-
  directory (cwd) fallback paths still produce a working watch list.
- Bootstrap creates the parent directory `<base>/see/` with mode
  `0o755` if it does not exist. First-run users do not have the
  directory.
- The embedded template's mode is `0o644`. It is identical in shape
  to a hand-written configuration: optional `watches` sequence and
  optional `prompt` string, both commented out, with a header
  comment naming each field and explaining that uncommenting enables
  it.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `watcher`: add a "First-run bootstrap materializes a default config
  file" requirement alongside the existing
  "Global configuration uses a strict YAML schema" requirement.
  No existing requirement changes; the existing "Missing
  configuration is empty configuration" scenario already covers the
  post-bootstrap load.

## Impact

- `config.example.yaml` (new): the embedded template, kept in the
  repository root next to `prompt.md` so editors and reviewers see
  the prose in pull requests.
- `config.go`: embed the template via `//go:embed`; add
  `ensureDefaultConfig(path string) error`; call it from the
  default-path branch of `loadStartupConfig` before `loadConfig`.
- `config_test.go`: add focused tests for the bootstrap path —
  writes-on-miss, no-op-on-present, no-op-on-`--config=-`,
  no-op-on-`--config=<path>`, parent-directory creation, and
  write-failure-is-non-fatal.
- `main.go`: one comment line documenting that `loadStartupConfig`
  now bootstraps the default path on miss; no control-flow change.
- `openspec/specs/watcher/spec.md`: add the new requirement and
  its scenarios when the delta is synchronized.
- `AGENTS.md`: append a paragraph to the Configuration section
  documenting the bootstrap behavior.
- No new dependencies, CLI flags, configuration-file fields, event
  types, or Terminal User Interface (TUI) code.