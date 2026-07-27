## 1. Config schema and validation

- [ ] 1.1 Add `LogDir string \`yaml:"log_dir"\`` to the `Config`
      struct in `config.go`, alongside `WorktreeRoot`. Comment it
      with the precedence rule and the default constant reference,
      matching the doc style of the other path fields.
- [ ] 1.2 Add `const defaultLogDir = "~/.cache/see/logs"` in
      `config.go`, mirroring `defaultWorkflowsDir` /
      `defaultWorktreeRoot`. Keep it a literal-tilde constant;
      callers expand at use time.
- [ ] 1.3 In `validateConfig` (`config.go`), add a branch for
      `cfg.LogDir`: when non-blank (after `strings.TrimSpace`), reject
      `**`, tilde-expand via `expandTilde`, and stash the expanded
      value back into `cfg.LogDir`. A whitespace-only value stays
      empty (untreated as "unset"). Mirror the `WorkflowsDir` block.
      Errors wrap with the `log_dir` field path.

## 2. Resolution and default change

- [ ] 2.1 In `eventlog.go`, replace `ensureLogDir()` with
      `resolveLogDir(cfg Config) (string, error)`. Precedence:
      `os.Getenv("SEE_LOG_DIR")` non-empty → use it; else
      `cfg.LogDir` non-blank → use it; else `defaultLogDir`. Run the
      chosen candidate through `expandTilde`. `MkdirAll` (mode
      `0o755`) the result; on failure return a wrapped error naming
      the resolved path. Preserve the existing fatal-startup-error
      contract.
- [ ] 2.2 Update the `ensureLogDir` doc comment (now on
      `resolveLogDir`) to describe the three-source precedence and
      the uniform tilde-expansion rule. Remove the
      `os.UserCacheDir`/see/logs` wording.
- [ ] 2.3 Audit every `ensureLogDir` call site (`rg ensureLogDir`).
      The only production caller is `main.go`; tests that referenced
      `ensureLogDir` directly are updated in section 5.

## 3. main() reorder

- [ ] 3.1 In `main.go`, move the `loadStartupConfig(*configFlag)`
      call (and its error handling) to **above** the log-directory
      resolution. Confirm `mode`, `--interval`, `flag.Parse`, and
      the `term.IsTerminal` check remain before it and are
      unaffected.
- [ ] 3.2 Replace `logDir, err := ensureLogDir()` with
      `logDir, err := resolveLogDir(cfg)`. Keep the
      `openEventLogger(logDir)` and `NewWatcher(*pi, logDir, ...)`
      calls exactly as-is.
- [ ] 3.3 Confirm no step between the moved `loadStartupConfig` and
      `resolveLogDir` reads `cfg` (a `rg cfg` in that range should
      show nothing). The downstream `w.Workflows = cfg.Workflows`,
      `validateWorkflows(cfg)`, `resolveLaneIsolation(...)`, and
      `resolveConfiguredTargets(cfg)` already run after config load.

## 4. Embedded template

- [ ] 4.1 Add a commented `# log_dir: "~/.cache/see/logs"` line to
      `config.example.yaml`, placed near the other path fields
      (`root_dir` block or the lane-isolation block). Confirm the
      file still decodes under the strict loader to a zero-value
      `Config` (comments only, no uncommented field).

## 5. Tests

- [ ] 5.1 `config_test.go`: add `validateConfig` cases for `log_dir`:
      blank (untreated, no expansion attempted), non-blank tilde
      expansion, whitespace-only treated as blank, `**` rejected.
      Assert the expanded value is stashed back and errors name the
      `log_dir` field path.
- [ ] 5.2 `eventlog_test.go` (or a new test in the package): add
      `resolveLogDir` cases covering all precedence branches —
      `SEE_LOG_DIR` set overrides config; `SEE_LOG_DIR` empty +
      config set uses config; both empty uses `defaultLogDir`;
      whitespace-only config falls through to default; tilde
      expansion in each of the three sources (assert no literal `~`
      in the resolved path); `MkdirAll` creates a missing directory
      and fails fatally on an uncreatable one (a path component that
      is a file).
- [ ] 5.3 Update any existing test that called `ensureLogDir`
      directly to call `resolveLogDir(Config{})` (or with an explicit
      `LogDir`), preserving the directory-creation assertion.
- [ ] 5.4 Run `go test -timeout 30s ./...` and confirm green.

## 6. Documentation

- [ ] 6.1 Update the "Configuration" section of `AGENTS.md`: add
      `log_dir` to the schema reference with its precedence
      (`SEE_LOG_DIR` > `log_dir` > `~/.cache/see/logs`), tilde
      expansion, and create-if-absent behavior. Note the macOS
      default relocation in a migration paragraph (logs move from
      `~/Library/Caches/see/logs/` to `~/.cache/see/logs/`; Linux
      unchanged; old logs are not migrated).

## 7. Verification

- [ ] 7.1 Run `go vet ./...` and `go test -timeout 30s ./...`; both
      must be green.
- [ ] 7.2 Run `rg -n 'ensureLogDir|os\.UserCacheDir' --include='*.go'`
      and confirm no production references to the removed function or
      the old default remain (only `resolveLogDir` and
      `defaultLogDir`).
- [ ] 7.3 `openspec validate add-log-dir-config` reports the change
      as ready for archive after implementation.
