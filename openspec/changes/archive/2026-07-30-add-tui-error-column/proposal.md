## Why

The TUI tells an operator *that* a repository failed — the `✗ failed`
glyph lights up the PHASE cell and the row climbs to priority class 1
— but not *why*. The failure reason is already carried into the model
(`RepoRow.LastErr`, set by `ChangeFailedMsg` and `RetryAttemptMsg`,
cleared by `ChangeStarted`/`ChangeDone`), yet `renderRow` never reads
it. A `ponytail:` comment in `tui/view.go` records the deliberate
trade: "the message itself lives in the JSONL (the operator can jq
for it)."

That trade rejects the TUI's reason for existing. The grid is a
glanceable dashboard; "glanceable" means the operator learns the
failure cause without leaving the terminal. Today they must copy the
repo name, open a shell, and `jq` the batch-level JavaScript Object
Notation Lines (JSONL) file to recover a string the model already
holds. On the first failed repo the watcher halts the whole batch, so
that round trip is the single most common operator action after a
failure — and it is pure friction for data already in memory.

## What Changes

- The TUI grid gains an **ERROR** column, rendered as the last
  column, that shows the row's current `LastErr`. For a `failed` row
  that is exactly the error that caused the failed state; for a
  `working` row mid-retry it is the previous attempt's error.
- The column is a **flex** column: its width is the terminal width
  minus the sum of the fixed columns active at that width, so it can
  never push a row past one physical line. It is shown only when that
  remainder is at least `errMinWidth` (`20`) columns — i.e. on
  terminals `>= 100` columns, given AGE already claims the `>= 80`
  tier. Narrower terminals keep today's `⚠` glyph + JSONL behavior
  unchanged.
- The error string is **collapsed to one line** before render (all
  runs of whitespace, including the `\r`/`\n` that git and `exec`
  errors are full of, become single spaces) and truncated to the flex
  width with a trailing `…`. This preserves the one-physical-line
  invariant that `remove-tui-log-path` established and is the change's
  central safety property: errors are *more* newline-prone than the
  log path that change deleted.
- The model, the event types, and the observer are **untouched**.
  `LastErr` already flows correctly end to end; this change is purely
  a render decision in `tui/view.go` plus its spec.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `tui`: the grid-rendering requirement gains the ERROR column in its
  column list, the width-tier rule that governs when it appears, and
  the single-line-collapse rule for the error text. The
  one-physical-line invariant is restated to cover error text
  explicitly. Two scenarios are added: a failed row shows its error;
  a multi-line error collapses to one physical line. The
  log-path-not-rendered prohibition and the AGE width tier are
  unchanged.

## Impact

- **Code**: one file, `tui/view.go`.
  - `View`: compute `showErr` and the flex `errWidth` alongside the
    existing `showAge`, and thread them into `renderHeader` and
    `renderRow`.
  - `renderHeader`: append an `ERROR` header cell when `showErr`.
  - `renderRow`: append an ERROR cell when `showErr`, sourced from a
    new `oneLine(s)` helper (`strings.Join(strings.Fields(s), " ")`)
    applied to `r.LastErr`, truncated to `errWidth`, falling back to
    `—` when `LastErr` is empty.
  - Add the `errMinWidth = 20` constant and the `oneLine` helper.
- **Specs**: one `MODIFIED` requirement on `tui`; no new capabilities.
  `watcher` and `event-log` are untouched (the error payload is
  already in `ChangeFailed`/`RetryAttempt`).
- **Dependencies**: zero added, zero removed.
- **Behavior**: at `>= 100` columns the grid shows why each retained
  row is in its current state. At `< 100` columns the grid is
  byte-identical to today. The batch JSONL file and the `--mode=log`
  stdout mirror are unchanged.
- **Risk**: low. The change is an additive render path gated by a new
  width tier; below the tier nothing changes. The one risk — a
  newline-laden error wrapping to a second line, reintroducing the
  `remove-tui-log-path` garble — is closed by the `oneLine` collapse
  plus flex-width truncation, and locked by a regression test that
  feeds a multi-line error and asserts exactly one physical line.
