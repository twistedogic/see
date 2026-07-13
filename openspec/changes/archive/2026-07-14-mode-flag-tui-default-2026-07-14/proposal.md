## Why

`see` was extended with a `--tui` flag by the `add-tui-grid` change, but the
flag is opt-in and defaults to `false`. In practice the operator running
`see` interactively almost always wants the live status grid — the
`log.Printf` mode is now the unusual case (CI, scripted pipelines). The
opt-in shape inverts the ergonomic default for the common path.

A second issue: the `--tui` flag is a special-case boolean. Future output
shapes (a `json` mode for machine consumers, a `watch` mode that
streams without rendering) would each want their own boolean flag, which
clutters the surface. Promoting the choice to a single `--mode` flag
with explicit values keeps the surface small and forward-compatible.

A third issue, exposed by the renaming: `main()`'s flag-to-mode dispatch
has grown nested `if *tuiFlag && !IsTerminal(...) { ... } else if *tuiFlag
{ ... }` branches that no test pins. Extracting a `selectRunMode`
function makes the dispatcher testable and shrinks the surface that
future mode changes have to touch.

## What Changes

- Replace the `-tui` boolean flag (default `false`) with a `-mode` string
  flag (default `"tui"`). The flag accepts `tui` and `log`.
- The default invocation (`see` with no flags) now starts in `tui` mode
  and renders the live status grid.
- `-mode=log` is the explicit opt-out and reproduces today's
  `log.Printf` behavior exactly.
- `-mode=tui` requires a TTY. When stdout is not a terminal, `see` exits
  with status 2 and a stderr hint naming `-mode=log`. This **inverts**
  the current "fall back to log mode" contract from
  `add-tui-grid`.
- Unknown values for `-mode` (including the empty string) exit with
  status 2, print `flag.Usage()`, and the stderr message names the
  valid set.
- Extract `selectRunMode(mode string, isTTY bool) (runMode, string)` in
  `main.go`. The function resolves the flag + TTY state into one of
  `modeLog`, `modeTUI`, or `modeUnknown` plus an error message.
- The `-tui` boolean flag is removed. Any invocation passing `-tui`
  fails with Go's `flag` package error ("flag provided but not
  defined").
- A small `TestSelectRunMode` table-driven test pins the matrix
  (`log`/TTY on or off, `tui`/TTY on or off, unknown values, empty
  string). This is the first direct test on `main()`'s flag dispatch.

**BREAKING**: the `-tui` flag is removed and the default mode changes
from log to tui. Piped invocations now exit non-zero with a TTY
required message instead of silently falling back to log mode. Anyone
scripted against `-tui` gets Go's "flag provided but not defined"
error. The tool has not been released externally, so clean break is
acceptable.

## Capabilities

### New Capabilities

(none — the change modifies the existing `tui` capability.)

### Modified Capabilities

- `tui`: replace the `--tui` boolean flag with a `--mode` flag
  accepting `tui` and `log`, default `tui`; invert the non-TTY
  fallback into a hard error; add scenarios for unknown mode values
  and missing-TTY under `--mode=tui`.

## Impact

- `main.go`:
  - Remove `flag.Bool("tui", ...)` and the `tuiFlag` variable.
  - Add `flag.String("mode", "tui", ...)` with the help text
    `"output mode (default \"tui\"); one of: tui, log"`.
  - Add `runMode` enum and `selectRunMode(mode string, isTTY bool)
    (runMode, string)` function. Return values are `modeLog`,
    `modeTUI`, or `modeUnknown` plus a stderr message; the unknown
    branch covers both invalid values and the empty string.
  - Rewrite the `main()` dispatch around `selectRunMode`. On
    `modeUnknown`, print the message, call `flag.Usage()`, and exit
    with status 2.
  - Drop the inline `term.IsTerminal` check from `main()`. TTY
    detection lives inside `selectRunMode` (it is a property of the
    `tui` mode, not of the dispatcher).
  - The `runTUI` path is unchanged: builds `tui.New()`, wires
    `tuiObserver`, runs the bubbletea program, signals
    `PiAgent.RedirectOutput = true`.
  - The `modeLog` path keeps today's `signal.NotifyContext` +
    `w.Watch` + `log.Printf` body unchanged.
- `main_test.go`:
  - New `TestSelectRunMode`: table-driven over six rows
    (`log`/TTY on, `log`/TTY off, `tui`/TTY on, `tui`/TTY off,
    `foo`/TTY on, `""`/TTY on). Each row asserts the returned mode
    enum and the returned message string.
  - No existing watcher/tui tests change.
- `openspec/specs/tui/spec.md`: `## MODIFIED Requirements` for the
  flag-exposure requirement (rename + default flip) and the
  non-TTY requirement (invert fallback to hard error);
  `## REMOVED Requirements` for the "Piped `--tui` falls back to log
  mode" scenario; `## ADDED Requirements` for unknown mode, missing
  TTY, and `--mode=log` parity scenarios.
- No dependency changes (`golang.org/x/term` already in `go.mod`).
- No changes to the `tui/` package, the `Watcher`, the `Observer`
  seam, or the agent invocation.