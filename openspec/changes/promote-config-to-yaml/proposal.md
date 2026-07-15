## Why

`see`'s global configuration is a plain-text watch list, so it cannot hold a persistent user-default agent prompt. Promoting the file to YAML Ain't Markup Language (YAML) adds the second configuration dimension while keeping multiline prompts readable and allowing the existing `--prompt` command-line override to remain the highest-precedence source.

## What Changes

- Replace the plain-text `os.UserConfigDir()/see/watches` file with a structured `os.UserConfigDir()/see/config.yaml` file.
- Add top-level `watches` and `prompt` fields. `watches` preserves the existing path, tilde expansion, glob, union, deduplication, and current-working-directory fallback behavior.
- Resolve the effective agent prompt in this order: a nonblank `--prompt` value, a nonblank YAML `prompt`, then the embedded `prompt.md` default.
- Make `--ignore-config` skip the whole YAML file, including both fields.
- Reject malformed YAML, unknown fields, and invalid field types at startup with an actionable error.
- **BREAKING**: stop reading the legacy `os.UserConfigDir()/see/watches` file. Existing entries must be moved into `config.yaml` under `watches`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `watcher`: Change global configuration from a plain watch-list file to structured YAML and add the configured-prompt precedence layer.

## Impact

- `config.go`: replace the watch-list-only loader with a strict YAML configuration loader and update the resolved configuration path.
- `main.go`: load configuration once, apply command-line/configuration/embedded prompt precedence, pass configured watches into discovery, and broaden `--ignore-config` help text.
- `discovery_test.go` and `main_test.go`: replace plain-text configuration fixtures and add precedence, strict parsing, missing-file, and ignore-configuration coverage.
- `go.mod` and `go.sum`: add a direct YAML parser dependency; no handwritten YAML subset.
- Operator configuration: migrate `watches` entries to `config.yaml`; the embedded `prompt.md` remains the final fallback.
