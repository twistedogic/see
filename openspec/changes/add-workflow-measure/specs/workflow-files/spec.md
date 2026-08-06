# workflow-files delta — add-workflow-measure

## MODIFIED Requirements

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
