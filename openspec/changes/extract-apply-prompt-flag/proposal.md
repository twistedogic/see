## Why

The prompt `see` sends to `pi` per agent run is hardcoded inside
`applyPrompt` in `main.go`. Operators running their own watch loops have
no way to tune that prompt without forking the binary, and there is no
in-tree record of what the prompt currently says — the answer lives in a
Go string literal that drifts unnoticed. Extract the prompt so it is
overridable from the CLI and human-reviewable as a file.

## What Changes

- New CLI flag `-prompt <template>` on `see` lets the caller override
  the agent prompt at startup. The template is a string with a single
  substitution token `{change}` which the runtime replaces with the
  active change name. Empty / unset falls back to the in-tree default.
- The default prompt moves out of `main.go` into a checked-in
  `prompt.md` at the repo root and is embedded into the binary via
  `//go:embed prompt.md` at compile time. Edits are a one-line file
  change; the build fails if `prompt.md` is missing.
- `applyPrompt(change string) string` is removed. It is replaced by
  `renderPrompt(template, change string) string` (one-liner,
  `strings.ReplaceAll`) and a `defaultPromptTemplate string` populated
  from the embedded file. `Watcher.work` calls the renderer instead of
  the deleted helper.
- `Watcher` gains a `PromptTemplate string` field with a
  `SetPromptTemplate(s string)` setter that normalizes empty/whitespace
  input to the default. `main()` calls the setter from the flag value.

No breaking changes to the `Watcher` constructor, the `Agent`
interface, or any public surface — only an additive field, a setter,
and a CLI flag.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `watcher`: The watcher accepts a user-supplied prompt template and
  renders it with the active change name before invoking the agent.
  When no template is supplied, the watcher uses a default embedded
  from `prompt.md` so today's behavior is preserved as the default.

## Impact

- `main.go`: delete `applyPrompt`; add `defaultPromptTemplate` (embedded),
  `renderPrompt`, `Watcher.PromptTemplate`, `SetPromptTemplate`, the
  `-prompt` flag, and the flag-to-setter wiring.
- `main_test.go`: drop `TestApplyPromptMentionsChange`; add
  `TestRenderPromptSubstitutesChange`,
  `TestDefaultTemplateMentionsChange`,
  `TestWatcherRendersUserPrompt`; extend `fakeAgent` with a
  `prompts []string` recorder.
- New file: `prompt.md` at repo root (the default prompt body).
- No dependency changes. No build-flag changes (no new tool, no new
  module — `embed` is stdlib).
- Behavior change visible only when `-prompt` is set: `pi` receives the
  user's rendered prompt instead of the embedded default. Today's run
  is byte-identical to before when `-prompt` is unset.
