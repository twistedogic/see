## Context

`see` is a batch driver: it walks a directory of git repos,
hands the first active openspec change in each repo to the `pi`
agent, retries on failure, and merges the result back. Today,
the only directory it walks is `os.Getwd()`. Operationally
this is fine for one workspace tree but a chore when repos
live in different places: the operator has to `cd` into each
parent directory in turn or restructure the filesystem.

The change introduces a discovery layer that resolves the
watch list from one of three sources, in order of decreasing
precedence: a repeatable `--watch` flag, a plain-text config
file at `$XDG_CONFIG_HOME/see/watches` (defaulting to
`~/.config/see/watches`), and `os.Getwd()` as a fallback. Each
entry is a tilde-expanded path or a shell-style glob. The
watcher itself becomes path-list-driven: it receives a
deduplicated, sorted slice of absolute paths and iterates it.
The batch driver no longer cares where each repo's path came
from.

## Goals / Non-Goals

**Goals:**

- Operators can list repos to watch in
  `$XDG_CONFIG_HOME/see/watches` once and forget about cwd.
- Operators can override or augment the config on the command
  line via a repeatable `--watch`.
- Patterns accept literal paths and shell-globs (`*`, `?`,
  `[abc]`); tilde (`~`) expands to the user's home directory.
- Each resolved entry auto-classifies as a repo (a `.git`
  child exists) or a parent-of-repos (no `.git` child at the
  entry; iterate `.git` children). The classification is
  automatic; no `type:` field, no per-entry knob in v1.
- Unresolvable entries (a path that does not exist, a glob
  that matches nothing, a parent with no `.git` children)
  emit a `Warning` event and are skipped. The batch continues.
- The current `--once` semantics, retry semantics, agent
  invocation, TUI rendering, and event streams are unchanged.

**Non-Goals:**

- Recursive-glob support (`**`). The cheapest implementation
  needs a dependency; the value is small. Operators who need
  recursive discovery can list multiple glob patterns or
  pre-process with `find`. The boundary is documented in
  the error message and the help text.
- Per-target knobs in the config (per-target retry count,
  per-target agent binary). The config is a flat list in v1.
  When a real second dimension appears, promote the file to
  a richer format.
- Per-repo or per-directory `.see.yaml` discovery. The
  author explicitly chose a single global config; walk-up
  discovery adds scope without v1 value.
- A new environment variable (e.g. `SEE_WATCHES`). Flags and
  the config file are sufficient.
- Editing or validating the config file in `see`. Operators
  edit the file with their editor; `see` reads it.
- A new TUI shape. The TUI already renders a flat list of
  repos keyed by path; new sources collapse naturally and
  need no schema changes.
- Re-expanding globs mid-run. Globs expand once at startup.
  New repos created under an already-resolved parent appear
  on the next scan (existing behavior). New repos created
  under a directory that was only matched via glob are not
  seen until the next `see` run.

## Decisions

### Decision: Three-layer source with cwd as fallback

Precedence, high to low:

1. `--watch <pattern>` (repeatable, additive within itself).
2. `--watch` entries ∪ contents of
   `$XDG_CONFIG_HOME/see/watches`, unless `--ignore-config`
   is set.
3. `os.Getwd()` as a fallback when steps 1 and 2 produced no
   entries.

Rationale: the cwd fallback preserves backwards compatibility
for operators who have not configured anything: running `see`
from a parent directory of repos keeps working.

Alternatives considered:

- *Replace cwd with an empty watch list when step 2 is empty.*
  Rejected: breaks every existing operator's workflow for no
  observable benefit.
- *Replace, not union, the config with the flag.* Rejected:
  `--ignore-config` already covers the "I want to ignore
  everything in the config and use only my flag" intent;
  making `--watch` destructive would surprise operators who
  routinely mix the two.

### Decision: Plain-text config, no YAML, no TOML

File: `$XDG_CONFIG_HOME/see/watches`, defaulting to
`~/.config/see/watches` when `XDG_CONFIG_HOME` is unset.

Format:

- One pattern per line.
- Lines starting with `#` are comments.
- Blank lines and whitespace-only lines are skipped.
- Tilde (`~`) expands to the user's home directory at
  resolve time. Environment-variable expansion (`$VAR`) is
  not performed.
- The file is otherwise opaque to `see`: no `type: repo`
  field, no per-line metadata.

Rationale: zero dependencies. The config holds one thing
(a flat list of patterns). Richer structure is speculative;
if the schema needs to grow, promote the file to TOML or
YAML at that time, not before.

Alternatives considered:

- *TOML via `github.com/pelletier/go-toml/v2`.* Rejected: the
  config holds a single-section flat list and the
  dependency is not justified for v1.
- *Walk-up `.see.yaml` discovery.* Rejected by the author.
  Keep the surface single and global.

### Decision: Stdlib `filepath.Match`, no `**`

Patterns use Go's `filepath.Match`:

- `*` matches any sequence of non-`/` characters.
- `?` matches any single non-`/` character.
- `[abc]` matches any character in the set.
- `**` is not a special token; it is two literal `*`
  characters and matches nothing useful at the filesystem
  level.

A pattern containing `**` is rejected at startup with:

```
see: '**' is not supported in watch patterns (no recursive
glob); list the paths you care about explicitly.
```

Rationale: `filepath.Match` is stdlib; recursive glob is not.
Adding a dependency for one feature, when the author
explicitly chose not to, is the wrong trade. The error
message is the explicit boundary.

