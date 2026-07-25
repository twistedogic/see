## Context

`see` reads a single YAML configuration file and a single repeatable CLI flag
to determine which git repositories to watch. Today the configuration field is
`watches: [path/glob, ...]` and the CLI flag is `--watch`. Both sources feed
`resolveWatchList`, which unions them under a precedence rule
(`--watch` replaces `watches` replaces `cwd`).

Three operator frictions drive this change:

- The "watch everything in `~/Dev/` except `bin`" case requires the operator to
  repeat `~/Dev/` in every entry and rely on shell-glob semantics for
  negation, which `filepath.Match` cannot express.
- Pattern validity errors (e.g. `**`, malformed brackets, an expansion that
  fails because `HOME` is unset) only surface during the first scan, after
  the watcher has started.
- The CLI and config carry the same shape, so the precedence rule exists
  twice in code and once in the spec. Removing one source simplifies the
  precedence story and removes a class of "which one wins" bugs.

## Goals / Non-Goals

**Goals**

- Express the common watch cases (everything, a single child, a wildcard
  subset, exclusions) directly in the configuration file.
- Make pattern-validity failures fatal at config-load time (exit `2` before
  the watcher starts), with errors scoped to the offending field.
- Keep `prompt`, `condition`, `commit`, the strict YAML decoder, the
  current-working-directory fallback, the one-level parent-of-repos descent,
  dedupe, and sort behavior unchanged.
- Stay within `filepath.Match` / `filepath.Glob` — no new dependencies.

**Non-Goals**

- Recursive (`**`) globs. `filepath.Match` does not support them and the
  project does not add a dependency.
- Backwards compatibility with the old `watches` field. Operators edit their
  config; the loader rejects `watches` as an unknown field.
- Additive CLI overrides (e.g. `--include` / `--exclude` flags). The config is
  the single source of truth.
- A migration period or dual-write of the old and new schemas.

## Decisions

### 1. Three new fields replace `watches`

`Config` gains `RootDir string`, `Include []string`, `Exclude []string`. The
`Watches` field is removed. The strict YAML decoder (`KnownFields(true)`)
turns the old `watches:` line in any existing config into a fatal
"unknown field" error, which is the desired clean cut.

Rationale: a single structured schema is the simplest expression of the
operator's intent. Keeping `watches` around as a deprecated alias would
double the maintenance surface for one transitional release.

### 2. Validation moves into `loadConfig`

A new private `validateConfig(cfg *Config) error` runs after the YAML decode
and before `loadConfig` returns. It runs three checks:

1. **`root_dir`** — if nonblank: tilde-expand via the existing `expandTilde`,
   `os.Stat` the result, require it to be a directory. Stash the expanded
   path back into `cfg.RootDir` so the resolver does not re-expand.
2. **`include`** and **`exclude`** — for each entry: reject `**` via the same
   string check `resolveTargets` uses today, probe `filepath.Match(entry,
   "test")` to catch `ErrBadPattern` (malformed bracket expressions), then
   tilde-expand and stash the expanded entry back into the slice. The probe
   matches against a throwaway `"test"` string so the syntax check fires
   without consulting the filesystem.
3. **`custom condition` validation** — call the existing
   `validateCustomConfig(cfg, cliPrompt)` with an empty `cliPrompt` (the CLI
   no longer has any watch input, but `--prompt` still exists and is passed
   in by `main` at a different call site; `loadConfig` only needs to check
   the in-config fields).

Errors include the field path so operators can fix without grepping:
`include[2]: '**' is not supported`, `root_dir "/nope": no such file or
directory`, `exclude[0]: invalid glob pattern: ...`. The existing
`fmt.Errorf("see: ...: %w", err)` wrap preserves the package convention.

`expandTilde` itself stays unchanged. We just call it earlier and surface its
errors through the same wrapping convention.

Rationale: shifting these checks from scan time to load time turns silent
"first scan discovered a bad pattern" warnings into an immediate exit `2`.
The cost is one extra `os.Stat` per config load, which is negligible.

### 3. Resolver is a single new function

`resolveConfiguredTargets(cfg Config) ([]string, []Warning, error)` replaces
both `resolveWatchList` and `resolveTargets`. Its flow:

