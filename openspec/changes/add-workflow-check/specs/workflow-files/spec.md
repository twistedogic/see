# workflow-files delta — add-workflow-check

## MODIFIED Requirements

### Requirement: Frontmatter holds condition, commit, model, check, and an ignored name

Each workflow file SHALL consist of two parts: a YAML frontmatter block
delimited by lines containing exactly `---`, and a body that follows the
closing delimiter. The frontmatter SHALL be decoded with the strict YAML
decoder. The decoded frontmatter SHALL accept exactly these keys:
`name`, `condition`, `commit`, `model`, `disable`, `check`. Any other
key SHALL cause startup to fail with an actionable error naming the file
path and the unknown key. The `name` key SHALL be parsed but ignored;
the workflow's name is the filename as described above. The `model` key
SHALL be optional; an absent or blank `model` SHALL be treated as
"unset" and SHALL NOT be passed to the agent. The `disable` key SHALL be
an optional boolean; an absent `disable` SHALL be treated as `false`
(enabled). The `check` key SHALL be optional; an absent or blank `check`
SHALL mean the workflow has no check gate (see workflow-condition). The
`condition` and `commit` keys SHALL be required; their values, after
trimming whitespace, SHALL contain at least one non-whitespace
character.

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

#### Scenario: Unknown frontmatter key fails startup

- **WHEN** a file's frontmatter contains an unrecognized key
- **THEN** startup fails with an actionable error naming the file path
  and the unknown key

#### Scenario: Blank required frontmatter field fails startup

- **WHEN** a file's frontmatter has a blank or absent `condition` or
  `commit`
- **THEN** startup fails with an actionable error naming the file path
  and the missing field

#### Scenario: Absent model is unset

- **WHEN** a file's frontmatter has a blank or absent `model`
- **THEN** `--model` is not passed to `pi` for that workflow

#### Scenario: Absent disable means enabled

- **WHEN** a file's frontmatter has no `disable` key
- **THEN** the workflow is enabled

#### Scenario: Disabled frontmatter parks the workflow

- **WHEN** `openspec.md` contains frontmatter with `disable: true`
- **THEN** the workflow is validated but removed from the evaluated list
