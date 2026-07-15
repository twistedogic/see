## Context

`see` invokes `pi` once per active openspec change, with a prompt that
tells the model to read the change, implement its tasks, run tests,
verify, archive, and sync specs. Today that prompt is a Go string
literal inside `applyPrompt(change string) string` in `main.go`. The
prompt has grown load-bearing: it is the only thing that steers `pi`
through the openspec workflow. Yet it is invisible to anyone reading
the repo, untunable without a fork, and untestable as content (tests
can only check that the change name appears in it).

This change extracts the prompt into a checked-in file
(`prompt.md`) embedded into the binary via `//go:embed`, exposes a
single CLI flag to override the template at startup, and replaces the
hardcoded helper with a generic, testable renderer.

## Goals / Non-Goals

**Goals:**

- One source of truth for the default prompt, reviewable in PRs as
  prose, editable without touching Go.
- Operator can override the prompt template from the CLI without
  forking or rebuilding.
- Render path is one pure function, exercised by tests on both the
  token substitution and the embedded default.
- No constructor churn, no `Agent` interface change, no breaking
  changes to any public surface.

**Non-Goals:**

- Templating beyond the single `{change}` token (no `{date}`, `{repo}`,
  `{branch}`, etc.).
- Per-run prompt variation (the template is fixed for the lifetime of
  one `see` invocation; live-reload would not be supported).
- Per-change prompts (one template handles every change).
- Reading the default from anywhere except the embedded `prompt.md`
  (no env var, no `~/.config/see/prompt.md`).

## Decisions

### Decision 1: `{change}` as the substitution token

Alternatives considered:

- **`%s` / `fmt.Sprintf`**: familiar to Go readers, but `%` is a
  legal character in prose ("spend 50% of effort"); a template that
  contains any `%`-spec would either format-error or silently
  mis-substitute.
- **`$change` shell-style**: ambiguous inside backtick-quoted strings
  and reads as a variable to anyone who has used shell or
  Makefiles — wrong vibe for content meant to be handed to a chat
  model.
- **`{{ change }}` Mustache**: standard, but doubles the noise for a
  single substitution and invites adding more tokens later (a
  slippery slope this change explicitly does not want to be on).
- **Two tokens (`{change}` + `{quoted-change}`)**: a workaround for
  the `%q` vs `%s` discussion. Adds a knob nobody asked for.

Chosen: a single literal token `{change}` replaced via
`strings.ReplaceAll`. Quoting (or not) is the file author's choice —
the embedded default keeps the existing quoted style around the token.

### Decision 2: Default lives in `prompt.md`, embedded via `//go:embed`

Alternatives considered:

- **`const defaultPromptTemplate = "..."` in `main.go`** (status
  quo, with the const extracted to a package-level symbol): zero new
  mechanics, but the content is still a Go string — unreadable in
  diffs, easy to forget to update alongside prose changes elsewhere,
  no compile guard if the file "is the prompt" is deleted.
- **Runtime read from disk** (`os.ReadFile("prompt.md")` on
  startup): editable without rebuild, but loses "the binary
  contains the prompt" as an invariant and surfaces filesystem
  permission errors at runtime.
- **`go:generate` from `prompt.md` to a `prompt.go` file**:
  reinventing `//go:embed` with extra steps and a code-gen step to
  maintain.

Chosen: `//go:embed prompt.md` at the top of `main.go`. The file is
in the repo root next to `main.go`; the directive reads at build
time and bakes the contents into the binary as a `string`; missing
`prompt.md` fails compilation, not runtime.

### Decision 3: `Watcher.PromptTemplate` field + `SetPromptTemplate` setter

Alternatives considered:

- **New parameter on `NewWatcher`**: forces every test constructor
  and the existing `main` wiring to thread a value through. Pulls
  a CLI-parsing concern into the watcher's construction site, which
  is the wrong place — the flag is parsed in `main()`, not in
  `NewWatcher`.
- **Untyped global**: `var promptTemplate string` at package level,
  read inside `work`. Loses isolation between `Watcher` instances
  (today there is exactly one, but the type is exported and could be
  used by tests).
- **Functional options**: idiomatic Go, but the entire watcher is
  constructor-style; mixing patterns for one knob is noise.

