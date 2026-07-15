## ADDED Requirements

### Requirement: Discovery resolves the watch list from layered sources

`see` SHALL resolve the list of repos to watch from three
sources, in order of decreasing precedence:

1. The repeatable `--watch <pattern>` flag.
2. The contents of the config file at
   `$XDG_CONFIG_HOME/see/watches` (defaulting to
   `~/.config/see/watches` when `XDG_CONFIG_HOME` is
   unset), unless `--ignore-config` is set.
3. The current working directory as a fallback when
   steps 1 and 2 produce no entries.

The flag entries and the config entries are unioned, not
replaced: an operator may pass `--watch` to add a single
repo to whatever the config already lists.

#### Scenario: No flag, no config falls back to cwd

- **WHEN** `see` is invoked with no `--watch`, the config
  file is not present, and the working directory contains
  two git repos as immediate subdirectories
- **THEN** the resolved watch list contains both repos
- **AND** neither the batch-level JavaScript Object
  Notation Lines (JSONL) stream nor the Terminal User
  Interface (TUI) reflects any new source

#### Scenario: Flag adds to config

- **WHEN** `see --watch /extra/repo` is invoked with a
  config file listing `~/work/*`
- **THEN** the resolved watch list contains every match
  of `~/work/*` plus `/extra/repo`
- **AND** duplicates between the two sources collapse to
  a single entry

#### Scenario: --ignore-config skips the config layer

- **WHEN** `see --ignore-config --watch ~/only/repo` is
  invoked with a config file listing `~/other/repo`
- **THEN** the resolved watch list contains only
  `~/only/repo`
- **AND** the config file's contents are not consulted

#### Scenario: Missing config file is not an error

- **WHEN** the config file does not exist
- **THEN** the resolution proceeds with the flag entries
  and the cwd fallback as if the file were empty
- **AND** no error is returned

#### Scenario: Malformed config line is fatal at startup

- **WHEN** the config file contains a line that fails
  read, parse, or tilde expansion
- **THEN** `see` prints a single message naming the
  offending line number and exits with status `2`
- **AND** the watcher does not start

### Requirement: Discovery patterns expand paths and shell-globs

Patterns SHALL expand in two ways:

- A leading `~` in any pattern SHALL be replaced with
  the user's home directory (`$HOME` when set, otherwise
  the result of `os.UserHomeDir()`).
- Shell-glob metacharacters in any pattern (`*`, `?`,
  `[abc]`) SHALL be expanded via `filepath.Match`
  against the filesystem. Patterns with no
  metacharacters SHALL be treated as literal paths.
- Environment-variable expansion (`$VAR`) SHALL NOT be
  performed.
- A pattern containing `**` SHALL be rejected at
  startup with a clear error message, because
  `filepath.Match` does not support recursive globs and
  the project chooses not to add a dependency for the
  feature.

#### Scenario: Literal path

- **WHEN** a pattern is `--watch /repos/myrepo` and the
  path exists
- **THEN** the pattern contributes `/repos/myrepo` to
  the resolved list (subject to dedupe)

#### Scenario: Tilde expansion

- **WHEN** the home directory is `/home/alice` and a
  pattern is `--watch ~/work`
- **THEN** the pattern contributes `/home/alice/work`
  to the resolved list

#### Scenario: Shell-glob expansion

- **WHEN** the home directory is `/home/alice` and a
  pattern is `--watch "~/work/*"` and `~/work` contains
  immediate subdirectories `repoA` and `repoB`
- **THEN** the pattern contributes
  `/home/alice/work/repoA` and `/home/alice/work/repoB`
  to the resolved list

#### Scenario: Glob with no matches emits a Warning

- **WHEN** a pattern is `--watch "~/work/*"` and
  `~/work` contains no immediate subdirectories
- **THEN** a `Warning` event is emitted naming the
  pattern
- **AND** the batch continues with the remaining entries

