## 1. Prerequisite

- [x] 1.1 Confirm `migrate-tui-to-bubbles-v2` is applied and archived before changing the Terminal User Interface (TUI) model or rendering contract.

## 2. Pi Activity Parser Regression Coverage

- [x] 2.1 Add failing parser tests for completed assistant text, known and unknown tool starts, correlated tool success and failure, ignored thinking/token/progress events, and sanitized non-JSON diagnostics.
- [x] 2.2 Add failing capture tests proving parsed activity arrives before process exit while the per-invocation JavaScript Object Notation Lines (JSONL) file retains the exact original bytes.
- [x] 2.3 Add failing boundary tests proving terminal controls are stripped, malformed input is non-fatal, oversized lines do not grow retained parser state, and a nil callback preserves log-mode behavior.

## 3. Live Activity Capture

- [x] 3.1 Add the bounded newline parser and minimal pi event decoding needed to produce sanitized one-line assistant and tool activity summaries.
- [x] 3.2 Tee successful per-invocation file writes into the parser only when an activity callback is configured, without changing agent exit results or raw capture.
- [x] 3.3 Extend the internal agent and watcher call boundary with the optional callback, wire it directly to the TUI in `runTUI`, and keep transient activity out of `eventLogger`.

## 4. Ticker Regression Coverage

- [x] 4.1 Add failing model and view tests for `waiting`, reset to `starting`, latest-activity replacement, fixed `pi ›` and `[q] quit` chrome, and exactly one footer line at wide and narrow widths.
- [x] 4.2 Add failing deterministic marquee tests for display-cell movement, visible loop gaps, reset on new activity and resize, static fitting text, and tick shutdown when overflow ends.

## 5. Animated Ticker

- [x] 5.1 Add the pi activity message and bounded ticker state, resetting activity and offset at the lifecycle boundaries required by the specification.
- [x] 5.2 Replace the quit-only footer with the width-aware combined ticker, prioritizing the quit hint on narrow terminals and preventing control sequences or wide characters from wrapping.
- [x] 5.3 Add a marquee command that rearms only while sanitized activity overflows and leaves the existing one-second AGE tick unchanged.

## 6. Verification

- [x] 6.1 Run `gofmt` on changed Go files and `go mod tidy`.
- [x] 6.2 Run `go test -timeout 30s ./...` and strict OpenSpec validation for `show-live-pi-ticker`.
