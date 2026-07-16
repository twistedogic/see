# AGENTS.md

Guidelines for AI agents and contributors working on this project.

## Documentation

- Never use acronyms without explaining them in documents. The first time an acronym appears, spell out the full term followed by the acronym in parentheses (e.g., "Application Programming Interface (API)"). Subsequent uses may use the acronym alone.

## Commit Messages

- When writing a commit message, never add your agent name as author or co-author. Commits must reflect the human contributor as author and must not include agent names in the author or co-author trailers.

## Bug Fixes

- When doing a bug fix, always start by reproducing the bug and add a failing test case before changing production code. The failing test must demonstrate the bug, and the fix must turn it green. Never merge a bug fix without a regression test.

## Testing

- `main` is a watch loop: `Watcher.Watch` (`main.go`) runs one
  `runOnce` pass immediately, then waits `Watcher.PollInterval`
  (`DefaultPollInterval` = five minutes via `NewWatcher`) after every
  successful pass until `SIGINT` or `SIGTERM` cancels the context.
  `--interval=0` restores the pre-default tight-poll loop; negative
  intervals are rejected at startup. Tests that spawn the binary hang
  the test runner, so:
  - Prefer unit tests that drive `Watcher.work` (or a single `runOnce`
    pass) directly with a `fakeAgent` and a `recordingObserver`
    (see `main_test.go`). Assert on the observed event sequence and
    the captured `Run` arguments — never on process exit codes or
    stdout.
  - Set `Watcher.PollInterval` to a short duration or zero in unit
    tests so the loop returns within a bounded deadline. A literal
    `Watcher{}` defaults to zero interval for this reason.
  - Always run `go test -timeout 30s ./...` (or shorter). A wedged
    poll loop or goroutine should fail fast at 30 seconds rather than
    hitting the runner's default 10-minute ceiling and masking the
    real bug under a generic timeout.
  - Reserve spawning the binary for manual smoke checks and one-shot
    `see --once` runs against a fixture repo, never inside an
    automated test.

## Technical Decisions

- When making a technical decision, do not give much weight to development cost and time. Instead, prefer correctness, readability, simplicity, and long-term maintainability. Short-term effort is a secondary concern; the chosen approach should be one we are willing to live with for years.

## Observability

- Always consider observability of the application in development.
- Prefer structured logging (key/value fields, consistent log levels, machine-parseable format) over unstructured log strings.
- For servers, prefer Prometheus metrics (counters, gauges, histograms) exposed on a standard scrape endpoint, in addition to structured logs.

## Maintenance of AGENTS.md

- Keep AGENTS.md up to date on key design decisions and development workflows. When a decision is made or a workflow changes, update this file in the same change so it remains the source of truth for future contributors and agents.
