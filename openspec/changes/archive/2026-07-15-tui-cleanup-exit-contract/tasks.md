## 1. Apply the spec delta to the tui capability

- [x] 1.1 Apply the `## MODIFIED Requirements` block to
  `openspec/specs/tui/spec.md`: replace the two existing
  scenarios (`Pressing q exits the TUI cleanly` and
  `SIGINT exits the TUI cleanly`) under the
  `--tui renders a live status grid of every scanned repo`
  requirement with the unified
  `q and SIGINT share the same exit-status rule` scenario.

- [x] 1.2 Apply the `## ADDED Requirements` block to
  `openspec/specs/tui/spec.md`: insert the new
  `--tui drains the watcher goroutine and closes the JSONL
  event logger before exit` requirement with its matching
  scenario, placed after the
  `--tui renders a live status grid of every scanned repo`
  requirement and before the
  `Watcher semantics are unchanged under --mode=tui`
  requirement.

## 2. Validate

- [x] 2.1 Run `openspec validate --change
  tui-cleanup-exit-contract --json` and confirm zero
  diagnostics.

## 3. Confirm the implementation already matches

- [x] 3.1 `git diff` against the previous commit on
  `main.go`, `tui/model.go`, `tui/program.go`,
  `eventlog.go`, and any test file is empty. The
  implementation already satisfies the new contract;
  this change is a documentation correction
  (`ponytail:` comments, defer ordering, and the
  `<-watchErr` wait are the runtime hooks named in the
  new requirement).
