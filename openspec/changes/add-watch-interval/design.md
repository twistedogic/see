## Context

`Watcher.Watch` currently performs an immediate `runOnce` pass and, after success, begins the next pass without waiting. This makes an idle process repeatedly inspect every configured repository and emit `RepoSeen` events as fast as repository operations allow. The source and project context already identify this tight poll loop as deferred debt.

The loop has two modes and two callers. `Watcher.Once` makes one pass and returns. Continuous mode runs from either log mode or the Terminal User Interface (TUI) until its context is cancelled or a pass fails. TUI shutdown depends on cancellation waking the watcher promptly, so an uninterruptible `time.Sleep` would violate the existing cleanup contract.

The separate `promote-config-to-yaml` change adds global prompt and watch settings. The requested interval is explicitly a command-line setting and does not need another YAML Ain't Markup Language (YAML) field.

## Goals / Non-Goals

**Goals:**

- Run the first scan immediately, then delay five minutes by default after each successful completed pass.
- Let operators configure the delay with `--interval` and Go duration syntax.
- Let an explicit zero interval preserve the current immediate-polling behavior.
- Reject negative intervals before watcher startup.
- Keep cancellation, once mode, pass-error handling, and retry timing correct.
- Cover the timing behavior with focused unit tests that do not spawn the long-running binary.

**Non-Goals:**

- Fixed-cadence scheduling measured from pass start times.
- A delay before the first pass or between retry attempts within a pass.
- Runtime interval reloads or a configuration-file interval field.
- New watcher events, a next-scan countdown, or TUI presentation changes.
- Parallel or overlapping scans.
- Repairing the pre-existing `runTUI` one-send/two-receive result-channel issue; that requires a separate reproduced bug and regression test.

## Decisions

### Use a completion-relative delay, not a ticker

After a successful `runOnce`, continuous mode waits for the configured duration before beginning the next pass. A pass taking two minutes with a five-minute interval therefore produces a seven-minute start-to-start gap.

The wait is a `select` between a timer channel and `ctx.Done()`. Cancellation returns `nil` immediately instead of waiting for the timer. `time.Sleep` is rejected because it delays signal and TUI shutdown. A ticker is rejected because it describes fixed start cadence, requires lifecycle cleanup, and can make the next pass run immediately after a long pass.

The loop checks cancellation before each pass. A non-nil `runOnce` error returns immediately without creating a timer. `Watcher.Once` returns after its first pass without waiting.

### Carry the interval as one Watcher field

Add `PollInterval time.Duration` to `Watcher`, parallel to its existing `RetryCount` and `Once` configuration. `Watcher.Watch` reads that field; no new scheduler or clock interface is introduced.

Register `--interval` with the standard `flag.Duration` parser and a shared `5 * time.Minute` default. Pass the parsed duration into the blessed `NewWatcher` construction path so production watchers are fully populated. Direct test literals can set a short interval or use the zero value deliberately.

`--interval` is preferred over `--poll-interval` because the tool has no other interval and the help text can state the exact boundary: “delay between completed scans.”

### Treat zero as an explicit compatibility mode

When `PollInterval == 0`, `Watcher.Watch` starts the next pass without an intentional wait, preserving the current behavior for operators who need it and keeping the `time.Duration` zero value meaningful. The loop still checks context before each pass.

Negative command-line durations are rejected immediately after flag parsing with an actionable error and exit status 2, before log setup, configuration loading, or watcher startup. Silently treating a negative duration as zero is rejected because a typo would create an unexpected busy loop.

### Keep interval state out of configuration and events

The flag applies process-wide for the process lifetime. The strict YAML configuration schema remains limited to its separately proposed `watches` and `prompt` fields. No event is emitted when waiting and the TUI does not display a countdown; adding either would expand public schemas without improving the core polling behavior.

### Verify behavior with real short timers and channels

Use a valid fixture repository with a channel-backed observer to record pass times. A short configured interval can prove that the second `RepoSeen` occurs after the first pass rather than immediately. A separate test uses a long interval, waits for the first pass, cancels the context, and requires `Watch` to return within a generous bounded deadline. Zero-delay, once-mode, and first-error tests pin their respective branches.

A fake clock is rejected: one standard-library timer and bounded channel assertions provide sufficient coverage without adding test-only production abstractions. All test commands retain the project’s 30-second timeout.

## Risks / Trade-offs

- **[Risk] New changes can wait almost five minutes after an idle pass.** → The first pass remains immediate and operators can choose a shorter `--interval`.
- **[Risk] `--interval=0` can consume Central Processing Unit (CPU) and generate a large event log.** → Zero is an explicit compatibility escape hatch; five minutes remains the default and negative values are rejected.
- **[Risk] Wall-clock timing assertions can be flaky on loaded machines.** → Assert only that a timer does not fire early, use short intervals with generous upper deadlines, and coordinate through channels rather than sleeps where possible.
- **[Trade-off] Completion-relative delay drifts relative to wall-clock boundaries.** → This is intentional: it prevents overlap and matches the requested sleep-after-completion semantics.
- **[Trade-off] Direct `Watcher` literals retain zero-delay behavior unless they set the field.** → Production uses `NewWatcher`; tests and internal callers using literals remain explicit and avoid hidden zero-value reinterpretation.

## Migration Plan

No data or configuration migration is required. Existing invocations acquire the five-minute default automatically. Operators requiring the previous behavior can add `--interval=0`; operators requiring faster polling can pass another non-negative Go duration. Rolling back to the previous binary restores unconditional immediate polling.

## Open Questions

None. Timing origin, first-pass behavior, zero and negative semantics, command-line scope, cancellation, and once/error behavior are resolved.
