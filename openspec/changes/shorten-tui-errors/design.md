## Context

Watcher failures currently become strings at event emission: `RetryAttempt.Err`, `ChangeFailed.Err`, and `InfraError.Err` carry `err.Error()`. The event logger serializes those strings before forwarding the same in-memory event to the Terminal User Interface (TUI) adapter. The TUI stores the forwarded string as `LastErr`, collapses whitespace, and truncates it to the flexible ERROR column.

This is correct for persistence but poor for compact display. The dirty-working-tree diagnostic repeats the repository path already present in the event, so a typical forty-column ERROR cell truncates before reaching the actionable words `dirty`, `commit`, or `stash`. The full diagnostic must remain in JavaScript Object Notation Lines (JSONL), while the TUI needs a concise summary.

The dirty-tree failure is constructed independently in branch and worktree lane setup. A shared typed error can centralize both the full and concise representations.

## Goals / Non-Goals

**Goals:**

- Preserve full, unmodified diagnostics in the existing exported event `Err` fields and JSONL output.
- Let errors opt into a concise TUI summary without changing ordinary error behavior.
- Make dirty-working-tree failures actionable within the current ERROR column.
- Apply summary selection consistently to retry rows, failed rows, and watcher infrastructure errors.
- Preserve wrapped-error behavior through Go's standard `errors.As` traversal.

**Non-Goals:**

- Automatically summarize arbitrary error strings.
- Add a detail pane, row selection, wrapping, configuration, or a wider ERROR column.
- Add an exported summary field to the JSONL event schema.
- Change retry, rollback, lane, or dirty-tree detection semantics.
- Shorten warnings, which currently reach the TUI only as a glyph and do not populate `LastErr`.

## Decisions

### Use an optional summary interface on custom errors

A private interface with a `Summary() string` method identifies errors that provide concise display text. A shared helper uses `errors.As` to find that interface anywhere in the wrapping chain and returns its summary; errors that do not implement it fall back to `err.Error()`.

This keeps summarization explicit and semantic. It avoids brittle string parsing, path stripping, or a registry keyed by concrete error text. `errors.As` also means existing `%w` wrapping does not erase the summary.

Alternatives considered:

- Make every error implement a new common type: rejected because most errors already fit the ERROR column or have no meaningful hand-written summary.
- Parse and shorten strings in the TUI: rejected because presentation code cannot reliably distinguish paths from useful diagnostic content.
- Give the interface a TUI-specific method name: rejected because the concise representation is a general error summary even though the TUI is its first consumer.

### Represent dirty working trees with one custom error type

A private dirty-working-tree error stores the repository path. Its `Error()` method preserves the current full diagnostic, including the path and remediation. Its `Summary()` method returns `dirty working tree; commit or stash`, which places the cause and action before truncation.

Both existing dirty-tree guards return this type. The path remains available in the full diagnostic and in the event's separate `Path` field.

A custom type is preferred over merely reordering the current strings because it carries the semantic error category through wrapping and supplies both required representations from one source of truth.

### Carry the summary in an unexported event field

`RetryAttempt`, `ChangeFailed`, and `InfraError` retain their exported `Err string` fields for the full diagnostic and gain an unexported in-memory TUI error field. Event construction sets both fields from the same error: `Err` from `err.Error()` and the private field from the summary helper.

Go's JSON encoder ignores unexported fields. The event logger therefore writes the same JSONL schema and full `Err` value as before, then forwards the original event—with its private field still present—to the TUI adapter. The adapter sends the private summary to the TUI message, falling back to exported `Err` defensively when the private field is empty.

This avoids an exported `DisplayErr` field that would permanently expand the event schema with presentation-only duplication. It also avoids changing TUI package message types, model state, or rendering logic: those components continue to receive and render one string.

### Keep full errors as the fallback

For errors without `Summary()`, the summary helper returns `err.Error()`. Existing agent exit errors, Git command failures, condition failures, and Bubble Tea errors therefore render exactly as they do today.

The change improves only errors that deliberately opt in. No generic truncation or heuristic can hide useful context from previously supported errors.

## Risks / Trade-offs

- **[A summary is lost at an event construction site]** → Centralize selection in one helper and add regression tests for retry, final failure, and watcher infrastructure event paths.
- **[JSONL accidentally gains or substitutes concise text]** → Keep the summary field unexported and test the encoded event: exported `Err` contains the full path and no summary field is serialized.
- **[Wrapping prevents custom summary discovery]** → Use `errors.As`, not a direct type assertion, and test a `%w`-wrapped dirty-working-tree error.
- **[The concise text is still truncated on the minimum twenty-column ERROR tier]** → Put the cause first (`dirty working tree`) so even truncation remains identifying; the full remediation remains available at wider widths and the full diagnostic remains in JSONL.
- **[Private event state is less visible to external consumers]** → Intentional: events are internal main-package values, while JSONL is the external contract and must remain full and stable.
