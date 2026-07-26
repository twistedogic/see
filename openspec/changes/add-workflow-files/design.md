## Context

Today, custom workflows live exclusively inside `config.yaml` under a `workflows:` block. Each entry is a single mapping of `name`, `prompt`, `condition`, `commit`, and (with the active `add-workflow-model` change) `model`. The block forces prompts into YAML scalars; long prompts with colons, backticks, or multiple paragraphs become awkward to author and review. Operators who keep their workflows in a dotfiles repository have to copy a YAML fragment, and sharing one workflow means sharing the surrounding block.

A `.md` file per workflow — frontmatter for the four metadata fields, body for the prompt — fits the natural shape of prose. One file is portable on its own; the directory of files is discoverable at a glance; the filename is the workflow name, so there is no second place to declare it. The trade-off is two sources of truth for the same thing (a `config.yaml` block and a `~/.config/see/workflows/` directory). Coexistence rules and a clear precedence handle that.

The leading constraint is that existing configurations with only a `workflows:` block in `config.yaml` must continue to work without changes. A second constraint is that startup validation must still fail fast on malformed workflow metadata, naming the offending source (file path or `workflows:` index) so operators can fix it without reading source.

## Goals / Non-Goals

**Goals:**
- Let an operator author a workflow as a single `.md` file with frontmatter metadata and a markdown body.
- Discover those files automatically from a configurable directory; default `~/.config/see/workflows/`.
- Keep `workflows:` in `config.yaml` working for operators who already use it; merge both sources into one ordered list.
- Preserve the existing validation contract: every workflow must have a nonblank name, prompt, condition, and commit; names must be unique across the merged set.
- Keep the implementation in the Go standard library plus the existing in-repo YAML decoder; no new dependency.
- Keep the implementation small (one parser, one loader, one merge step, two tests at each boundary).

**Non-Goals:**
- A `--workflows-dir` CLI flag. The config field is the only surface; it parallels `root_dir` and `worktree_root`. Add a flag only if operators ask.
- Recursive subdirectories under `workflows_dir`. One flat directory of `.md` files is the entire scope.
- Per-project workflow directories (`<repo>/.see/workflows/`). The global directory is the only source.
- Auto-conversion of legacy `workflows:` entries into `.md` files on startup. Both sources stay readable; `workflows:` remains accepted without a deprecation warning in this change.
- Hot-reload of `.md` files during a `see` run. The directory is read once at startup.
- A new glob syntax or a `workflows_dir_glob` knob. `*.md` only, no other extensions.
- A sample workflow file written to disk on first run (matching how `config.yaml` is bootstrapped). Operators can copy from the example in `AGENTS.md`.

## Decisions

### `WorkflowsDir` is an optional `Config` field

`Config` gains `WorkflowsDir string \`yaml:"workflows_dir"\``. The strict decoder already accepts a string field with no extra plumbing, and `validateConfig` already calls `expandTilde` for path-shaped strings; the only new step is to reject `**` (consistent with `include` / `exclude`) and to surface the resolved path. A blank value falls back to the default at the call site, so an empty `Config{}` still loads cleanly. The default `~/.config/see/workflows/` is resolved in `main()` (or wherever the loader merges), exactly the same place `WorktreeRoot` falls back to `~/.cache/see/worktrees`.

A path that exists but is not a directory is rejected at startup with an actionable error; a missing path is treated as "no `.md` workflows" and startup proceeds. The asymmetry is intentional: an explicit-but-broken path is a misconfiguration the operator needs to see; a default-but-missing path is just "no workflows defined yet".

### Filename is the workflow name; frontmatter `name:` is parsed but ignored

The filename sans `.md` becomes `WorkflowConfig.Name`. This means renaming a file renames the workflow, which renames its lane digest and log filename — the same identity blast radius as renaming a `workflows:` entry, and consistent with the principle that file identity and workflow identity are the same thing. `name:` in frontmatter is decoded into the struct but never read. A future maintainer reading the parser can see the field exists and that it is intentionally ignored; silent disagreement between filename and frontmatter is impossible because the value is never used.

A glob of `*.md` (sorted alphabetically) gives a deterministic execution order. Locale-sensitive sort is the standard library's default (`filepath.Match` + `sort.Strings`); operators who need explicit ordering can prefix filenames (`01-openspec.md`, `02-deps.md`). No `order:` frontmatter field — the simplest knob is the filename itself.

### Frontmatter schema is `condition`, `commit`, `model`, and `name`

The frontmatter is decoded with the existing strict decoder (`KnownFields(true)`) into a struct:

```go
type workflowFileFrontmatter struct {
    Name      string `yaml:"name"`      // ignored; filename is authoritative
    Condition string `yaml:"condition"` // required, nonblank
    Commit    string `yaml:"commit"`    // required, nonblank
    Model     string `yaml:"model"`     // optional; blank = use pi default
}
```

Unknown keys fail startup naming the file and the offending key. The same strictness as `config.yaml` — surprising the operator who pastes a typo into a frontmatter key is the lesser evil compared to silently dropping the field. `name:` being allowed (and ignored) is the one exception, and is documented in the field comment.

`model` is the field added by the active `add-workflow-model` change. With this change in place, `model` is settable in three places: `config.yaml` `workflows[].model`, the per-workflow `.md` frontmatter, and (future) the in-progress OpenSpec-compat scope if it grows one. The precedence inside a single source is "the field as written"; between sources, the merge step below picks one or the other.

