## 1. Resolve watch list with precedence, not union

- [ ] 1.1 In `main.go`, rewrite the body of `resolveWatchList`
  (around `main.go:538`) to short-circuit at the first non-empty
  source instead of concatenating `cliWatches` and `cfgWatches`.
  New behavior: if `len(cliWatches) > 0`, use `cliWatches`
  exclusively; else if `len(cfgWatches) > 0`, use `cfgWatches`
  exclusively; else fall back to `cwd` as today. Keep the
  function signature (`cliWatches, cfgWatches []string`)
  unchanged so `main()` keeps calling it the same way.
- [ ] 1.2 Update the docstring above `resolveWatchList` to describe
  the precedence rule ("the first source that contributes at least
  one entry wins") and to cross-reference
  `selectPromptTemplate` in `config.go` as the matching rule for
  the prompt template. The docstring stays short — one paragraph
  above the function, no per-rule expansion.

## 2. Rewrite tests to pin the new contract

- [ ] 2.1 In `discovery_test.go`, replace
  `TestResolveWatchListUnionsCLIAndConfig` (around
  `discovery_test.go:215`) with
  `TestResolveWatchListCLIR-eplacesConfig`. The new test passes
  one CLI repo and one configured repo and asserts the resolved
  list contains only the CLI repo (not the union).
- [ ] 2.2 In `discovery_test.go`, delete
  `TestResolveWatchListOverlappingSourcesDedupe` (around
  `discovery_test.go:298`). The dedupe-across-CLI-and-config case
  is no longer reachable under the precedence rule: when both
  sides name the same repo, the CLI list wins entirely and the
  configured entry is not consulted. The dedupe behavior of
  `resolveTargets` is still covered by its own tests in
  `discovery_test.go`; the union-and-dedupe interaction is no
  longer a behavior of this function.
- [ ] 2.3 In `discovery_test.go`, leave
  `TestResolveWatchListCLIOnly`,
  `TestResolveWatchListConfigOnly`, and
  `TestResolveWatchListFallsBackToCWD` byte-identical. They
  already pin the three reachable paths (CLI only, config only,
  cwd fallback) under the new rule.

## 3. Verify

- [ ] 3.1 Run `go test -timeout 30s ./...` and confirm every test
  in `main_test.go`, `discovery_test.go`, `config_test.go`, and
  `tui/tui_test.go` passes. The expected test changes are listed
  in tasks 2.1 and 2.2; no other test should need edits.
- [ ] 3.2 Run `go vet ./...` and confirm clean output.
- [ ] 3.3 Run `gofmt -l .` and confirm no files need reformatting.
- [ ] 3.4 Run `openspec validate make-cli-flags-override-config`
  and confirm the change is still valid after the implementation
  edits (the spec delta and the design were validated before the
  code changes; this is a sanity check that nothing drifted).
