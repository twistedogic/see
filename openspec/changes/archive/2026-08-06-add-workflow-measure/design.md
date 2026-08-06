# Design — add-workflow-measure

## The seam

The measure gate is a per-workflow improvement gate that spans an entire
attempt: it captures a **baseline** immediately before the agent runs and
a **candidate** immediately after a passing `check`, then compares them.
Unlike `check` (a single post-agent correctness gate), measure has a
pre-agent phase because the baseline must reflect the clean state the
agent starts from, independent of any committed metric file the agent
could read or alter.

The gate slots into the existing landing path between "check passed" and
"stage changes," because a change that does not improve the metric
should be discarded exactly like a change that fails the check — never
committed, rolled back to a clean slate, retried.

```
resolve change → ensure lane/worktree
   │
   ▼
workflow has measure? ── no ──▶ run agent (no baseline, no {metric})   [unchanged]
   │ yes
   ▼
run measure (baseline)  ──▶ parse float64 ──▶ hold in memory
   │  (failure → measureFailedError → rollback → retry)
   ▼
render prompt + commit with {metric} = baseline
   │
   ▼
run agent ──▶ modifies code/config
   │  (agent failure → existing rollback, unchanged)
   ▼
[check gate, if configured: fail → checkFailedError → rollback → retry]
   │ pass / no check
   ▼
working tree dirty? ── no ──▶ run measure (candidate) anyway when
   │ yes                    measure configured (non-deterministic
   ▼                        measures may differ on an unchanged tree)
run measure (candidate) ──▶ parse float64
   │  (failure → measureFailedError → rollback → retry)
   ▼
candidate > baseline ?
   ├─ yes ──▶ stage + catch-up commit ({metric} rendered)            [unchanged mechanics]
   └─ no   ──▶ rollback to clean slate + return *measureFailedError
```

The "run candidate anyway on a clean tree" choice is deliberate: a
benchmark metric is often non-deterministic, so an agent that changed
nothing may still produce a different score. Short-circuiting on a clean
tree would hide that. The cost is one extra measure run for deterministic
measures on no-op attempts; acceptable.

## Per-mode landing and rollback

