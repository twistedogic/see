## Why

`Watcher.Watch` immediately starts another scan after every successful pass, repeatedly invoking repository discovery and emitting events even when nothing has changed. A configurable five-minute delay removes this busy polling while preserving immediate startup and an explicit zero-delay escape hatch.

## What Changes

- **BREAKING**: Change continuous watching from immediate repeated scans to a five-minute delay after each successful completed pass by default.
- Add a top-level `--interval` command-line flag using Go duration syntax to configure the delay between completed scans.
- Keep the first scan immediate, make the delay interruptible by context cancellation, and preserve immediate return on pass errors.
- Define `--interval=0` as disabling the delay and restoring immediate polling; reject negative intervals at startup.
- Preserve `--once` behavior without introducing a delay before or after its single pass.
- Keep retry timing, YAML Ain't Markup Language (YAML) configuration, event schemas, and Terminal User Interface (TUI) presentation unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `watcher`: Add the configurable delay contract to the continuous `Watcher.Watch` loop and command-line interface.

## Impact

- `main.go`: add the interval setting, command-line flag, validation, and context-aware inter-pass wait.
- `main_test.go`: cover delayed repetition, prompt cancellation, zero-delay behavior, and preserved once/error behavior without spawning the long-running binary.
- `openspec/specs/watcher/spec.md`: define the new continuous-watch timing contract when the delta is synchronized.
- `AGENTS.md` and `openspec/config.yaml`: replace tight-loop guidance with the new default interval and retain timeout-safe test guidance.
- No new dependencies, event types, configuration-file fields, or TUI code.
