## Context

`see` currently reads one global configuration value: watch patterns from the plain-text file `os.UserConfigDir()/see/watches`. The agent prompt has a separate two-level selection: `--prompt` when nonblank, otherwise the build-time `prompt.md`. A persistent user-default prompt therefore has nowhere to live.

The watch-config design deliberately deferred a structured format until a second configuration dimension appeared. The requested prompt is that second dimension. The prompt is naturally multiline, so a YAML Ain't Markup Language (YAML) block scalar is substantially easier to edit than an escaped JavaScript Object Notation string.

The code currently loads watch configuration inside `resolveWatchList`, while prompt selection happens earlier when constructing `Watcher`. The promoted configuration must be loaded once so both consumers see the same file and `--ignore-config` can bypass it atomically.

## Goals / Non-Goals

**Goals:**

- Provide one global `config.yaml` containing watch patterns and a user-default prompt.
- Preserve command-line watch union, discovery, rendering, and embedded-prompt behavior.
- Give a nonblank `--prompt` value precedence over the configured prompt.
- Reject configuration mistakes before the watcher starts.
- Keep the configuration path aligned with the existing `os.UserConfigDir` implementation on every operating system.

**Non-Goals:**

- Per-repository or per-change prompts.
- Runtime configuration reloads; configuration remains fixed for the process lifetime.
- Additional prompt tokens beyond `{change}`.
- Environment-variable expansion in watch patterns or prompt text.
- Support for the legacy `watches` file after promotion.
- A command that creates, edits, or migrates configuration.
- A handwritten subset of YAML.

## Decisions

### Use one `config.yaml` with two top-level fields

The global file becomes `filepath.Join(os.UserConfigDir(), "see", "config.yaml")` with this shape:

```yaml
watches:
  - "~/Dev/*"

prompt: |-
  Apply the OpenSpec change "{change}".
```

`watches` is a sequence of strings and `prompt` is a string. Either field may be omitted. An omitted or empty `watches` sequence preserves the current-working-directory fallback; an omitted or blank `prompt` falls through to the embedded default.

This replaces the legacy `watches` file rather than layering a second file beside it. Two global files would make `--ignore-config`, error precedence, documentation, and future fields harder to reason about.

Alternatives considered:

- Keep `watches` and add a separate `prompt.md`: smallest code change, but not a configuration field and leaves two configuration surfaces.
- Use JavaScript Object Notation: available in the standard library, but hostile to multiline prompt editing.
- Merge or fall back to the legacy file: smoother migration, but preserves an obsolete format with no release history requiring it.

### Parse with `go.yaml.in/yaml/v3`, not a custom parser

Add the maintained version 3 YAML module as a direct dependency. Decode into:

```go
type Config struct {
    Watches []string `yaml:"watches"`
    Prompt  string   `yaml:"prompt"`
}
```

Use a decoder with known-field checking enabled. The loader accepts a missing or empty file as a zero-value `Config`, but rejects malformed YAML, unknown fields, wrong field types, and additional documents. Errors identify `config.yaml` and preserve parser line information.

A parser dependency is justified here: YAML quoting, block scalars, aliases, typing, and line-aware errors are not a few safe lines of standard-library code. A handwritten subset would advertise YAML while implementing something incompatible.

### Load configuration once before resolving consumers

Replace `watchConfigPath` and `loadWatchConfig` with a path function and one strict `loadConfig(path) (Config, error)` loader. A small startup helper applies `--ignore-config`: when ignored, it returns a zero-value `Config` without resolving or reading the file.

`main` then passes `Config.Watches` into watch-list resolution and `Config.Prompt` into prompt selection. `resolveWatchList` becomes a pure coordinator over command-line and configured watch slices rather than performing file input/output itself.

This avoids parsing the same file twice and ensures prompt and watches come from one configuration snapshot.

### Use three-level, nonblank prompt precedence

The effective prompt template is selected as follows:

1. Nonblank `--prompt` value.
2. Nonblank `Config.Prompt` value.
3. Embedded `prompt.md`.

A tiny pure selector chooses between the first two inputs; the existing `Watcher.SetPromptTemplate` normalization supplies the embedded fallback. Blank means absent at every layer—there is no separate sentinel for an intentionally empty agent prompt.

The existing `renderPrompt` behavior remains unchanged: every `{change}` token is replaced and all other brace-delimited text is preserved.

### Make `--ignore-config` atomic

`--ignore-config` skips `config.yaml` entirely. Its watch entries do not participate in discovery, its prompt does not participate in precedence, and malformed contents do not cause startup failure. Command-line watches and prompt values still apply, followed by the current-working-directory and embedded-prompt fallbacks.

The help text changes from naming the old watches file to stating that the global configuration file is skipped.

### Remove the legacy format without compatibility code

The old `os.UserConfigDir()/see/watches` path is not read, merged, or inspected. Operators migrate each non-comment line into the YAML `watches` sequence. There are no repository tags and no legacy file on the current machine, so compatibility code would create a permanent second path for speculative users.

## Risks / Trade-offs

- **[Risk] Existing watch entries stop applying after upgrade.** → Mark the change as breaking and provide a direct before/after migration example.
- **[Risk] YAML accepts surprising implicit types.** → Decode into concrete string fields and fail when values have the wrong type; quote watch globs in examples.
- **[Risk] Configuration typos silently disable behavior.** → Enable known-field checking and fail before starting the watcher.
- **[Risk] A new dependency expands the supply-chain surface.** → Add one focused parser dependency and avoid wrappers or secondary configuration packages.
- **[Risk] Documentation promises the wrong platform path.** → Describe the path as `os.UserConfigDir()/see/config.yaml` and give platform examples rather than claiming one cross-platform literal path.
- **[Trade-off] Blank `--prompt` cannot mean “send an empty prompt.”** → Preserve the existing blank-as-default contract; an empty agent prompt has no established use case.

## Migration Plan

1. Replace each legacy watch line with an item under `watches` in `config.yaml`:

   ```text
   ~/Dev/*
   /var/repos
   ```

   becomes:

   ```yaml
   watches:
     - "~/Dev/*"
     - "/var/repos"
   ```

2. Optionally add a literal-block `prompt` field.
3. Remove the old `watches` file after verifying startup.

Rollback restores the prior binary, which ignores `config.yaml`; recreating the old `watches` file is required if watch configuration must survive rollback.

## Open Questions

None. The configuration format, clean replacement, prompt precedence, strictness, and ignore behavior are resolved.
