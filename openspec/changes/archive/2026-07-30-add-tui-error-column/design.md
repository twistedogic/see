## Context

`see`'s TUI (`tui` package) renders a priority grid of scanned repos
via Bubble Tea. The error from a failed (or retrying) run is already
captured on the row:

```go
// tui/model.go
case RetryAttemptMsg:
    r.LastErr = msg.Err          // previous attempt's error, live mid-retry
case ChangeFailedMsg:
    r.LastErr = msg.Err          // final error, the cause of the failed state
case ChangeStartedMsg, ChangeDoneMsg:
    r.LastErr = ""               // cleared on (re)start and on success
```

`LastErr` flows from `err.Error()` in `main.go`
(`ChangeFailed{Err: err.Error()}`, `RetryAttempt{Err: ...}`), through
`tuiObserver.Observe` → `tui.ChangeFailedMsg`/`RetryAttemptMsg`, into
the row. The plumbing is complete and correct. What is missing is
purely a consumer in `renderRow`: a `ponytail:` comment there records
that the message "lives in the JSONL (the operator can jq for it)" and
so the field is written but never read.

Three facts frame the change:

1. **The data is in memory, free.** Adding the column needs no new
   field, message, model case, or observer arm. It is a render-only
   change — the inverse shape of `remove-tui-log-path`, which had to
   delete a field, a message, a model case, and an observer arm.
2. **The one-physical-line invariant is load-bearing.**
   `remove-tui-log-path` fought an *unbounded* second line (the log
   path) that wrapped to a third line and smeared the frame; it
   reasserted that every row is exactly one physical line regardless
   of width. This change brings inline detail *back*, so it must not
   reopen that defect. Errors are worse than paths here: a path is one
   long token, but `err.Error()` from git/`exec` is routinely
   multi-line (`\n`-separated command output, wrapped messages).
3. **There is no room at the existing width tier.** The five fixed
   columns (REPO 24, CHANGE 30, PHASE 10, RETRY 8, AGE 8) sum to 80,
   and AGE already claims the `>= 80` tier. An ERROR column needs a
   higher tier and must be width-bounded to that tier.

## Goals / Non-Goals

**Goals:**

- Show each row's `LastErr` inline so a failed row's cause is visible
  without leaving the TUI.
- Preserve the one-physical-line invariant absolutely, including for
  multi-line errors.
- Reuse the existing `LastErr` data path; touch no model, event, or
  observer code.

**Non-Goals:**

- Wrapping long errors onto multiple lines, or adding a per-row
  detail/selection view. If full error text becomes a requirement,
  that is a separate change with a different shape (a focused detail
  pane), not a column.
- Surfacing the error at `< 100` columns. Narrow terminals keep the
  `⚠` glyph and the JSONL fallback; the floor is a tunable constant.
- Changing what `ChangeFailed`/`RetryAttempt` carry, when they fire,
  or which sinks receive them. `watcher` and `event-log` are
  untouched.
- Adding a CLI flag or config knob to toggle the column.

## Decisions

### Decision 1: Render whenever `LastErr != ""` (failed rows and retrying rows), not failed-only

**Choice.** The ERROR cell renders `r.LastErr` whenever it is
non-empty, at the `>= 100` width tier. For a `failed` row this is the
final error — precisely "the error that causes the failed state." For
a `working` row between attempts it is the previous attempt's error.

**Rationale.** `LastErr` is already maintained to exactly this
semantics (set on `RetryAttempt` and `ChangeFailed`, cleared on
`ChangeStarted`/`ChangeDone`). Rendering it verbatim needs zero model
logic. The retry visibility is a strict bonus: seeing *why* attempt 1
failed while attempt 2 runs is more useful than a blank cell, and it
costs nothing. A failed-only column would describe the same data
through a narrower window for no code savings.

**Alternatives considered.**

- *Render only when `r.Phase == PhaseFailed`.* Rejected: it gates the
  same field on phase for marginal semantic purity, hides useful
  retry context, and gains nothing in code (the render test is
  `LastErr != ""` either way; the phase check is one extra branch for
  one less signal). If the column header ever reads ambiguously
  during retry, the fix is the header label, not suppressing the data.
- *Add a separate `FinalErr` field cleared differently.* Rejected:
  pure additive complexity for a distinction the operator does not
  need at a glance.

### Decision 2: Flex width, not a fixed-width tier

**Choice.** ERROR is the last column. Its width is
`errWidth = m.width - fixedSum`, where `fixedSum` is the sum of the
fixed columns active at this width (72 without AGE, 80 with). The
column is shown only when `errWidth >= errMinWidth` (`20`), so it
appears at `m.width >= 100` (AGE is already on at `>= 80`). Content
is truncated to `errWidth` with a trailing `…`.