```
1. If cfg.RootDir == "":
       return [os.Getwd()], nil, nil   // cwd fallback
2. If cfg.Include is empty:
       candidates = directories in cfg.RootDir
   Else:
       candidates = ⋃ filepath.Glob(filepath.Join(cfg.RootDir, p))
                    for p in cfg.Include
3. If cfg.Exclude is non-empty:
       candidates = candidates filtered to those where
                    no p in cfg.Exclude satisfies
                    filepath.Match(p, filepath.Base(c))
4. For each c: classifyTarget(c)         // unchanged
5. dedupeAndSort(candidates)             // unchanged
```

Step 2's two branches share a common output: an unsorted list of absolute
paths. Step 3's exclude filter matches against `filepath.Base(c)` so an
operator writing `exclude: [bin, playground*]` does not need to anchor the
pattern to `root_dir`. Step 4 and 5 are byte-for-byte the existing logic.

`classifyTarget` and `dedupeAndSort` stay where they are in `discovery.go`
and are not modified. `resolveTargets` is deleted.

Rationale: the resolver is small enough to live next to `classifyTarget` and
the directory helpers, and `discovery.go` already owns the
"filesystem → repository list" responsibility.

### 4. `--watch` flag and `multiFlag` are deleted

`main.go` drops the `watchFlag multiFlag` declaration, the
`flag.Var(&watchFlag, "watch", ...)` registration, and the `cliWatches`
argument from `resolveWatchList`. `multiFlag` and its `String` / `Set`
methods are deleted. `--prompt`, `--config`, `--interval`, `--retry`,
`--mode`, `--once` are unchanged.

Operators who today pass `--watch <path>` on the command line will see the
standard flag-package error (`flag provided but not defined: -watch`) on
startup. That is loud and obvious, which is the right behavior for a removed
flag.

Rationale: the flag duplicate of the config is the main thing the change is
trying to remove. Keeping it would re-introduce the two-source precedence
problem the change exists to dissolve.

### 5. Configuration template is updated, not regenerated

`config.example.yaml` keeps its structure (YAML comments + a header that
decodes to a zero-value `Config`) but the commented examples now show
`root_dir`, `include`, and `exclude` instead of `watches`. The
`//go:embed` directive in `config.go` is unchanged.

Rationale: the embedded template is the discoverable artifact operators see
on first run; showing the new schema in the template is the cheapest possible
documentation update.

## Risks / Trade-offs

- **Hard-failing on `root_dir` existence** → operator with a network-mounted
  root that is briefly unavailable gets a fatal error instead of a graceful
  retry. Acceptable: the operator can mount the volume before starting
  `see`, and "directory does not exist" is almost always a typo at config
  time.
- **Empty `include` means "every child"** → an operator who writes
  `include: []` to mean "include nothing" gets the opposite of what they
  wrote. Mitigation: the docs and the embedded template make "leave the
  sequence unset or empty to include every child" explicit; if a future
  operator hits this foot-gun we can add a startup warning when `include` is
  present-but-empty.
- **Exclude matches basename only** → an operator who writes
  `exclude: [work/bin]` expecting it to drop `~/Dev/work/bin` will be
  surprised when it matches nothing. Mitigation: the spec and the embedded
  template call out the basename semantics explicitly.
- **Test churn is large** → `config_test.go`, `discovery_test.go`, and
  `main_test.go` all reference `watches` or `--watch` fixtures and
  invocations. Mechanical, but easy to miss a stray reference. Mitigation:
  the implementation tasks include a `grep` check before the change is
  considered done.
- **Existing operator configs break loudly** → the strict decoder makes
  this unavoidable. Mitigation: the error message names the offending field
  (`field watches not declared in struct`), and the change ships the same
  release as any operator-visible docs update.

## Migration Plan

This is a single-release breaking change. There is no multi-step migration:

1. Land the code, template, spec, and tests together.
2. Operators update `config.yaml`: rewrite `watches: [...]` as `root_dir:`
   plus optional `include` / `exclude`. The loader's error message names
   `watches` so the change is grep-able.
3. Operators who relied on `--watch` remove it from their shell aliases or
   scripts. The standard `flag` error makes this obvious on first run.

Rollback is `git revert`. The change is a clean cutover of one config field
plus a flag removal; no data migration, no dual-write, no state to unwind.

## Open Questions

None blocking. The user-facing decisions (root_dir / include / exclude,
validate at load, remove `--watch`) were settled during exploration. The
only minor design call not explicitly confirmed is **whether to hard-fail at
load when `root_dir` is set but does not exist** (the design above assumes
yes); that is recorded as a risk above and can flip to a soft check without
changing the spec, so it is not worth a separate question.