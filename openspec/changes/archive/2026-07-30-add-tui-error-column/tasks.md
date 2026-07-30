## 1. Tests first (define the new behavior on current code)

- [x] 1.1 In `tui/tui_test.go`, add a test that drives one repo through
      `RepoSeen`, `ChangeStarted`, `RetryAttempt` (carrying an error),
      then `ChangeFailed` (carrying the final error) at width 120, and
      asserts `View()` renders an `ERROR` column header as the final
      column and the failed row's cell contains the final error text.
      Confirm it **fails** on current code: no ERROR column is
      rendered and `LastErr` is never read.
- [x] 1.2 Add a test that feeds a `ChangeFailedMsg` whose `Err` is a
      multi-line, CRLF-laden string (e.g. `"fatal: conflict\nlines\r\nmore"`)
      at width 120, and asserts the failed row occupies exactly one
      physical line and the rendered ERROR cell contains no newline.
      Confirm it **fails** on current code (no column yet); this is
      the regression guard for the `remove-tui-log-path` invariant and
      must stay green after implementation.
- [x] 1.3 Add a test at width 90 (between the AGE tier 80 and the
      ERROR tier 100) asserting the ERROR column is absent from both
      header and rows while AGE is still present, and that a failed
      row is otherwise identical to today. Confirm it passes on
      current code (it defines the no-op floor).

## 2. Render the ERROR column

- [x] 2.1 In `tui/view.go`, add the `errMinWidth = 20` constant and a
      `oneLine(s string) string` helper returning
      `strings.Join(strings.Fields(s), " ")`.
- [x] 2.2 In `View`, compute `fixedSum` from the active fixed columns
      (72 base; +8 when `showAge`), then `showErr = m.width -
      fixedSum >= errMinWidth` and `errWidth = m.width - fixedSum`.
      Thread `showErr` and `errWidth` into `renderHeader` and
      `renderRow`.
- [x] 2.3 In `renderHeader`, append an `ERROR` header cell of width
      `errWidth` (left-aligned) when `showErr`.
- [x] 2.4 In `renderRow`, when `showErr`, append an ERROR cell of
      width `errWidth` sourced from `truncate(oneLine(r.LastErr),
      errWidth)` when `r.LastErr != ""`, else `—`.
- [x] 2.5 Remove the now-superseded `ponytail:` comment in
      `renderRow` that says the message "lives in the JSONL (the
      operator can jq for it)" — the column is now that operator
      shortcut.
- [x] 2.6 Confirm tests 1.1 and 1.2 now pass; 1.3 still passes.

## 3. Verification

- [x] 3.1 Run `go vet ./...` and `go test -timeout 30s ./...`; both
      must be green.
- [x] 3.2 Run `rg -n 'LastErr' tui/` and confirm `LastErr` is read in
      `renderRow` and set/cleared only in `tui/model.go` (no new
      writes introduced by this change).
- [x] 3.3 `openspec validate add-tui-error-column` reports the change
      as ready for archive after implementation.
