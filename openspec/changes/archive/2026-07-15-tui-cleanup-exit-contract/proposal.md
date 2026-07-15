## Why

The current spec for the Terminal User Interface (TUI) quit behavior
has two scenarios (one for `q`, one for Signal-Interrupt (SIGINT))
with subtly different exit-status promises, and the `q` scenario's
promise ("status 0 always") does not match the actual behavior in
`--once` mode where the watcher can return an error before `q`
reaches the model. The cleanup chain — cancel the watcher context,
drain the watcher goroutine, close the JavaScript Object Notation
Lines (JSONL) event logger — is implied by the code but never pinned
in spec language. Per AGENTS.md, the spec is the source of truth;
drift between spec and code is a maintenance hazard and a future
contributor trap.

## What Changes

- Collapse the existing `q` and SIGINT exit scenarios into a single
  unified scenario that honestly describes the exit-status rule in
  `main()` and `runTUI` (non-zero iff `Watcher.Watch` returned a
  non-nil error before the watcher's context was cancelled, OR
  `tea.Program.Run` returned an error).
- Add a new Requirement "`--tui` drains the watcher goroutine and
  closes the JSONL event logger before exit" with a matching
  Scenario pinning the cleanup-chain contract.

No code, no test, no dependency changes. The implementation in
`runTUI` already satisfies both the unified exit-status rule and the
new cleanup-chain requirement; only the spec language is brought
into alignment.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `tui`: the exit-status rule for `q` and SIGINT is unified into
  one Scenario, and a new Requirement + Scenario pins the cleanup
  chain (cancel watcher context, drain watcher goroutine, close
  JSONL event logger) before the process exits.

## Impact

- `openspec/specs/tui/spec.md` — Scenario replacements and one new
  Requirement/Scenario pair.
- No source code, no test files, no dependencies, no flags.
- No runtime behavior change. The implementation already satisfies
  the contract; the spec is brought into alignment with the code.
