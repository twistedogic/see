## ADDED Requirements

### Requirement: Watcher runs only one pass when Once is true
When `Watcher.Once` is true, `Watcher.Watch` SHALL return after the first
call to `runOnce` completes, whether that call returned an error or nil.
When `Watcher.Once` is false (the default), `Watcher.Watch` SHALL keep
calling `runOnce` in a loop until the supplied `context.Context` is
cancelled or `runOnce` returns a non-nil error; in either termination
case `Watcher.Watch` SHALL return.

#### Scenario: Once mode returns after a successful pass
- **WHEN** `Watcher.Once` is true and `runOnce` returns nil
- **THEN** `Watcher.Watch` returns nil after that single pass
- **THEN** `Watcher.Watch` does not call `runOnce` again

#### Scenario: Once mode returns after a failed pass
- **WHEN** `Watcher.Once` is true and `runOnce` returns a non-nil error
- **THEN** `Watcher.Watch` returns that error
- **THEN** `Watcher.Watch` does not call `runOnce` again

#### Scenario: Default mode loops until context is cancelled
- **WHEN** `Watcher.Once` is false and the supplied context is cancelled
  before any pass returns an error
- **THEN** `Watcher.Watch` returns nil

#### Scenario: Default mode stops on first pass error
- **WHEN** `Watcher.Once` is false and `runOnce` returns a non-nil error
- **THEN** `Watcher.Watch` returns that error without calling `runOnce`
  again

#### Scenario: Once mode preserves the per-pass contract
- **WHEN** `Watcher.Once` is true and the first pass runs against a repo
  with an active change
- **THEN** the existing per-pass requirements (branch creation, rollback,
  merge) still apply unchanged to that pass