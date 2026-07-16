## Context

`see` reads its configuration from
`os.UserConfigDir()/see/config.yaml`. The path is resolved by
`configPath()` and consumed by `loadConfig()` via
`loadStartupConfig()`. The current contract is:

- File missing → `loadConfig` returns a zero-value `Config`; the
  current-working-directory (cwd) fallback applies. No file is
  written.
- File present, valid → fields populate; flags still override.
- File present, malformed → startup fails with exit status 2.

The missing-file branch is silent. A first-time user invoking `see`
gets a working watch loop (cwd fallback) but no signal that there is
a configuration surface they could be using, what fields are
available, or where the configuration file would have lived. The
documented surface (the `--config` flag, the `watches:` and
`prompt:` fields, the `prompt.md` default) is invisible until the
user reads source code or the project documentation.

## Goals / Non-Goals

**Goals:**

- Make the configuration surface discoverable on first run by
  materializing a commented template at the default path.
- Preserve every existing behavior: flag precedence, cwd fallback,
  strict decoding, malformed-file failure, `--config=-` skip
  semantics, `--config=<path>` resolution.
- Keep the bootstrap invisible to `main()`. All branching stays
  inside `loadStartupConfig`.
- Make bootstrap failure non-fatal so an unwritable home directory
  does not block startup.

**Non-Goals:**

- Migrating existing users. The bootstrap only fires on a missing
  default path; users who already have a configuration file are
  untouched.
- Changing the strict-decoding schema. The template is purely
  comments and decodes to a zero-value `Config` that the loader
  already returns for the missing case.
- Adding a `see init` subcommand, an interactive prompt, or any
  user-facing ceremony. The bootstrap is silent and idempotent.
- Touching `discovery.go`, `eventlog.go`, `tui/`, `prompt.md`, or
  the `--config` semantics beyond what is described above.

## Decisions

### Decision: Bootstrap lives inside `loadStartupConfig`, not in `main()`

`loadStartupConfig` is the single boundary between the
`--config` flag and the loader. Putting `ensureDefaultConfig` here
keeps the call site (`main.go`) free of branching and keeps the
write confined to the default-path branch.

Alternatives considered:

- **`main()` calls bootstrap, then `loadStartupConfig`**: spreads
  the policy across two files and forces `main` to know whether
  the default path is in play.
- **Bootstrap inside `loadConfig`**: blurs the read/write boundary
  and forces every `loadConfig` caller to reason about whether the
  write should fire.

### Decision: Bootstrap is all-comments

The template contains only YAML comments and a YAML header. After
`loadConfig` parses it, the result is the same zero-value `Config`
that the missing-file branch returns. This means the bootstrap has
zero behavioral effect — the user gets a working watch loop
identical to today's behavior, plus a file they can edit.

Alternatives considered:

- **Pre-filled default with `watches: ["~/Dev/*"]`**: changes the
  effective watch list on first run. Surprising. Users who never
  read the docs would suddenly have a `~/Dev/*` glob applied,
  which may not match their layout.
- **Pre-filled prompt that duplicates `prompt.md`**: redundant
  with the embedded default; would silently override the embedded
  prompt without the user knowing.

### Decision: Write failure is non-fatal

A read-only home directory, a disk-full condition, or a permission
error should not block startup. `ensureDefaultConfig` returns the
error; `loadStartupConfig` logs a one-line notice to standard
error (stderr) and proceeds with a zero-value `Config`. The cwd
fallback still applies.

Rationale: the bootstrap is a discoverability aid, not a
prerequisite. The strict-decoding requirement still errors fatally
when a configuration file exists but is malformed — that error is
about user-authored content, not about our bootstrap attempt.

### Decision: Parent directory is created

`MkdirAll(<base>/see, 0o755)` runs before the write. First-run
users do not have `~/.config/see/`. The cost is one directory
inode per machine; the benefit is that the bootstrap path matches
the documented `configPath()` resolution.

Alternatives considered:

- **Error out and tell the user to create the directory**:
  shiftesthe bootstrap ceremony onto the user, which defeats the
  discoverability goal.
- **Skip the write if the directory is absent**: silently
  swallows the read-only-home case, leaving the user with no
  configuration file and no signal that one was attempted.

### Decision: Bootstrap does not run for `--config=<path>`

A user who passes `--config=/some/path.yaml` has named a file.
Whether that file exists is the loader's concern, not ours. If
the user wants the bootstrap, they pass no `--config` and let the
default path resolve.

Rationale: `--config=<path>` is the documented mechanism for
swappable configuration files (per-project configs, shared
dotfiles). The bootstrap would conflict with that use case by
silently writing a file the user did not name.

### Decision: Bootstrap does not run for `--config=-`

`--config=-` is the documented "do not load any configuration"
sentinel. The user has explicitly opted out of the configuration
layer. Writing a file contradicts that opt-out.

## Risks / Trade-offs

- **Surprise file on disk.** A user who reads every directory
  carefully may notice a `config.yaml` they did not create.
  → Mitigation: the template is pure comments. After
  `loadConfig`, behavior is identical to the missing-file case.
  The first-run experience documents the bootstrap in
  `AGENTS.md` so the next reviewer does not have to infer it.

- **Race between two concurrent `see` invocations.** Both see the
  file missing; both write the same template; last write wins.
  → Mitigation: the writes are idempotent (same content). No
  atomicity needed.

- **Bootstrap on a read-only filesystem.** The write fails. The
  loader continues with a zero-value `Config`.
  → Mitigation: non-fatal by design. The user still gets a
  working watch loop via cwd fallback. A stderr notice identifies
  the path and the error.

- **User deletes the file and re-runs.** We re-bootstrap. This
  matches user intent — they have explicitly returned to the
  "no configuration" state and asked us to materialize one
  again.

- **User edits the bootstrap template and breaks it.** Strict
  decoding already rejects malformed YAML on load. The existing
  "Malformed config line is fatal at startup" requirement covers
  this; bootstrap behavior is irrelevant to it.

## Migration Plan

No migration. Existing users (anyone with a configuration file
already at the default path) are unaffected: bootstrap does not
overwrite.

## Open Questions

None. The three design choices that were flagged during
exploration (all-comments vs. pre-filled template, silent vs.
noisy write, MkdirAll vs. error-out) are resolved above in favor
of "all-comments, silent on success, noisy on failure, MkdirAll
the parent directory."