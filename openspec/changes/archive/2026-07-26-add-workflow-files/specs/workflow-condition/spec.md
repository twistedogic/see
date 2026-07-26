## ADDED Requirements

### Requirement: Workflows may be supplied as Markdown files

A workflow MAY be supplied as a single `.md` file under `workflows_dir` instead of as an entry in `config.yaml` `workflows:`. The workflow's `name` SHALL be the filename without its `.md` extension. The file SHALL consist of YAML frontmatter (between two `---` lines) plus a Markdown body; the frontmatter SHALL carry `condition`, `commit`, and an optional `model`, and the body SHALL be the workflow's prompt. Workflows supplied as files SHALL participate in the same validation, ordering, and identity rules as workflows supplied through `config.yaml`.

#### Scenario: A single workflow file is equivalent to a config.yaml entry

- **WHEN** `workflows_dir` contains `openspec.md` with a valid frontmatter and body
- **THEN** startup succeeds
- **AND** `openspec` is one workflow in the merged set
- **AND** its `prompt`, `condition`, `commit`, and `model` come from the file
- **AND** its evaluation order matches its filename's alphabetical position

#### Scenario: A workflow file with a conflicting config.yaml entry is rejected

- **WHEN** `workflows_dir` contains `openspec.md`
- **AND** `config.yaml` `workflows:` contains an entry with `name: openspec`
- **THEN** `see` exits with status `2` before watching
- **AND** the error names the file path and the `workflows:` index

#### Scenario: A frontmatter name does not rename the workflow

- **WHEN** `workflows_dir` contains `openspec.md` with frontmatter `name: apply-openspec`
- **THEN** the workflow's effective name is `openspec`, the filename
- **AND** the workflow's lane digest is derived from `openspec`, not from `apply-openspec`

#### Scenario: A workflow file's body becomes its prompt

- **WHEN** `workflows_dir` contains `openspec.md` whose body is `Apply {change} and verify.`
- **AND** the normalized change is `add-dark-mode`
- **THEN** the agent receives `Apply add-dark-mode and verify.` for that workflow

#### Scenario: A workflow file with a blank body is rejected

- **WHEN** `workflows_dir` contains `openspec.md` whose body is empty or whitespace-only
- **THEN** `see` exits with status `2` before watching
- **AND** the error names the file path and identifies the missing prompt body

#### Scenario: A workflow file with an unknown frontmatter key is rejected

- **WHEN** `workflows_dir` contains `openspec.md` whose frontmatter has a key other than `name`, `condition`, `commit`, or `model`
- **THEN** `see` exits with status `2` before watching
- **AND** the error names the file path and the unknown key

#### Scenario: The config.yaml workflows block remains accepted

- **WHEN** configuration contains `workflows:` with one entry
- **AND** `workflows_dir` is absent, missing, or empty
- **THEN** startup succeeds
- **AND** the `workflows:` entry runs as the only workflow
- **AND** no `.md` workflow files are required