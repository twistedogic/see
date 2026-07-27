## Why

`see` writes two kinds of logs — the batch-level JavaScript Object
Notation Lines (JSONL) event stream and one per-invocation agent
log — to a directory resolved from a single source: the `SEE_LOG_DIR`
environment variable, falling back to `os.UserCacheDir()/see/logs/`.
There is no way to set the location from `config.yaml`, so it is the
odd one out among path-like settings: `root_dir`, `workflows_dir`,
`worktree_root`, and (at the default) the worktree root are all
configurable and tilde-expanded, while the log directory is
environment-only and raw `os.Getenv` (so `SEE_LOG_DIR=~/x` silently
creates a literal directory named `~`).

Two things make this worth fixing now:

1. **Consistency.** `worktree_root` already defaults to
   `~/.cache/see/worktrees` as a *hardcoded* literal-tilde constant,
   not `os.UserCacheDir()/see/worktrees`. The codebase has therefore
   already voted for a uniform home-relative cache root over the
   platform cache directory. The log directory following the same
   pattern makes `~/.cache/see/` the single home for `see`'s
   cache-ish artifacts (`logs/` next to `worktrees/`) instead of
   splitting logs into `~/Library/Caches/see/logs` on macOS and
   `~/.cache/see/logs` on Linux.
2. **Discoverability.** Operators who already edit `config.yaml` to
   point `root_dir` and `workflows_dir` at their layout expect the
   log directory to live in the same file. The environment variable
   remains the override, but the common case gets a config field.

## What Changes

- Add a top-level `log_dir` string field to the global configuration
  schema, snake_case to match `root_dir`, `workflows_dir`, and
  `worktree_root`.
- Change the default log directory from
  `os.UserCacheDir()/see/logs/` to `~/.cache/see/logs`, the same
  hardcoded home-relative constant shape already used by
  `defaultWorktreeRoot`.
- Resolve the log directory with one precedence rule:
  `SEE_LOG_DIR` (non-empty) > `log_dir` config (non-blank) >
  `~/.cache/see/logs`. The environment variable stays the hard
  override so existing operators with it set keep getting what they
  expect; the config field is the new common path.
- Route **all three** sources through `expandTilde`, fixing the
  latent literal-`~` bug where `SEE_LOG_DIR=~/x` creates a directory
  literally named `~`. The config field gets the same tilde rule as
  `root_dir`.
- Reorder `main()` so `loadStartupConfig` runs **before** log
  directory resolution and event log opening. Today `ensureLogDir`
  runs first; a config-driven `log_dir` is impossible without the
  flip. The flip is mechanical: nothing between the two calls today
  uses the loaded config.
- Keep create-if-absent behavior (`MkdirAll` mode `0o755`) and fatal
  startup failure on an uncreatable directory, identical to today.
  No new validation rule beyond "tilde-expand the configured value
  in `validateConfig`, like the other path fields".
- No public surface changes outside the new config field. No new CLI
  flag (there is no `--root-dir` or `--workflows-dir` either; the
  environment variable remains the CLI-side escape hatch).

**MIGRATION** (macOS-visible): logs move from
`~/Library/Caches/see/logs/` to `~/.cache/see/logs/`. Linux already
lands in `~/.cache/see/logs` via the XDG Base Directory Specification,
so nothing moves there. Old logs are not touched (just orphaned) —
no auto-migration code, since JSONL is observability, not data.
Operators with `SEE_LOG_DIR` set are unaffected.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `watcher`: the "Default log location is the OS cache directory"
  requirement is replaced by "Log directory resolves from env,
  config, or a default". The two existing scenarios (no-env-default,
  env-overrides) are rewritten and six new scenarios are added
  covering the config field, env-over-config precedence, tilde
  expansion in each source, the `--config=-` skip, directory
  creation, and the fatal uncreatable-directory startup error. The
  "PiAgent writes agent output to a JSONL file per run" requirement
  is unchanged in prose (the directory is still guaranteed to exist
  before `Run`); only the resolution source and default change.

## Impact

- **Code**: ~30 lines across three files.
  - `config.go`: add `LogDir string \`yaml:"log_dir"\`` to `Config`;
    add a tilde-expand + whitespace-trim branch in `validateConfig`
    alongside the `WorkflowsDir` handling; add a `defaultLogDir`
    constant mirroring `defaultWorktreeRoot`.
  - `eventlog.go`: replace `ensureLogDir()` (env-only, no tilde)
    with `resolveLogDir(cfg Config) (string, error)` that applies
    the precedence ladder and `expandTilde`s every source before
    `MkdirAll`.
  - `main.go`: move the `loadStartupConfig(*configFlag)` call above
    `resolveLogDir(cfg)` / `openEventLogger`; pass `cfg` into the
    resolver. `NewWatcher`'s signature is unchanged (it already
    takes `logDir string`).
- **Specs**: one `MODIFIED` requirement on `watcher`; two scenarios
  rewritten, six added.
- **Dependencies**: zero added, zero removed.
- **Behavior**: on macOS the default log directory moves from
  `~/Library/Caches/see/logs/` to `~/.cache/see/logs/`; on Linux no
  change. `SEE_LOG_DIR=~/x` now expands the tilde instead of making
  a literal `~` directory. A configured `log_dir` takes effect.
- **Risk**: low. The precedence rule is local and the new default is
  already the pattern used by `worktree_root`. The one behavioral
  surprise — logs relocating on macOS — is captured by a migration
  note and gated by the spec delta and new tests at every layer.
  The `main()` reorder touches no logic between the two moved calls.
