## Why

`see` watches a directory of repositories and dispatches an agent against
each active OpenSpec change it finds. Today, the agent's stdout and stderr
are inherited from the parent process in log mode and squashed by the
alt-screen in TUI mode — both produce a stream of execution noise mixed
into the user's view of `see`'s orchestration events, and the user has
no way to find or follow the agent's output without scrolling terminal
scrollback.

The principle: `see` orchestrates; the agent narrates itself. The two
streams never share the user's view. Orchestration stays on the user's
terminal (or in the Bubbletea pane); execution is captured to a per-run
JSONL file in the OS cache directory, surfaced by path in both modes.

## What Changes

- `PiAgent.Run` writes the agent's combined stdout and stderr to a
  `.jsonl` file in the OS cache directory for each invocation. One file
  per `agent.Run` call — retries produce distinct files because each
  invocation gets a fresh timestamp.
- A new `LogPath` event variant in the sealed `Event` interface lets the
  TUI pane display the file path.
- Log mode (`--mode=log`) prints the path to stderr after each run via
  `log.Printf`.
- The `SEE_LOG_DIR` environment variable overrides the default cache
  directory.
- **BREAKING**: the `Agent` interface gains a `change` parameter so
  `PiAgent` can encode the change name in the filename; the
  `PiAgent.RedirectOutput` field is removed (capture is now invariant).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `watcher`: add requirements for per-run JSONL log capture, surfacing
  of the log path in TUI via the new `LogPath` event, surfacing of the
  log path in log mode via stderr, the `SEE_LOG_DIR` override, and the
  failure-to-capture contract (warn and drop, do not fail the run).

## Impact

- `main.go`:
  - `PiAgent.Run` creates the log dir, opens the per-run file, redirects
    stdout and stderr to it, and closes it on return.
  - New `logPathFor(repo, change string) (string, error)` helper computes
    the file path and ensures the directory exists.
  - `Watcher.work` invokes `w.agent.Run(ctx, repo, change, prompt)` and,
    after it returns, emits `LogPath` to the observer (when capture
    succeeded) so TUI mode can render it.
  - The log-mode branch in `main` and the `runTUI` path both emit
    `log.Printf("see: log → %s", path)` after a successful capture.
  - The `Event` interface gains `LogPath`. `PiAgent.RedirectOutput`
    removed. `Agent.Run` signature changes to `(ctx, path, change, prompt)`.
- `tui/`: `events.go` gains `LogPathMsg`; `program.go` `ChanObserver`
  gains a `LogPath` method; `model.go` and `view.go` render the path
  in the event list.
- `main_test.go`: existing `fakeAgent.Run` signature updated; new tests
  for capture, failure-to-capture, env override, and `LogPath` event
  emission.
- No new dependencies. The `Agent` interface change is technically
  breaking, but only `fakeAgent` (in `main_test.go`) implements it
  outside `main.go`; migration is a one-line update per call site.