## Context

The TUI footer in `tui/view.go` and `tui/model.go` hand-rolls two
distinct concerns:

1. The label: `tickerQuit = "[q] quit"` is a string constant
   referenced in six width-math call sites inside `renderFooter`,
   `tickerMiddleWidth`, and the narrow-terminal truncation branch.
   Five tests (`ticker_test.go:17,39,69,103`, `tui_test.go:40,732`)
   pin the literal `"[q] quit"` as a substring of the rendered
   footer.
2. The matcher: `msg.String() == "q" || msg.String() == "ctrl+c"`
   in `model.go` is a stringly-typed check. Adding a new binding
   means editing both literals, and a drift between them would not
   be caught by any test.

The two literals are a duplicated source of truth for one concept
("what keys quit the program and what label do we render"). The
broader codebase already uses `charm.land/bubbles/v2/table` for the
repository grid (the `add-tui-grid` and `migrate-tui-to-bubbles-v2`
changes), so adopting `charm.land/bubbles/v2/key` and
`charm.land/bubbles/v2/help` from the same module is on-pattern.

The footer is also a single physical line that combines the
activity marquee and the quit hint. The quit hint is
right-anchored; the activity marquee shares the row's middle width
with it. Splitting the footer into three rows (activity, separator,
help) gives the marquee the full terminal width and lets the help
bar use the bubbles package's built-in truncation with `…`.

## Goals / Non-Goals

**Goals:**

- The footer is a projection of a typed `keymap` struct. Adding a
  binding is a one-field change plus, at most, a `key.Matches` case
  in the `Update` switch.
- The hand-rolled `[q] quit` literal and the
  `msg.String() == "q" || msg.String() == "ctrl+c"` matcher leave
  the codebase.
- The footer uses `charm.land/bubbles/v2/help` for the help bar,
  matching the bubbles convention for key bindings.
- The footer renders three rows: activity, separator, help. The
  activity row keeps its existing marquee behavior. The separator
  is a styled `─` rule. The help row is `help.ShortHelpView` (or
  `FullHelpView` when toggled).
- The `?` key toggles the help bar between the short and full
  renderings. The toggle starts in the short rendering.
- The five existing tests that pin `[q] quit` are updated to pin
  the new shape (no broken-string regressions). All other existing
  tests stay green without modification.
- The `tui` capability spec accurately reflects the three-row
  footer, the help-bar source, and the `?` toggle.

**Non-Goals:**

- New key bindings beyond `q` (quit) and `?` (help toggle). A
  speculative `r` (refresh) is explicitly out of scope and is not
  declared in the keymap.
- Restyling the repository grid. Column widths, phase colors,
  priority ordering, and the `viewportCap` are unchanged.
- Changing watcher semantics, agent invocation, retry policy, JSONL
  schema, or any CLI flag.
- A custom help-bar widget. The `charm.land/bubbles/v2/help`
  package is used as-shipped (with the default dark styles).
- A `FullHelp` content change. The full view reuses the same two
  bindings in two columns; it is not a separate "press `?` for full
  help" screen with extra entries.

## Decisions

**Use `charm.land/bubbles/v2/key` and `charm.land/bubbles/v2/help`
as shipped.**

`bubbles` is already a dependency (the `table` widget is used by
the repository grid). `key` and `help` are sub-packages of the same
module — `go.mod` does not change. The package exposes a
`KeyMap` interface (`ShortHelp()` and `FullHelp()`) that the help
package's `View(k KeyMap)` dispatches into. Implementing the
interface is two methods, not a framework.

Alternatives considered:

- Hand-roll a `keymap` struct but render the help bar manually:
  duplicates the bubbles separator-and-ellipsis logic and leaves
  the codebase with a half-bubbles, half-hand-rolled footer. The
  whole point of the move is to stop hand-rolling.
- Vendor a different keymap library (`tcell`-style): bubbles is
  the codebase's TUI framework. Adding a second TUI library is
  worse than the problem.

**Two bindings: `q` (quit) and `?` (help toggle).**

`Quit` declares `WithKeys("q", "ctrl+c")` and `WithHelp("q", "quit")`.
`Help` declares `WithKeys("?")` and `WithHelp("?", "help")`. The
matcher and the label are kept in the same struct field; `key.Matches`
checks `Keys`, and the help bar reads `Help`. There is no parallel
literal to drift.

`r` (refresh) was considered and dropped: nothing in the
implementation actually triggers a rescan on `r` today, and
speculative bindings are a known anti-pattern (per `AGENTS.md`:
"no unrequested abstractions"). Adding it would be a one-line
edit later, so deferring is free.

