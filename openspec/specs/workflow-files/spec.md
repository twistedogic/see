# workflow-files

## Purpose

Define how `see` discovers and parses Markdown workflow files under `workflows_dir`
and merges them with `config.yaml` `workflows:` entries into a single ordered
list evaluated per repository.
## Requirements
### Requirement: workflows_dir selects the workflow file source

The configuration schema SHALL accept an optional `workflows_dir` string field. A non-empty value SHALL be tilde-expanded and used as the directory `see` reads at startup. A blank or absent value SHALL fall back to the default `~/.config/see/workflows/`. The recursive `**` glob pattern SHALL NOT be accepted in the path. A path that exists but is not a directory SHALL cause startup to fail with an actionable error naming the path. A path that does not exist SHALL be treated as "no `.md` workflows" and SHALL NOT cause startup to fail.

#### Scenario: Default directory is used when the field is absent

- **WHEN** configuration contains no `workflows_dir` field
- **THEN** `see` reads from the default `~/.config/see/workflows/`

#### Scenario: Configured directory is used when the field is set

- **WHEN** configuration sets `workflows_dir` to `~/work/wf`
- **THEN** `see` reads from the tilde-expanded directory
- **AND** the default directory is not consulted

#### Scenario: Recursive glob is rejected

- **WHEN** configuration sets `workflows_dir` to a path containing `**`
- **THEN** startup fails with an actionable error naming the field

#### Scenario: Path that exists but is not a directory is rejected

- **WHEN** configuration sets `workflows_dir` to a path that resolves to a regular file
- **THEN** startup fails with an actionable error naming the path

#### Scenario: Missing directory is a silent no-op

- **WHEN** the resolved directory does not exist on disk
- **THEN** `see` continues startup with zero `.md` workflows
- **AND** `workflows:` entries in `config.yaml` are still loaded

### Requirement: Workflow files are discovered by `*.md` glob

`see` SHALL list direct children of `workflows_dir` whose names match the `*.md` glob. Hidden files (basename starting with `.`) SHALL be skipped. Subdirectories SHALL NOT be traversed. The list SHALL be sorted alphabetically by basename before parsing. Each matched file SHALL be parsed into one workflow candidate; the directory of matched files SHALL NOT be treated as a workflow itself.

#### Scenario: Alphabetical order is the execution order

- **WHEN** `workflows_dir` contains `02-deps.md` and `01-openspec.md`
- **THEN** the `01-openspec` workflow is evaluated before the `02-deps` workflow

#### Scenario: Hidden files are skipped

- **WHEN** `workflows_dir` contains `.disabled.md` alongside `openspec.md`
- **THEN** only `openspec.md` is parsed

#### Scenario: Subdirectories are not traversed

- **WHEN** `workflows_dir` contains a `nested/` subdirectory and `openspec.md`
- **THEN** only `openspec.md` is parsed
- **AND** files inside `nested/` are not consulted

#### Scenario: Non-`.md` files are ignored

- **WHEN** `workflows_dir` contains `README.txt` and `openspec.md`
- **THEN** only `openspec.md` is parsed
- **AND** `README.txt` is silently ignored

### Requirement: Filename is the workflow name

Each discovered file SHALL produce one workflow whose `name` is the file's basename with the trailing `.md` extension removed. The leading `.` in a hidden file's name SHALL NOT cause that file to be parsed (the discovery filter already excludes hidden files). The workflow name SHALL be the only authoritative name for that workflow; no other source of name overrides it.

#### Scenario: Basename sans extension is the workflow name

- **WHEN** `workflows_dir` contains `openspec.md`
- **THEN** the parsed workflow has `name` equal to `openspec`

#### Scenario: A renamed file changes the workflow identity

- **WHEN** `openspec.md` is renamed to `apply-openspec.md` between startup runs
- **THEN** the two runs select different lane digests
- **AND** the per-agent log filename uses the new digest

