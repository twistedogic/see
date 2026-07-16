## Why

`see`'s flag and config precedence is currently inconsistent: the prompt
template already follows "command-line > config > embedded default"
(`selectPromptTemplate`, `config.go:121`), but the watch list does not.
`--watch` is *unioned* with the `watches` sequence in `config.yaml`
(`resolveWatchList`, `main.go:538`), so passing one `--watch` does not
replace the configured watches — it adds to them. The result is two
different mental models for two related configuration knobs and an
introspectable mismatch between spec prose ("flag entries and
configured entries SHALL be unioned") and the project's stated
guideline that flags override defaults. Aligning the two makes the
precedence rule one consistent rule across the whole CLI surface, and
keeps `--ignore-config` for the one case the override model does not
cover (a malformed config that should not block startup).

## What Changes

- Change `resolveWatchList` so that **non-empty** `--watch` flags
  **replace** the configured `watches` instead of unioning with them.
  The CLI list becomes the only watch source when present. When no
  `--watch` is passed, the configured `watches` still apply. When
  both are empty, the current-working-directory fallback still applies.
  **BREAKING** for any operator that relied on `--watch` adding to a
  non-empty configured list — they now have to spell out the full set
  on the command line when they want to extend it.
- Mirror the prompt precedence in the spec: one rule
  (CLI > config > default) for every layered knob. The watcher spec's
  "Discovery resolves the watch list from layered sources" requirement
  is rewritten to describe precedence instead of union; the
  "Flag adds to config" scenario is removed; a new "Flag replaces config"
  scenario takes its place.
- `--ignore-config` keeps its current meaning (skip the config file
  entirely, so a malformed `config.yaml` does not block startup). It
  is the escape hatch for the case the precedence model does not
  cover, not a synonym for "use the command line only."
- No public surface changes outside the precedence rule. `multiFlag`,
  `--watch`, `--prompt`, `--ignore-config`, `config.yaml`, the
  embedded `prompt.md` default, and `resolveWatchList`'s signature
  stay byte-identical except for the union → replace step inside
  `resolveWatchList`.

## Capabilities

### New Capabilities

None. This change does not introduce any new spec.

### Modified Capabilities

- `watcher`: the "Discovery resolves the watch list from layered
  sources" requirement changes from union semantics to CLI > config >
  default precedence semantics. The "Flag adds to config" scenario is
  removed; a "Flag replaces config" scenario is added. The
  "--ignore-config skips the config layer" scenario stays (it already
  asserts the post-change behavior). All other scenarios under this
  requirement stay byte-identical.

## Impact

- **Code**: ~5 lines change inside `resolveWatchList`
  (`main.go:538-552`); the function becomes a precedence ladder
  (CLI → config → cwd) instead of a concatenation. The
  `TestResolveWatchListUnionsCLIAndConfig` and
  `TestResolveWatchListOverlappingSourcesDedupe` tests in
  `discovery_test.go` are rewritten to assert the replace semantics;
  the other three `resolveWatchList` tests stay byte-identical.
- **Specs**: one `MODIFIED` requirement on the `watcher` capability;
  one scenario removed, one scenario added.
- **Dependencies**: zero added, zero removed.
- **Behavior**: the watch-list resolution rule changes from "union"
  to "replace". An operator that previously passed `--watch /extra`
  on top of a non-empty `watches:` to extend their list now has to
  pass the full set on the command line. An operator that only ever
  passes `--watch` without a `watches:` config, or only ever relies
  on the configured `watches:`, sees no change.
- **Risk**: medium. The behavioral change is local but real; any
  user that depends on the union path will see their watch list
  shrink after upgrading. The change is gated by a spec delta and
  two rewritten tests so the new contract is pinned at every layer.
