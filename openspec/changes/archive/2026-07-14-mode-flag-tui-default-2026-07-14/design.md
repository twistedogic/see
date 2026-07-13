## Context

The `--tui` flag from `add-tui-grid` is a boolean with one default
(`false`). Three observations motivate this change:

1. The operator running `see` interactively almost always wants the
   TUI. The log-mode default exists because the tool shipped without
   a TUI; once a TUI exists, it should be the default.
2. The boolean shape leaves no room for a third mode without adding a
   second flag. `--mode` is forward-compatible: `tui`, `log`, and any
   future value (`json`, `watch`) live in one place.
3. `main()`'s flag-to-mode dispatch has accumulated nested conditions
   (`if *tuiFlag && !IsTerminal(...) { ... } else if *tuiFlag { ... }`)
   that no test pins. Extracting a pure function turns that surface
   into a table-driven test.

The watcher, the observer seam, and the `tui/` package are untouched
by this change. The contract between the watcher and any subscriber
stays the same: `Observer` events fire at phase boundaries regardless
of mode. Only the dispatch at the top of `main()` changes.

## Goals / Non-Goals

**Goals:**
- Default `see` invocation (no flags) renders the TUI.
- `-mode=tui` and `-mode=log` are explicit, mutually exclusive ways
  to choose the output mode.
- `-mode=tui` without a TTY fails loudly (exit 2) with a stderr hint
  pointing at `-mode=log`. No silent fallback.
- Unknown `--mode` values (including the empty string) fail loudly
  (exit 2) and print `flag.Usage()` so the operator sees the valid
  set.
- `selectRunMode` is a pure function with no side effects, suitable
  for direct unit testing.
- The existing 14+ watcher/tui tests stay green without modification.
- The `tui` capability spec accurately reflects the new contract.

**Non-Goals:**
- New output modes (`json`, `watch`, etc.). The flag shape supports
  them but they are not part of this change.
- Concurrent repo execution, structured logging, Prometheus metrics.
  These are independent follow-ups.
- Changing watcher semantics, agent invocation, retry policy, or git
  rollback. The mode flag is a transport concern, not a behavior
  change.
- Deprecating the `-tui` flag as an alias. The tool is unreleased and
  clean break is acceptable.

## Decisions

**`-mode` is a string flag with values `tui` and `log`, default `tui`.**

Go's `flag.String` is the standard answer. The default value is set
by the flag package, so `selectRunMode` always receives a non-empty
string in normal flow. The empty-string edge case (operator passes
`-mode=` with no value) is treated as unknown and fails loudly.

Alternatives considered:
- Keep `-tui` as a deprecated bool alias: adds a second flag with no
  ergonomic benefit. The tool has no external consumers yet.
- Use a custom flag type with a `ValidModes()` method: ceremony for a
  two-value enum.

**`selectRunMode` is a pure function returning `(runMode, string)`.**

```go
type runMode int
const (
    modeUnknown runMode = iota
    modeLog
    modeTUI
)

func selectRunMode(mode string, isTTY bool) (runMode, string)
```

Both error paths (unknown value, missing TTY) return `modeUnknown`
plus a stderr message. `main()` prints the message, calls
`flag.Usage()`, and `os.Exit(2)`s. The caller is uniform over both
error kinds; `selectRunMode` doesn't know about exit codes.

The function takes `isTTY bool` as an argument rather than calling
`term.IsTerminal` itself. The test pins the function's behavior
without needing a real TTY; `main()` does the one real detection and
passes the result.

Alternatives considered:
- Inline `switch` in `main()`: testability is the whole reason to
  extract.
- Returning an `error` instead of `(runMode, string)`: forces the
  caller to format the message; keeping the message in the function
  makes the table-driven test read naturally.

**TTY detection moves inside `selectRunMode`.**

Today, `main()` checks `term.IsTerminal(...)` before deciding whether
to honor `-tui`. After this change, TTY is a property of the `tui`
mode: `modeLog` ignores it, `modeTUI` requires it. The check belongs
in `selectRunMode`, not in the dispatcher. `main()` does the one real
detection and passes the boolean in.

**`flag.Usage()` is called on error before exit.**

Go's `flag` package auto-generates usage text from the registered
flags. A user typing `see --mode=foo` sees the message naming valid
values plus the standard usage line, then exit 2. Conventional for
Go CLIs.

Alternatives considered:
- Print only the message, skip `flag.Usage()`: leaves the operator
  without context about which flag is wrong or what its valid values
  are.

**The `-tui` flag is removed entirely.**

No deprecated alias. Go's `flag` package prints "flag provided but
not defined" and exits 2 on `-tui` invocations, which is the natural
failure mode for a renamed flag.

## Risks / Trade-offs

- **CI / piped invocations now fail instead of silently falling
  back** → Operators who pipe `see | tee log.txt` will get "stdout is
  not a TTY" errors. They need `-mode=log` to keep that workflow.
  The error message names `-mode=log` explicitly so the fix is one
  flag away. This is a deliberate loud-failure contract; if the
  operator wants silent fallback, they can opt in with `-mode=log`.
- **Removing `-tui` breaks any scripted invocations that used the
  flag** → The tool has no external consumers (no releases, single
  operator). Acceptable clean break.
- **Help text becomes the operator's first hint that `-mode=log`
  exists** → The string help text enumerates the valid set
  (`"one of: tui, log"`), so a user reading `--help` sees both
  values immediately.
- **No new tests on the rest of `main()`** → Out of scope. The
  extracted `selectRunMode` covers the new branch; the rest of
  `main()` (flag registration, signal handling, `log.Fatal` paths)
  remains untested. Worth a follow-up but not blocking.
- **Spec churn is larger than the code change** → Five new
  scenarios, two modified scenarios, one removed scenario. The
  capability stays accurate, which is the point of spec-driven
  workflow.

## Migration Plan

No migration path. The tool is unreleased. Anyone with an old
invocation updates it once.

Rollback: revert the commit. The pre-change state has the `-tui`
flag with default `false` and the silent fallback contract.

For users on the previous default who want log mode after the flip:
`see --mode=log`.