### Requirement: Frontmatter holds condition, commit, model, and an ignored name

Each workflow file SHALL consist of two parts: a YAML frontmatter block delimited by lines containing exactly `---`, and a body that follows the closing delimiter. The frontmatter SHALL be decoded with the strict YAML decoder. The decoded frontmatter SHALL accept exactly these keys: `name`, `condition`, `commit`, `model`, `disable`, `check`, `measure`. Any other key SHALL cause startup to fail with an actionable error naming the file path and the unknown key. The `name` key SHALL be parsed but ignored; the workflow's name is the filename as described above. The `model` key SHALL be optional; an absent or blank `model` SHALL be treated as "unset" and SHALL NOT be passed to the agent. The `disable` key SHALL be an optional boolean; an absent `disable` SHALL be treated as `false` (enabled). The `check` key SHALL be optional; an absent or blank `check` SHALL mean the workflow has no check gate (see workflow-condition). The `measure` key SHALL be optional; an absent `measure` SHALL mean the workflow falls back to the convention script at `~/.config/see/measure/<workflow-name>.sh`, and a blank `measure` SHALL fail startup with an actionable error naming the file path and the `measure` field (see workflow-condition). The `condition` and `commit` keys SHALL be required; their values, after trimming whitespace, SHALL contain at least one non-whitespace character.

#### Scenario: All four keys present

- **WHEN** `openspec.md` contains frontmatter with `name`, `condition`, `commit`, and `model`
- **THEN** the workflow's effective name is `openspec` (the filename)
- **AND** the workflow's `condition` is the value of the `condition` key
- **AND** the workflow's `commit` is the value of the `commit` key
- **AND** the workflow's `model` is the value of the `model` key (or unset if blank)

#### Scenario: Frontmatter name is ignored

- **WHEN** `openspec.md` contains frontmatter with `name: apply-openspec`
- **THEN** the workflow's effective name is `openspec`, not `apply-openspec`

#### Scenario: Missing opening delimiter fails startup

- **WHEN** a file in `workflows_dir` does not begin with a `---` line
- **THEN** startup fails with an actionable error naming the file path

#### Scenario: Missing closing delimiter fails startup

- **WHEN** a file in `workflows_dir` has only one `---` line
- **THEN** startup fails with an actionable error naming the file path

#### Scenario: Unknown frontmatter key fails startup

- **WHEN** a file's frontmatter contains an unrecognized key
- **THEN** startup fails with an actionable error naming the file path and the unknown key

#### Scenario: Blank condition or commit fails startup

- **WHEN** a file's frontmatter has a blank or absent `condition` or `commit`
- **THEN** startup fails with an actionable error naming the file path and the offending field

#### Scenario: Blank model is treated as unset

- **WHEN** a file's frontmatter has a blank or absent `model`
- **THEN** the workflow's effective `model` is the empty string
- **AND** the agent is invoked without a `--model` flag

#### Scenario: Absent disable is treated as enabled

- **WHEN** a file's frontmatter has no `disable` key
- **THEN** the workflow's effective `disable` is `false`
- **AND** the workflow is evaluated normally

#### Scenario: Frontmatter disable parks the file workflow

- **WHEN** `openspec.md` contains frontmatter with `disable: true`
- **THEN** the workflow is loaded and passes validation
- **AND** the workflow is removed from the evaluated list before the run loop
- **AND** the workflow does not run for any watched repository

#### Scenario: A complete frontmatter with check loads

- **WHEN** `openspec.md` contains frontmatter with `name`,
  `condition`, `commit`, `model`, and `check: go test ./...`
- **THEN** startup succeeds
- **AND** the workflow's effective name is `openspec`, the filename
- **AND** the workflow's check gate is `go test ./...`

#### Scenario: An absent check means no check gate

- **WHEN** `openspec.md` contains frontmatter with `condition` and
  `commit` but no `check` key
