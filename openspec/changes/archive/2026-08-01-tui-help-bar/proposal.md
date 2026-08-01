## Why

The TUI footer currently hand-rolls its quit hint: `tickerQuit = "[q] quit"` is
a string constant in `tui/view.go` (used in six width-math call sites) and
`msg.String() == "q" || msg.String() == "ctrl+c"` is a stringly-typed matcher
in `tui/model.go`. The two literals are a duplicated source of truth — adding
a new binding means editing both, and a label/matcher drift would not be
caught by any test. The single-line footer also makes the activity marquee
share width with the quit hint, so the activity window shrinks by the
help chrome on every render.

`charm.land/bubbles` already ships `key.Binding` and `help.Model` for exactly
this. Adopting them turns the footer into a projection of a typed `keymap`
struct, makes new bindings a one-field change, and gives us a conventional
bubbles help bar (separator, gray styling, `…` truncation, optional full
help view) for free. The TUI is the only bubbles consumer in this codebase,
so the convention has nothing to argue with.

## What Changes

- Add a `keymap` type in `tui/` with `Quit` and `Help` `key.Binding` fields
  and the `ShortHelp` / `FullHelp` methods required by
  `charm.land/bubbles/v2/help.KeyMap`.
- Replace the hand-rolled quit matcher in `tui/model.go` with a
  `key.Matches` switch over the `keymap` (`q` / `ctrl+c` for `Quit`,
  `?` for `Help`).
- Replace the hand-rolled `[q] quit` literal in `tui/view.go` with
  `help.ShortHelpView(m.keys.ShortHelp())`. The `tickerQuit` constant and
  its six width-math call sites leave.
- Split the single-line footer into three rows: the existing `pi › <activity>`
  marquee on top, a styled `─` separator in the middle, and the bubbles help
  bar on the bottom. The activity row keeps its full marquee behavior; the
  separator and help rows are static.
- The `?` key toggles `help.Model.ShowAll` between the short one-line help
  (`q quit • ? help`) and the full multi-column help (`q quit / ? help`).
  `ShowAll` starts `false` (the bubbles default).
- The `Model` gains `help help.Model` and `keys keymap` fields. `NewModel`
  initializes them. The `WindowSizeMsg` handler calls
  `m.help.SetWidth(m.width)` so the help package can truncate with `…` on
  narrow terminals.
- `renderContent`'s `fixedLines` becomes a small computed value: the
  constant `3` is replaced by `3 + 1 + helpLines` where `helpLines` is
  `1` when `ShowAll` is `false` and `2` when it is `true` (the full help
  view stacks key and desc per column). The infrastructure-error
  `+1` branch is unchanged.
- Five existing tests that pin the literal `[q] quit` are rewritten to
  pin the new shape (the help line on the last row, the activity line
  on the row above the separator, narrow-width truncation with `…`).

No CLI flag, no watcher behavior, no agent invocation, no JSONL schema,
and no event-stream change.

**BREAKING**: the rendered footer is now three rows tall where it was one,
so terminals that previously fit the cap of ten repository rows will fit
nine. The cap is unchanged (still `viewportCap = 10`); the budget is
smaller. The visible-cell help text is also visibly different (gray
bubbles style with `•` separators instead of plain `[q] quit`). Both
breaks are intentional and confined to `--mode=tui`.

## Capabilities

### New Capabilities

(none — the change modifies the existing `tui` capability.)

### Modified Capabilities

- `tui`: the footer is no longer a single row; it is a three-row block
  (activity marquee, separator, bubbles help bar) sourced from a typed
  `keymap` struct. The `?` key toggles the help bar between short and
  full renderings. The existing `q` / `ctrl+c` quit behavior and the
  one-line activity marquee are preserved. The grid layout, viewport
  cap, and column rules are unchanged.

## Impact

- `tui/model.go`:
  - Add `help help.Model` and `keys keymap` fields to `Model`.
  - Initialize them in `NewModel` (`help.New()`, `defaultKeymap()`).
  - In the `tea.KeyPressMsg` branch, replace the `msg.String()` chain
    with a `switch key.Matches(msg, m.keys.Quit) { case true: tea.Quit
    }` and a second case for `m.keys.Help` that toggles
    `m.help.ShowAll` and returns `nil` (the help package is otherwise
    stateless).
  - In the `tea.WindowSizeMsg` branch, add
    `m.help.SetWidth(msg.Width)` before the existing marquee reset.
- `tui/view.go`:
  - Remove `tickerQuit` and the six sites that read it.
  - Rewrite `renderFooter` to return
    `m.renderActivityLine() + "\n" + m.renderSeparatorLine() + "\n" +
    m.help.ShortHelpView(m.keys.ShortHelp())` (or `FullHelpView` when
    `ShowAll` is true; both come from `m.help.View(m.keys)`).
  - The marquee code (`renderActivityLine`, `displayWindow`,
    `marqueeCmd`, `marqueeTickMsg`, `tickerMiddleWidth`,
    `tickerOverflow`) is preserved — only the right-edge width math
    changes because the help bar no longer shares the row.
  - Update `renderContent`'s `fixedLines` to a computed expression
    (see *What Changes*).
- `tui/keys.go` (new file): the `keymap` struct, `defaultKeymap`,
  `ShortHelp`, and `FullHelp`. No `Update` logic lives here.
- `tui/ticker_test.go`:
  - `TestTickerLifecycleAndLatestActivity`: assert "pi › …" on the
    second-to-last row and the help text on the last row instead of
    both on the last row.
  - `TestTickerFooterIsOneLine…`: rename to `TestTickerFooterIsThreeLines…`;
    assert no `\r\n` within each row and that all three rows exist.
  - `TestTickerMarqueeOnlyRunsForOverflow…`: assert the marquee moves
    only the activity row, not the separator or the help row.
  - `TestTickerResetOnNewActivityAndResize` and
    `TestTickerLoopContainsVisibleGap` are unchanged.
- `tui/tui_test.go`:
  - `TestFooterNoLongerShowsPhaseCounts`: update to assert the help
    bar is on the last row and does not contain phase counts.
  - `TestUpdateQuitsOnQ` and other semantic tests are unchanged —
    they assert on commands and event sequences, not on the
    rendered footer string.
- `openspec/specs/tui/spec.md`:
  - `## MODIFIED Requirements` for the footer / ticker requirement
    (one row → three rows, help bar replaces the `[q] quit` literal,
    `?` toggles short/full help).
  - `## MODIFIED Requirements` for the "renders a live status grid"
    requirement's one-sentence footer clause.
  - `## ADDED Requirements` for: the typed `keymap` source-of-truth
    contract; the `?` toggle; the help-bar truncation behavior.
  - `## MODIFIED Requirements` scenarios for narrow-terminal and
    overflowing-activity behavior (chrome is on its own line now).
- No dependency changes — `charm.land/bubbles/v2` is already in
  `go.mod` (it brings `key` and `help` as sub-packages of the same
  module).
- No changes to `main.go`, the watcher, the observer seam, the
  `pi` agent invocation, the JSONL event log, or any CLI flag.
