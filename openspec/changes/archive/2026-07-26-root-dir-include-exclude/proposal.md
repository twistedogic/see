## Why

`see` configures the repositories it watches through a flat `watches` list of
independently-expanded path-or-glob patterns. Operators who want "everything in
`~/Dev/` except `bin`" or "only my `playground-rust` repo in `~/Dev/`" have to
repeat the root directory in every entry and fall back on the shell's glob
semantics, which `filepath.Match` does not match. A structural change — a single
`root_dir` plus optional `include` / `exclude` glob filters — expresses the
common cases in one place, makes the operator's intent legible in the config
file, and gives `see` a deterministic point at which to validate the schema
instead of deferring errors to the first scan.

The same change retires the `--watch` command-line flag. With a structured
config, the CLI duplicate is redundant: the same precedence rule (`flag`
replaces `config` replaces `cwd`) collapses to `config` replaces `cwd`, and
operators who want one-off adjustments edit the config rather than juggle two
sources of truth.

## What Changes

- **BREAKING** Replace the `watches` sequence in `config.yaml` with three new
  optional top-level fields:
  - `root_dir`: a single base directory path (`~` and `~/path` expand; env-var
    expansion is not performed).
  - `include`: a sequence of glob patterns relative to `root_dir`. Omitting or
    leaving the sequence empty means "every immediate child of `root_dir`".
  - `exclude`: a sequence of glob patterns matched against the basename of
    each candidate from the `include` step. Omitting or leaving the sequence
    empty means "exclude nothing".
- **BREAKING** Remove the `--watch` flag and its `multiFlag` type. CLI has no
  watch input; the config file is the single source of truth.
- Promote the existing pattern-validity checks (tilde expansion, `**`
  rejection, `filepath.Match` syntax check, `root_dir` existence) from
  scan-time to config-load-time. Misconfigurations now exit with status `2`
  before the watcher starts instead of as a warning during the first scan.
- Preserve the current-working-directory fallback for operators who do not
  set `root_dir`.
- Preserve the existing classification rules: an immediate child of `root_dir`
  with a `.git` subdirectory is a repository; a directory without `.git` is a
  parent-of-repos whose `.git` children are watched; everything else is a
  warning. The existing one-level descent into parent-of-repos is unchanged.
- Preserve `prompt`, `condition`, `commit`, and the strict YAML decoder
  (unknown fields, wrong types, malformed input, multiple documents all
  remain fatal).
- Update the embedded `config.example.yaml` template to show the new schema
  with commented examples.
- Update tests and the existing watch-resolution requirement in the watcher
  spec to match the new flow.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `watcher`: The watch-list discovery requirements change from "flat
  `watches` sequence with `--watch` flag precedence" to "`root_dir` plus
  optional `include`/`exclude` filters, validated at load". The
  schema-validation requirement updates the list of accepted top-level fields.
  Precedence rules simplify to "configured root or cwd fallback".

## Impact

- `config.go` — `Config` struct gains `RootDir`, `Include`, `Exclude`; loses
  `Watches`. Add a `validateConfig` step in `loadConfig`. Keep
  `validateCustomConfig` unchanged.
- `discovery.go` — `resolveTargets` is removed. Replace `resolveWatchList`
  with a config-only resolver that builds the candidate set from
  `RootDir`/`Include`, applies `Exclude`, and classifies each candidate via
  the existing `classifyTarget` and `dedupeAndSort` helpers.
- `main.go` — drop `multiFlag`, the `--watch` registration, and the
  `watchFlag` value from `resolveWatchList`. The flag-helper section
  simplifies.
- `config.example.yaml` — replace the `watches` example with `root_dir`,
  `include`, `exclude` examples. Keep the `prompt`/`condition`/`commit`
  examples.
- `discovery_test.go` — drop `resolveWatchList` tests and `resolveTargets`
  tests. Add `resolveConfiguredTargets` tests for include, exclude, empty
  fields, and the cwd fallback.
- `config_test.go` — update every `watches:` fixture to use `root_dir:` (or
  a fixture that exercises the new fields). The
  `TestLoadConfigIgnoresLegacyWatchesFile` test is deleted; its premise no
  longer exists.
- `main_test.go` — strip every `--watch <path>` invocation from the spawn
  tests; configure the equivalent via the test config files.
- `openspec/specs/watcher/spec.md` — update the discovery, schema, and
  pattern-validity requirements to match the new flow. Drop scenarios that
  assert the `--watch` precedence rule.
- Documentation in `AGENTS.md` (the "Configuration" section) is updated in
  the same change so the schema reference matches the new fields.
- No new dependencies; the project still uses `filepath.Match` / `filepath.Glob`.