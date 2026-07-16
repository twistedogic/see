## Why

`--ignore-config` is a boolean escape hatch whose only remaining job is to make a
malformed `config.yaml` equivalent to a missing one. After the recent
`make-cli-flags-override-config` and `promote-config-to-yaml` changes, the
normal "bypass valid config" case is already covered by the precedence rule
(`--watch` and `--prompt` override the configured values). The flag exists
only for the malformed-config recovery case, costs a separate boolean knob in
an otherwise string-valued layered-config model, and forces two requirements
and two scenarios to carry an "unless `--ignore-config` is set" carve-out.

Replacing it with a string-valued `--config <path>` collapses the escape
hatch into the same shape as `--watch` and `--prompt`, removes the carve-out
language, and earns a new capability (swappable config files, including
per-project configs and shared dotfiles) for the same surface cost.

## What Changes

- Add `--config <path>` flag (string-valued).
  - **Unset or empty**: load the default `os.UserConfigDir()/see/config.yaml`.
  - **Non-empty path**: tilde-expand the path and load that file.
  - **`-`**: skip the configuration entirely (replaces `--ignore-config`).
- **BREAKING**: Remove the `--ignore-config` boolean flag. Anyone scripting
  it must switch to `--config=-`.
- `loadStartupConfig` drops its `bool` parameter; the new path-resolution
  logic moves the empty-string and skip-sentinel branches into the loader.
- The watch-resolution and prompt-selection requirements drop the "unless
  `--ignore-config` is set" clause and replace the two affected scenarios
  with versions keyed on `--config=-`. A new scenario covers the
  `--config=<path>` overload.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `watcher`: the discovery-resolves-watch-list requirement and the
  watcher-renders-agent-prompt requirement both lose the `--ignore-config`
  carve-out and gain the `--config=-` / `--config=<path>` semantics. Two
  scenarios are rewritten and one new scenario is added.

## Impact

- `main.go`: flag definition (one line replaced) and the `loadStartupConfig`
  call site (one argument changed).
- `config.go`: `loadStartupConfig` signature changes from `(bool)` to
  `(string)`; the `if ignoreConfig` branch is replaced by an empty-string
  default-resolution branch plus a `-` sentinel check; tilde expansion moves
  one call site.
- `config_test.go`: delete `TestLoadStartupConfigIgnoreConfigSkipsMalformedFile`;
  add three tests covering unset / explicit path / `-` sentinel.
- `openspec/specs/watcher/spec.md`: two requirement clauses reworded; two
  scenarios rewritten; one new scenario added.
- `AGENTS.md`: the "Configuration loading and `--ignore-config`" section is
  renamed and rewritten to describe `--config`.
- No change to `discovery.go`, `eventlog.go`, `tui/`, or `prompt.md`.
- No new dependencies. No CLI surface added beyond what is described above;
  one boolean removed.