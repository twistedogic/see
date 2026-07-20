## 1. Configuration and Compatibility Contract

- [ ] 1.1 Add failing configuration tests for `condition` and `commit`, strict type/unknown-field rejection, custom-mode prompt/commit validation, and blank-condition OpenSpec fallback selection.
- [ ] 1.2 Extend `Config`, startup validation, and `config.example.yaml` with the custom workflow fields while preserving existing prompt precedence and zero-configuration OpenSpec behavior.
- [ ] 1.3 Add regression tests proving that a missing condition retains current OpenSpec discovery, branch naming, archive completion, rollback, and default commit behavior.

## 2. Condition Resolution and Identity

- [ ] 2.1 Add failing unit tests for platform-shell execution, exit `0`/`1`/error semantics, stderr diagnostics, context cancellation, trailing newline normalization, and empty, whitespace-only, or multiline output rejection.
- [ ] 2.2 Implement the custom condition resolver using the platform shell and watcher context, with normalized stdout as the change value.
- [ ] 2.3 Add failing tests for full Secure Hash Algorithm 256-bit (SHA-256) branch identity, stable repeated output, distinct changed output, and hash-based per-agent log filenames.
- [ ] 2.4 Implement change hashing and use the digest for custom branch and log path components so raw condition output never becomes a path.
- [ ] 2.5 Add failing tests that `{change}` is replaced in both prompt and commit templates, then reuse the existing renderer for both values.

## 3. Persistent Custom Branch Lifecycle

- [ ] 3.1 Add failing watcher tests for clean-tree enforcement, first-run branch creation, same-branch resume without reset, and refusal when an existing lane is not checked out.
- [ ] 3.2 Implement custom branch creation and resume semantics without changing the legacy OpenSpec branch path.
- [ ] 3.3 Add failing regression tests proving that failure on an existing lane restores its pre-attempt tip and untracked files without deleting prior commits, while failure on a newly-created lane restores the original branch and deletes only that new lane.
- [ ] 3.4 Implement mode-aware rollback, including best-effort cleanup warnings and preservation of ignored files.
- [ ] 3.5 Add failing tests for level-triggered repeated runs, condition exit `1` idle behavior, condition changes selecting a different lane, and retry re-resolution.
- [ ] 3.6 Wire condition resolution into the polling and retry flow so every true pass runs the agent and every blank-condition pass uses the compatibility resolver.

## 4. Catch-up Commit Behavior

- [ ] 4.1 Add failing tests for rendering the custom commit message, committing leftover staged changes, preserving agent-created commits, and treating an unchanged successful run as a warning-free no-op.
- [ ] 4.2 Implement staged-diff detection and custom catch-up commits while leaving the successful automation lane checked out.

## 5. Events, Logs, and Terminal User Interface

- [ ] 5.1 Add failing event logger and watcher tests for `RepoSeen.HasChange`, including custom work, custom idle, condition failure, and OpenSpec fallback cases; assert that `HasOpenspec` is absent from JSONL.
- [ ] 5.2 Rename repository availability state from `HasOpenspec` to `HasChange` across watcher events, JavaScript Object Notation Lines (JSONL), the observer adapter, and TUI messages/model state.
- [ ] 5.3 Add failing TUI tests showing normalized custom changes in the CHANGE column, idle rows for either resolver, and unchanged phase/warning behavior; update the model and view to satisfy them.

## 6. Documentation and Verification

- [ ] 6.1 Update `AGENTS.md` and user-facing configuration documentation with the custom condition shell contract, stdout-to-`{change}` behavior, commit templating, persistent branch ownership, level-triggered polling, OpenSpec compatibility fallback, and the JSONL field migration from `HasOpenspec` to `HasChange`.
- [ ] 6.2 Run `gofmt` on changed Go files and `go test -timeout 30s ./...`, fixing any regressions.
- [ ] 6.3 Run the race-enabled test suite with a bounded timeout and validate the OpenSpec change artifacts.
