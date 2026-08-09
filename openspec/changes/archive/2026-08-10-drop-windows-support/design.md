## Context

`see` executes workflow `condition`, `check`, and `measure` commands through a
shell, and cancels the shell's process group on context cancellation. To
accommodate Windows it carries: a `runtime.GOOS == "windows"` shell-selection
fork at three call sites in `main.go`, a build-tagged pair
(`condition_process_windows.go` no-op stub + `condition_process_unix.go` real
implementation) for the process-group `SysProcAttr`, and 14
`runtime.GOOS == "windows"` test-skip guards in `main_test.go`.

None of this is verified: `.github/workflows/ci.yml` runs a single
`ubuntu-latest` job, there is no Windows release asset, and the test guards
skip themselves on the platform they nominally protect. The current state is
therefore *theater support* — source branches claiming a platform the test
suite declines to vouch for.

## Goals / Non-Goals

**Goals:**
- Remove the untested Windows code paths so the source reflects what CI
  actually verifies.
- Collapse the platform-shell fork to a single `/bin/sh -c` invocation at each
  site.
- Delete the `configureConditionCommand` build-tagged abstraction; inline the
  process-group logic next to its call sites.
- Remove the self-skipping test guards.
- Align the `workflow-condition` spec with Unix-only reality: drop the Windows
  shell clause, de-Windows the trailing-newline scenario, and remove the "on
  Unix" qualifier from process-group cancellation.

**Non-Goals:**
- Add a Windows CI job (this is the opposite change).
- Preserve Windows compileability. The build will fail on Windows after the
  change; that is the intended quiet signal.
- Add a `//go:build !windows` package constraint or a README banner. This is
  the *quiet* variant — the code reflects reality without a proclamation.
- Migrate any Windows operator. None is known, and the path was never
  verified.

## Decisions

### Decision 1: Quiet removal, not a loud declaration

The Windows code is deleted and the Unix `syscall` plumbing (`Setpgid`,
`syscall.SIGKILL`) becomes unconditional. No `//go:build !windows` constraint,
no README banner, no `%AppData%` migration note.

**Why:** A loud constraint is a maintenance artifact that exists to announce
support for a platform we are deliberately dropping. The code itself — which
references `syscall.SysProcAttr{Setpgid: true}` and `syscall.SIGKILL`, neither
present in the Windows `syscall` package — already makes `see` fail to compile
on Windows. That compile failure is a more honest signal than a banner: it is
involuntary and cannot drift out of sync with the code.

**Alternative considered:** loud declaration (`//go:build !windows` on the
package, README "Unix-only" note, explicit error message). Rejected — it
preserves a concept (Windows as a known, intentional non-target) that adds
documentation surface for a platform nobody runs. The user explicitly chose
quiet.

**Consequence to name:** today `see` *compiles* on Windows (the stub provides
a no-op `configureConditionCommand`). After this change it does not. The flip
from *compiles-but-untested* to *does-not-compile* is the feature, not a
surprise.

### Decision 2: One unconditional `configureProcessGroup` in `main.go`, build-tagged pair deleted

The process-group setup (`Setpgid: true` plus a `Cancel` closure that
`SIGKILL`s the process group) has **three** callers —
`resolveCustomCondition`, `runCheck`, `runMeasure` — not one, and six
statements, not two. Inlining it at each site would triplicate that
block. Instead the Unix body moves into `main.go` as a single
unconditional helper, renamed from the condition-flavored
`configureConditionCommand` to the accurate `configureProcessGroup`, and
the build-tagged pair (`condition_process_windows.go` stub +
`condition_process_unix.go` body) is deleted.

**Why:** the build-tagged split existed *solely* to host the platform
fork. With no fork, the file indirection has no remaining justification
— but the helper itself still earns its keep by deduplicating six lines
across three call sites.

**Alternatives considered:**
- Inline at each call site: rejected — triplicates six lines for no gain.
- Keep a separate non-tagged `condition_process.go`: rejected — a
  one-function file has no cohesion advantage over a helper in `main.go`.

### Decision 3: Keep `\r` in the stdout trim set

The condition/measure normalization is `strings.TrimRight(s, "\r\n")`. The
`\r` stays in the trim set even though Windows is gone.

**Why:** removing `\r` is a behavior change to the normalized change value for
any odd input that ends in a carriage return, for zero benefit — `/bin/sh`
output does not end in `\r`, but a misbehaving wrapper script or a piped tool
could. Keeping the trim set is strictly more defensive at no cost.

**Alternative considered:** narrow to `strings.TrimRight(s, "\n")` for
"honesty". Rejected — more fragile, no measurable gain.

**Spec consequence:** the *behavior* (trimming `\r`) is retained, so the spec
keeps documenting it. Only the *Windows framing* goes: the "Windows trailing
newline is removed" scenario is renamed to describe the behavior it actually
demonstrates (trimming a trailing carriage return), not the platform that
motivated it.

## Risks / Trade-offs

- **An unknown Windows operator loses a compiling `see`.** → Mitigation: no
  such operator is known, and the path was untested and self-skipping. Git
  history preserves the implementation; re-adding requires reintroducing the
  platform fork *and* a CI job to verify it, which is the correct bar.
- **A contributor is surprised that `see` no longer builds on Windows.** →
  Mitigation: this is the intended quiet signal. The loud build constraint was
  an explicit non-goal; if surprise becomes a real problem, the constraint can
  be added in a follow-up without redesign.
- **A Windows reference survives somewhere in the repo after the sweep.** →
  Mitigation: tasks include a repo-wide grep for `windows`, `cmd.exe`,
  `runtime.GOOS`, `%AppData%`, and `Setpgid`-conditional framing to confirm
  zero remaining references before the change lands.
