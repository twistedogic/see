## 1. Configuration

- [x] 1.1 Add `WorkflowsDir string \`yaml:"workflows_dir"\`` to `Config` in
      `config.go`. No new validation logic in `validateConfig` beyond the
      existing `expandTilde` and `**` rejection pattern; the rest happens in
      the loader step.
- [x] 1.2 Resolve the effective `workflows_dir` in a new helper
      `resolveWorkflowsDir(cfg Config) (string, error)`: tilde-expand the
      configured value when nonblank; fall back to the default
      `~/.config/see/workflows/`; stat the path and return an actionable
      error if it exists but is not a directory; return the path unchanged
      if it does not exist.
- [x] 1.3 Add a config_test.go case asserting `WorkflowsDir` round-trips
      through the strict decoder and that `**` is rejected.

## 2. Frontmatter and body parser

- [x] 2.1 Create `workflow_files.go` containing
      `parseWorkflowFile(path string) (WorkflowConfig, error)`. The parser
      reads the file, splits it at the first two `---` lines, decodes the
      frontmatter strictly into a `workflowFileFrontmatter` struct
      (`Name` ignored, `Condition`, `Commit`, `Model`), and returns the
      body as `WorkflowConfig.Prompt` after trimming one leading blank line
      and trailing whitespace.
- [x] 2.2 Define `workflowFileFrontmatter` with the four fields and a
      comment on `Name` stating it is ignored (filename is authoritative).
- [x] 2.3 Surface actionable errors that name the file path: missing
      opening `---`, missing closing `---`, unknown frontmatter key, blank
      `condition`, blank `commit`, blank body.
- [x] 2.4 Add a unit test covering: happy path with all four keys,
      `name:` frontmatter is ignored, missing opening delimiter, missing
      closing delimiter, unknown frontmatter key, blank condition, blank
      commit, blank body, body with leading newline trimmed, body with
      trailing whitespace trimmed, multi-line body preserved.

## 3. Discovery

- [x] 3.1 Add `loadWorkflowFiles(dir string) ([]WorkflowConfig, error)`
      that returns `nil, nil` when `dir` does not exist; otherwise globs
      `*.md` in `dir`, sorts matches alphabetically, skips hidden files
      (basename starting with `.`), and calls `parseWorkflowFile` for each.
- [x] 3.2 Add a unit test asserting: alphabetical sort, hidden file
      skipped, non-`.md` file ignored, missing dir returns `nil, nil`,
      subdirectory ignored (only direct children).

## 4. Merge and validation

- [x] 4.1 In `loadStartupConfig` (or a new helper called from `main`),
      resolve `workflows_dir`, call `loadWorkflowFiles`, and prepend the
      returned slice to `cfg.Workflows` before `validateWorkflows` runs.
- [x] 4.2 Detect name collisions between `.md` files and `cfg.Workflows`
      before validation; on collision return an actionable error naming
      the file path and the `workflows:` index. The merged slice is built
      with `.md` files first (alphabetical) and `config.yaml` entries
      after in declared order.
- [x] 4.3 Add a unit test asserting: both sources contribute (alphabetical
      `.md` first, then declared order), collision produces an actionable
      error, empty `workflows_dir` does not affect `config.yaml` workflows,
      and `config.yaml` `workflows:` still works when `workflows_dir` is
      absent or missing.

## 5. End-to-end startup

- [x] 5.1 Add a startup-level test in `config_test.go` (or a new
      `workflow_files_test.go`) that points `WorkflowsDir` at a `t.TempDir`
      containing one `.md` workflow, runs the full load, and asserts the
      merged slice contains the file's workflow with `Prompt` set to the
      body and `Model` passed through.
- [x] 5.2 Add a startup-level test asserting a missing `WorkflowsDir` is
      a silent no-op when `config.yaml` already supplies workflows.
- [x] 5.3 Add a startup-level test asserting a non-directory path under
      `WorkflowsDir` produces an actionable error naming the path.

## 6. Documentation

- [x] 6.1 Update `AGENTS.md` configuration schema with one bullet
      describing `workflows_dir`: optional string, tilde-expanded, default
      `~/.config/see/workflows/`, recursive `**` rejected, missing
      directory is a no-op, non-directory path fails startup.
- [x] 6.2 Update `AGENTS.md` custom workflows section with one paragraph
      describing `.md` workflows: frontmatter keys (`condition`, `commit`,
      `model`, with `name` ignored), body as prompt, filename as workflow
      name, alphabetical execution order, collision with `workflows:`
      rejected at startup.
- [x] 6.3 Update `config.example.yaml` to include a commented
      `workflows_dir:` line and a brief pointer to the new source.
- [ ] 6.4 Run `gofmt` on the changed Go files.
- [ ] 6.5 Run `go test -timeout 30s ./...`.
- [ ] 6.6 Run `openspec sync-specs --change add-workflow-files` to promote
      the delta specs into `openspec/specs/`.
- [ ] 6.7 Run `openspec validate --change add-workflow-files` and confirm
      no issues.