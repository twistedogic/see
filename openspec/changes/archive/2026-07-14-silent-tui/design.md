## Context

`see` is a batch driver that walks git repos under a working
directory, hands the first active openspec change in each to the
`pi` agent, retries on failure, and auto-commits on success. Two
output modes exist: `--mode=tui` renders a bubbletea grid in the
alternate screen; `--mode=log` writes `log.Printf` lines to stderr.
Today, both modes leak ~20 messages per batch to stdout or stderr
from `main.go` — every rollback step's warning, every progress
announcement, every cleanup hiccup. In TUI mode these messages
corrupt the alternate screen; in log mode they are the only
persistent trail of what the watcher did, and they vanish when the
process exits.

The change makes both modes silent and routes the full event stream
into a single batch-level JavaScript Object Notation Lines (JSONL)
file. The TUI becomes a pure view of that stream; the JSONL is the
record.

## Goals / Non-Goals

**Goals:**

- Eliminate every `log.Printf` and `fmt.Fprintln(os.Stderr, ...)`
  call that fires while a mode is active.
- Guarantee that every agent invocation produces a JSONL file
  (the existing per-invocation files, never the empty-string
  fallback).
- Capture every watcher event into a single batch-level JSONL that
  survives process exit and is queryable with `jq`.
- Reduce the TUI grid to a five-column, pure-state view with a
  single `⚠` glyph for warnings.
- Keep the pre-TUI-mode stderr exception (startup errors before
  `runTUI` is entered, e.g. `--mode=tui` invoked without a Terminal
  Typewriter (TTY)).

**Non-Goals:**

- Backwards compatibility with operators who currently `tail -f`
  stderr in `--mode=log`. They will see no terminal output and must
  switch to tailing the JSONL. This is documented as a breaking
  change in the proposal.
- Adding structured logging beyond what the event stream already
  carries. Every log line migrates to an event; no new log surface
  is introduced.
- Changing agent invocation, retry policy, git rollback, or merge
  semantics.
- Reformatting the per-invocation JSONL content (the agent's stdout
  is still captured byte-for-byte).

## Decisions

### Decision: Fail-fast on log directory, no graceful fallback

`logPathFor` currently falls back to "run without capture" when
`MkdirAll` fails. We remove that branch. Instead, `main()` calls
`ensureLogDir()` before the watcher starts; on failure it writes
one stderr line and exits `2`. `PiAgent.Run` then operates against a
directory that is guaranteed to exist and be writable.

Rationale: the fallback's only consumer was a stderr warning that
contradicts this change's "silent modes" goal. With the warning
removed, the fallback has no UI to surface itself through, and an
unguarded `os.Create` failure at agent-run time would now be a
silent loss of the agent's output — strictly worse than the
fail-fast contract.

Alternative considered: keep `logPathFor`'s fallback but route the
warning through the observer (so it shows up as a `Warning` event
in the JSONL). Rejected — silently dropping agent output is the
wrong default; an operator who can't write logs probably can't
operate the agent either. Fail fast and let them fix the directory.

### Decision: One batch-level JSONL, one eventLogger, fan-out in TUI

