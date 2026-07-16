## Context

`see` reads layered configuration from two places — a global YAML
file at `os.UserConfigDir()/see/config.yaml` and the command-line
flags parsed in `main.go` — and applies them at startup. Today the
two knobs that participate in layering use different precedence
rules:

- **Prompt template**: `--prompt` (CLI) wins over `config.yaml`
  `prompt`, which wins over the embedded `prompt.md` default.
  Implemented by `selectPromptTemplate` in `config.go:121`.
- **Watch list**: `--watch` (CLI) entries are *unioned* with the
  `watches` sequence in `config.yaml`. Implemented by
  `resolveWatchList` in `main.go:538`.

The two-knob, two-rule state is observable in the running binary:
`see --watch /extra --prompt "..."` overrides the configured prompt
but extends the configured watch list. The intent of layered
configuration in `see` is "config is the default, flags override";
the prompt template already implements that intent, and the watch
list diverges from it without a recorded reason. The project
guideline in `AGENTS.md` ranks correctness, readability, simplicity,
and long-term maintainability above short-term effort; aligning the
two rules under one documented precedence is the long-term-correct
shape.

`--ignore-config` already exists as a separate escape hatch for the
case the precedence rule does not cover — a malformed configuration
file that should not block startup. That role is preserved unchanged.

## Goals / Non-Goals

**Goals:**

- One precedence rule across every layered configuration knob in
  `see`: command-line value (if non-empty) > configured value > the
  embedded / hard-coded default. The watch list joins the prompt
  template on this rule.
- Keep the `--ignore-config` escape hatch for the malformed-config
  startup case, with its current meaning (skip the file entirely).
- Pin the new contract at every layer (spec, code, tests) so future
  contributors cannot re-introduce the union-by-default asymmetry.
- Keep the public CLI surface, the configuration file format, and
  the embedded prompt default byte-identical.

**Non-Goals:**

- No new flags, no new config keys, no new exit codes.
- No change to `--ignore-config`'s meaning beyond preserving it.
- No change to the prompt precedence rule — it already matches the
  intended model.
- No change to the current-working-directory fallback behavior
  beyond the precedence ladder's "step 3" position.
- No change to watch-pattern expansion (tilde, glob, `**`
  rejection) — those are governed by separate requirements and
  stay unchanged.

## Decisions

- **`resolveWatchList` becomes a precedence ladder, not a
  concatenation.** The body short-circuits at the first non-empty
  source: if `--watch` produced any entries, use them; else if the
  configured `watches` produced any entries, use those; else fall
  back to `cwd`. The function signature is unchanged (`cliWatches,
  cfgWatches []string`) so `main()` keeps calling it the same way.
  *Alternative considered:* add an explicit `mergedWatches []string`
  step that picks at the call site. Rejected — pushes the rule into
  `main()`, where the existing `selectPromptTemplate` already lives,
  and makes the precedence rule harder to find when reading the
  watch-list code path. Keeping the rule in `resolveWatchList`
  keeps it next to its own tests.

- **CLI presence, not non-empty content, is the override signal.**
  An empty `--watch` (the default when the operator passes no
  `--watch` at all, or passes `--watch ""` which the existing
  `multiFlag.Set` accepts as a literal pattern) is treated the same
  as "no CLI source". The shell-glob and `resolveTargets` paths
  already reject the empty pattern, so an empty string from the CLI
  contributes nothing and falls through to the configured source.
  `len(cliWatches) > 0` is the override predicate. *Alternative
  considered:* make the rule "use the CLI list only if it
  contributed at least one non-empty entry after expansion."
  Rejected — keeps the rule split across two layers (precedence +
  expansion) and complicates `resolveWatchList`'s contract. The
  current expansion layer already produces the resolved list; the
  precedence layer decides which resolved list wins.

