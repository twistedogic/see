# watcher delta — apply-ponytail-audit

## ADDED Requirements

### Requirement: Watcher's retry loop returns the error from the final attempt
`Watcher.runOnce` SHALL retry `Watcher.work` for a given repo up to
the count passed to the `Watcher` constructor (formerly known as
`RetyCount`, renamed to `RetryCount`). If any attempt returns a nil
error, the loop SHALL stop and `runOnce` SHALL move to the next
repo. If every attempt returns a non-nil error, the loop SHALL
return the error from the final attempt. The loop SHALL emit a
`RetryAttempt` event before each retry after the first attempt.

#### Scenario: Succeeds on the first attempt
- **WHEN** `Watcher.work` returns nil on the first call
- **THEN** the retry loop does not invoke `work` again
- **THEN** the loop returns nil

#### Scenario: Succeeds on a later attempt
- **WHEN** `Watcher.work` returns `err1` then `err2` then nil
- **THEN** the loop returns nil after the third call

#### Scenario: Exhausts retries with errors
- **WHEN** `Watcher.work` returns `err1`, `err2`, `err3` over
  three calls
- **THEN** the loop returns `err3` after the third call

#### Scenario: Zero retries is a no-op
- **WHEN** the watcher is constructed with a retry count of `0`
- **THEN** `work` is not invoked
- **THEN** the loop returns nil
  *(ponytail: documented ceiling — a `-retry 0` misconfiguration
  silently succeeds. If this becomes load-bearing, add an
  explicit guard.)*