**The footer is three rows.**

The activity row keeps its existing marquee (`tickerMiddleWidth`,
`displayWindow`, `marqueeCmd`, `marqueeTickMsg`,
`tickerOverflow`). The only change is that the right edge of the
marquee window is no longer the help bar — it is the terminal's
right edge — so the `tickerMiddleWidth` math loses the
`- runewidth.StringWidth(tickerQuit)` term and becomes
`m.width - runewidth.StringWidth(tickerPrefix) - 1`.

The separator row is a styled `─` repeated to `m.width` cells. The
color matches the help package's default separator tone
(`#3C3C3C` dark) so the rule visually attaches to the help row.

The help row is `m.help.View(m.keys)`, which dispatches to
`ShortHelpView` or `FullHelpView` based on `m.help.ShowAll`. The
`help` package's `SetWidth(m.width)` is called in the
`WindowSizeMsg` branch, so the built-in `…` truncation handles
narrow terminals.

`fixedLines` in `renderContent` becomes a computed value:

```go
helpLines := 1
if m.help.ShowAll {
    helpLines = 2
}
fixedLines := 3 + 1 + helpLines  // summary, header, activity, sep, help
if m.infraErr != "" {
    fixedLines++
}
```

Alternatives considered:

- Put the activity and help on the same row with a narrower
  marquee: the original design, but the marquee loses 8–21 columns
  to the help bar depending on the number of bindings. The
  full-width activity row is a better use of horizontal space.
- Make the separator optional (operator preference): a config
  field is out of scope. The separator is always rendered.
- Use `lipgloss` to draw a boxed separator instead of a plain
  `─`: lipgloss boxing uses different cell widths in some
  terminals and is not worth the correctness risk for a one-pixel
  rule.

**`help.ShowAll` starts `false` (the short view).**

The bubbles default. Operators see `q quit • ? help` on first
paint. `?` toggles to the two-line full view. Toggling back
restores the short view and grows the viewport by one row.

## Risks / Trade-offs

- **Footer height grows from one row to three** → Terminals that
  previously fit `viewportCap = 10` rows at 24-line height still
  fit 10 (the cap is unchanged; 24 − 5 = 19, cap clips at 10).
  Tighter terminals (≤ 14 lines) lose one row from the table
  viewport. At a 12-line terminal, the table drops from 9 rows to
  8. Acceptable cost for the help-bar clarity and full-width
  marquee.
- **The visible help text changes from `[q] quit` to
  `q quit • ? help` (with bubbles' gray styling and `•`
  separator)** → Operators accustomed to the bracket style see a
  different footer. The help package's styles are configurable, so
  a follow-up could re-introduce brackets via
  `WithHelp("[q]", "quit")` and zero-styled `Styles` if the
  operator feedback demands it. The proposal explicitly accepts
  this break.
- **Five existing tests pin the literal `[q] quit`** → Each test
  needs a mechanical edit to pin the new shape. The edits are
  listed in the `tasks.md`. The semantic tests
  (`TestUpdateQuitsOnQ`, the agent-output scenario, the priority
  viewport scenarios) are unchanged.
- **The `?` key is captured by bubbletea and never reaches the
  agent** → No conflict with the `pi ›` activity stream. Confirmed
  by the bubbles architecture: `tea.KeyPressMsg` is handled
  inside the model's `Update` and the agent is a separate
  subprocess. No extra wiring.
- **`m.help.SetWidth` is the only new state in `Model`** → The
  help package is otherwise stateless from our perspective. The
  `WindowSizeMsg` handler already updates `m.width`; calling
  `m.help.SetWidth(m.width)` next to it is a one-line addition.
- **Spec churn is larger than the code change** → One
  `MODIFIED Requirements` block for the grid requirement (full
  21-scenario block, one sentence changed), one
  `MODIFIED Requirements` block for the ticker requirement (full
  8-scenario block, three sentences changed and one new scenario),
  and one `ADDED Requirements` block for the typed keymap
  contract. The capability stays accurate, which is the point of
  spec-driven workflow.

## Migration Plan

No migration. The change is internal to the `tui/` package; no
CLI flag, configuration field, or event schema changes. Existing
invocations of `see --mode=tui` get the new footer on next run
without operator action.

Rollback: revert the merge. The pre-change state has the
single-row footer with the `[q] quit` literal; restoring it
returns the codebase to the current spec.

For operators who prefer the bracket look after rollout: file a
follow-up change to override `WithHelp("[q]", "quit")` and zero
the `Styles` in the help package, so the rendered text is
byte-identical to the pre-change footer while the keymap struct
stays in place.
