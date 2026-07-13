## Context

`see` is an orchestrator: it watches a directory of repos and dispatches
an agent against any active OpenSpec change it finds. Today, the
orchestration stream (`see`'s own `log.Printf` calls and Observer
events) and the execution stream (the agent's stdout and stderr) share
the user's terminal:

- **log mode**: `PiAgent.Run` leaves `cmd.Stdout` and `cmd.Stderr` nil,
  so the agent inherits the parent's stdio. Agent output interleaves
  with `see`'s log lines.
- **TUI mode**: `runTUI` sets `PiAgent.RedirectOutput = true`, which
  redirects both streams to `os.Stderr`. Bubbletea's `WithAltScreen()`
  hides stderr writes during a run; after exit, the squashed lines
  appear briefly above the restored terminal and scroll off.

In both modes, the agent's execution details are either noise mixed
into orchestration, or noise that's invisible while it's happening and
unfindable afterward. There is no way to follow what the agent did
without digging through terminal scrollback, and no persistent record
of a run.

This change separates the streams permanently: orchestration stays on
the user's terminal (or in the Bubbletea pane); execution is captured
to a per-run JSONL file in the OS cache directory, surfaced by path
in both modes.

## Goals / Non-Goals

**Goals:**

- `PiAgent.Run` writes the agent's combined stdout+stderr to a
  `.jsonl` file in the OS cache directory for each invocation.
- One file per `agent.Run` call. Retries produce distinct files.
- Both modes surface the log path so the user can find it:
  - TUI: a new `LogPath` event in the sealed `Event` interface.
  - log: `log.Printf("see: log → %s", path)` after the run.
- `SEE_LOG_DIR` environment variable overrides the default location.
- Capture failures warn and proceed without capture — capture is
  observability, not correctness, and must never break a run.
- `PiAgent.RedirectOutput` field is removed.

**Non-Goals:**

- Parsing or processing the captured logs. The `.jsonl` extension
  reflects that the agent runs in `--mode json`; we capture bytes, we
  do not interpret them.
- Log rotation, retention, or cleanup. Operational concern; future work.
- Streaming the agent's output back into the UI in real time. The
  path is enough for the user to `tail -f` if they want.
- A flag to disable capture. The whole point is to never let agent
  output flood the user's view.
- A general "logging" framework, structured fields, log levels for
  `see`'s own log lines. Out of scope.
- Migration of pre-existing log files. There are none — nothing is
  being deprecated.

## Decisions

### Decision 1: Location is `os.UserCacheDir()/see/logs/` with `SEE_LOG_DIR` override

Alternatives considered:

- **`os.UserCacheDir()` only, no override**: simplest. Power users who
  want a different location have no escape hatch.
- **`~/.local/share/see/logs/` (XDG_DATA_HOME)**: durable "evidence"
  semantics. The OS may purge cache dirs under pressure, which is the
  wrong default for something a user might want to inspect days later.
- **`~/.see/logs/`**: simple, but ignores OS conventions for cache vs
  data dirs.
- **`/tmp/see/`**: ephemeral; lost on reboot; users cannot inspect
  yesterday's runs.

Chosen: `os.UserCacheDir()/see/logs/` as the default, with the
`SEE_LOG_DIR` env var replacing the entire path (not just the leaf)
when set. Stdlib gives the OS-correct base directory; the env var is
the power-user escape hatch. The cache semantics match "trace, not
evidence" — if the user wants persistent history, they set
`SEE_LOG_DIR` to a stable location.

### Decision 2: Per-run granularity = per `agent.Run` invocation

Alternatives considered:

- **Per `Watcher.work` call (one file across retries)**: a single
  `agent.Run` call corresponds to one retry. Collapsing all retries
  into one file makes it harder to diff attempt 1's failure from
  attempt 2's success.
- **Per `runOnce` iteration (one file for all repos in a pass)**: loses
  the per-repo and per-change partition key.
- **Per `see` process (one file for the whole watch invocation)**: same
  problem, worse.

Chosen: one file per `agent.Run` call. The timestamp in the filename
distinguishes retries; the user can `ls -lt` to find the most recent
attempt. This matches the user's stated "per run per repo per change"
partitioning.

### Decision 3: TUI surface = new `LogPath` event in the sealed `Event` interface

Alternatives considered:

- **`log.Printf` to stderr from inside `work()`**: invisible during a
  TUI run because of `WithAltScreen()`. Same anti-pattern we're fixing
  for the agent.
- **A new Bubbletea pane that streams log lines into the UI**: a much
  bigger UI change; requires a goroutine that reads the file; the
  `tail -f` use case is already served by the path.
- **Embed the path in the existing `ChangeStarted` event payload**:
  couples two concerns (`ChangeStarted` is about starting a unit of
  work; the log path belongs to the run that already happened).

Chosen: a new `LogPath` event, emitted by `work()` after `agent.Run`
returns. Fits the existing sealed-interface pattern; type-switch arm
in `tuiObserver.Observe`; `tui.ChanObserver` gains a `LogPath` method
mirroring `RepoSeen`/`ChangeStarted`/etc. No special cases.

### Decision 4: Failure-to-capture = log warning, run proceeds without capture

Alternatives considered:

- **Fail the run when capture cannot be set up**: couples evidence
  (capture) to correctness (the run). A read-only repo dir or full disk
  would then break the orchestrator, which is the wrong tradeoff.
- **Retry capture on failure**: adds complexity; capture is observability,
  not a critical path.
- **Silent drop**: the user gets no signal that something is wrong.

Chosen: when `os.MkdirAll` of the log dir or `os.Create` of the file
fails, `PiAgent.Run` logs a warning to stderr, skips redirection
entirely, and runs the agent as if there were no capture. The run's
success or failure depends solely on the agent's exit status. A
warning keeps the failure discoverable in `see`'s log stream without
breaking the run.

### Decision 5: `PiAgent.RedirectOutput` field is removed

Alternatives considered:

- **Keep the field, set it to `true` everywhere**: dead config; future
  readers will wonder why it exists.
- **Rename to `LogCaptureDisabled` (default `false` = capture on)**: a
  knob no one asked for.

Chosen: remove the field. Capture is now an invariant of `PiAgent`;
there is no caller that wants capture off. The struct shrinks by one
field.

### Decision 6: `Agent` interface gains a `change` parameter (**BREAKING**)

Alternatives considered:

- **Keep signature, parse change from the prompt string**:
  `applyPrompt(change)` produces text that contains the change name in
  quotes, but parsing that is brittle and adds coupling.
- **Keep signature, omit change from the filename**: loses the "per
  change" partition the user asked for; a `tail` of the log dir can't
  be scoped to a change.
- **Pass change via a mutable field on `PiAgent`**: caller would have
  to set it before each `Run` call. Concurrency-unsafe if `PiAgent` is
  ever shared across goroutines; today it isn't, but the constraint
  would be invisible to readers.

Chosen: extend the interface to `Run(ctx, path, change, prompt)`. One
production call site (`main.go:work`) and one test impl
(`main_test.go:fakeAgent`) need updating — confirmed by grep. Marked
**BREAKING** in the proposal for transparency.

### Decision 7: Filename = `<repo-base>--<change>--<utc-ts>--<pid>.jsonl`

Alternatives considered:

- **Hash the full repo path** (`sha256[:12]`): unambiguous across
  different parent dirs with the same basename, but unreadable.
- **Sanitize the full path** (`/` and non-word chars → `_`): ugly and
  hard to scan visually.
- **Git remote URL**: stable across clones, but not always configured.

Chosen: `filepath.Base(repo)` for the repo part. Collision across
different parent dirs with the same basename is rare and produces a
last-writer-wins outcome rather than data loss — the previous run's
log is still on disk in its own file, the new run overwrites only if
the timestamp + PID also collide (which `work()`'s sequential retries
do not). Greppability wins over theoretical uniqueness.

The `<utc-ts>` is `time.Now().UTC().Format("20060102T150405")` (second
resolution). Same-second + same-PID collisions require two retries to
start within one second in the same process — `work()` does not do
this.

## Risks / Trade-offs

- **Log dir cannot be created** (read-only filesystem, full disk, no
  `$HOME`) → captured in Decision 4: warn and drop capture, run
  proceeds. Worst case the user sees agent output appear inline in
  log mode (no capture) and not at all in TUI mode (alt-screen hides
  it). Both are the pre-change behavior.
- **Disk fills up over time** → no mitigation in this change. Future
  work: rotation / retention. Document in follow-up tasks if it
  becomes a problem.
- **`SEE_LOG_DIR` set to an unwritable path** → same path as default
  failure: warn and drop capture.
- **`Agent.Run` signature change is breaking** → only `fakeAgent` is
  affected; one-line update in `main_test.go`.
- **Repo-basename collision** across different parent dirs → last
  writer wins; the prior file is preserved up to the overwrite moment.
  Rare; user accepted in Decision 7.
- **Same-second + same-PID retry collision** → `work()` does not start
  two retries within a second (sequential retries; the agent typically
  takes seconds to minutes). Risk is theoretical.

## Migration Plan

No migration. Existing `RedirectOutput: true` callers (only `runTUI`)
are updated as part of this change. There is no public API and no
external consumer of `PiAgent` or `Agent`.

## Open Questions

- Should the log path be rendered as an OSC 8 hyperlink in the TUI,
  so terminals that support it (`tmux`, `iTerm2`, modern Windows
  Terminal) make it clickable? Nice-to-have, out of scope for this
  change. The path is plain text; terminals that support hyperlinks
  can be configured to detect `file://` URIs or a custom scheme.
- Should `see` provide a `see logs <repo-or-change>` subcommand to
  list/tail logs? Operational concern; out of scope.