- **THEN** startup succeeds
- **AND** the workflow has no check gate

#### Scenario: A frontmatter measure overrides the convention script

- **WHEN** `openspec.md` contains frontmatter with `condition`,
  `commit`, and `measure: ./bench.sh`
- **THEN** startup succeeds
- **AND** the workflow's resolved measure command is `./bench.sh`
- **AND** the convention script at `~/.config/see/measure/openspec.sh` is
  not consulted

#### Scenario: An absent measure falls back to the convention script

- **WHEN** `openspec.md` contains frontmatter with `condition` and
  `commit` but no `measure` key
- **AND** `~/.config/see/measure/openspec.sh` exists
- **THEN** startup succeeds
- **AND** the workflow's resolved measure command is the contents of that
  convention script

#### Scenario: A present blank measure fails startup

- **WHEN** `openspec.md` contains frontmatter with `measure:` set to an
  empty string or whitespace only
- **THEN** startup fails with an actionable error naming the file path
  and the `measure` field

### Requirement: Body is the prompt

The portion of the file following the closing `---` line SHALL be the workflow's prompt. The prompt SHALL have one leading blank line (if present) and all trailing whitespace removed. Interior newlines SHALL be preserved. The resulting value SHALL contain at least one non-whitespace character; a blank or whitespace-only body SHALL cause startup to fail with an actionable error naming the file path. The `{change}` substitution SHALL apply to the body identically to the YAML `prompt` field.

#### Scenario: Standard Markdown body becomes the prompt

- **WHEN** `openspec.md` ends with `---` followed by `Apply change {change}.`
- **THEN** the workflow's prompt is `Apply change {change}.`

#### Scenario: Multi-line body is preserved

- **WHEN** the body of `openspec.md` is multiple paragraphs separated by blank lines
- **THEN** the workflow's prompt preserves those paragraphs verbatim
- **AND** the `{change}` substitution operates on the rendered prompt

#### Scenario: Blank body fails startup

- **WHEN** `openspec.md` has a closing `---` line followed by no non-whitespace content
- **THEN** startup fails with an actionable error naming the file path
- **AND** the error identifies the missing prompt body

### Requirement: Workflows are merged with the config.yaml block

`see` SHALL merge the `.md` workflows with the `workflows:` entries from `config.yaml` into one ordered list. `.md` workflows SHALL be appended first, in alphabetical order by filename. `config.yaml` workflows SHALL follow, in their declared order. A workflow name that appears in both sources SHALL cause startup to fail with an actionable error naming the file path and the `workflows:` index. The merged list SHALL then pass through the existing `workflows:` validation: every workflow SHALL have nonblank `name`, `prompt`, `condition`, and `commit`; every `name` SHALL be unique within the merged list.

#### Scenario: Both sources contribute workflows

- **WHEN** `workflows_dir` contains `openspec.md`
- **AND** `config.yaml` contains `workflows:` with one entry named `deps`
- **THEN** the merged list contains `openspec` followed by `deps`

#### Scenario: Collision between sources fails startup

- **WHEN** `workflows_dir` contains `openspec.md`
- **AND** `config.yaml` contains `workflows:` with one entry named `openspec`
- **THEN** startup fails with an actionable error naming the file path and the `workflows:` index
- **AND** the watcher does not start

#### Scenario: Empty config.yaml workflows block still loads .md files

- **WHEN** `config.yaml` has no `workflows:` block
- **AND** `workflows_dir` contains `openspec.md` and `deps.md`
- **THEN** the merged list contains `openspec` followed by `deps`
- **AND** startup succeeds

#### Scenario: Empty workflows_dir still loads config.yaml workflows

- **WHEN** `workflows_dir` exists but contains no `.md` files
- **AND** `config.yaml` contains `workflows:` with two entries
- **THEN** the merged list contains the two `config.yaml` entries in declared order
- **AND** startup succeeds

