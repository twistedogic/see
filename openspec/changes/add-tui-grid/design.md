## Context

`Watcher.work` and `Watcher.runOnce` are the only places where the
watcher's state transitions happen. Today they communicate state out of
the process via two channels:

1. `log.Printf` calls inside `Watcher.work` for human-readable progress.
2. A returned `error` from `Watcher.runOnce` for terminal failure.

There is no structured state representation, so a renderer has nothing
to subscribe to. To add a TUI without re-shaping the watcher, we need a
seam: a small `Observer` interface the watcher writes to at each phase
boundary, and an `Event` type per boundary so subscribers can pattern-
match.

The watcher stays sequential (the user explicitly chose sequential-first
for this change). The grid is observability, not control — its value is
making the existing halt-on-first-failure and retry behavior visible,
not changing those semantics.

The `see-isolate-agent-runs-on-branch` change is in flight and touches
`Watcher.work`'s git phases. The observer seam added here should sit
*outside* the git-phase machinery so both changes land independently:
the observer fires on logical events (started, done, failed), the git
phases are an internal implementation detail of `work()`.

## Goals / Non-Goals

**Goals:**
- A `--tui` flag renders a live grid of every scanned repo.
- Default behavior (no flag) is byte-for-byte unchanged: same `log.Printf`
  output, same exit semantics, same halt-on-first-failure contract.
- The grid shows every git repo in `wd/`, including ones without
  `openspec/`, so an operator can see "this repo isn't wired up" at a
  glance.
- PTY guard: `--tui` without a TTY (CI, piped output) warns and falls
  back to log mode.
- The watcher emits events through `Observer` at phase boundaries.
  Log mode never wires an observer; `nil` is the no-op default.
- TUI rendering is unit-testable via string snapshots of `View()`.
- Existing watcher tests stay green without modification to their
  assertions.

**Non-Goals:**
- Concurrent execution of multiple repos. Sequential first; a separate
  change can introduce a worker pool later and reuse the observer.
- Streaming `pi`'s `--mode json` JSONL output into a log pane. The
  agent's output is redirected to stderr in TUI mode so it doesn't
  corrupt the screen; a follow-up change can pipe it through a parser
  and render it. This change keeps that pipeline simple.
- Interactive controls beyond `[q]` quit. Pause, retry, focus — all
  useful, all out of scope for v1.
- Resizable columns, mouse interaction, theming. The grid is fixed-
  layout and uses a single Lip Gloss style set.
- Persisting TUI state across restarts. The watcher already auto-commits
  on success; the TUI is purely a render layer.

## Decisions

**Observer interface lives in `main.go`, not a new package.**

The interface is a tiny contract used only by `Watcher` and the TUI's
adapter function. Putting it in a separate `observer` package would
split a 30-line type definition across two directories for no benefit.

Alternatives considered:
- Dedicated `observer` package: one type per file, import cycle risk
  with `tui`, no upside.
- Generic observer with type parameter: Go generics are fine but the
  interface here is small and untyped events compose well; generics
  add ceremony without payoff.

**Sealed `Event` interface via `isEvent()` marker method.**

`type Event interface { isEvent() }` keeps the type set closed without
generics. Each concrete event is a small struct. The watcher calls
`observer.Observe(ev)` and the TUI type-switches in `Update`. Cheap,
explicit, no runtime registration.

Alternatives considered:
- `any`-typed events with a discriminator string: loses static checking,
  easier to typo.
- A separate `Observer` per event type (e.g. `RepoSeenObserver`,
  `ChangeStartedObserver`): combinatorial explosion for a 5-event set.

**TUI lives in a new `tui` package.**

Self-contained MVU. Easy to snapshot-test `View()`. Easy to swap the
renderer later (a `json` renderer, an HTML renderer) without touching
`main.go`'s watcher wiring.

Alternatives considered:
- All TUI code in `main.go`: bloats `main.go` past readability, mixes
  concerns, hard to test in isolation.
- TUI as a sub-package of `main`: not possible without `internal/`,
  and `internal/` is heavier than warranted for one package.

**Grid is hand-rolled with `lipgloss`, not Bubbles' `table` widget.**

The Bubbles `table` widget is built for navigation: arrow-key row
selection, sortable headers, alternating row styles. The TUI grid
here is read-only observability — selecting rows, sorting, paging are
all out of scope. A hand-rolled `lipgloss.JoinHorizontal` over
`lipgloss.NewStyle().Width(n).Align(lipgloss.Left)` per column is ~40
lines and matches the actual requirements.