A single `eventLogger` type owns the batch-level file. It implements
`Observer` by JSON-encoding each event and writing one line per
event to the file. In TUI mode it holds an optional second observer
(the TUI's `ChanObserver`) and forwards each event to it after
writing to disk. In log mode the second observer is nil.

Rationale: makes the TUI a pure view of the event stream — same
code path, same event ordering, identical records. Single file per
batch (named `see--<utc-timestamp>--<pid>.jsonl`) means `jq` over
one file gives the operator the full timeline; no cross-file
correlation.

Alternative considered: write one JSONL per observer (one for TUI
state, one for log mode, one for crash diagnostics). Rejected —
three file sinks means three formats and three records; the user
asked for one source of truth.

### Decision: Warning and InfraError as new event types

`Warning{Path, Change, Msg}` carries per-repo cleanup warnings that
are not failures (a step inside rollback or completion hiccupped).
`InfraError{Where, Err}` carries watcher-level or TUI-level
failures (`Watcher.Watch` returned an error; bubbletea returned an
error from `Run`). Both flow through the same `eventLogger`.

Rationale: keeps the `Event` sealed-interface pattern intact. No
new code path for "side channels"; everything is an event the
observer fan-out handles uniformly.

Alternative considered: extend `ChangeFailed` and `RetryAttempt` to
carry warning text. Rejected — these events are tied to specific
phase boundaries (retry exhaustion, work returning an error);
warnings are not bounded the same way and would muddy the
contract.

### Decision: TUI grid loses the ERR column, gains a ⚠ glyph

The grid goes from six columns (`REPO | CHANGE | PHASE | RETRY | AGE
| ERR`) to five (`REPO | CHANGE | PHASE | RETRY | AGE`). The `⚠`
glyph is appended to the repo basename when `RepoRow.Warning` is
true. `RepoRow.LastErr` still gets populated by `RetryAttempt` and
`ChangeFailed` events (the JSONL captures it) but is never
rendered.

Rationale: the user explicitly asked to drop the message columns
because the JSONL is the source of truth for messages. A pure-state
grid is faster to scan, narrower (fits on small terminals without
threshold logic), and matches the "view of the event stream"
mental model. `Warning` becomes a state modifier — same lifecycle
as `Phase` — and shares column placement discipline.

`RepoRow.Warning` is cleared on the next `ChangeStarted` for that
row. The footer shows a `warning` counter alongside the phase
counters; the counter increments on `Warning` events and
decrements when a row's warning is cleared.

### Decision: InfraError renders as a banner between body and footer

A single `InfraErrorMsg` field on the bubbletea model (latest wins;
no history). Rendered as a `!`-prefixed full-width line between the
last row and the footer summary. Banner appears only when the
field is non-empty.

Rationale: rare, infra-level. Banner format matches the user's
non-row-scoped channel choice; doesn't crowd the grid; doesn't get
silently absorbed into a row's state.

### Decision: View column-width thresholds collapse to one

The current `View` has two thresholds (`showAge := width >= 80`,
`showErr := width >= 100`). Removing `ERR` deletes the second
threshold. A single `showAge := width >= 80` remains; below that
the grid shows `REPO | CHANGE | PHASE | RETRY` only.

Rationale: five columns already fit on 80-column terminals; the
extra complexity was load-bearing for the dropped column.

### Decision: Tests pin the new contract, the old fallback test dies

`TestPiAgentProceedsWhenLogDirCannotBeCreated` is deleted — its
assertion (`logPath == ""` on capture failure) contradicts the new
guarantee that `logPath` is non-empty. Replace with a test that
asserts `main` exits `2` when `SEE_LOG_DIR` cannot be created (the
new `ensureLogDir` contract).

Rationale: the deleted test pinned the fallback behaviour we are
explicitly removing; keeping it would either require reintroducing
the fallback or breaking the test. The new test pins the
fail-fast contract so the regression cannot return silently.

## Risks / Trade-offs

- **[Silent log mode for operators who script stderr]** → Mitigation:
  the proposal is marked BREAKING and the JSONL path is documented
  in the design. Operators gain a richer, machine-parseable record
  in exchange for losing the human-readable stderr stream.
- **[JSONL file grows unbounded across a long batch]** → Mitigation:
  one file per batch (PID + UTC timestamp in the name), not one
  per event. A typical batch is tens to hundreds of events; the
  file is small. If this becomes a problem, a later change can add
  a size cap or rotation.
- **[eventLogger write errors are silently dropped]** → The
  `json.Encoder.Encode` error is currently swallowed (best-effort).
  If the disk fills mid-batch we lose tail events. → Mitigation:
  log a one-shot `InfraError{Where: "event-log", Err: ...}` and
  stop. Out of scope for this change; tracked as a follow-up if
  it becomes a real problem.
- **[The new `Warning` event duplicates information already in the
  agent's per-invocation JSONL]** → True for warnings whose root
  cause was inside the agent run (e.g. agent exited non-zero but
  rollback hit a warning). The batch-level JSONL is a different
  audience (the operator, not a future debugger) and writes
  watcher-side messages that never appear in the agent's file.
  Acceptable duplication.
- **[Removing the "log dir unavailable" stderr warning loses an
  early-failure signal for `SEE_LOG_DIR=foo/bar` typos]** → The
  fail-fast exit-2 message is now the only signal. Mitigation: the
  startup error message names the directory it tried to create.

## Migration Plan

The change is non-additive to the wire-level behaviour of `see` —
operators running `--mode=tui` get a cleaner grid and operators
running `--mode=log` get no terminal output but a JSONL they can
tail. No data migration. No version bump required (no public API).
The breaking change (`--mode=log` silent) is documented in the
proposal.

Rollback: revert the commit. No persistent state is touched.

## Open Questions

- Should the batch-level JSONL also include a final
  `BatchEnded{Summary}` event with phase counts and wall-clock
  duration? The user did not ask for it; the JSONL already has the
  raw events needed to compute it. YAGNI.
- Should `ensureLogDir` print the directory it tried to create on
  failure? Useful for debugging `SEE_LOG_DIR` typos. The proposal
  says "single stderr line"; pinning the directory name in that
  line is cheap and probably worth doing.