- **`--ignore-config` is unchanged.** It keeps its meaning of
  "skip the configuration file at startup so a malformed file does
  not block the run," with `main()` passing a zero-value `Config`
  to `resolveWatchList` (effectively `cfgWatches == nil`). The
  precedence rule's "step 2" branch handles that case naturally:
  `--ignore-config --watch /foo` resolves to `/foo`; `--ignore-config`
  alone falls through to `cwd`. No new test for `--ignore-config`
  semantics is needed — the existing
  `TestResolveWatchListConfigOnly` covers the "no config" case by
  passing `nil`, which is what `loadStartupConfig` returns under
  `--ignore-config`. *Alternative considered:* rename
  `--ignore-config` to make its precedence-only role explicit and
  add a separate `--allow-malformed-config` for the escape hatch.
  Rejected — `--ignore-config` already serves the escape-hatch role,
  the rename is a separate ergonomic question, and the priority of
  this change is the precedence rule, not the flag naming.

- **`selectPromptTemplate` is the reference shape for the new
  watch-list rule.** Both knobs now use the same pattern: pick the
  first non-empty layer, document the layers in source order, name
  the fallback explicitly. A short comment in `resolveWatchList`
  points at `selectPromptTemplate` so a future contributor changing
  one rule finds the other. *Alternative considered:* factor a
  generic `pickFirstNonEmpty[T](layers ...[]T) []T` helper and use
  it for both knobs. Rejected — one helper, one caller is
  premature abstraction; the two rules are short and stay readable
  next to their own knobs. The cross-reference comment is enough.

- **Test rewrites are scoped to the union assertions.**
  `TestResolveWatchListUnionsCLIAndConfig` becomes a test that
  asserts CLI replaces config when both are present, and
  `TestResolveWatchListOverlappingSourcesDedupe` is removed (the
  dedupe behavior is still tested transitively by
  `resolveTargets`'s own tests and by the unchanged
  `TestResolveWatchListConfigOnly` / `TestResolveWatchListCLIOnly`
  tests). The two unchanged tests stay byte-identical and pin the
  cwd fallback and the no-config / no-CLI paths. *Alternative
  considered:* keep the union test and add a "replace" test side
  by side. Rejected — two tests for the same code path with
  opposite expected outputs is the kind of artifact that lures a
  future contributor into thinking both behaviors exist. The
  replacement is the behavior now.

## Risks / Trade-offs

- [Any operator that relied on `--watch` extending a non-empty
  configured `watches:` will see their watch list shrink after
  upgrading] → Mitigation: the precedence rule matches the
  prompt-template behavior and matches the project's stated
  intent in `AGENTS.md`, so the new contract is the documented
  one; the breaking case is also the case that was inconsistent
  with the rest of the CLI. The spec delta and the rewrite of
  the union test make the new contract loud at every layer. The
  escape hatch `--ignore-config --watch ...` stays available for
  any operator that wants the legacy behavior.

- [A future contributor could re-introduce the union shape
  because the implementation looks like a small change to one
  helper function] → Mitigation: the precedence comment in
  `resolveWatchList` cross-references `selectPromptTemplate`,
  the spec prose names the rule explicitly ("the first source
  that contributes at least one entry SHALL be the only source
  consulted"), and the two rewritten tests pin the contract at
  the unit level.

- [`len(cliWatches) > 0` as the override predicate is sensitive
  to how `multiFlag` represents an explicitly-passed empty
  string] → Mitigation: `multiFlag.Set` accepts empty strings
  and `resolveTargets` rejects them at expansion time, so the
  empty case contributes no resolved entry and falls through
  to the configured layer. The behavior is correct under both
  "operator passed `--watch ""`" and "operator passed no
  `--watch` at all." A test for the empty-CLI-list-falls-through
  case is already implicit in `TestResolveWatchListConfigOnly`.

## Migration Plan

This is a behavior change inside a single-binary CLI with no
on-disk format changes and no new flags. Migration:

1. Land the change on a topic branch.
2. Run `go test -timeout 30s ./...` and confirm every test in
   `discovery_test.go` and `config_test.go` passes with the
   rewritten test set called out in the tasks doc.
3. Run `go vet ./...` and `gofmt -l .` to confirm clean output.
4. Run `openspec validate make-cli-flags-override-config` and
   confirm the change is still valid after the implementation
   edits.
5. Merge via the project's normal flow. No rollback plan is
   needed beyond a normal `git revert`; the prior commit is the
   previous (union) behavior and reverts cleanly if needed.

## Open Questions

- None. The change is scoped to one precedence rule, one helper
  function, and one spec requirement. The escape-hatch role of
  `--ignore-config` is preserved as-is, so no flag-naming or
  ergonomic question is open.
