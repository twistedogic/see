## ADDED Requirements

### Requirement: Retry helper returns error from the last attempt
`retryN(n, f)` SHALL invoke `f` up to `n` times. If any invocation returns a
nil error, `retryN` SHALL return nil immediately. If every invocation returns
a non-nil error, `retryN` SHALL return the error from the final invocation.

#### Scenario: Succeeds on first attempt
- **WHEN** `f` returns nil on the first call
- **THEN** `retryN` returns nil and does not invoke `f` again

#### Scenario: Succeeds on a later attempt
- **WHEN** `f` returns `err1` then `err2` then nil
- **THEN** `retryN` returns nil after the third call

#### Scenario: Exhausts retries with errors
- **WHEN** `f` returns `err1`, `err2`, `err3` over three calls
- **THEN** `retryN` returns `err3` after the third call

#### Scenario: Zero retries is a no-op
- **WHEN** `retryN(0, f)` is called
- **THEN** `f` is not invoked and `retryN` returns nil
  *(ponytail: documented ceiling — a `-retry 0` misconfiguration silently
  succeeds. If this becomes load-bearing, add an explicit guard.)*