## 1. Add failing bootstrap tests

- [x] 1.1 Add a test proving that `ensureDefaultConfig(path)` writes
      the embedded template to a missing path, creates the parent
      directory with mode `0o755`, and creates the file with mode
      `0o644`. Pin `userConfigDir` to a temp directory so the test
      does not touch the user's real config dir.
- [x] 1.2 Add a test proving that `ensureDefaultConfig` is a no-op
      when the target file already exists, regardless of the file's
      current contents (empty, comments-only, valid, malformed).
- [x] 1.3 Add a test proving that `loadStartupConfig("-")` does not
      resolve or write the default path, even when the default
      directory is unwritable.
- [x] 1.4 Add a test proving that `loadStartupConfig(<path>)` does
      not write the default path, even when the named file is
      absent and the default directory is unwritable.
- [x] 1.5 Add a test proving that `loadStartupConfig("")` continues
      with a zero-value configuration (and emits a one-line stderr
      notice) when the bootstrap write fails — for example, when
      the parent directory exists with mode `0o555`.
- [x] 1.6 Run the focused config tests with a 30-second timeout and
      confirm the new tests fail before production edits.

## 2. Add the bootstrap template

- [x] 2.1 Add `config.example.yaml` at the repository root next to
      `prompt.md`. The file MUST contain only YAML comments and a
      YAML header — no uncommented fields — so that loading the
      file produces a zero-value configuration under the strict
      schema.
- [x] 2.2 In `config.go`, embed the file via `//go:embed` alongside
      the existing `defaultPromptTemplate` embed for `prompt.md`.
      Use a distinct variable name (`defaultConfigTemplate`) and a
      brief comment mirroring the `prompt.md` rationale.

## 3. Implement ensureDefaultConfig

- [x] 3.1 Add `ensureDefaultConfig(path string) error` to `config.go`.
      The function SHALL `MkdirAll(filepath.Dir(path), 0o755)` and
      then `WriteFile(path, []byte(defaultConfigTemplate), 0o644)`.
      It SHALL NOT call `os.Stat` first; the existing branch in
      `loadStartupConfig` already gates on the absent case.
- [x] 3.2 Wire `ensureDefaultConfig` into the default-path branch
      of `loadStartupConfig`. After resolving the default path and
      before calling `loadConfig`, call `ensureDefaultConfig` and,
      on error, write one line to standard error (stderr) naming
      the path and the underlying error, then continue. The
      `--config=-` and `--config=<path>` branches MUST NOT call
      `ensureDefaultConfig`.
- [x] 3.3 Run the focused config tests and confirm every scenario
      from task 1 is green: writes-on-miss, no-op-on-present,
      skipped-on-`-`, skipped-on-`<path>`, write-failure-is-non-fatal.

## 4. Update project guidance

- [x] 4.1 Append a paragraph to the Configuration section of
      `AGENTS.md` documenting the first-run bootstrap behavior:
      the path written, the template's all-comments shape, the
      `--config` interaction, and the non-fatal write-failure
      contract.
- [x] 4.2 Update `openspec/config.yaml` only if a new shape_choice
      or test caveat needs to land. The current entries do not
      require updates; skip if nothing changes.

## 5. Validate and synchronize

- [x] 5.1 Run `gofmt` on `config.go` and `config_test.go`, then run
      `go test -timeout 30s ./...` and `go vet ./...`.
- [x] 5.2 Run `go build ./...` and verify `//go:embed` resolves
      `config.example.yaml` (a missing file fails the build with
      a directive error).
- [x] 5.3 Run `openspec validate add-default-config-bootstrap
      --type change --strict` and resolve every reported issue.
- [x] 5.4 Synchronize the watcher delta into the main specification
      (`openspec/specs/watcher/spec.md`) after implementation and
      verification.
- [x] 5.5 Archive `add-default-config-bootstrap` only after every
      implementation and validation task is complete.