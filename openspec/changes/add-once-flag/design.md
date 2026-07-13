## Context

`Watcher.Watch` runs `runOnce` in a tight `for { ... }` loop until either the
context is cancelled or `runOnce` returns an error. Today, there is no way to
ask it to stop after a single pass. The two callers of `Watch` are `main`
(in log mode) and `runTUI` (in TUI mode). Both currently treat "exit" as
"ctx was cancelled by the other side":

- log mode: signal handler cancels ctx → `Watch` returns.
- TUI mode: user presses `q` → `prog.Run()` returns → `main` cancels ctx →
  `Watch` returns.

There is no path from "watcher exits on its own" → "TUI exits". Adding
`--once` in TUI mode requires closing that loop.

## Goals / Non-Goals

**Goals:**

- `see --once --mode=log` exits cleanly after one pass.
- `see --once --mode=tui` exits cleanly after one pass, including the TUI.
- The default (no `--once`) behavior is unchanged.
- No new abstractions: the `Once` knob is one struct field; the TUI wiring is
  one extra goroutine in `runTUI`.
- Add tests for `Watcher.Watch` with `Once` true and false — closes a gap
  where `Watch` itself is currently untested.

**Non-Goals:**

- A general "run N times" mode (`--max-passes=N`). Add only if a use case
  appears; right now `Once` is enough.
- Changing how `runOnce` itself behaves. The once-flag is purely a loop
  concern, owned by `Watch`.
- Documenting `--once` in user-facing docs beyond the flag's own help string.
  The project has no manual.

## Decisions

### Decision 1: `Once` is a field on `Watcher`, not a separate method

Alternatives considered:

- **Separate `WatchOnce` method on `Watcher`**: would force a rename of the
  internal `runOnce` to `RunOnce` (exported) so main could call it directly,
  churning ~13 call sites in `main_test.go`. Adds nothing the field approach
  doesn't already give us.
- **Pass `once` as a parameter to `Watch`**: changes the signature of a
  function that tests don't even call today, and conflicts with the way
  `Watcher` already carries configuration (`RetyCount`).

The field approach mirrors `RetyCount`: an input knob on the Watcher value,
read inside `Watch`. No signature churn, no rename churn.

### Decision 2: Wire watcher-exit → prog-quit with one extra goroutine in `runTUI`

Today, `runTUI` has exactly one goroutine: the watcher. The flow on user
quit is `prog.Run() returns → cancel() → watcher exits`. To support
`--once`, we need the reverse: `watcher exits → prog quits`. The cheapest
way is a second goroutine that calls `prog.Quit()` once the watcher has
returned.

This goroutine runs unconditionally, not gated on `Once`. In the loop-mode
case the watcher only exits after `cancel()` runs (which runs after
`prog.Run()` returns), so the second goroutine fires on an already-exited
program; bubbletea's `Program.Quit()` early-returns when `p.started` is
false, so the call is a safe no-op. Symmetric code path, no special cases.

### Decision 3: Do not guard against the `prog.Run()`-not-started race

`w.Watch` with `--once` on a fixture with zero repos could in principle
return before `prog.Run()` is called, causing the second goroutine to
call `prog.Quit()` on a not-yet-started program. `Quit()` handles this
case (early return), but the program will then sit open waiting for
user input. In practice, `runOnce` does at least one `os.ReadDir` plus
a `git rev-parse HEAD` per repo; it cannot complete in the microseconds
between `tui.New()` and `prog.Run()`. A regression test (`--once` against
a directory with no repos exits 0) would catch it if it ever bites. No
explicit guard needed.

## Risks / Trade-offs

- **Race: watcher exits before `prog.Run()` starts (see Decision 3).**
  Mitigation: regression test against a directory with no repositories.
- **`Once` field is a one-off knob that 99% of `Watcher` users don't set.**
  Acceptable: it's the same shape as `RetyCount` (also zero-default), and
  the alternative — a separate method — would rename internal symbols.
- **Spec requirement becomes the only thing documenting the loop.** The
  watcher spec previously said nothing about `Watch` itself; now it does.
  Worth it because `Watch` is now configurable and should be tested.