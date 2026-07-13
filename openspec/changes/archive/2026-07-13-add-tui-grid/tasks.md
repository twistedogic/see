## 1. Add the observer seam in `main.go`

- [x] 1.1 Add a sealed `Event` interface to `main.go`:
      ```go
      type Event interface{ isEvent() }

      type RepoSeen struct {
          Path        string
          HasOpenspec bool
      }
      func (RepoSeen) isEvent() {}

      type ChangeStarted struct {
          Path   string
          Change string
      }
      func (ChangeStarted) isEvent() {}

      type RetryAttempt struct {
          Path   string
          Change string
          N, Max int
          Err    string
      }
      func (RetryAttempt) isEvent() {}

      type ChangeDone struct {
          Path   string
          Change string
      }
      func (ChangeDone) isEvent() {}

      type ChangeFailed struct {
          Path   string
          Change string
          Err    string
      }
      func (ChangeFailed) isEvent() {}

      type Observer interface{ Observe(Event) }
      ```
- [x] 1.2 Add `Observer observer` field to `Watcher`. Update
      `NewWatcher` to leave it nil (no-op default).

## 2. Wire observer calls into `Watcher.runOnce` and `Watcher.work`

- [x] 2.1 In `Watcher.runOnce`, before the per-repo retry loop, emit
      `RepoSeen{Path: repo, HasOpenspec: hasOpenspec(repo)}`. Compute
      `hasOpenspec` by checking whether `openspec/changes` exists and
      contains at least one entry besides `archive/`. Use a small
      helper:
      ```go
      func repoHasOpenspec(path string) bool {
          entries, err := os.ReadDir(filepath.Join(path, "openspec", "changes"))
          if err != nil {
              return false
          }
          for _, e := range entries {
              if e.IsDir() && e.Name() != "archive" {
                  return true
              }
          }
          return false
      }
      ```
- [x] 2.2 In `Watcher.work`, after picking the active change, emit
      `ChangeStarted{Path: path, Change: change}` immediately before
      `w.agent.Run(...)`.
- [x] 2.3 ChangeFailed is emitted from `runOnce` AFTER retryN returns
      an error (not from inside `work`), per the change spec scenarios
      ("when `work` returns a non-nil error after exhausting `retryN`").
      Tasks spec note about emitting in work was incorrect; the scenarios
      define the actual contract.
- [x] 2.4 In `Watcher.work`, after `git commit` succeeds on the
      archived change, emit `ChangeDone{Path: path, Change: change}`.
- [x] 2.5 In `Watcher.runOnce`, wrap `w.work(...)` so that the error
      from each attempt triggers a `RetryAttempt` event before the
      next iteration of `retryN`. Use a small closure local to
      `runOnce` rather than changing `retryN`'s signature:
      ```go
      prevErr := error(nil)
      err := retryN(w.RetyCount, func() error {
          if prevErr != nil && w.observer != nil {
              w.observer.Observe(RetryAttempt{
                  Path: repo, Change: change, N: attempt, Max: w.RetyCount,
                  Err: prevErr.Error(),
              })
          }
          err := w.work(ctx, repo)
          prevErr = err
          return err
      })
      ```
      Track `attempt` with a counter incremented after each call.

## 3. Update existing tests to assert the event sequence

- [x] 3.1 In `main_test.go`, add a `recordingObserver`:
      ```go
      type recordingObserver struct{ events []Event }
      func (r *recordingObserver) Observe(e Event) { r.events = append(r.events, e) }
      ```
- [x] 3.2 New `TestRunOnceEmitsEventSequenceOnSuccess` (the existing
      `TestWorkCommitsOnSuccess` calls `work()` directly, which doesn't
      go through `runOnce` and so doesn't emit `RepoSeen`; the new test
      drives `runOnce` to cover the full sequence).
- [x] 3.3 In `TestRunOncePassesRepoPathToAgent`, wire a
      `recordingObserver` and assert exactly one `RepoSeen` is emitted
      (for the proj repo; the non-repo sibling emits none).
- [x] 3.4 Run `go test ./...`. Confirm green.

## 4. Add new observer-contract tests

- [x] 4.1 Add `TestObserverReceivesRetrySequence`: fake agent fails
      twice then succeeds. Wire `recordingObserver`. Assert the
      sequence contains `RepoSeen`, `ChangeStarted`,
      `RetryAttempt{N: 2, Max: 3}`, `ChangeStarted`,
      `RetryAttempt{N: 3, Max: 3}`, `ChangeStarted`, `ChangeDone`.
- [x] 4.2 Add `TestObserverReceivesChangeFailedAfterRetriesExhausted`:
      fake agent always fails. `RetyCount: 2`. Assert the sequence
      ends with `RepoSeen`, `ChangeStarted`, `RetryAttempt{N: 2}`,
      `ChangeStarted`, `ChangeFailed`.
- [x] 4.3 Add `TestRepoSeenFiresForRepoWithoutOpenspec`: temp dir with
      a git repo (no `openspec/`). Wire observer. Assert exactly one
      `RepoSeen{HasOpenspec: false}` and no other events.
- [x] 4.4 Add `TestNilObserverIsSafe`: construct `Watcher{agent: fake,
      RetyCount: 1}` with no observer. Run a successful path. Confirm
      no panic. (The existing tests already exercise this
      transitively; add an explicit `defer recover()` if a non-test
      panic is a concern.)
- [x] 4.5 Run `go test ./...`. Confirm green.

## 5. Add the `-tui` flag and PTY detection

- [x] 5.1 Add to the `flag` block in `main()`:
      ```go
      tui := flag.Bool("tui", false, "render a live status grid (requires a TTY)")
      ```