Chosen: a field + setter. The setter is the natural place for the
"empty / whitespace-only input → default" normalization so `work`
reads as "render and invoke" with no policy inline.

### Decision 4: `renderPrompt` is a pure `strings.ReplaceAll`

```go
func renderPrompt(template, change string) string {
    return strings.ReplaceAll(template, "{change}", change)
}
```

Alternatives considered:

- **`text/template`**: standard, but overkill for one substitution,
  and the template's error surface (parse failures, undefined
  actions) leaks into `see` for no value.
- **Regex pass with a single capture**: same outcome as
  `ReplaceAll`, more lines, slightly faster on pathological inputs
  that don't exist.

Chosen: one line. Trivial to test, trivial to read. If a second
token ever earns its keep, the function grows a `map[string]string`
or a real template; today it doesn't.

### Decision 5: `-prompt <template>` flag, empty → default

The flag is `flag.String("prompt", "", "override the agent prompt template; {change} is replaced with the active change name")`. Default is empty; the setter maps empty (or whitespace-only) to `defaultPromptTemplate`.

Alternatives considered:

- **Make `-prompt` required**: would force every existing invocation
  to start typing the default — not an improvement.
- **Read from env var** (`SEE_PROMPT`): extra surface, redundant
  with the flag.

Chosen: flag-only. `main()` parses, calls `w.SetPromptTemplate(*promptFlag)`. Empty flag value preserves today's behaviour exactly because `prompt.md` contains today's prompt.

### Decision 6: Embed site lives next to `applyPrompt`'s former home

`main.go` already imports nothing exotic and is the only place that
talks to the agent. Putting the embed directive there keeps the
"prompt default" knowledge in one file. Tests do not need access to
the embedded var directly — they exercise
`renderPrompt(defaultPromptTemplate, "add-foo")` through package-
internal access, and `Watcher.PromptTemplate` rendering through the
`fakeAgent` recorder.

## Risks / Trade-offs

- **[Risk] Token collision in user prompts.** A user who happens to
  write `{change}` in their own template will have it substituted.
  → **Mitigation:** documented in flag help text; one token, no
  escape syntax by design. If a user genuinely needs the literal
  string they can add a follow-up change for an escape rule.

- **[Risk] Build fails if `prompt.md` is missing.** The `//go:embed`
  directive errors at compile time when its target is absent.
  → **Mitigation:** that's the intended behavior — a deleted default
  becomes an obvious build break instead of a silent runtime
  regression. Documented in `prompt.md`'s reviewers' guide (a
  one-line `AGENTS.md` cross-reference).

- **[Risk] Trailing newline in `prompt.md` reaches `pi`.** The
  embed preserves whatever the file ends with.
  → **Mitigation:** cosmetic at the `pi` boundary; if it ever
  matters, a `.TrimRight` at the embed site is a one-line fix.
  Not preempted.

- **[Risk] `applyPrompt` test deletion may surprise readers.** The
  removed test was the only behavioural assertion on the default
  prompt body.
  → **Mitigation:** replaced by
  `TestDefaultTemplateMentionsChange`, which exercises the same
  invariant against the new code path (embedded var, not function).

- **[Trade-off] Setter has policy (`""` → default).** Concentrating
  the policy in the setter is good encapsulation, but means tests
  that want to assert "unset watcher renders default" must use the
  setter or set `PromptTemplate = ""` directly. Both work; the
  setter is the documented path.

## Migration Plan

Single PR, no rollout. The binary ships the embedded `prompt.md`
contents verbatim. Run `see --once` (or any existing invocation
pattern) without `-prompt` and the rendered prompt `pi` receives is
byte-identical to today's `applyPrompt("add-foo")` output for the
default change. Run with `-prompt '...'` and `pi` receives the
caller's rendered prompt.

Rollback is `git revert` of the PR — `applyPrompt` is the only
deleted function and reverting the diff restores it; the new flag
and field become inert unused code.

## Open Questions

None. All forks surfaced during exploration were resolved:
template-token strategy, file vs const, field vs constructor param,
renderer complexity, optional-vs-required flag, and quoted-vs-bare
default wording. The quoted wording is preserved in `prompt.md`.
