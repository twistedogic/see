## ADDED Requirements

### Requirement: Watcher renders the agent prompt from a configurable template

`Watcher.work` SHALL derive the prompt passed to `Agent.Run` by
substituting the literal token `{change}` in a template string with
the active change name. The template SHALL be taken from
`Watcher.PromptTemplate`; if `Watcher.PromptTemplate` is empty or
contains only whitespace, the watcher SHALL use a default template
embedded into the binary from the in-tree file `prompt.md` at the
repo root.

A `Watcher.SetPromptTemplate(s string)` setter SHALL normalize the
input by trimming surrounding whitespace and treating the empty
result as "use the embedded default". The setter is the documented
mutator on the field; assigning to `PromptTemplate` directly is
permitted but bypasses the normalization.

The renderer SHALL replace every occurrence of `{change}` in the
template with the active change name. No other tokens are defined;
any other `{name}` substring is preserved verbatim, including its
literal `{` and `}` characters.

#### Scenario: Empty PromptTemplate uses the embedded default

- **WHEN** a `Watcher` is constructed with no call to
  `SetPromptTemplate` (or `Watcher.PromptTemplate == ""`) and
  `Watcher.work` invokes `Agent.Run` for change `add-foo`
- **THEN** the prompt passed as the 4th argument to `Agent.Run`
  contains the substring `add-foo`
- **THEN** the prompt body equals the embedded `prompt.md`
  contents with `{change}` substituted by `add-foo`

#### Scenario: Whitespace-only template normalizes to the default

- **WHEN** `SetPromptTemplate("   ")` is called on a `Watcher`
- **THEN** `Watcher.PromptTemplate` equals the embedded default,
  not the whitespace string

#### Scenario: User-supplied template renders with the change name

- **WHEN** `SetPromptTemplate("Apply the change {change} now")`
  is called on a `Watcher` and `Watcher.work` invokes `Agent.Run`
  for change `add-foo`
- **THEN** the prompt passed to `Agent.Run` is
  `"Apply the change add-foo now"`

#### Scenario: Multiple substitutions are all replaced

- **WHEN** `SetPromptTemplate("first {change} second {change}")`
  is called on a `Watcher` and `Watcher.work` invokes `Agent.Run`
  for change `add-foo`
- **THEN** the prompt passed to `Agent.Run` is
  `"first add-foo second add-foo"`

#### Scenario: Unknown tokens are preserved verbatim

- **WHEN** `SetPromptTemplate("Apply {change} in {repo} on {date}")`
  is called on a `Watcher` and `Watcher.work` invokes `Agent.Run`
  for any change
- **THEN** the prompt passed to `Agent.Run` substitutes only
  `{change}`; `{repo}` and `{date}` appear unchanged in the output

#### Scenario: Empty change name still renders

- **WHEN** `SetPromptTemplate("prefix {change} suffix")` is set
  and `Watcher.work` invokes `Agent.Run` for an empty change name
- **THEN** the prompt passed to `Agent.Run` is `"prefix  suffix"`
  with a single space where the change name was

#### Scenario: prompt.md build-time embedding

- **WHEN** the binary is built without a `prompt.md` file at the
  repo root (alongside `main.go`)
- **THEN** the build fails at compile time with an error pointing
  at the `//go:embed` directive
- **THEN** no binary is produced