Alternative considered: pre-process patterns with
`find … -name`. Documented as a workaround in `--help`.

### Decision: Auto-classify via `.git`, no explicit field

Each resolved entry is classified with one stat:

- `filepath.Stat(filepath.Join(p, ".git"))` returns nil →
  treat the entry as a repo, run the agent on it directly.
- Stat returns an error and `p` is a regular file or
  missing → emit `Warning{Path: p, Msg: ...}` and skip.
- Stat returns an error and `p` is a directory without a
  `.git` child → treat the entry as a parent-of-repos,
  iterate its immediate children for `.git`. If no `.git`
  children exist, emit `Warning{Path: p, Msg: ...}` and
  skip.

Rationale: every reasonable entry fits one of the three
buckets. An explicit `type:` field would let operators
declare a parent-only entry that contains no `.git`
children yet, but that benefit does not justify the knob
or the schema documentation.

### Decision: Dedupe and sort by absolute path

After tilde and glob expansion, every entry is converted to
an absolute path via `filepath.Abs`. The final watch list
is sorted and deduplicated by absolute path. Two entries
that resolve to the same path (for example `--watch
~/work/foo` and a config line containing `/abs/path/foo`)
are collapsed to one repo.

Rationale: predictable order in the JSONL and the TUI.
Without dedupe, the same repo would be hit twice per scan.

### Decision: Watcher API takes a path list

`Watcher.Watch(ctx, repos []string)` replaces
`Watcher.Watch(ctx, wd string)`. `Watcher.runOnce` iterates
the slice and no longer calls `os.ReadDir(wd)`. The per-repo
processing (retry, agent invocation, event emission) is
otherwise unchanged.

Rationale: the discovery logic is otherwise impossible to
unit-test without spinning up a temporary parent directory
of repos per test. Putting the list at the watcher
boundary also keeps the watcher honest about its contract:
it acts on a flat list of repo paths and does not know
how they were chosen.

Alternative considered: keep `Watcher.Watch(ctx, wd)` and
inject the resolver as an interface. Rejected: an
interface with one implementation is overhead for no
benefit; the list-injection form is strictly simpler.

### Decision: Warning, not error, for unresolvable entries

A `--watch /nonexistent`, a glob that matches nothing, a
parent directory with no `.git` children — each emits a
single `Warning` event and is skipped. The batch proceeds
to the next resolved entry. `see` exits `0` on `--once`,
continues watching otherwise.

Rationale: silent skip is hostile to operators debugging a
typo. A `Warning` event surfaces the problem in the
batch-level JSONL and in the TUI as the existing `⚠` glyph,
without aborting the batch.

A malformed config file (a line that fails to expand, an
OS read error, a permission error) is different: it is a
startup-time failure. `main()` prints a message to stderr
naming the offending line number and exits `2`.

## Risks / Trade-offs

- **Operators expect `**` recursive discovery.** Mitigated
  by the explicit error at startup and a one-line `--help`
  note. Document the limit; users who want recursion can
  fan out their config or pre-process with `find`.
- **Operators expect walk-up `.see.yaml` discovery.**
  Documented as out of scope. Walk-up adds a discovery
  algorithm to test and a per-repo state model; a single
  global config is the v1 surface.
- **New repos created under a glob are not seen mid-run.**
  The batch driver re-scans its already-resolved repo list
  each pass; directories the glob matched at startup
  continue to be scanned. Directories the glob would have
  matched later are not added; restart `see` to see them.
  Matches the "globs expand once" principle.
- **The config file lives outside the repo.** An operator
  cloning a fresh checkout forgets to also set up
  `$XDG_CONFIG_HOME/see/watches`. The cwd fallback covers
  the "I forgot" case for the most common workflow (a
  tree of repos under one parent).
- **Shell expansion of `--watch` patterns.** An operator
  who runs `see --watch "~/work/*"` from a shell with
  globstar enabled may see glob expansion happen in the
  shell before `see` parses the flag. This is shell
  behaviour, not `see` behaviour. `--help` should warn
  operators to quote patterns containing `*` if their
  shell expands them.
- **Hard-coded `~/.config/see/watches` location.** Mostly
  portable on Linux and macOS; Windows resolves to
  `%AppData%`. `os.UserConfigDir` covers both; no per-OS
  paths in code.

## Migration Plan

The change is purely additive.

- Existing operators who run `see` from a directory of
  repos see no behavioral change: the cwd fallback drives
  the watch list as before.
- Operators who want the new behavior add patterns to
  `$XDG_CONFIG_HOME/see/watches` (or pass `--watch` on
  the command line) and `see` reads them on the next run.
- No version bump. No data migration. No breaking change
  in the TUI, the JSONL shape, or the watcher contract
  beyond the function signature change.

Rollback: revert the commit. The config file at
`$XDG_CONFIG_HOME/see/watches` is plain text; its
existence becomes a no-op once `see` is rolled back (the
parser no longer runs).

## Open Questions

- Should a `--config` flag allow picking a non-default
  config file path (for testing or for running `see`
  against a per-project config checked into the repo)?
  Out of scope for v1; can be added without breaking
  anything if it is wanted later.
- Should pattern lines in the config support `\`-escapes
  (spaces, `~` as literal)? Stdlib glob does not, and v1
  does not need to; revisit if a real use case surfaces.
- Should `see` print a summary at startup ("watching N
  repos from K sources: …") for inspectability? Useful
  for debugging config typos. Cheap to add; can be folded
  into a `RepoSeen` event with a special `IsSummary`
  field, or a new `Started` event. Defer until asked.
