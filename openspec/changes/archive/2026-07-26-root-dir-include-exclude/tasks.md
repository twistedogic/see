## 1. Config schema and validation

- [x] 1.1 Replace `Watches []string` on `Config` with `RootDir string`,
      `Include []string`, `Exclude []string`. Update `Config` struct,
      keep `Prompt`, `Condition`, `Commit` unchanged.
- [x] 1.2 Add a private `validateConfig(cfg *Config) error` in
      `config.go`. It tilde-expands `RootDir` (stat + require dir),
      and for each `Include` / `Exclude` entry: rejects `**`, probes
      `filepath.Match(entry, "test")` for `ErrBadPattern`, and
      tilde-expands. Errors wrap with field path
      (`root_dir`, `include[N]`, `exclude[N]`).
- [x] 1.3 Call `validateConfig` from `loadConfig` after decode,
      before returning. Keep the existing strict decoder, multi-doc
      check, and missing-file / empty-file handling unchanged.
- [x] 1.4 Keep `validateCustomConfig` as-is; call it from `main` as
      today.

## 2. Resolver

- [x] 2.1 Add `resolveConfiguredTargets(cfg Config) ([]string,
      []Warning, error)` to `discovery.go`. Flow: blank `RootDir`
      → cwd fallback; `Include` empty → immediate children of
      `RootDir`; else union of `filepath.Glob(join(RootDir, p))`;
      filter by `filepath.Match(p, filepath.Base(c))` for each
      `Exclude` entry; classify each remaining candidate via
      existing `classifyTarget`; dedupe + sort via existing
      `dedupeAndSort`.
- [x] 2.2 Delete `resolveTargets` from `discovery.go`.
- [x] 2.3 Delete `resolveWatchList` and the `multiFlag` type from
      `main.go`. Update the call site in `main()` to use
      `resolveConfiguredTargets(cfg)` directly.

## 3. CLI flag removal

- [x] 3.1 Remove the `watchFlag multiFlag` declaration, the
      `flag.Var(&watchFlag, "watch", ...)` registration, and the
      `"watch"` mention in any usage text or `AGENTS.md` references.
- [x] 3.2 Run `grep -rn '\-\-watch\|watchFlag\|multiFlag\|Watches'
      --include='*.go' --include='*.md'` and confirm only the new
      code paths reference watch-list handling.

## 4. Embedded template

- [x] 4.1 Update `config.example.yaml` so the commented examples
      show `root_dir`, `include`, `exclude` instead of `watches`.
      Keep `prompt`, `condition`, `commit` examples. Confirm the
      file still decodes under the strict loader to a zero-value
      `Config` (only comments + header).

## 5. Tests

- [x] 5.1 `config_test.go`: rewrite every `watches:` fixture to
      use `root_dir:` (or a fixture that exercises the new fields
      for the new validation behavior). Rename
      `TestLoadConfigWrongTypeWatches` to
      `TestLoadConfigWrongTypeRootDir`. Delete
      `TestLoadConfigIgnoresLegacyWatchesFile`.
- [x] 5.2 `config_test.go`: add tests for the new
      `validateConfig` paths: blank `RootDir` (cwd), `RootDir`
      missing, `RootDir` is a file, `Include`/`Exclude` with `**`,
      `Include`/`Exclude` with malformed brackets, tilde expansion
      failure. Each test asserts the error message names the field
      path.
- [x] 5.3 `discovery_test.go`: delete the `resolveWatchList*` tests
      and the `resolveTargets*` tests. Add
      `resolveConfiguredTargets` tests covering: blank `RootDir`
      (cwd fallback), `Include` literal name, `Include` wildcard,
      empty `Include` (every child), `Exclude` basename filter,
      empty `Exclude`, combined `Include` + `Exclude`, and
      classification passthrough (repo + parent-of-repos).
- [x] 5.4 `main_test.go`: grep for `--watch` and `watchFlag`,
      strip every occurrence from spawn-style tests, configure the
      equivalent via the test config files. Confirm the binary
      rejects unknown `--watch` if any test still passes it
      intentionally.
- [x] 5.5 Run `go test -timeout 30s ./...` and confirm green.

## 6. Documentation

- [x] 6.1 Update the "Configuration" section of `AGENTS.md` so the
      schema reference lists `root_dir`, `include`, `exclude`
      instead of `watches`. Update the example block.
- [x] 6.2 Update the migration note in `AGENTS.md` (the legacy
      plain-text `watches` file section) to describe the
      `watches` → `root_dir` + `include` + `exclude` rewrite, not
      just the YAML conversion.

## 7. Verification

- [x] 7.1 Run `go vet ./...` and `go test -timeout 30s ./...`;
      both must be green.
- [x] 7.2 Run `rg -n 'watches\b|Watches\b|watchFlag\b|multiFlag\b|
      --watch\b' . --glob '!openspec/changes/**' --glob
      '!.git/**'` and confirm no stale references remain outside
      this change's own delta spec and archived history.
- [x] 7.3 `openspec validate root-dir-include-exclude` (or
      `openspec status --change root-dir-include-exclude`) reports
      the change as ready for archive after implementation.