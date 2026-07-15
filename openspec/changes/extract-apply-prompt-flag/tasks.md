## 1. Tests first

- [ ] 1.1 Extend `fakeAgent` in `main_test.go` with a `prompts []string`
  recorder that captures the 4th `Run` argument on every call
- [ ] 1.2 Add `TestRenderPromptSubstitutesChange` asserting that
  `renderPrompt("Apply {change}", "add-foo")` returns
  `"Apply add-foo"` and that an empty change name produces a single
  space in the substituted position
- [ ] 1.3 Add `TestDefaultTemplateMentionsChange` asserting that
  `renderPrompt(defaultPromptTemplate, "add-foo")` contains the
  substring `"add-foo"`
- [ ] 1.4 Add `TestWatcherRendersUserPrompt` constructing a
  `Watcher{PromptTemplate: "Apply {change} now"}` (via setter),
  driving `work` against a fakeAgent, and asserting the recorded
  prompt equals `"Apply add-foo now"` for change `add-foo`
- [ ] 1.5 Add `TestWatcherFallsBackToEmbeddedDefault` exercising
  `SetPromptTemplate("   ")` and asserting the embedded default
  flows through to `Agent.Run`

## 2. Implementation

- [ ] 2.1 Create `prompt.md` at the repo root with the existing
  `applyPrompt` body using `{change}` as the substitution token, e.g.

  ```
  Apply the openspec change {change}: read its proposal and tasks,
  implement them, run the tests, verify, then archive the change.
  Sync specs if needed.
  ```
- [ ] 2.2 Replace `applyPrompt` in `main.go` with
  `//go:embed prompt.md` + `var defaultPromptTemplate string`,
  and add `renderPrompt(template, change string) string` as a
  one-liner using `strings.ReplaceAll`
- [ ] 2.3 Add `PromptTemplate string` to the `Watcher` struct and
  `func (w *Watcher) SetPromptTemplate(s string)` that trims `s`
  and assigns the embedded default when the trimmed value is empty
- [ ] 2.4 Change `Watcher.work`'s call site from
  `applyPrompt(change)` to `renderPrompt(w.PromptTemplate, change)`,
  so an empty `PromptTemplate` produces an empty substitution
  result instead of the embedded default
- [ ] 2.5 Update `Watcher.work` to resolve the effective template
  before calling the renderer: if `w.PromptTemplate == ""`, use
  `defaultPromptTemplate` instead, then call
  `renderPrompt(effectiveTemplate, change)`
- [ ] 2.6 Add a `-prompt` flag in `main()` of type `flag.String`
  with usage `"override the agent prompt template; {change} is
  replaced with the active change name"` and wire it through
  `w.SetPromptTemplate(*promptFlag)` after constructing the watcher

## 3. Cleanup

- [ ] 3.1 Delete the now-unused `applyPrompt` function from
  `main.go`
- [ ] 3.2 Delete `TestApplyPromptMentionsChange` from
  `main_test.go` (superseded by `TestDefaultTemplateMentionsChange`
  and `TestRenderPromptSubstitutesChange`)
- [ ] 3.3 Verify no other references to `applyPrompt` remain
  (`rg applyPrompt main.go main_test.go` → empty)

## 4. Verify

- [ ] 4.1 `go build ./...` succeeds (proves the `//go:embed`
  directive resolves against `prompt.md`)
- [ ] 4.2 `go test ./...` passes (covers unit + end-to-end contract)
- [ ] 4.3 Spot-check: build and run `see --once` against this repo
  with one active change, then inspect the JSONL log under
  `SEE_LOG_DIR` to confirm the prompt `pi` received matches the
  embedded default rendered with the change name
