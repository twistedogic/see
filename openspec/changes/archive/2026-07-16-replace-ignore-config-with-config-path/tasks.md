## 1. Flag plumbing

- [x] 1.1 Replace the `--ignore-config` bool flag with a `--config <path>`
      string flag in `main.go`. Default value `""`; help text references
      the system config path and the `-` sentinel.
- [x] 1.2 Update the `loadStartupConfig(*ignoreConfig)` call site in
      `main.go` to pass `*configFlag` (string).

## 2. Loader refactor

- [x] 2.1 Add a package-level constant `configPathNone = "-"` in
      `config.go` with a one-line ponytail comment naming the convention
      and the upgrade path (env-var override if the sentinel ever
      becomes a footgun).
- [x] 2.2 Change `loadStartupConfig(ignoreConfig bool)` signature to
      `loadStartupConfig(configFlag string)` and update its doc comment
      to describe the three modes.
- [x] 2.3 Replace the `if ignoreConfig { return Config{}, nil }` branch
      with: sentinel check (return zero-value), empty-string default
      (call `configPath()` and `loadConfig`), explicit path (call
      `expandTilde` and `loadConfig`).
- [x] 2.4 Verify `go build ./...` succeeds after the signature change
      before adding tests.

## 3. Tests

- [x] 3.1 Delete `TestLoadStartupConfigIgnoreConfigSkipsMalformedFile`
      from `config_test.go`.
- [x] 3.2 Add `TestLoadStartupConfigUnsetLoadsDefaultPath`: with no
      flag and a non-empty `config.yaml` at the resolved default path,
      the loader returns the parsed configuration.
- [x] 3.3 Add `TestLoadStartupConfigExplicitPath`: with `--config=...`
      pointing at a fixture, the loader returns the parsed fixture and
      does not read the default path (use a malformed default as proof
      that the default was bypassed).
- [x] 3.4 Add `TestLoadStartupConfigSkipSentinel`: with `--config=-`
      and a malformed default `config.yaml`, the loader returns a
      zero-value `Config` without error.
- [x] 3.5 Add `TestLoadStartupConfigTildeExpansion`: with
      `--config=~/...` and `$HOME` pointing at a temp dir containing a
      fixture, the loader returns the parsed fixture.
- [x] 3.6 Run `go test -timeout 30s ./...` and confirm all tests pass.

## 4. Documentation

- [x] 4.1 In `AGENTS.md`, rename the section "Configuration loading and
      `--ignore-config`" to "Configuration loading and `--config`" and
      rewrite the prose to describe the three modes (unset, explicit
      path, `-` sentinel).
- [x] 4.2 Update the `--ignore-config` quote in the prompt-precedence
      paragraph to reference `--config=-`.

## 5. Validation

- [x] 5.1 Run `go test -timeout 30s ./...` once more after the AGENTS.md
      change (no test impact expected; sanity check).
- [x] 5.2 Run `openspec validate replace-ignore-config-with-config-path --strict`
      and resolve any reported issues.
- [x] 5.3 Manually invoke `see --help` (against a fixture repo or with
      `--help` only, never in an automated test) to confirm the new
      `--config` flag is present and `--ignore-config` is gone.