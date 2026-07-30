## 1. Migration Preconditions and Regression Coverage

- [ ] 1.1 Confirm `show-age-only-while-working` and `shorten-tui-errors` are applied so the migration preserves their AGE and concise-error behavior.
- [ ] 1.2 Extend TUI regression coverage for unchanged responsive grid output, table navigation keys remaining inert, and alternate-screen declaration through the Bubble Tea v2 view contract.

## 2. Charm v2 Dependency and Model Migration

- [ ] 2.1 Replace the Bubble Tea and Lip Gloss v1 modules with compatible stable `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, and `charm.land/lipgloss/v2` dependencies.
- [ ] 2.2 Adapt the TUI model, key handling, program construction, observer bridge, and test messages to Bubble Tea v2, including declarative alternate-screen mode.

## 3. Bubbles Table Rendering

- [ ] 3.1 Project the existing prioritized, height-fitted repository rows and responsive columns into an unfocused Bubbles v2 table with zero-padding, non-selecting styles.
- [ ] 3.2 Replace custom header, cell joining, and truncation code with the table view while preserving the summary, empty state, infrastructure error, footer, one-line rows, phase colors, column thresholds, and ten-row cap.
- [ ] 3.3 Remove local rendering helpers and direct dependencies made redundant by the Bubbles table.

## 4. Verification

- [ ] 4.1 Run `gofmt` on changed Go files and `go mod tidy`.
- [ ] 4.2 Run `go test -timeout 30s ./...` and strict OpenSpec validation for `migrate-tui-to-bubbles-v2`.
