## Why

Custom workflows today must be declared inline in `config.yaml` under a `workflows:` block. The block forces prompts into YAML scalars, which is awkward for prose that runs multiple paragraphs, includes backticks, or contains colons. Operators who want to keep their workflows in a dotfiles repo or share one workflow without the rest must copy-paste a YAML fragment. A workflow `.md` file per workflow — frontmatter for metadata, body for the prompt — fits the natural shape of a prompt better and is portable on its own.

## What Changes

- Add an optional `workflows_dir` field to the global configuration. Default `~/.config/see/workflows/`; tilde-expanded like `root_dir`. A non-empty value points at a directory `see` reads at startup.
- Discover `*.md` files directly inside `workflows_dir` (no recursion). Sort alphabetically; use each filename (sans `.md`) as the workflow's `name`.
- Parse each file as frontmatter (YAML between two `---` lines) plus body (Markdown). Frontmatter accepts `condition`, `commit`, `model`, and `name`. `condition`, `commit`, and `model` are consumed; the `name` key is parsed but ignored (filename is authoritative). Unknown keys fail startup with the file path and offending key.
- Treat the body as the workflow's prompt. Trim one leading blank line and trailing whitespace; preserve interior newlines. The `{change}` substitution applies unchanged.
- Merge `.md` workflows with the existing `workflows:` block from `config.yaml`. `.md` workflows run first (in alphabetical order); `config.yaml` workflows run after them in their declared order.
- A workflow name present in both sources is rejected at startup with an actionable error naming the file and the `workflows:` index.
- A missing `workflows_dir` is a silent no-op (no `.md` workflows). A path that exists but is not a directory is rejected at startup.
- No CLI flag for `workflows_dir`; the config field is the only surface, mirroring `root_dir` and `worktree_root`.
- Update `AGENTS.md` and `config.example.yaml` to describe the new source alongside `workflows:`.

## Capabilities

### New Capabilities

- `workflow-files`: how `see` discovers workflows from a `workflows_dir`, parses frontmatter and body, derives the workflow name from the filename, validates each file, and merges with the `config.yaml` `workflows:` block.

### Modified Capabilities

- `workflow-condition`: add requirements that workflows MAY also be supplied as `.md` files in `workflows_dir`, that the filename is the workflow name, that frontmatter `name:` is ignored, that body is the prompt, and that collision with a `config.yaml` workflow of the same name fails startup.

## Impact

- `config.go`: new `WorkflowsDir string \`yaml:"workflows_dir"\`` on `Config`. New helper `loadWorkflowFiles(dir string) ([]WorkflowConfig, error)`. New frontmatter/body parser (~50 lines, hand-rolled, no new dependency). `loadStartupConfig` (or a new loader) merges `.md` workflows with `cfg.Workflows`, validates the merged set with the existing `validateWorkflows` contract (nonblank fields, unique names).
- `main.go`: no behavioral change to `Watcher` / `runOneWorkflow` beyond reading one new field at startup. The merged `[]WorkflowConfig` is what the existing custom-mode loop already consumes.
- `config.example.yaml`: a commented `workflows_dir:` line plus a one-paragraph pointer to `~/.config/see/workflows/`.
- `AGENTS.md`: one new section in the configuration schema describing `workflows_dir`, precedence with `workflows:`, validation rules, filename-as-name, body-as-prompt semantics, and the deprecation note that `workflows:` remains supported.
- `openspec/specs/workflow-condition/spec.md`: one new requirement covering the file source, plus a clarifying scenario that the `name:` frontmatter key is ignored.
- `openspec/specs/workflow-files/spec.md`: new capability describing discovery, parsing, validation, body semantics, and the merge contract.
- No new dependencies.
- No breaking changes: `workflows:` in `config.yaml` continues to work; a user with only `config.yaml` workflows sees no behavior change.