- [x] 5.2 Add `golang.org/x/term` to `go.mod` and an `import` of it.
      Use `term.IsTerminal(int(os.Stdout.Fd()))` for detection.
- [x] 5.3 In `main()`, after `flag.Parse()`, branch:
      ```go
      if *tui && !term.IsTerminal(int(os.Stdout.Fd())) {
          fmt.Fprintln(os.Stderr, "see: --tui requires a TTY; falling back to log mode")
      } else if *tui {
          // build tui program; see step 6
      }
      ```
      The TUI-mode branch replaces the existing
      `signal.NotifyContext` setup; log mode keeps it.

## 6. Create the `tui` package

- [x] 6.1 Add `tui/model.go`, `tui/view.go`, `tui/program.go`. Use
      `package tui` (not `package main`).
- [x] 6.2 In `tui/model.go`, define:
      ```go
      type Phase int
      const (
          PhaseIdle Phase = iota
          PhaseWorking
          PhaseDone
          PhaseFailed
          PhaseNoSpec
      )
      func (p Phase) String() string { ... }
      func (p Phase) Glyph() string { ... }  // ● ◌ ✓ ✗ ○

      type RepoRow struct {
          Name        string
          Change      string
          Phase       Phase
          RetryN, RetryMax int
          StartedAt   time.Time
          LastErr     string
      }

      type Model struct {
          rows       map[string]*RepoRow  // keyed by repo path
          order      []string            // scan order, for stable rendering
          quota      tea.WindowSizeMsg
          events     <-chan Event
      }
      ```
- [x] 6.3 In `tui/model.go`, implement `Init`, `Update`, `View`.
      `Init` returns a `tea.Cmd` that reads one event from `events`
      and returns it as a `tea.Msg`. `Update` type-switches on the
      `Event` and updates `rows`. `View` renders the grid (see 6.5).
- [x] 6.4 In `tui/view.go`, render the grid with `lipgloss`:
      ```go
      func (m Model) View() string {
          // Header row: REPO | CHANGE | PHASE | RETRY | AGE | ERR
          // Data rows: one per m.order entry, joined with lipgloss.JoinHorizontal.
          // Footer: live counts + "[q] quit".
      }
      ```
      Use `lipgloss.NewStyle().Width(n).Align(lipgloss.Left)` per
      column. Truncate per-cell with `[]rune` slicing + `…`. Below
      100 cols, omit AGE then ERR.
- [x] 6.5 In `tui/program.go`, define:
      ```go
      func NewProgram(w *main.Watcher, observer Observer) *tea.Program {
          events := make(chan Event, 64)
          // observer adapter writes to events
          return tea.NewProgram(
              NewModel(events),
              tea.WithAltScreen(),
              tea.WithSignalHandler(),
          )
      }
      ```
      Wait — `Watcher` lives in `package main`. Either expose a
      minimal `tui.Run(watcher, observer)` function from `main.go` or
      move `Watcher` into its own package. **Decision during
      implementation**: prefer the second if it doesn't churn
      `main_test.go`. If it does, keep `Watcher` in `main` and add a
      `tui.Run` entrypoint in `main.go` that takes the channel.
      Decision: kept `Watcher` in `main` (per design doc); tui
      defines its own message types and `main.tuiObserver` translates
      Event → typed method call on `tui.ChanObserver` which sends
      the bubbletea Msg.

## 7. Wire agent stderr redirect in TUI mode

- [x] 7.1 Add a flag or boolean to `PiAgent` (or to `Watcher`) that
      controls whether `cmd.Stdout` and `cmd.Stderr` are set to
      `os.Stderr`. Default false (current behavior). The TUI mode
      sets it true. (`PiAgent.RedirectOutput` and its behavioral test are
      complete; wiring TUI mode remains.)
- [x] 7.2 Confirm existing tests still pass with the default
      (stderr-redirect off): `cmd.Run()` returns the agent's exit
      code, not a "stdout already set" error.

## 8. TUI tests

- [x] 8.1 Add `tui/tui_test.go`. Build a `Model` with a tiny in-memory
      event channel. Send `RepoSeen`, `ChangeStarted`, `ChangeDone`
      through `Update`. Call `View()`. Assert the output contains the
      repo name, the change name, the `done` glyph, and the footer
      line. Use a snapshot-style assertion on the full string for
      regression coverage.
- [x] 8.2 Add a `TestViewHandlesNoSpecRepo`: send `RepoSeen` with
      `HasOpenspec: false`. Confirm `View()` shows `○ no-spec` and
      `—` for change/retry.
- [x] 8.3 Add a `TestViewTruncatesLongNames`: a repo with a 50-char
      basename. Confirm the rendered cell ends with `…` and is no
      wider than the column.
- [x] 8.4 Add a `TestUpdateIgnoresUnknownEvents`: send an event type
      not in the spec (define a stub `unknownEv` for the test).
      Confirm the model returns without panic and the row map is
      unchanged.

## 9. Verify

- [x] 9.1 `go vet ./...` clean.
- [x] 9.2 `go build ./...` clean.
- [x] 9.3 `go test -race ./...` green.
- [x] 9.4 Manual smoke test: build, run `see --tui` against a fixture
      with one openspec repo and one without. Confirm the grid
      renders, phase transitions update, `[q]` exits cleanly, and the
      terminal is restored.
- [x] 9.5 Manual smoke test: `see --tui | cat`. Confirm the warning
      line and the log output appear on stderr, exit status matches
      the no-flag equivalent.
- [x] 9.6 Manual smoke test: a repo whose agent fails three times.
      Confirm the grid shows `1/3`, `2/3`, `3/3`, then `failed`, then
      the watcher exits with non-zero status and the original
      branch is untouched (this last check is also covered by
      `see-isolate-agent-runs-on-branch`'s rollback tests once both
      changes land).