## 1. Add the typed `keymap` and `help` fields

- [x] 1.1 Create `tui/keys.go` with a `keymap` struct that holds
      `Quit` and `Help` `key.Binding` fields and implements
      `ShortHelp() []key.Binding` and `FullHelp() [][]key.Binding`
      (the `charm.land/bubbles/v2/help.KeyMap` interface).
- [x] 1.2 Add a `defaultKeymap()` constructor. `Quit` declares
      `WithKeys("q", "ctrl+c")` and `WithHelp("q", "quit")`.
      `Help` declares `WithKeys("?")` and `WithHelp("?", "help")`.
- [x] 1.3 In `tui/model.go`, add `help help.Model` and
      `keys keymap` fields to `Model`. Initialize them in
      `NewModel` via `help.New()` and `defaultKeymap()`.
- [x] 1.4 Add a unit test in `tui/keys_test.go` that pins the
      keymap: `key.Matches` on a `q` and `ctrl+c`
      `tea.KeyPressMsg` returns true for `keys.Quit`;
      `key.Matches` on `?` returns true for `keys.Help`; the
      help strings are `q quit` and `? help`. Run
      `go test -timeout 30s ./tui/...` and confirm green.

## 2. Wire the `Update` switch to the keymap

- [x] 2.1 In `tui/model.go`, replace the `msg.String() == "q" ||
      msg.String() == "ctrl+c"` branch in the `KeyPressMsg`
      case with:
      ```go
      switch {
      case key.Matches(msg, m.keys.Quit):
          return m, tea.Quit
      case key.Matches(msg, m.keys.Help):
          m.help.ShowAll = !m.help.ShowAll
          return m, nil
      }
      ```
- [x] 2.2 In the `tea.WindowSizeMsg` branch, add
      `m.help.SetWidth(msg.Width)` next to the existing
      `m.width = msg.Width` assignment.
- [x] 2.3 Confirm `TestUpdateQuitsOnQ` in `tui/tui_test.go`
      still passes (it asserts on the `tea.Quit` command, which
      the refactor preserves). Run
      `go test -timeout 30s ./tui/...` and confirm green.

## 3. Replace the hand-rolled quit hint with the help bar

- [x] 3.1 In `tui/view.go`, delete the `tickerQuit` constant and
      all six call sites that read it (`tickerMiddleWidth`, the
      `width <= quitWidth` branch in `renderFooter`, and the
      footer composition lines).
- [x] 3.2 Rewrite `renderFooter` to return three rows joined by
      `\n`:
      ```go
      return m.renderActivityLine() + "\n" +
          m.renderSeparatorLine() + "\n" +
          m.help.View(m.keys)
      ```
      `m.help.View(m.keys)` dispatches to `ShortHelpView` or
      `FullHelpView` based on `m.help.ShowAll`.
- [x] 3.3 Add `renderSeparatorLine()`: a styled `─` repeated to
      `m.width` cells. Use the help package's default separator
      tone (`lipgloss.Color("#3C3C3C")` dark). At zero or
      negative width, return an empty string.
- [x] 3.4 Update `tickerMiddleWidth`: drop the
      `- runewidth.StringWidth(tickerQuit)` term so the activity
      row uses the full terminal width minus the `pi ›` prefix
      and one space. The marquee math (`displayWindow`,
      `tickerOverflow`, `marqueeCmd`, `marqueeTickMsg`) is
      unchanged.
- [x] 3.5 In `renderContent`, replace the `fixedLines := 3`
      literal with:
      ```go
      helpLines := 1
      if m.help.ShowAll {
          helpLines = 2
      }
      fixedLines := 3 + 1 + helpLines
      if m.infraErr != "" {
          fixedLines++
      }
      ```
- [x] 3.6 Run `go build ./...` and `go test -timeout 30s
      ./tui/...`. Expect red: five existing tests pin the literal
      `[q] quit`. Confirm the build passes and only the expected
      five tests fail.

## 4. Update the five tests that pin `[q] quit]`

- [x] 4.1 In `tui/ticker_test.go`, rewrite
      `TestTickerLifecycleAndLatestActivity`: assert `pi › waiting`
      on the second-to-last line and the help text on the last
      line.
- [x] 4.2 In `tui/ticker_test.go`, rename
      `TestTickerFooterIsOneLineAndKeepsQuitHintOnNarrowWidths`
      to `TestTickerFooterIsThreeLinesAndHelpTruncatesOnNarrowWidths`.
      Assert no `\r\n` within any of the three rows, that all
      three rows exist, and that the help row contains a trailing
      `…` when the help text exceeds the terminal width.
- [x] 4.3 In `tui/ticker_test.go`, update
      `TestTickerMarqueeOnlyRunsForOverflowAndMovesByDisplayCell`
      to assert the marquee moves only the activity row, not the
      separator or help row.
- [x] 4.4 In `tui/tui_test.go`, update
      `TestFooterNoLongerShowsPhaseCounts` to assert the help
      bar is on the last line and does not contain phase counts.
- [x] 4.5 Run `go test -timeout 30s ./tui/...`. Expect green.

## 5. Add coverage for the new `?` toggle and keymap contract

- [x] 5.1 In `tui/tui_test.go`, add `TestHelpToggleFlipsShortAndFull`:
      construct a model at width 80, assert the rendered help row
      contains `q quit` and `? help` separated by ` • `; press
      `?`; assert the rendered help row now contains a newline
      (two-line full view) and the same bindings in two columns;
      press `?` again; assert the short view returns.
- [x] 5.2 In `tui/tui_test.go`, add
      `TestHelpBarIsProjectionOfKeymap`: assert that the help
      row text equals `m.help.View(m.keys)` and that no
      `tickerQuit`-style string literal remains in `tui/view.go`
      (`grep -n tickerQuit tui/view.go` returns nothing).
- [x] 5.3 In `tui/tui_test.go`, add `TestHelpWidthTracksWindowSize`:
      send `WindowSizeMsg{Width: 5}` and assert the help row
      truncates with `…` (the help package's behavior); send
      `WindowSizeMsg{Width: 80}` and assert the full help text
      fits.
- [x] 5.4 Run `go test -timeout 30s ./tui/...`. Expect green.

## 6. Final validation and review

- [x] 6.1 Run `go vet ./...`, `go build ./...`,
      `go test -race -timeout 30s ./...`. Expect green.
- [x] 6.2 Run `openspec validate tui-help-bar`. Expect green; the
      change is complete with `proposal.md`, `design.md`,
      `specs/tui/spec.md`, and `tasks.md` populated.
- [x] 6.3 Manual smoke check (not CI): build the binary, run
      `see --mode=tui` against a fixture repo with one openspec
      change. Visually confirm: footer is three rows, separator
      is visible, help row shows `q quit • ? help`, pressing `?`
      toggles to the two-line full view, `q` quits cleanly.
- [x] 6.4 Run `openspec archive tui-help-bar --yes` to sync the
      delta into `openspec/specs/tui/spec.md` and archive the
      change directory.
