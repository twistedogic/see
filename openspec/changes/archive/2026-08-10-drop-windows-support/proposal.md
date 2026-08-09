## Why

`see` carries Windows-specific code paths (shell selection, a build-tagged
process-group stub, 14 test guards) but does not test, release, or exercise
Windows: CI runs a single `ubuntu-latest` job, there is no Windows release
asset, and the Windows test guards skip themselves. This is theater support —
branches in source claiming a platform the test suite declines to vouch for.
Untested platform support is maintenance debt: every `runtime.GOOS` fork and
the `configureConditionCommand` abstraction exist only to accommodate a
platform nobody verifies.

## What Changes

- **BREAKING**: `see` no longer builds on Windows. The `syscall`
  process-group plumbing (`Setpgid`, `syscall.SIGKILL`) becomes unconditional
  and does not exist in the Windows `syscall` package, so a Windows build
  fails at compile time rather than shipping an untested binary.
- The platform-shell contract becomes `/bin/sh -c` only; the `cmd.exe /C`
  branch is removed from condition, check, and measure execution.
- The `configureConditionCommand` build-tagged file pair
  (`condition_process_windows.go`, `condition_process_unix.go`) is deleted;
  the Unix process-group logic is relocated into `main.go` as the unconditional
  `configureProcessGroup` helper (three call sites, six statements — a helper
  beats triplicating it).
- The 14 `runtime.GOOS == "windows"` test-skip guards in `main_test.go` are
  removed.
- The `workflow-condition` capability spec drops its Windows shell clause,
  its Windows-specific trailing-newline scenario, and the "on Unix" qualifier
  on process-group cancellation.

## Capabilities

### New Capabilities
<!-- None. -->

### Modified Capabilities
- `workflow-condition`: the platform-shell requirement becomes Unix-only
  (`/bin/sh -c`); the Windows-specific trailing-newline scenario is removed;
  process-group cancellation is no longer qualified "on Unix" and applies to
  condition, check, and measure uniformly.

## Impact

- **Code**: `main.go` (three shell-selection sites collapse to a single
  `/bin/sh -c` invocation each), `condition_process_windows.go` and
  `condition_process_unix.go` (deleted), `main_test.go` (14 skip guards
  removed).
- **Specs**: `openspec/specs/workflow-condition/spec.md` — platform-shell
  requirement, trailing-newline scenarios, process-group language in the
  condition, check, and measure sections.
- **Docs**: `AGENTS.md` config-path and shell-contract prose that mentions
  Windows, `cmd.exe`, or `%AppData%`.
- **Users**: any Windows operator (none known; the path is untested and the
  guards self-skip) loses the ability to compile `see`. Re-adding Windows
  later requires re-introducing the platform fork alongside a CI job that
  verifies it; git history preserves the current implementation as a
  reference.
