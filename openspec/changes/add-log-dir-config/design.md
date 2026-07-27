## Context

`see`'s log directory today has two properties that are inconsistent
with the rest of its path configuration:

1. **Single source.** `ensureLogDir` (`eventlog.go:18`) consults only
   `SEE_LOG_DIR` and the platform cache directory. Every other
   path-like setting (`root_dir`, `workflows_dir`, `worktree_root`)
   resolves from a config field, with the same tilde-expansion rule.
2. **Raw `os.Getenv`.** `SEE_LOG_DIR` is read without tilde
   expansion, so `SEE_LOG_DIR=~/logs` creates a directory literally
   named `~`. Configured path fields all go through `expandTilde`
   (`config.go:55`), so this is an asymmetry, not a deliberate
   choice.

Meanwhile the codebase has already chosen uniformity over platform
convention for *one* cache-ish directory: `defaultWorktreeRoot`
(`main.go:727`) is the hardcoded literal-tilde constant
`~/.cache/see/worktrees`, not `os.UserCacheDir()/see/worktrees`. So
on macOS, `see` already writes worktrees under `~/.cache/see/` while
writing logs under `~/Library/Caches/see/`. Logs following the same
home-relative constant closes the split and makes `~/.cache/see/`
the single root for `see`'s ephemeral artifacts:

```
~/.cache/see/
├── logs/        ← new default (was os.UserCacheDir()/see/logs)
└── worktrees/   ← existing defaultWorktreeRoot
```

The change proposed here is small and local: add the config field,
make the default match `worktree_root`'s shape, and route every
source through `expandTilde`. It does not alter branch/worktree
mode, the JSONL line envelope, the per-invocation filename format,
or the `LogPath` event contract — all of those operate on a
directory that is already resolved.

## Why `SEE_LOG_DIR` wins over the config field

There are two defensible precedence orders once a config field
exists:

```
A. SEE_LOG_DIR > log_dir config > default   (env is the override)
B. log_dir config > SEE_LOG_DIR > default   (config is authoritative)
```

The rest of the codebase uses **CLI flag > config field > default**
for layered knobs (`--prompt`, `--worktree`, `--worktree-root`).
`SEE_LOG_DIR` is an *environment variable*, and there is no
`--log-dir` flag, so neither maps cleanly onto the existing
precedence ladder.

The deciding factor: `SEE_LOG_DIR` is the **existing, documented**
override. Operators who set it (CI hosts, systemd units, shared
dotfiles) have a reasonable expectation that it keeps winning. A
config field that silently shadowed a forgotten `SEE_LOG_DIR`
exported in a shell rc file would be a debugging trap. **Option A**
preserves that expectation and makes the config field the common,
non-surprising path. It is also the smaller rule to explain: "env
overrides config overrides default," one sentence.

Adding a `--log-dir` flag for full flag > env > config > default
consistency was considered and rejected (YAGNI): there is no
`--root-dir` or `--workflows-dir` flag either, so a `--log-dir`
flag would be inconsistent the *other* way. The environment variable
is the CLI-side escape hatch, consistent with how `see` treats its
other non-flag overrides.

## The `main()` reorder

This is the part of the change that touches the most lines, and it
is purely mechanical. Today (`main.go` ~1045-1075):

```
ensureLogDir()          ← resolves dir from env only
openEventLogger(dir)
NewWatcher(pi, dir, ...)
...
loadStartupConfig()     ← config loads AFTER the dir is fixed
```

A config-driven `log_dir` is impossible in this order: the directory
is resolved and the event log file is opened before the config is
parsed. The fix is to load config first, then resolve the directory
from it:

```
loadStartupConfig()              ← moved up
resolveLogDir(cfg)               ← was ensureLogDir()
openEventLogger(dir)
NewWatcher(pi, dir, ...)
```

The reorder is safe because nothing between the two calls today uses
the loaded `cfg`: the `mode`/`term.IsTerminal` check, the
`--interval` guard, and `flag.Parse` all run on CLI inputs only, and
the config-dependent steps (`validateWorkflows`,
`resolveLaneIsolation`, `resolveConfiguredTargets`,
`SetPromptTemplate`) all already come *after* `loadStartupConfig`.
Moving config loading up by ~15 lines changes no behavior other than
making `cfg` available to `resolveLogDir`.

`NewWatcher`'s signature is unchanged: it already takes
`logDir string`, and the resolved directory is still a string by the
time it is passed in. `PiAgent.Run` is unchanged: it still receives
a directory that is guaranteed to exist.

## Why fix the tilde bug in the same change

`SEE_LOG_DIR=~/x` creating a literal `~` directory is a latent bug
that has not bitten anyone only because operators who set the
variable tend to use absolute paths. Folding the fix in is free once
`resolveLogDir` routes every source through `expandTilde` — the
alternative is to special-case the env var to stay raw, which
preserves an asymmetry for no benefit. The risk is negligible: a
literal directory named `~` is never what anyone wants, and an
operator relying on that behavior is depending on a bug.

## Why no `log_dir` validation beyond tilde expansion

`root_dir` is stat-checked in `validateConfig` and must *already
exist* as a directory, because it points at a repo root the operator
already has. The log directory is different: it is *created if
absent* (`MkdirAll`), which is the point of a logs directory — an
operator should be able to point `log_dir` at a path that does not
exist yet and have `see` make it. So the only config-time work is
tilde expansion (for consistency with the other path fields); the
existence/writability verdict is deferred to `MkdirAll`, whose
failure is already a fatal startup error today. No new validation
rule is added.

## Alternatives considered

- **Keep `os.UserCacheDir()` as the default, only add the config
  field.** Rejected: it leaves the macOS/Linux split and the
  inconsistency with `worktree_root`'s hardcoded `~/.cache/see/`
  default. The default change is the smaller, more consistent end
  state.
- **Option B precedence (config > env).** Rejected per the section
  above; it shadows a documented override.
- **Add a `--log-dir` flag.** Rejected (YAGNI); inconsistent with the
  absence of `--root-dir` / `--workflows-dir`.
- **Auto-migrate old logs from `~/Library/Caches/see/logs` on
  macOS.** Rejected; JSONL is observability, not data, and a
  best-effort copy adds failure modes for no durable value. A
  migration note in the proposal and `AGENTS.md` is sufficient.
