## 1. Reproduce the garble (failing test first)

- [x] 1.1 In `tui/tui_test.go`, add a regression test that drives one
      repo through `RepoSeen`, `ChangeStarted`, `LogPathMsg` (a path
      longer than the terminal width), and `ChangeDone` at a narrow
      width (e.g. 60 columns). Assert the rendered `View()` does not
      contain the path string and the repo's row occupies exactly one
      physical line. Confirm it **fails** on the current code — the
      path renders and wraps to a third line. This is the required
      bug reproduction before any production change.

## 2. Stop rendering the path

- [x] 2.1 In `tui/view.go` `renderRow`, delete the
      `if r.LogPath != "" { row += "\n      " + r.LogPath }` block so
      every row is a single `lipgloss.JoinHorizontal` line.
- [x] 2.2 In `tui/view.go` `fitToHeight`, collapse to single-line
      accounting: drop the `rowLines = 2` branch and the
      log-path-continuation comment; every retained row costs exactly
      one line.
- [x] 2.3 Confirm the 1.1 regression test now passes.

## 3. Delete the dead TUI-side plumbing

- [x] 3.1 In `tui/model.go`, remove the `case LogPathMsg` arm of
      `Update` and the `LogPath string` field on `RepoRow`.
- [x] 3.2 In `tui/events.go`, remove the `LogPathMsg` type.
- [x] 3.3 In `main.go`, remove the `case LogPath` arm of
      `tuiObserver.Observe`. Leave the `LogPath` event type and its
      three emit sites (`workResolvedWorktree`, the custom branch
      path, and the OpenSpec branch path) untouched — they feed the
      batch JSONL file and the `--mode=log` stdout mirror.

## 4. Test cleanup and invariant

- [x] 4.1 Delete `tui/tui_test.go` `TestViewRendersLogPathWhenSet`
      (it asserts the now-removed render).
- [x] 4.2 Rework the 1.1 regression test so it no longer depends on
      the deleted `LogPathMsg`: drive a repo through `RepoSeen`,
      `ChangeStarted`, and `ChangeDone` at a narrow width and assert
      its row is exactly one physical line and `View()` contains no
      `.jsonl` substring. Add a direct `fitToHeight` assertion that it
      never returns more rows than the height budget allows (one line
      each).
- [x] 4.3 Confirm `TestViewOmitsLogPathWhenUnset` still holds as the
      baseline one-line row shape (summary + header + 1 row + footer).

## 5. Verification

- [x] 5.1 Run `go vet ./...` and `go test -timeout 30s ./...`; both
      must be green.
- [x] 5.2 Run `rg -n 'LogPathMsg|r\.LogPath|case LogPath' tui/ main.go`
      and confirm no TUI-side references remain — the only hits are
      the `main.LogPath` event type and its emit sites in `main.go`.
- [x] 5.3 `openspec validate remove-tui-log-path` reports the change
      as ready for archive after implementation.
