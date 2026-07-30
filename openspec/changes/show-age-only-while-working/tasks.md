## 1. Regression Coverage

- [ ] 1.1 Add a failing TUI regression test that starts a row, transitions it through `working`, `done`, `failed`, and `idle`, and proves AGE displays elapsed time only while working and `—` in every other phase, including when `StartedAt` remains set.

## 2. AGE Rendering

- [ ] 2.1 Gate elapsed AGE rendering on `PhaseWorking` while preserving the existing zero-start fallback, AGE column width threshold, and one-second tick behavior.

## 3. Verification

- [ ] 3.1 Run `gofmt` on changed Go files and `go test -timeout 30s ./...`.
- [ ] 3.2 Run strict OpenSpec validation for `show-age-only-while-working` and confirm its scenarios are covered by the regression test.