| mode | measure cwd | on improve | on no-improve / measure error (clean slate) |
|---|---|---|---|
| branch + custom | lane checkout (= operator's) | catch-up commit; lane stays | reset --hard to pre-attempt tip + `git clean -fd`; lane kept if pre-existing |
| worktree + auto | worktree dir | catch-up commit + rebase + ff-merge; lane/worktree removed | worktree removed + lane deleted (`-D`); operator untouched |
| worktree + manual | worktree dir | catch-up commit + rebase; lane/worktree left for review | worktree removed + lane deleted (`-D`); operator untouched |

All three failure paths reuse the existing rollback functions
(`rollbackWorkflowLane`, `rollbackWorktree`) unchanged; a measure failure
is simply a new caller of each, exactly as `check` failure already is.
The baseline-measure failure (before the agent) also routes through
rollback: at that point the tree is clean (the agent has not run), so the
rollback is effectively a no-op on the working tree in branch mode and a
worktree removal in worktree mode.

## The two definition forms and precedence

The resolved measure command is selected in this order (mirrors the
`prompt` precedence shape — explicit wins, convention is the fallback):

1. A nonblank frontmatter `measure:` value (or `config.yaml`
   `workflows:` `measure:` value). This is an arbitrary shell string —
   an inline script, a repo-relative invocation, or an absolute path.
2. Otherwise, a regular file at
   `~/.config/see/measure/<workflow-name>.sh`. If it exists, `see` reads
   its contents and runs them through the platform shell (so the file
   needs no execute bit and no shebang; it is treated exactly like an
   inline `measure:` string whose text is the file body).
3. Otherwise, the workflow has no measure gate.

A present-but-blank `measure:` is a startup error (consistent with
`check`); a blank `measure:` with no convention file is "no measure"
and is not an error. The convention directory is a fixed default
(`~/.config/see/measure/`); it is **not** coupled to `workflows_dir`
(workflows also come from `config.yaml`, so coupling would mislead), and
it is **not** operator-configurable — an operator who wants the script
elsewhere uses the frontmatter `measure:` field with an explicit path,
which already covers relocation. A missing convention directory
contributes no scripts and is not an error.

## `{metric}` versus `{change}`

Both are single literal-token substitutions rendered before the agent
runs, with unknown tokens left verbatim. They differ in source and
scope:

| token | source | rendered into | when |
|---|---|---|---|
| `{change}` | normalized condition stdout | `prompt`, `commit`, `check`, `measure` | always |
| `{metric}` | baseline measure value | `prompt`, `commit` | only when a measure gate is resolved |

The `measure` command is the *producer* of `{metric}`, so it is not a
consumer of it (a measure string referencing `{metric}` leaves it
literal). The value substituted is the normalized baseline string — the
same bytes that were parsed as float64 for comparison — so the agent
sees exactly the number `see` measured.

## Comparison semantics

Measure output is normalized exactly like a condition value: trailing
`\r`/`\n` stripped, must be single-line, must contain non-whitespace.
The normalized value is then parsed with `strconv.ParseFloat(..., 64)`.
Baseline and candidate are each parsed independently.

- candidate **strictly greater than** baseline (after parsing both) →
  improve → land.
- candidate equal to or less than baseline → no improvement →
  `measureFailedError`.
- either value failing to parse, or the command exiting nonzero, or
  empty/multiline output → `measureFailedError`.

Higher-is-better is the only direction shipped. An operator optimizing
for "lower is better" (latency, error rate) makes the measure script
emit the inverse (e.g. `echo $(-1 * $ms)` or `1/error_rate`); a
direction knob is deferred until a real need appears.

The comparison is naive point-to-point. Non-deterministic measures
(benchmarks with variance) can spuriously "improve" on an unchanged
tree, or fail to improve on a genuinely better one. This is the
operator's measure script to solve (seeding, averaging, warm-up); `see`
does not smooth, threshold, or retry the measurement itself. A
`ponytail:`-style ceiling worth noting in code: a single noisy sample
decides each attempt.

## Integrity model: show the value, hide the ruler

The agent receives the score to beat (`{metric}` in the prompt) but not
the measurement mechanism. This is the standard autoresearch integrity
split: the agent optimizes against a known target without being able to
peek at the grading rubric mid-test, which is what prevents hill-climbing
and reward hacking against the raw metric.

The strength of the guarantee is determined by **where the measure
script lives**, which the operator chooses by which definition form they
use:

| measure defined as | script location | agent can run it casually? |
|---|---|---|
| convention file `~/.config/see/measure/<name>.sh` | out-of-repo | no |
| inline frontmatter `measure:` string | out-of-repo (the `.md`) | no |
| frontmatter `measure:` pointing at a repo path | in-repo | yes |

So two of three forms give tamper-**resistance**: the agent, running in
the repository working directory, does not encounter the script and
cannot self-test against it. The residual gap is a determined agent that
reads `~/.config/see/measure/<name>.sh` by absolute path on a shared
filesystem — that requires a sandbox `see` does not provide, and closing
it is explicitly out of scope. The baseline value never touches a file
the agent can read (it lives in `see`'s process memory until it is
rendered into the prompt), so it cannot be read out-of-band either.

## Retry and the terminal event

A measure failure returns a `*measureFailedError` sentinel carrying the
rendered command, exit code, baseline, candidate, and stderr. Because it
is an error, it flows through `runWithRetry` and is retried like an agent
or check error: the agent gets up to `RetryCount` fresh attempts per
poll, each re-resolving the change, re-measuring the baseline, and
re-running from a clean slate. `RetryAttempt` events carry the prior
error's summary between attempts, unchanged.

The terminal event after the final attempt is selected by type, in this
priority (each attempt fails at exactly one gate, so the final error is
exactly one type):

- `errors.As(err, &measureFailedError{})` → emit `MeasureFailed`
- else `errors.As(err, &checkFailedError{})` → emit `CheckFailed`
- else → emit `ChangeFailed`

`MeasureFailed` replaces `ChangeFailed`/`CheckFailed` for the
measure-failure outcome; the three are mutually exclusive per repository
per pass. For an autoresearch loop where most attempts legitimately fail
to improve, `MeasureFailed` is the expected, non-alarming terminal state
of a poll that found no improvement this round.

## Alternatives considered

- **Commit-then-measure against a committed baseline (rejected).** Let
  the `check` script read a committed metric file as the baseline,
  avoiding a new pre-agent phase. Rejected because the baseline would
  live in the repository where the agent can read and game it, and
  because a stale or agent-tampered baseline would corrupt the
  comparison. A fresh pre-agent measurement on the clean tree is robust
  and keeps the baseline in `see`'s memory.
- **Fold measure into `check` via a baseline env var (rejected).** `see`
  captures the baseline and exports it to `check`, which then measures
  and compares. Rejected because `{metric}` in the prompt still requires
  `see` to hold the baseline before the agent, so `see` already has both
  values by the candidate phase — pushing the comparison into the
  operator's `check` script splits one concept across two places and
  loses the dedicated `MeasureFailed` signal. A first-class `measure`
  field is simpler to document and to observe.
- **Lower-is-better / multi-metric / direction knob (rejected for v1).**
  Shipped higher-is-better scalar only. An operator inverts in the
  script; multi-objective optimization is deferred until a real need
  appears.
- **Treat no-improvement as a clean poll-end, not a retryable failure
  (rejected).** End the poll after one non-improving attempt without a
  `MeasureFailed` event, so the loop is one-experiment-per-poll.
  Rejected because it diverges from how `check` already behaves (retry
  on failure within a poll) and removes `RetryCount` as a per-poll
  experiment budget. Consistency with `check` wins; an operator wanting
  one experiment per poll sets `RetryCount` to 1.
- **Sandbox the agent for a hard integrity guarantee (rejected).** Run
  `pi` in a container or separate filesystem namespace so the measure
  script is provably unreachable. Rejected as out of scope: `see` is a
  watcher/orchestrator, not a sandbox runtime, and today it shells out
  to `pi` in the repository working directory with no filesystem
  isolation. Tamper-resistance via out-of-repo script location is the
  honest fit for `see`'s shape.