#### Scenario: **-containing pattern is rejected

- **WHEN** a pattern contains `**`
- **THEN** `see` prints an error explaining that `**`
  is not supported, points at alternatives (multiple
  patterns or `find … -path`), and exits with status
  `2`

### Requirement: Discovery classifies each entry as a repo or a parent-of-repos

Each resolved entry SHALL be classified by stat'ing the
entry itself:

- An entry whose root contains a `.git` file or
  directory SHALL be treated as a single repo.
- An entry whose root is a directory and does not
  contain `.git` SHALL be treated as a parent-of-repos;
  each immediate child with `.git` SHALL be added to
  the resolved list.
- An entry that does not exist, is a regular file, or
  is a parent-of-repos with no `.git` children SHALL
  emit a `Warning` event and be skipped.

#### Scenario: Entry is a single repo

- **WHEN** `--watch /repos/myrepo` resolves to a path
  with `.git/`
- **THEN** the resolved list contains `/repos/myrepo`
- **AND** `Watcher.work` runs against it directly

#### Scenario: Entry is a parent-of-repos

- **WHEN** `--watch /work` resolves to a directory with
  immediate `.git/` children `repoA` and `repoB`
- **THEN** the resolved list contains `/work/repoA` and
  `/work/repoB`
- **AND** `Watcher.work` runs against each

#### Scenario: Entry is a parent with no repo children

- **WHEN** `--watch /work` resolves to a directory with
  no `.git/` children
- **THEN** a `Warning` event is emitted naming the path
- **AND** the batch continues without that entry

#### Scenario: Entry is missing

- **WHEN** `--watch /nonexistent` resolves to a path
  that does not exist
- **THEN** a `Warning` event is emitted naming the path
- **AND** the batch continues without that entry

### Requirement: Discovery dedupes and sorts the watch list

After all entries are resolved and classified, the final
watch list SHALL be deduplicated by absolute path and
sorted in ascending path order. Two entries that resolve
to the same absolute path (for example a literal config
entry and a glob match that includes the same repo)
SHALL appear exactly once.

#### Scenario: Overlapping sources collapse

- **WHEN** the flag list contains `/abs/path/repo` and
  the config contains `~/work/*` where `~/work/repo`
  resolves to the same absolute path
- **THEN** the final watch list contains
  `/abs/path/repo` exactly once

#### Scenario: Stable ordering for the TUI

- **WHEN** the resolved list contains
  `/repos/zeta`, `/repos/alpha`, and `/repos/mu`
- **THEN** the final list is ordered
  `/repos/alpha`, `/repos/mu`, `/repos/zeta`
- **AND** the Terminal User Interface (TUI) renders the
  same order on every scan

### Requirement: Watcher iterates the resolved watch list

`Watcher.Watch` SHALL accept a `repos []string`
argument instead of a `wd string` argument.
`Watcher.runOnce` SHALL iterate `repos` directly,
applying the per-repo processing (retry, agent
invocation, event emission) that it applies today. The
watcher's discovery responsibilities
(`os.ReadDir`, repo classification, deduplication)
SHALL move to the new discovery layer in `main`.

#### Scenario: Watcher sees only what the discovery layer resolved

- **WHEN** `Watcher.Watch(ctx, repos)` is invoked with
  a slice of three absolute paths
- **THEN** `Watcher.runOnce` runs against exactly those
  three paths
- **AND** `Watcher.runOnce` does not call `os.ReadDir`
  on any directory

#### Scenario: Watcher's per-repo contract is unchanged

- **WHEN** `Watcher.Watch(ctx, repos)` is invoked with
  a slice of paths
- **THEN** the existing per-repo requirements (branch
  creation, rollback, merge, retry, agent invocation,
  event emission) still apply to each entry
- **AND** the Terminal User Interface (TUI) and the
  batch-level JSONL event stream observe the same
  events per repo as before the change
