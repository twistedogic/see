## 1. Add the YAML dependency and failing configuration tests

- [ ] 1.1 Add `go.yaml.in/yaml/v3` as the single direct YAML Ain't Markup Language (YAML) parser dependency.
- [ ] 1.2 Replace the legacy plain-text configuration fixtures in `discovery_test.go` with tests for a valid `config.yaml`, multiline prompt preservation, missing and empty files, unknown fields, wrong field types, malformed input, multiple documents, and the ignored legacy `watches` file.
- [ ] 1.3 Add a test proving `--ignore-config` returns an empty configuration without reading a malformed `config.yaml`.
- [ ] 1.4 Add tests for prompt precedence: nonblank command-line prompt over configured prompt, blank command-line prompt falling through to configured prompt, and both blank falling through to the embedded default.
- [ ] 1.5 Run the focused tests before production edits and confirm they fail for the missing YAML loader and prompt-selection behavior.

## 2. Promote the configuration loader

- [ ] 2.1 Replace `watchConfigPath` with a path resolver for `os.UserConfigDir()/see/config.yaml`, retaining the injectable `userConfigDir` lookup used by tests.
- [ ] 2.2 Replace `loadWatchConfig` with a `Config` struct and strict `loadConfig(path) (Config, error)` decoder for `watches` and `prompt`; accept missing or empty files and reject malformed input, extra documents, unknown fields, and wrong field types with path-aware errors.
- [ ] 2.3 Add the small ignore-aware startup loader that returns a zero-value `Config` without resolving or reading the file when `--ignore-config` is set.
- [ ] 2.4 Remove the legacy line scanner and all production references to the old `watches` path.
- [ ] 2.5 Run the focused configuration tests and confirm they pass.

## 3. Wire watches and prompt precedence

- [ ] 3.1 Change `resolveWatchList` to accept command-line and configured watch slices, union them, and preserve the current-working-directory fallback without performing configuration file input/output.
- [ ] 3.2 Add the minimal pure prompt selector that chooses a nonblank command-line prompt before the configured prompt, leaving embedded fallback normalization to `Watcher.SetPromptTemplate`.
- [ ] 3.3 Update `main` to load configuration once, pass `Config.Watches` to watch resolution, and set the watcher prompt from the command-line/configuration selection.
- [ ] 3.4 Update `--ignore-config` help text to describe skipping the whole global `config.yaml` file rather than only a watches file.
- [ ] 3.5 Update `AGENTS.md` with the global YAML configuration path, schema, clean migration from `watches`, and prompt precedence so the changed workflow remains discoverable.
- [ ] 3.6 Run the prompt, discovery, and watcher tests and confirm command-line overrides, configured defaults, ignored configuration, watch union, and embedded fallback are green.

## 4. Validate and synchronize

- [ ] 4.1 Run `gofmt` on changed Go files, then run `go test -timeout 30s ./...` and `go vet ./...`.
- [ ] 4.2 Run `go build ./...` to verify the YAML dependency and embedded prompt compile together.
- [ ] 4.3 Run `openspec validate --change promote-config-to-yaml` and resolve every reported issue.
- [ ] 4.4 Sync the watcher delta into the main specification after implementation and verification.
- [ ] 4.5 Archive `promote-config-to-yaml` only after every implementation task and validation check is complete.
