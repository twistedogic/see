## Why

The Terminal User Interface (TUI) AGE column continues showing elapsed time after work has completed or failed, and can keep increasing when a later scan returns the repository to idle. AGE is useful as a live work timer, but misleading when the repository is not actively working.

## What Changes

- Render elapsed AGE only while a repository is in the `working` phase.
- Render the existing no-value placeholder `—` in AGE while the repository is `idle`, `done`, or `failed`.
- Preserve the AGE column's existing width threshold and one-second updates during active work.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `tui`: Limit elapsed AGE display to actively working repository rows and use `—` for every other phase.

## Impact

- Affected code: TUI row rendering and focused regression coverage in `tui/`.
- No watcher events, model fields, command-line options, dependencies, or persistence formats change.