Alternatives considered:
- Bubbles `table`: wrong shape of widget. Would either be configured
  to ignore its own interactivity (fighting the library) or expose
  controls that aren't in scope.
- Plain `text/tabwriter`: no styling, no per-cell color, weaker output
  than lipgloss for the same line count.

**PTY detection via `golang.org/x/term`.**

`golang.org/x/term.IsTerminal(int(fd))` is the standard Go answer.
Bubbletea depends on it transitively, so adding it directly is one
line in `go.mod`.

Alternatives considered:
- `os.Stat` on `/dev/tty`: portable, but `/dev/tty` doesn't exist on
  Windows and the project's test matrix should not depend on the
  agent's host OS.
- Bubbletea's own TTY detection: not exposed as a public helper. Roll
  our own via `x/term`.

**Six-column grid: REPO · CHANGE · PHASE · RETRY · AGE · ERR.**

Each column has a clear single value. Width budget at 120 cols:
REPO 24, CHANGE 30, PHASE 10, RETRY 8, AGE 8, ERR remaining. Below
100 cols, drop AGE then ERR (the columns least useful when the grid
is squashed). At 80 cols, just show REPO and PHASE.

**Bubble Tea owns signal handling in TUI mode.**

`tea.NewProgram(...).Run()` returns on SIGINT. The watcher's existing
`signal.NotifyContext` becomes a fallback for non-TUI mode. Two paths,
one flag-switch in `main()`.

Alternatives considered:
- Keep `signal.NotifyContext` and pass the context to the TUI:
  possible but bubbletea wants to own Ctrl+C cleanly, and double-
  handling signals races.

**Agent output redirected to stderr in TUI mode.**

`PiAgent.Run`'s `exec.Command` does not capture agent output today
(it inherits the parent's stdout/stderr). In TUI mode the parent
stdout is the alternate screen, so agent output would corrupt the
grid. Solution: when `--tui` is set, `PiAgent.Run` adds
`cmd.Stdout = os.Stderr; cmd.Stderr = os.Stderr`. The agent's JSONL
output still reaches the operator on stderr (visible if they switch
back from the alt screen, or piped to a log file), but never lands
inside the TUI's render.

Alternatives considered:
- Capture agent stdout in a goroutine and feed it to the TUI as a
  `tea.Msg`: real value, real complexity. Out of scope for this
  change; the design notes the path for a follow-up.
- Discard agent output in TUI mode: hides useful debugging info
  (especially useful for watching what `pi` did). Rejected.

## Risks / Trade-offs

- **The `pi` JSONL stream is unreadable inside the TUI** → Live
  visibility into the agent's reasoning is the natural next feature.
  Capture + parse + render as a log pane is a clear follow-up; this
  change leaves the redirect-to-stderr hook so the data is not lost.
- **The grid competes for terminal width with long repo or change
  names** → Truncate with ellipsis at the column width; the full
  names are still visible in `--no-tui` mode and in stderr. The
  truncation is per-cell and stable across renders (no jitter).
- **`tea.Program` panics corrupt the terminal** → Wrap the run in a
  `defer recover()` that prints a fallback message to stderr and
  calls `tea.ShowCursor` before returning. Cheap insurance.
- **Bubbletea's deps are larger than the rest of `see`'s surface**
  → Acceptable. The TUI is the largest UX change in the project's
  near future; one round of dependency additions is fine.
- **The observer seam invites other observers (metrics, structured
  logging, OpenTelemetry)** → That's the point. The interface is
  designed so adding a second observer is a struct field, not a code
  change. A future change can introduce a Prometheus observer without
  touching the watcher.
- **TUI mode in a non-TTY without the fallback fires today** → The
  fallback warns on stderr and proceeds with log mode. If users
  pipe `see --tui | cat` they get logs with a one-line warning. No
  silent corruption.
- **`recordingObserver` in tests grows unbounded** → Fine for tests;
  the observer is local to each test and is not retained.

## Migration Plan

No migration. `--tui` is opt-in. Users who don't pass it get the
existing behavior.

Rollback: removing the observer calls from `Watcher.work` and the
`--tui` branch from `main()` restores the pre-change behavior. The
`tui/` package can be deleted in one commit.