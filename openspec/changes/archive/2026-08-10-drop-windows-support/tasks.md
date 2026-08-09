## 1. Collapse platform-shell forks in main.go

- [x] 1.1 In `resolveCustomCondition`, replace the `runtime.GOOS == "windows"` shell selection with an unconditional `/bin/sh -c` invocation
- [x] 1.2 In `runCheck`, apply the same unconditional `/bin/sh -c` collapse
- [x] 1.3 In `runMeasure`, apply the same unconditional `/bin/sh -c` collapse

## 2. Collapse process-group plumbing to one unconditional helper

- [x] 2.1 Add unconditional `configureProcessGroup` (`Setpgid: true` + `SIGKILL` on cancel) to `main.go`; update the three call sites from `configureConditionCommand` to `configureProcessGroup`
- [x] 2.2 Delete `condition_process_windows.go`
- [x] 2.3 Delete `condition_process_unix.go`
- [x] 2.4 Swap the `runtime` import for `syscall` in `main.go`

## 3. Remove Windows test scaffolding

- [x] 3.1 Remove the 12 `if runtime.GOOS == "windows" { t.Skip(...) }` blocks from `main_test.go`
- [x] 3.2 Inline `platformCondition(unix, windows)` → `unix` at all call sites, delete the helper, and drop the now-unused `win` local variables
- [x] 3.3 Remove the now-unused `runtime` import from `main_test.go`

## 4. Align documentation

- [x] 4.1 In `AGENTS.md`, drop the Windows config-path clause (`Windows: %AppData%/see/config.yaml`) from the Configuration section
- [x] 4.2 In `AGENTS.md`, simplify the platform-shell contract mentions (`/bin/sh -c` on Unix, `cmd.exe /C` on Windows) to `/bin/sh -c` only (Shell contract, Check gate, Measure gate sections)

## 5. Verify

- [x] 5.1 Run `go build ./...` and confirm the package still builds on the supported (Unix) platform
- [x] 5.2 Run `task test` (`go test -timeout 30s ./...`) and confirm green
- [x] 5.3 Grep the repo for residual references: `windows`, `cmd.exe`, `runtime.GOOS`, `%AppData%`, `Setpgid`-conditional framing — confirm zero remaining in `*.go`, `*.md` (except this change's own artifacts), and CI
