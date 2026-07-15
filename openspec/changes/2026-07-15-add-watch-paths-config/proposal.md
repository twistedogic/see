## Why

Today, `see` watches exactly one location: the current working
directory. Each immediate subdirectory that contains a `.git`
folder is treated as a repo to work on. Operators with code in
multiple, scattered locations — a few repos under `~/work`,
one at `/var/repos`, one at `~/scratch/playground` — have to
either run `see` once per parent directory, `cd` into a parent
and back out, or restructure their filesystem to fit the tool.

The change lets `see` resolve its watch list from three layered
sources: a repeatable `--watch` flag, a plain-text config file at
`$XDG_CONFIG_HOME/see/watches`, and the current working directory
as a fallback. Each entry is a path or a shell-style glob (`*`,
`?`, `[abc]`); the watch list is resolved once at startup and
deduplicated.

## What Changes

- Add a `--watch <pattern>` flag, repeatable. Each pattern is a
  path or shell-glob (a shell-style filename pattern using `*`,
  `?`, and `[abc]`). `~` expands to the user's home directory.
  `**` is rejected with a clear error: the project uses stdlib
  `filepath.Match`, which does not support recursive globs, and
  the project chooses not to add a dependency for the feature.
- Add a `--ignore-config` flag that skips the config layer.
  Useful for one-off shell scripting and for testing.
- Read `$XDG_CONFIG_HOME/see/watches` at startup when present.
  The file is plain text: one pattern per line, `#` for
  comments, blank lines ignored. A missing file is not an
  error; a malformed line is fatal with the line number.
- Layer the three sources: `--watch` ∪ config ∪ cwd-fallback,
  deduplicated by absolute path. The current working directory
  is the fallback only when both the flag and the config
  contribute no entries.
- Resolve all entries through a new `discovery` package:
  tilde-expand, glob-expand via `filepath.Match`, then classify
  each match. A resolved path with a `.git` child is a single
  repo. A resolved path without `.git` is treated as a
  parent-of-repos and iterated for `.git` children (the current
  behavior of the cwd path). Paths that miss classification
  (files, missing entries, parents with no `.git` children)
  emit a `Warning` event and are skipped.
- Replace `Watcher.Watch(ctx, wd string)` with
  `Watcher.Watch(ctx, repos []string)`. The watcher becomes
  path-list-driven; `Watcher.runOnce` no longer calls
  `os.ReadDir(wd)`. The rest of the watcher (per-repo processing,
  retry loop, agent invocation) is unchanged.

## Capabilities

### Modified Capabilities

- `watcher`: `Watcher.Watch` now accepts a pre-resolved list
  of repo paths instead of a directory to read.
  `Watcher.runOnce` iterates that list. The discovery layer
  (layered sources, glob expansion, repo-vs-parent
  classification, cwd fallback) is added under a `Discovery`
  section in the watcher spec.

## Impact

- New file: `discovery.go` in the `main` package —
  tilde-expand, glob-expand, classify, dedupe.
- New file: `config.go` in the `main` package — load the
  plain-text `watches` file from `$XDG_CONFIG_HOME/see/watches`
  (defaulting to `~/.config/see/watches`).
- `main.go`: two new flags (`--watch`, `--ignore-config`); the
  block that calls `os.Getwd()` and threads `path` into
  `runTUI` and `w.Watch` is replaced by a call to the new
  resolution layer.
- `eventlog.go`: unchanged. `Warning` events emitted during
  resolution flow through the existing observer path.
- `tui/`: unchanged. The Terminal User Interface (TUI) already
  renders a flat list of repos keyed by path; new sources
  collapse naturally into the same `RepoSeenMsg` shape.
- `go.mod`, `go.sum`: unchanged. No new dependency.
- `main_test.go`: new tests for the config loader, tilde and
  glob expansion, classification, dedupe, and the
  `--ignore-config` short-circuit. Existing watcher tests that
  exercised the `Watcher.Watch(ctx, wd)` signature are updated
  to pass an explicit path list.
- `openspec/specs/watcher/spec.md`: a delta adds new
  requirements under a `Discovery` section; no existing
  requirement is modified or removed.
- Operators who run `see` with no flags and no config see no
  behavioral change: the cwd fallback still drives the watch
  list and produces the same `RepoSeen` events the TUI and
  JavaScript Object Notation Lines (JSONL) already render.