**Rationale.** A flex width is structurally incapable of overflowing
the line: the column is *defined* as the remainder, so by construction
`fixedSum + errWidth == m.width`. That is the cheapest possible proof
of the one-physical-line invariant, which is this change's central
safety property and the lesson of `remove-tui-log-path`.

**Alternatives considered.**

- *Fixed width (e.g. `ERROR(40)`) at a `>= 120` tier.* Rejected: it
  re-creates the exact failure mode `remove-tui-log-path` fixed. If
  `fixedSum + 40 > m.width` at the tier boundary (e.g. width 100 with
  a 40-wide column and 80 of fixed columns), lipgloss renders all
  fixed widths and the terminal wraps — the garble returns. Defending
  against it means a tier high enough that the floor is always clear
  (`>= 120`), which delays the column past where most operators run
  and still trusts arithmetic the flex approach makes tautological.
- *Shrink CHANGE to make room at `>= 80`.* Rejected: it churns an
  existing, tested column and shortens `workflow: <change>` labels to
  surface the column earlier than the terminal can comfortably hold
  it. The ERROR column earns its own tier instead.

### Decision 3: Collapse to one line via `strings.Fields`, then truncate

**Choice.** A `oneLine(s string) string` helper returns
`strings.Join(strings.Fields(s), " ")`. Applied to `r.LastErr` before
truncation, it turns every run of whitespace (`\r`, `\n`, `\t`,
unicode spaces, leading/trailing) into a single space. The result is
then truncated to `errWidth`.

**Rationale.** `err.Error()` from git/`exec` is frequently
multi-line. Any surviving `\n` would make the row two physical lines
and reopen the `remove-tui-log-path` defect — the single highest
risk in this change. `strings.Fields` is the standard-library one-liner
that collapses *all* whitespace categorically, so the invariant holds
for inputs the author has not imagined (form feeds, tabs, CRLF
pairs). It is also the minimal expression of intent: "one line,
spaces only."

**Alternatives considered.**

- *Replace only `\r` and `\n` with a space (`strings.NewReplacer`).*
  Rejected: it leaves tabs and other whitespace untouched, so a tab-
  padded git message could still misalign or push content past the
  truncation point. `strings.Fields` is the same line count and
  strictly stronger.
- *Preserve newlines as a visible ` ⏎ ` marker.* Rejected: the
  invariant is one *physical* line, and a marker does not change that
  a multi-line source fits poorly in a truncated one-liner. The full
  text remains in the JSONL; the column is a summary.

### Decision 4: Empty `LastErr` renders `—`, matching the existing sentinel

**Choice.** When `LastErr` is empty (idle, done, or a fresh
`working` row that has not yet failed), the ERROR cell renders `—`,
the same sentinel CHANGE/RETRY/AGE use for "no value."

**Rationale.** It keeps the column visually populated and aligned at
every width, and reuses the grid's existing "no value" convention
rather than inventing a blank cell or a new glyph.

## Risks / Trade-offs

- **[Reintroducing the `remove-tui-log-path` wrap]** → The central
  risk. Closed by Decision 2 (flex width cannot overflow) and
  Decision 3 (`strings.Fields` cannot emit a newline), and locked by
  a regression test that feeds a CRLF-laden multi-line error at the
  `>= 100` tier and asserts the row is exactly one physical line.
  *Mitigated.*
- **[Column unseen on 80-column terminals]** → At `< 100` columns
  the grid is byte-identical to today; operators on narrow terminals
  keep the `⚠` glyph and fall back to the JSONL, exactly as now. The
  floor (`errMinWidth = 20`) is a single constant, tunable later if
  100 proves too high. *Accepted.*
- **[Truncation hides the actionable part of the error]** → Real
  possibility: a long wrapped message may truncate before the
  meaningful token. The column is a *summary* and a triage signal
  ("this repo failed because of a merge conflict" vs. "because the
  agent errored"); the full text remains in the JSONL and the
  per-invocation log. A future detail pane is the right shape for
  full text, not a wider column. *Accepted.*
- **[Retry errors shown under a header named `ERROR` while phase is
  `working`]** → Mild cosmetic ambiguity, not a correctness issue.
  The PHASE cell (`● working`) and RETRY cell (`1/3`) already
  disambiguate the state; the ERROR cell's content is then read as
  "the last attempt's error." If it reads wrong in practice, relabel
  the header (`LAST ERR`) in a follow-up; do not suppress the data.
  *Accepted.*
