## Context

The current `tui` spec encodes two exit-status scenarios
(`Pressing q exits the TUI cleanly` and `SIGINT exits the TUI
cleanly`) that contradict each other in `--once` mode and
contradict the actual behavior of `runTUI` and `main()` in
different ways. The cleanup chain that `runTUI` already implements
— cancel the watcher context, drain the watcher goroutine via the
`watchErr` channel, and close the JSONL event logger via the
LIFO defer on `events.Close()` — has no canonical spec language
tying it together.

The implementation in `runTUI` and `main()` already satisfies both
the unified exit-status rule and the cleanup-chain contract. The
deferred `events.Close()` is registered in `main()` *before*
`runTUI` runs, so it flushes after the watcher goroutine drains;
the explicit `cancel()` plus `<-watchErr` wait inside `runTUI`
sits between the bubbletea program returning and `runTUI`
returning. `model.go` already returns `tea.Quit` on both `q` and
`ctrl+c`. The bubbletea v1.3.10 signal handler reliably converts
SIGINT into the same `tea.KeyCtrlC` path that `q` takes, so the
two keys produce the same observable cleanup chain.

There is no runtime bug. The change is a documentation
correction: bringing the spec into alignment with the code, per
AGENTS.md's rule that the spec is the source of truth.

## Goals / Non-Goals

**Goals:**

- Spec language that matches the actual exit-status gate in
  `main()` and `runTUI`.
- One new requirement pinning the cleanup-chain contract that the
  existing code already satisfies.
- One unified scenario for `q` and SIGINT that replaces the two
  divergent scenarios.

**Non-Goals:**

- No code changes to `tui/model.go`, `tui/program.go`, `runTUI`,
  or any other source file.
- No new tests. `TestUpdateQuitsOnQ` plus the existing wiring
  carry the weight.
- No new dependencies.
- No refactor of `runTUI` for testability.
- No change to the actual runtime behavior of `q` or SIGINT.

## Decisions

### Decision 1 — keep the current exit-code path; spec rewording only

**Choice.** Keep the current implementation as-is. Rewrite the
spec so the exit-status rule is expressed as "non-zero iff
`Watcher.Watch` or `tea.Program.Run` returned a non-nil error
before the process exits," which is what the gate in `main()`
already does.

**Alternatives considered.**

- Add `var hadFailure atomic.Bool` on `Watcher`, set inside
  `runOnce` only when `ChangeFailed` is emitted, read at `Watch`
  return. Treat "we cancelled mid-work" as a benign nil result.
  Rejected — the user opted for the lazy path that preserves
  current behavior.
- Use `errors.Is(err, context.Canceled)` walking at `Watch`
  return. Rejected — brittle to future error paths and provides
  no behavior the user is asking for.
- Drop the SIGINT scenario's "non-zero if a repo failure was in
  flight" line entirely (force both keys to always exit 0).
  Rejected — loses the signal that a watched failure is the
  *cause* of an exit rather than a side-effect of cancellation.

### Decision 2 — skip end-to-end tests; trust existing wiring

**Choice.** Rely on `TestUpdateQuitsOnQ` in `tui/tui_test.go` and
the existing code for the cleanup-chain assertion. No new tests.

**Alternatives considered.**

- Refactor `runTUI` to expose the cleanup phase as a separately
  testable function. Rejected — refactoring without a payoff in
  tests is waste.
- Integration tests that spawn the compiled binary under
  different exit scenarios. Rejected — slower than unit tests
  and adds a CI surface.
- A small unit test that just asserts `runTUI`'s pre-exit
  ordering (cancel before `<-watchErr` wait before return).
  Rejected — the user opted to skip end-to-end tests.

### Decision 3 — derive spec wording from the actual exit gate

**Choice.** The new unified scenario's exit-status rule is
expressed in terms of `Watcher.Watch`'s return value and
`tea.Program.Run`'s return value — the two values that feed
`main()`'s `if err := runTUI(...); err != nil { os.Exit(1) }`
gate.

**Rationale.** This phrasing is implementation-grounded and
impossible to misread. The previous phrasing ("same status as
the equivalent log-mode invocation, non-zero if a repo failure
was in flight") was vague about what "in flight" means and
diverged from the q scenario's "status 0 always."

## Risks / Trade-offs

- **[Risk]** A future refactor of `runTUI` could quietly break
  the cleanup chain (cancel-before-wait ordering, defer ordering
  for `events.Close()`, `recover()` in `ChanObserver.Send`)
  without anything in the test suite catching it. **Mitigation:**
  the existing `ponytail:` comments on the relevant lines in
  `main.go` document the intent for the next reader; the spec
  rewording makes the contract explicit and grep-able.

- **[Trade-off]** The unified "non-zero iff a non-nil error
  reached the exit gate" reading of "repo failure was in flight"
  conflates "we cancelled mid-work" with "the work failed on its
  own." The current implementation already treats both as
  non-zero exits, so this is a deliberate match to current
  behavior rather than a new limitation. Future cleanup-quality
  work could revisit this by introducing the `atomic.Bool` from
  Decision 1's first alternative, with the spec wording updated
  accordingly.

- **[Risk]** If a reader treats the new requirement "`--tui`
  drains the watcher goroutine and closes the JSONL event
  logger before exit" as a *new* commitment that the project
  must *implement*, they may add code where none is needed.
  **Mitigation:** the requirement explicitly names the existing
  defer and `<-watchErr` wait as the runtime hooks that satisfy
  it, so the language clearly anchors on the current
  implementation.
