## 1. Model activity and viewport selection

- [x] 1.1 Add deterministic discovery and meaningful-activity sequence metadata to `tui.Model` and `tui.RepoRow` without removing retained repository rows or changing the existing scan-order state.
- [x] 1.2 Update message handling so first discovery establishes fallback ordering, meaningful lifecycle messages refresh activity, and repeated `RepoSeenMsg` messages for existing rows do not refresh activity.
- [x] 1.3 Add a render-only selector that ranks working rows, failed rows, warning-bearing rows, and remaining rows in that order, sorts within each class by recent activity with stable discovery tie-breaking, and caps the result at ten repository entries.

## 2. Summary and bounded layout

- [x] 2.1 Derive live summary counts from all retained rows, including total, working, done, idle, failed, warnings, and visible `shown / total` counts.
- [x] 2.2 Render the summary table above the repository header using the existing styling dependencies and compact behavior for narrow terminal widths.
- [x] 2.3 Render the selected viewport instead of the full repository order, preserve existing columns and warning glyphs, and remove duplicated phase and warning totals from the footer while retaining the quit hint and infrastructure error.
- [x] 2.4 Apply the terminal-height budget so the summary, header, footer, infrastructure error, and complete repository entries fit without splitting log-path continuations; never render more than ten repository entries.

## 3. Regression coverage and verification

- [x] 3.1 Add tests proving the summary counts all retained repositories and reports the visible count independently of the ten-row viewport.
- [x] 3.2 Add tests proving the viewport cap, attention-priority ordering, meaningful-activity ordering, and stable tie-breaking while retaining all rows in the model.
- [x] 3.3 Add tests proving repeated `RepoSeenMsg` events do not change recency, while lifecycle events do, and that warning and phase transitions preserve existing row behavior.
- [x] 3.4 Add layout tests for narrow/short terminals, complete log-path entries, footer controls, row truncation, and existing custom/OpenSpec display behavior; update assertions that depended on the old footer summary position.
- [x] 3.5 Run `gofmt` on changed Go files and run `go test -timeout 30s ./...`.
