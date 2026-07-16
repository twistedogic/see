## Context

The watch-resolution and prompt-selection rules both layer three sources:
command-line flag, configured value, embedded default. Today the "no config
at all" mode is exposed as a boolean escape hatch (`--ignore-config`) that
short-circuits the file read at the loader boundary. After the recent
`make-cli-flags-override-config` change, every layered knob except the file
itself has a string-valued CLI override (`--watch`, `--prompt`). The boolean
escape hatch is the lone outlier, and its only remaining job — making a
malformed `config.yaml` equivalent to a missing one — is the same as "load
zero config," which the loader already supports.

This change unifies the surface by replacing the boolean with a string
flag, keeping the escape hatch as a sentinel value, and adding a swappable-
config capability for the same surface cost.

## Goals / Non-Goals

**Goals:**

- Collapse `--ignore-config` and "point at a different config file" into a
  single string-valued `--config <path>` flag.
- Preserve the malformed-config escape hatch as `--config=-`.
- Preserve the default behavior (no flag → load the system config path) for
  the 99% case.
- Tilde-expand the path value to stay consistent with `--watch`.
- Drop the "unless `--ignore-config` is set" carve-out from both
  requirements; the rules simplify to "unless `--config=-` is set."
- Remove the `ignoreConfig` parameter from `loadStartupConfig` and the
  matching branch, so the loader is once again a pure function of a path.

**Non-Goals:**

- No new config schema, fields, or file formats. The file `config.yaml`
  reads today is the file `--config=<path>` will read.
- No per-project auto-discovery (e.g. `./.see.yaml`). `--config=./.see.yaml`
  works because the loader takes any path, but auto-discovery is out of
  scope.
- No environment-variable override (`SEE_CONFIG`). Flag-only for now; can
  be added later if a real use case appears.
- No backward-compatibility shim for `--ignore-config`. The flag is one
  month old with no observed scripts; hard-cutting is the ponytail answer.

## Decisions

### Decision: `--config <path>` is a single string flag, not a flag pair

Three flags were considered: `--config=<path>` plus `--no-config` boolean,
or `--config=<path>` plus an env var. A single string flag is the minimum
that expresses "default / explicit path / skip" without introducing a
second flag or an env-var sidecar. The empty string is indistinguishable
from "unset" in `flag.String`, so a non-empty sentinel (`-`) carries the
"skip" mode.

**Alternatives considered:**

- `--config` plus `--no-config`: two flags for one decision. Adds a knob
  without removing `--ignore-config`'s surface cost.
- `SEE_CONFIG` env var: introduces a second configuration channel that
  must be documented and tested, and interacts non-obviously with the
  flag (flag wins vs env wins vs flag overrides only-when-set). Skipped
  as speculative.
- `--config` with no value at all (treating presence as skip): would
  require a custom `flag.Value` and complicate the help text. Skipped.

### Decision: `-` is the skip sentinel

POSIX convention (`tar x -`, `git log -`) makes `-` a familiar "no input"
marker. Empty string cannot serve as the sentinel because it is also the
default for "unset." A bespoke token like `none` would be discoverable
but unfamiliar.

### Decision: tilde expansion in the loader, not at flag parse time

The existing `expandTilde` helper in `config.go` is already package-level;
calling it from `loadStartupConfig` is one line. Tilde expansion at flag
parse time would force the help-text default to be the resolved path,
which leaks `$HOME` into `--help` output and surprises users on shared
machines. Doing it in the loader keeps the flag default as `""` and the
help text as "default: `$XDG_CONFIG_HOME/see/config.yaml`."

### Decision: hard cut `--ignore-config`

The flag was added ~1 month ago (in `add-watch-paths-config`), preserved
deliberately through two subsequent refactors, and broadened in scope in
the most recent YAML promotion. No scripts are observed in the repo or
the project's own `AGENTS.md` examples. A deprecated alias would add 3
lines and a stderr warning for an audience of zero.

### Decision: loader signature changes from `(bool)` to `(string)`

The boolean parameter only existed to forward the flag value. With the
flag itself changing type, the parameter changes type too. The branch
that handled the boolean is replaced by a sentinel check (`"-"`), an
empty-string default (`configPath()`), and a passthrough (`loadConfig`).

## Risks / Trade-offs

- **Breaking change for any user scripting `--ignore-config`.** Mitigation:
  flag is one month old; the error message on the unrecognized flag
  (`flag provided but not defined: -ignore-config`) is self-explanatory;
  the change is documented in `AGENTS.md` under the Configuration section.
- **`-` sentinel leaks into help text, tests, and scenarios.** Mitigation:
  the leak is a one-time tax; a named constant (`configPathNone`) keeps
  the sentinel value in one place so a future rename is local.
- **Tilde expansion duplicates 6 lines of code if the watch resolver and
  the loader both want it.** Mitigation: `expandTilde` is already a
  package-level helper; both call sites use the same function. No
  duplication, just a second caller.
- **The new `--config=<path>` capability is unused on day one.** Mitigation:
  the change does not add behavior that needs to be justified by usage;
  it removes the boolean, which is the headline. The path-loading branch
  is two lines and earns its keep by completing the symmetry with
  `--watch` / `--prompt`.

## Migration Plan

No deployment steps. Users with no `config.yaml` and no flag see no
change. Users with a valid `config.yaml` and no flag see no change. The
only observable difference is the help text (`--config <path>` replaces
`--ignore-config`) and the failure mode for any user who has scripted
`--ignore-config` (Go's `flag` package reports an unrecognized flag).

## Open Questions

- Should `--config=-` be mentioned in the help text for `--watch` and
  `--prompt`, on the theory that users who want a one-off run without
  config are likely to reach for any of the three? Defaulting to no:
  the three flags have orthogonal purposes and the help text is already
  clear about each.
- Should the system config path itself be overridable via an env var
  (`SEE_CONFIG_DIR`)? Out of scope for this change; can be added later
  if it earns its keep.