### Body is the prompt; trim leading blank lines and trailing whitespace

Standard markdown frontmatter is delimited by two `---` lines; everything after the closing delimiter is the body. Operators naturally put a blank line between the closing `---` and the first content line; trimming that single leading newline keeps prompts readable in both the file and the resolved value. Trailing whitespace (including a single trailing newline) is stripped via `strings.TrimRight(body, " \t\r\n")`. Interior newlines are preserved verbatim. `{change}` substitution applies unchanged.

The trim happens in the parser, not at the call site, so the value stored on `WorkflowConfig.Prompt` matches what the operator wrote after the trim. A whitespace-only body fails the same `validateWorkflows` "nonblank prompt" check that already exists, so no new validation is needed for empty bodies.

### A hand-rolled frontmatter parser, no new dependency

The parser is a single function (`parseWorkflowFile(path string) (WorkflowConfig, error)`) with three steps:

1. Read the file with `os.ReadFile`.
2. Split into frontmatter and body at the first two `---` lines.
3. Decode the frontmatter bytes with `yaml.NewDecoder(bytes.NewReader(...))` using the strict decoder.

`embed.FS` is not used; per-file reads at startup are fine. The whole parser fits in one screen. Tests cover: missing closing `---`, missing opening `---`, unknown frontmatter key, blank body, body with leading newline, body with trailing whitespace, and a happy path with all four frontmatter keys present.

### Source order is `.md` files first, then `config.yaml` entries

The merged slice is built as `append(mdWorkflows, cfg.Workflows...)`. `.md` files run first (in alphabetical order); `config.yaml` workflows run after them in their declared order. The principle is "the new format takes the front of the queue". A collision (same name in both sources) is a startup error naming both surfaces — not silent precedence — because silent disagreement is the kind of bug that takes 3am to find. The error message names the file path and the `workflows:` index so the operator can pick one to delete.

Validation runs once on the merged slice via the existing `validateWorkflows` function. No new validator: every merged `WorkflowConfig` already has the four nonblank fields (after the parser fills `Prompt` from the body) and the existing uniqueness check fires on cross-source collisions.

### `workflows:` in `config.yaml` is not deprecated in this change

The user's question "are we replacing `workflows:`?" gets the answer "no, not yet". `config.yaml` workflows continue to work; `.md` workflows are additive. A future change can introduce a `Warning` event for `workflows:` entries and eventually drop the block; that is a separate, deliberate decision. This change leaves `AGENTS.md`'s `workflows:` documentation intact and adds a parallel section for `workflows_dir`.

## Risks / Trade-offs

- **Filename rename changes workflow identity** → A rename of `openspec.md` to `openspec-apply.md` changes the lane digest and the log filename. The same blast radius exists for renaming a `workflows:` entry. Documented in `AGENTS.md` next to the other identity rules.
- **Frontmatter `name:` is silently ignored** → A copy-pasted file from a tool that emits `name:` frontmatter will have its `name:` discarded. Mitigated by the field comment ("ignored; filename is authoritative") and the `AGENTS.md` note. A future maintainer who reads the parser will see the explicit comment.
- **Sort order is locale-sensitive** → `sort.Strings` uses the runtime locale; on most CI boxes this is `C` / `en_US.UTF-8` and stable. Operators who need explicit ordering can prefix filenames. The risk is theoretical (sorting file names with non-ASCII characters); the mitigation is "use ASCII filenames" in `AGENTS.md`.
- **Strict frontmatter rejects unknown keys** → A future maintainer adding `description:` to the frontmatter schema must update both the parser struct and the spec. The same is true for `config.yaml`; consistency wins over forgiving-on-typo.
- **Two sources of truth** → Cross-source collisions are an error, not a precedence. Operators with both `workflows:` in `config.yaml` and matching files in `workflows_dir` must rename one. Documented in `AGENTS.md` and surfaced as a startup error.
- **No hot reload** → Editing a workflow file requires restarting `see`. The watcher reads the directory once at startup. The cost is one `restart` per workflow edit; the benefit is no in-memory state to keep in sync with the filesystem.

## Migration Plan

1. Add `WorkflowsDir` to `Config` and the loader, plus `loadWorkflowFiles` and the frontmatter parser in a new file (`workflow_files.go`).
2. Wire the loader into `loadStartupConfig` (or a new helper called from `main`) so `cfg.Workflows` becomes the merged slice before the existing `validateWorkflows` runs.
3. Add a regression test for each boundary: frontmatter parser (happy path, missing `---`, unknown key, blank body), file discovery (alphabetical sort, dotfile skip, missing dir, dir-is-file), merge (collision = error, source order = `.md` first), and end-to-end (a `~/.config/see/workflows/` with one workflow survives startup).
4. Update `AGENTS.md` (one new section in the configuration schema, plus a paragraph in the custom workflows section) and `config.example.yaml` (one commented `workflows_dir:` line).
5. Run `gofmt` and `go test -timeout 30s ./...`.
6. Run `openspec sync-specs --change add-workflow-files` to promote the delta specs into `openspec/specs/`.
7. Re-run `openspec validate --change add-workflow-files` and confirm no issues.

Rollback: the change is additive. Removing the new code path leaves `config.yaml` `workflows:` exactly as it is today. No data migration is required.

## Open Questions

None. A `--workflows-dir` CLI flag, per-project `.see/workflows/` directories, recursive globs, and hot reload are deliberate non-goals; revisit if operators ask.