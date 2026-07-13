## 1. Extract `selectRunMode` and add table-driven test

- [x] 1.1 In `main.go`, define the `runMode` enum:
      ```go
      type runMode int
      const (
          modeUnknown runMode = iota
          modeLog
          modeTUI
      )
      ```
- [x] 1.2 Implement `selectRunMode(mode string, isTTY bool) (runMode,
      string)`:
      - `mode == "log"` → `modeLog`, `""` (TTY ignored).
      - `mode == "tui"` and `isTTY` → `modeTUI`, `""`.
      - `mode == "tui"` and `!isTTY` → `modeUnknown`,
        `"see: --mode=tui requires a TTY; rerun with --mode=log"`.
      - any other value (including `""`) → `modeUnknown`,
        ``fmt.Sprintf(`see: unknown --mode=%q (want: tui, log)`, mode)``.
- [x] 1.3 In `main_test.go`, add a table-driven
      `TestSelectRunMode` covering the six rows:
      `(log, true)`, `(log, false)`, `(tui, true)`,
      `(tui, false)`, `(foo, true)`, `("", true)`. Each row asserts
      both return values exactly. Run `go test ./...` and confirm
      green.

## 2. Replace `-tui` flag with `-mode` and rewrite `main()` dispatch

- [x] 2.1 In `main()`, replace
      `flag.Bool("tui", false, "render a live status grid (requires a TTY)")`
      with
      `flag.String("mode", "tui", "output mode (default \"tui\"); one of: tui, log")`.
      Drop the `tuiFlag` variable. The `-tui` flag is gone.
- [x] 2.2 Replace the inline dispatch with:
      ```go
      mode, msg := selectRunMode(*modeFlag, term.IsTerminal(int(os.Stdout.Fd())))
      if mode == modeUnknown {
          fmt.Fprintln(os.Stderr, msg)
          flag.Usage()
          os.Exit(2)
      }
      if mode == modeTUI {
          ctx, cancel := context.WithCancel(context.Background())
          defer cancel()
          if err := runTUI(ctx, &w, *pi, path); err != nil {
              log.Fatal(err)
          }
          return
      }
      // mode == modeLog
      ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
      defer cancel()
      if err := w.Watch(ctx, path); err != nil {
          log.Fatal(err)
      }
      ```
- [x] 2.3 Remove the now-unused inline `term.IsTerminal` check from
      `main()` (it lives inside `selectRunMode`). The
      `golang.org/x/term` import stays.
- [x] 2.4 Run `go vet ./...`, `go build ./...`, `go test -race ./...`.
      Confirm green. Existing watcher/tui tests must pass without
      modification.

## 3. Smoke-test the new flag surface

- [x] 3.1 Build the binary. Run `see` against a fixture with one
      openspec repo. Confirm the TUI renders (operator visually
      verifies; not a CI assertion).
- [x] 3.2 Run `see --mode=log` against the same fixture. Confirm
      `log.Printf` lines on stderr, no TUI, halt-on-first-failure
      intact.
- [x] 3.3 Run `see --mode=tui | cat` against the fixture. Confirm
      stderr contains the TTY-required message, exit status 2.
- [x] 3.4 Run `see --mode=foo` and `see --mode=`. Confirm both exit 2
      with the unknown-mode message and `flag.Usage()` output.
- [x] 3.5 Run `see --tui` (the old flag). Confirm Go's "flag provided
      but not defined" error and exit 2.

## 4. Sync the `tui` capability spec and archive

- [x] 4.1 Update `openspec/specs/tui/spec.md`:
      - Replace the existing `## ADDED Requirements` block with
        `## MODIFIED Requirements`, `## REMOVED Requirements`, and
        `## ADDED Requirements` blocks per the change spec delta
        (rename flag, flip default, invert non-TTY fallback, add
        unknown-mode and missing-TTY scenarios, add `--mode=log`
        parity scenario, remove the piped-fallback scenario).
- [x] 4.2 Run `openspec validate mode-flag-tui-default-2026-07-14`
      and confirm the change validates.
- [x] 4.3 Run `openspec archive mode-flag-tui-default-2026-07-14
      --yes` to sync the delta into `openspec/specs/tui/spec.md` and
      archive the change directory.
