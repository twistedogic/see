## 1. Config loader (`config.go`)

- [ ] 1.1 Add `loadWatchConfig() ([]string, error)` in
  `config.go`. Locate the file at
  `os.UserConfigDir()/see/watches`. A missing file
  returns `(nil, nil)` (cwd fallback covers it). A
  present file with an unreadable or malformed line
  returns an error carrying the line number.
- [ ] 1.2 Implement tilde expansion for each non-comment
  line: replace a leading `~` with `$HOME` (or
  `os.UserHomeDir()` when `$HOME` is unset). Do not
  expand `$VAR` patterns.
- [ ] 1.3 Add a test that writes a known config to a
  temp directory, overrides `os.UserConfigDir` (for
  example by setting `XDG_CONFIG_HOME` for the test),
  and asserts the returned slice (comments stripped,
  blanks stripped, tilde expanded) matches
  expectations.
- [ ] 1.4 Add a test that asserts a malformed line
  returns an error carrying the line number.
- [ ] 1.5 Add a test that asserts a missing config file
  returns `(nil, nil)`.

## 2. Discovery layer (`discovery.go`)

- [ ] 2.1 Implement `resolveTargets(patterns []string)
  (repos []string, warnings []Warning, err error)`:
  - Tilde-expand each pattern first.
  - Glob-expand via `filepath.Match` for patterns
    containing `*`, `?`, or `[…]`. Treat patterns with
    no metacharacters as literal paths.
  - Reject patterns containing `**` at startup with a
    clear error message.
  - For each resolved match: classify via
    `filepath.Stat(filepath.Join(p, ".git"))`. Repo with
    `.git` → single entry. Directory without `.git` →
    iterate immediate `.git` children. Anything else →
    emit a `Warning` and skip.
- [ ] 2.2 Implement `dedupeAndSort(paths []string)
  []string`: convert each to absolute via
  `filepath.Abs`, sort, dedupe.
- [ ] 2.3 Add a test that asserts a `--watch
  "~/work/*"`-style pattern expands to the expected
  matches against a temp directory tree.
- [ ] 2.4 Add a test that asserts a glob with no matches
  emits a `Warning` and is skipped without aborting
  the batch.
- [ ] 2.5 Add a test that asserts a `**`-containing
  pattern is rejected with an error carrying the
  offending pattern.
- [ ] 2.6 Add a test that asserts dedupe collapses two
  sources pointing at the same absolute path into one
  entry.

## 3. Watcher API change (`main.go`)

- [ ] 3.1 Change `Watcher.Watch(ctx, wd string)` to
  `Watcher.Watch(ctx, repos []string)`.
- [ ] 3.2 In `Watcher.runOnce`, replace the
  `os.ReadDir(wd)` loop with a `for _, repo := range
  repos` loop. The body (skip-on-missing-`.git`,
  `GetCurrentCommit`, `runWithRetry`) is unchanged in
  shape; the loop now operates on a fixed list.
- [ ] 3.3 Remove the `os` import or the readdir
  call site if no other uses remain.

## 4. main wiring

- [ ] 4.1 Add two new flags: `--watch` (a
  `flag.Var`-driven string slice via a small
  `multiFlag` helper type) and `--ignore-config`
  (bool).
- [ ] 4.2 Replace the `os.Getwd()` block in `main()`
  with the new resolution: load the watch config
  (unless `--ignore-config` is set), union with
  `--watch` entries, fall back to `os.Getwd()` if both
  are empty, dedupe, sort, resolve through
  `discovery.resolveTargets`.
- [ ] 4.3 Plumb the resolved repo slice into
  `runTUI(ctx, &w, events, repos)` and `w.Watch(ctx,
  repos)`.
- [ ] 4.4 When the resolution layer emits warnings,
  forward them through the existing `eventLogger` so
  they land in the batch-level JSONL and (in TUI mode)
  in the bubbletea grid as `⚠` glyphs.
- [ ] 4.5 Update `--help` text via the new flags'
  `Usage` strings: include the pattern syntax
  (path or shell-glob, with the `**` caveat) and a
  one-line pointer to
  `$XDG_CONFIG_HOME/see/watches`.

## 5. Test updates (`main_test.go`)

- [ ] 5.1 Add a test that creates a temp directory
  containing two fixture repos, calls the resolution
  layer with the temp directory as a parent entry, and
  asserts both repos appear in the resolved list.
- [ ] 5.2 Add a test that runs the resolution layer
  with no flag and no config, asserts the cwd fallback
  fires, and asserts existing behavior is preserved
  (one repo per `.git` child of cwd).
- [ ] 5.3 Add a test that writes a config file, calls
  the resolution layer with `--ignore-config = true`
  and a single `--watch`, and asserts the config file
  is not consulted.
- [ ] 5.4 Update existing tests that called
  `Watcher.Watch(ctx, wd)` to call the new signature
  with an explicit path list.

## 6. Validation

- [ ] 6.1 Run `go build ./...` and `go test ./...` and
  confirm green.
- [ ] 6.2 Run `openspec validate add-watch-paths-config`
  (or the appropriate change id) and confirm green.
- [ ] 6.3 Manual smoke: write a config file pointing
  at three repos; run `see --mode=log --once`; confirm
  three JSONL files and three `RepoSeen` events in
  the batch-level JSONL.
- [ ] 6.4 Manual smoke: run `see --mode=tui` with no
  config and no flag; confirm the grid renders the
  cwd-fallback repos (existing behavior).
- [ ] 6.5 Manual smoke: pass `--watch "/nonexistent/*"`
  to `see --mode=log --once`; confirm a single
  `Warning` event in the JSONL and a clean exit with
  code `0`.
- [ ] 6.6 Manual smoke: pass `--watch "~/work/**"` to
  `see --mode=log --once`; confirm an exit `2` and a
  clear stderr message naming `**` as the cause.
