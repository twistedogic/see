## ADDED Requirements

### Requirement: Command-line interval configures continuous polling

`see` SHALL expose a top-level `--interval` flag parsed with Go duration syntax. The flag SHALL default to `5m` and SHALL set the process-wide delay between successful completed scans. A value of `0` SHALL disable the delay. A negative value SHALL produce an actionable command-line error and exit status 2 before the watcher starts.

The interval SHALL remain a command-line-only setting. It SHALL NOT add a field to global configuration.

#### Scenario: Default interval is five minutes

- **WHEN** `see` is invoked without `--interval` in continuous mode
- **THEN** the watcher uses a five-minute interval between successful completed scans

#### Scenario: Operator selects a shorter interval

- **WHEN** `see --interval=30s` is invoked in continuous mode
- **THEN** the watcher uses a 30-second interval between successful completed scans

#### Scenario: Zero disables the delay

- **WHEN** `see --interval=0` is invoked in continuous mode
- **THEN** the watcher starts another pass after a successful pass without an intentional delay

#### Scenario: Negative interval is rejected

- **WHEN** `see --interval=-1s` is invoked
- **THEN** `see` reports that `--interval` must be non-negative
- **AND** exits with status 2 before the watcher starts

### Requirement: Continuous watcher waits after successful passes

When `Watcher.Once` is false, `Watcher.Watch` SHALL start its first `runOnce` pass immediately. When that pass succeeds and `Watcher.PollInterval` is greater than zero, `Watcher.Watch` SHALL wait for the full interval measured from completion of that pass before starting the next pass. Scans SHALL NOT overlap.

The wait SHALL be interruptible by the supplied `context.Context`. Cancellation before a pass or during the wait SHALL make `Watcher.Watch` return `nil` without waiting for the interval to elapse. A non-nil `runOnce` error SHALL still return immediately without waiting or starting another pass.

#### Scenario: First pass starts immediately

- **WHEN** continuous `Watcher.Watch` starts with a five-minute interval and a live context
- **THEN** it invokes `runOnce` without an initial five-minute delay

#### Scenario: Delay begins after pass completion

- **WHEN** a continuous watcher has a five-minute interval and a successful pass completes at time T
- **THEN** the next pass does not start before T plus five minutes
- **AND** no second pass overlaps the completed pass

#### Scenario: Cancellation interrupts the delay

- **WHEN** a successful pass completes and the context is cancelled while `Watcher.Watch` is waiting for the next pass
- **THEN** `Watcher.Watch` returns `nil` promptly
- **AND** does not start another pass

#### Scenario: Pass error bypasses the delay

- **WHEN** `runOnce` returns a non-nil error in continuous mode
- **THEN** `Watcher.Watch` returns that error immediately
- **AND** does not wait for the interval or invoke `runOnce` again

#### Scenario: Once mode never waits

- **WHEN** `Watcher.Once` is true and the single `runOnce` pass succeeds
- **THEN** `Watcher.Watch` returns `nil` immediately after that pass
- **AND** does not wait for `Watcher.PollInterval`

#### Scenario: Retry timing is unchanged

- **WHEN** a repository attempt fails and `Watcher.RetryCount` permits another attempt within the same `runOnce` pass
- **THEN** the retry begins according to the existing retry contract without waiting for `Watcher.PollInterval`
