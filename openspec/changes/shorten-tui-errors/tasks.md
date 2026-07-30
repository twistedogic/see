## 1. Regression Coverage

- [ ] 1.1 Add a failing regression test for a dirty-working-tree error that requires the full path-bearing diagnostic and the concise `dirty working tree; commit or stash` summary, including discovery through a `%w` wrapper.
- [ ] 1.2 Add failing event-path tests proving retry, final-failure, and watcher infrastructure events retain the full exported `Err` while the in-memory TUI value receives the concise summary.
- [ ] 1.3 Add a failing event-logger test proving JavaScript Object Notation Lines (JSONL) output contains the full dirty-working-tree diagnostic and does not serialize a presentation-only summary field.

## 2. Concise Error Plumbing

- [ ] 2.1 Add the private optional summary interface, `errors.As`-based fallback helper, and shared dirty-working-tree error type; replace both duplicate dirty-tree error constructions with that type.
- [ ] 2.2 Carry concise text in unexported fields on retry, final-failure, and infrastructure events while preserving their exported full `Err` values.
- [ ] 2.3 Update the main-to-TUI observer adapter to forward concise text when present and fall back to the full `Err` for ordinary errors, without changing TUI package message or rendering types.

## 3. Verification

- [ ] 3.1 Run `gofmt` on changed Go files and run `go test -timeout 30s ./...`.
- [ ] 3.2 Run strict OpenSpec validation for `shorten-tui-errors` and confirm all proposal tasks and specification scenarios are represented by the implementation and regression tests.
