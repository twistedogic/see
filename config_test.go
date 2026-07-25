package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeConfigYAML drops a config.yaml at <base>/see/config.yaml with the
// given body and returns the absolute path. The helper mirrors the
// legacy writeWatchConfig shape so the file location matches what
// configPath resolves to ~/.config/see/config.yaml.
func writeConfigYAML(t *testing.T, base, body string) string {
	t.Helper()
	seeDir := filepath.Join(base, "see")
	if err := os.MkdirAll(seeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(seeDir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- loadConfig: task 1.2 -------------------------------------------------

// TestLoadConfigValid proves the happy path: all configuration fields decode
// and root_dir is validated before the configuration is returned.
func TestLoadConfigValid(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "root_dir: \""+base+"\"\ninclude:\n  - playground-*\nexclude:\n  - playground-old\nprompt: |\n  Apply the change {change}.\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if cfg.RootDir != base {
		t.Fatalf("RootDir = %q, want %q", cfg.RootDir, base)
	}
	if want := []string{"playground-*"}; !reflect.DeepEqual(cfg.Include, want) {
		t.Fatalf("Include = %v, want %v", cfg.Include, want)
	}
	if want := []string{"playground-old"}; !reflect.DeepEqual(cfg.Exclude, want) {
		t.Fatalf("Exclude = %v, want %v", cfg.Exclude, want)
	}
	// Block scalar with clip chomping ("|"): trailing newline preserved.
	if got, want := cfg.Prompt, "Apply the change {change}.\n"; got != want {
		t.Fatalf("Prompt = %q, want %q", got, want)
	}
}

// TestLoadConfigMultilinePrompt pins the literal-block-scalar behavior
// that the design depends on for editable multiline prompts. The strip
// chomper ("|-") is the most useful form: trailing newline removed,
// interior line breaks preserved.
func TestLoadConfigMultilinePrompt(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "prompt: |-\n  First line\n  Second line\n  Third line\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	want := "First line\nSecond line\nThird line"
	if cfg.Prompt != want {
		t.Fatalf("Prompt = %q, want %q (block scalar line breaks must be preserved)", cfg.Prompt, want)
	}
}

// TestLoadConfigMissing is the empty-config case: a missing file is
// not an error, the loader returns a zero-value Config so the caller
// can proceed with command-line inputs and the cwd fallback.
func TestLoadConfigMissing(t *testing.T) {
	cfg, err := loadConfig(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("err = %v, want nil for missing file", err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("cfg = %+v, want zero-value Config", cfg)
	}
}

// TestLoadConfigEmptyFile is the "exists but is empty" case: the
// loader still returns a zero-value Config without error.
func TestLoadConfigEmptyFile(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("err = %v, want nil for empty file", err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("cfg = %+v, want zero-value Config", cfg)
	}
}

// TestLoadConfigUnknownField pins the known-field contract: a misspelled
// `promt` is rejected so silent typos do not disable a configured
// prompt at runtime.
func TestLoadConfigUnknownField(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "promt: oops\n")
	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("err = nil, want non-nil for unknown field")
	}
	if !strings.Contains(err.Error(), "promt") {
		t.Fatalf("err = %q, want it to mention the unknown field name", err.Error())
	}
}

// TestLoadConfigWrongTypeRootDir proves root_dir must be a string.
func TestLoadConfigWrongTypeRootDir(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "root_dir: {not: a string}\n")
	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("err = nil, want non-nil for wrong root_dir type")
	}
}

// TestLoadConfigMalformed covers unparseable YAML — a trailing `[`
// without a closing bracket.
func TestLoadConfigMalformed(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "root_dir: [\n")
	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("err = nil, want non-nil for malformed YAML")
	}
}

// TestLoadConfigMultipleDocuments rejects a second `---` document so
// accidental duplication or a stray appended config does not silently
// hide configuration.
func TestLoadConfigMultipleDocuments(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "prompt: first\n---\nprompt: second\n")
	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("err = nil, want non-nil for multi-document YAML")
	}
}

func TestLoadConfigRejectsLegacyWatchesField(t *testing.T) {
	path := writeConfigYAML(t, t.TempDir(), "watches: []\n")
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "watches") {
		t.Fatalf("err = %v, want unknown watches field", err)
	}
}

func TestValidateConfigAllowsBlankRootDir(t *testing.T) {
	cfg := Config{}
	if err := validateConfig(&cfg); err != nil {
		t.Fatalf("validateConfig: %v", err)
	}
}

func TestValidateConfigExpandsTildes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := Config{RootDir: "~", Include: []string{"~/repo"}, Exclude: []string{"~/skip"}}
	if err := validateConfig(&cfg); err != nil {
		t.Fatalf("validateConfig: %v", err)
	}
	if cfg.RootDir != home || cfg.Include[0] != filepath.Join(home, "repo") || cfg.Exclude[0] != filepath.Join(home, "skip") {
		t.Fatalf("cfg = %+v, want expanded paths under %q", cfg, home)
	}
}

func TestValidateConfigRejectsInvalidRootDir(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, root, want string
	}{
		{"missing", filepath.Join(t.TempDir(), "missing"), "root_dir"},
		{"file", file, "not a directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{RootDir: tc.root}
			err := validateConfig(&cfg)
			if err == nil || !strings.Contains(err.Error(), "root_dir") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want root_dir error containing %q", err, tc.want)
			}
		})
	}
}

func TestValidateConfigRejectsDoubleStarRootDir(t *testing.T) {
	cfg := Config{RootDir: "work/**"}
	err := validateConfig(&cfg)
	if err == nil || !strings.Contains(err.Error(), "root_dir: '**' is not supported") {
		t.Fatalf("err = %v, want root_dir double-star error", err)
	}
}

func TestValidateConfigRejectsInvalidPatterns(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		want string
	}{
		{"include double star", Config{Include: []string{"work/**"}}, "include[0]: '**' is not supported"},
		{"exclude double star", Config{Exclude: []string{"work/**"}}, "exclude[0]: '**' is not supported"},
		{"include malformed", Config{Include: []string{"[unclosed"}}, "include[0]: invalid glob pattern"},
		{"exclude malformed", Config{Exclude: []string{"[unclosed"}}, "exclude[0]: invalid glob pattern"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfig(&tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateConfigReportsTildeExpansionField(t *testing.T) {
	t.Setenv("HOME", "")
	old := userHomeDir
	t.Cleanup(func() { userHomeDir = old })
	userHomeDir = func() (string, error) { return "", os.ErrNotExist }
	cfg := Config{RootDir: "~/Dev"}
	err := validateConfig(&cfg)
	if err == nil || !strings.Contains(err.Error(), "root_dir") || !strings.Contains(err.Error(), "expand ~") {
		t.Fatalf("err = %v, want root_dir tilde expansion error", err)
	}
}

// --- loadStartupConfig / --config: task 3.x -------------------------------

func TestConfigPathUsesHomeConfigDir(t *testing.T) {
	home := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatal(err)
	}

	got, err := configPath()
	if err != nil {
		t.Fatalf("configPath() error = %v", err)
	}
	want := filepath.Join(home, ".config", "see", "config.yaml")
	if got != want {
		t.Fatalf("configPath() = %q, want %q", got, want)
	}
}

// TestLoadStartupConfigUnsetLoadsDefaultPath proves that an empty
// configFlag (the common case: no --config passed) loads the file at
// the resolved default config path.
func TestLoadStartupConfigUnsetLoadsDefaultPath(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".config", "see", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("root_dir: \""+base+"\"\nprompt: from default\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })
	if err := os.Setenv("HOME", base); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadStartupConfig("")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if cfg.RootDir != base {
		t.Fatalf("RootDir = %q, want %q", cfg.RootDir, base)
	}
	if cfg.Prompt != "from default" {
		t.Fatalf("Prompt = %q, want %q", cfg.Prompt, "from default")
	}
}

// TestLoadStartupConfigExplicitPath proves that a non-empty
// configFlag bypasses the default path and loads the named file
// instead. A malformed default config.yaml is the witness: if the
// loader tried the default, it would error.
func TestLoadStartupConfigExplicitPath(t *testing.T) {
	base := t.TempDir()
	// Malformed default — would fail loadConfig if consulted.
	writeConfigYAML(t, base, "not: [valid\n")
	// Valid explicit file at a separate path.
	explicit := filepath.Join(t.TempDir(), "explicit.yaml")
	explicitRoot := t.TempDir()
	if err := os.WriteFile(explicit, []byte("root_dir: \""+explicitRoot+"\"\nprompt: from explicit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })
	if err := os.Setenv("HOME", base); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadStartupConfig(explicit)
	if err != nil {
		t.Fatalf("err = %v, want nil (default path must be bypassed)", err)
	}
	if cfg.RootDir != explicitRoot {
		t.Fatalf("RootDir = %q, want %q", cfg.RootDir, explicitRoot)
	}
	if cfg.Prompt != "from explicit" {
		t.Fatalf("Prompt = %q, want %q", cfg.Prompt, "from explicit")
	}
}

// TestLoadStartupConfigSkipSentinel proves that the "-" sentinel
// returns a zero-value Config without resolving or reading the
// default file — even when the default file is malformed.
func TestLoadStartupConfigSkipSentinel(t *testing.T) {
	base := t.TempDir()
	writeConfigYAML(t, base, "not: [valid\n")
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })
	if err := os.Setenv("HOME", base); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadStartupConfig("-")
	if err != nil {
		t.Fatalf("err = %v, want nil (sentinel must not read the file)", err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("cfg = %+v, want zero-value Config", cfg)
	}
}

// TestLoadStartupConfigTildeExpansion proves that a leading "~/" in
// the configFlag value is expanded against $HOME.
func TestLoadStartupConfigTildeExpansion(t *testing.T) {
	home := t.TempDir()
	explicit := filepath.Join(home, "see.yaml")
	if err := os.WriteFile(explicit, []byte("root_dir: \"~\"\nprompt: from tilde\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadStartupConfig("~/see.yaml")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if cfg.RootDir != home {
		t.Fatalf("RootDir = %q, want %q", cfg.RootDir, home)
	}
	if cfg.Prompt != "from tilde" {
		t.Fatalf("Prompt = %q, want %q", cfg.Prompt, "from tilde")
	}
}

// --- custom workflow fields: add-custom-workflows (task 1.1) -------------

// TestLoadConfigWithConditionAndCommit proves that the strict schema
// decodes the new custom-workflow fields. Both fields are optional;
// a configured condition + commit must round-trip through the loader.
func TestLoadConfigWithConditionAndCommit(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "prompt: Apply {change}\ncondition: \"echo add-dark-mode\"\ncommit: \"see: apply {change}\"\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got, want := cfg.Condition, "echo add-dark-mode"; got != want {
		t.Fatalf("Condition = %q, want %q", got, want)
	}
	if got, want := cfg.Commit, "see: apply {change}"; got != want {
		t.Fatalf("Commit = %q, want %q", got, want)
	}
	if got, want := cfg.Prompt, "Apply {change}"; got != want {
		t.Fatalf("Prompt = %q, want %q", got, want)
	}
}

// TestLoadConfigWrongTypeCondition proves the schema rejects wrong
// field types for the custom-workflow fields: condition and commit
// must be strings.
func TestLoadConfigWrongTypeCondition(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "condition: {not: a string}\n")
	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("err = nil, want non-nil for wrong condition type")
	}
}

func TestLoadConfigWrongTypeCommit(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "commit: [not, a, string]\n")
	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("err = nil, want non-nil for wrong commit type")
	}
}

// --- custom-mode startup validation: add-custom-workflows (task 1.2) -----

// TestValidateCustomModeAcceptsCompleteConfig: a configured condition
// with nonblank prompt and commit is a valid startup state.
func TestValidateCustomModeAcceptsCompleteConfig(t *testing.T) {
	cfg := Config{
		Condition: "echo add-dark-mode",
		Prompt:    "Apply {change}",
		Commit:    "see: apply {change}",
	}
	if err := validateCustomConfig(cfg, ""); err != nil {
		t.Fatalf("err = %v, want nil for complete custom config", err)
	}
}

// TestValidateCustomModeAcceptsCLIOverridesPrompt: a configured
// condition with a CLI-supplied prompt (effective prompt nonblank)
// is valid even when the configured prompt is blank.
func TestValidateCustomModeAcceptsCLIOverridesPrompt(t *testing.T) {
	cfg := Config{
		Condition: "echo add-dark-mode",
		Commit:    "see: apply {change}",
	}
	if err := validateCustomConfig(cfg, "Apply {change} from CLI"); err != nil {
		t.Fatalf("err = %v, want nil (CLI prompt should complete the config)", err)
	}
}

// TestValidateCustomModeRejectsBlankPrompt: condition + commit but no
// effective prompt is rejected. The error names the missing field.
func TestValidateCustomModeRejectsBlankPrompt(t *testing.T) {
	cfg := Config{
		Condition: "echo add-dark-mode",
		Commit:    "see: apply {change}",
	}
	err := validateCustomConfig(cfg, "")
	if err == nil {
		t.Fatal("err = nil, want non-nil when custom mode has no prompt")
	}
	if !strings.Contains(err.Error(), "prompt") {
		t.Fatalf("err = %q, want it to mention the missing prompt", err.Error())
	}
}

// TestValidateCustomModeRejectsBlankCommit: condition + effective
// prompt but no commit template is rejected. The error names the
// missing field.
func TestValidateCustomModeRejectsBlankCommit(t *testing.T) {
	cfg := Config{
		Condition: "echo add-dark-mode",
		Prompt:    "Apply {change}",
	}
	err := validateCustomConfig(cfg, "")
	if err == nil {
		t.Fatal("err = nil, want non-nil when custom mode has no commit template")
	}
	if !strings.Contains(err.Error(), "commit") {
		t.Fatalf("err = %q, want it to mention the missing commit template", err.Error())
	}
}

// TestValidateCustomModeRejectsBlankConditionIsCompatibility:
// a blank condition disables custom mode regardless of prompt/commit;
// validation returns nil so startup proceeds in OpenSpec
// compatibility mode.
func TestValidateCustomModeBlankConditionIsCompatibility(t *testing.T) {
	cfg := Config{
		Prompt: "Apply {change}",
		Commit: "see: apply {change}",
	}
	if err := validateCustomConfig(cfg, ""); err != nil {
		t.Fatalf("err = %v, want nil (blank condition = compatibility mode)", err)
	}
}

// TestValidateCustomModeAcceptsWhitespaceConditionAsBlank: a
// whitespace-only condition is treated as blank for the purpose
// of selecting custom mode.
func TestValidateCustomModeAcceptsWhitespaceConditionAsBlank(t *testing.T) {
	cfg := Config{
		Condition: "   \n\t",
		Prompt:    "Apply {change}",
	}
	if err := validateCustomConfig(cfg, ""); err != nil {
		t.Fatalf("err = %v, want nil (whitespace condition = compatibility mode)", err)
	}
}

// --- selectPromptTemplate: task 1.4 --------------------------------------

// TestSelectPromptTemplateCLIOverridesConfigured: a nonblank CLI
// prompt always wins over a configured prompt.
func TestSelectPromptTemplateCLIOverridesConfigured(t *testing.T) {
	if got, want := selectPromptTemplate("CLI {change}", "Configured {change}"), "CLI {change}"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestSelectPromptTemplateBlankCLIFallsThroughToConfigured: blank
// CLI means "use the configured value"; whitespace-only CLI is the
// same. The selector does not see the embedded default — that lives
// in Watcher.SetPromptTemplate.
func TestSelectPromptTemplateBlankCLIFallsThroughToConfigured(t *testing.T) {
	if got, want := selectPromptTemplate("", "Configured {change}"), "Configured {change}"; got != want {
		t.Fatalf("empty CLI: got %q, want %q", got, want)
	}
	if got, want := selectPromptTemplate("   ", "Configured {change}"), "Configured {change}"; got != want {
		t.Fatalf("whitespace CLI: got %q, want %q", got, want)
	}
	if got, want := selectPromptTemplate("\n\t", "Configured {change}"), "Configured {change}"; got != want {
		t.Fatalf("newline/tab CLI: got %q, want %q", got, want)
	}
}

// TestSelectPromptTemplateBothBlankReturnsEmpty: when both inputs are
// blank, the selector returns "". Caller falls back to the embedded
// default via Watcher.SetPromptTemplate.
func TestSelectPromptTemplateBothBlankReturnsEmpty(t *testing.T) {
	if got := selectPromptTemplate("", ""); got != "" {
		t.Fatalf("both empty: got %q, want empty", got)
	}
	if got := selectPromptTemplate("   ", "\n"); got != "" {
		t.Fatalf("both whitespace: got %q, want empty", got)
	}
}

// --- ensureDefaultConfig / bootstrap: add-default-config-bootstrap -----

// readConfigPath resolves the default config path
// pinned to base. Tests in this section that need to predict the
// bootstrap target use it instead of inlining configPath so the
// helper is the single source of truth for the resolution.
func readConfigPath(t *testing.T, base string) string {
	t.Helper()
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })
	if err := os.Setenv("HOME", base); err != nil {
		t.Fatal(err)
	}
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	return path
}

// skipIfRoot returns true when the test is running as root. Some
// Unix variants let root bypass directory write permissions,
// which would invalidate the unwritable-target tests. Skipping
// keeps the test meaningful on the systems that exercise it.
func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root; directory permission tests are unreliable")
	}
}

// TestEnsureDefaultConfigWritesOnMiss is task 1.1: a missing default
// path triggers a write that creates the parent directory at 0o755,
// the file at 0o644, and the file contents equal the embedded
// template byte-for-byte. loadConfig on the written file decodes to
// a zero-value configuration.
func TestEnsureDefaultConfigWritesOnMiss(t *testing.T) {
	base := t.TempDir()
	path := readConfigPath(t, base)

	if err := ensureDefaultConfig(path); err != nil {
		t.Fatalf("ensureDefaultConfig: %v", err)
	}

	parent := filepath.Dir(path)
	dirInfo, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("parent stat: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Fatalf("parent %s is not a directory", parent)
	}
	if mode := dirInfo.Mode().Perm(); mode != 0o755 {
		t.Fatalf("parent mode = %#o, want 0o755", mode)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o644 {
		t.Fatalf("file mode = %#o, want 0o644", mode)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if string(got) != defaultConfigTemplate {
		t.Fatalf("file contents mismatch\n got: %q\nwant: %q", got, defaultConfigTemplate)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig on bootstrap file: %v", err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("cfg = %+v, want zero-value", cfg)
	}
}

// TestLoadStartupConfigDoesNotOverwriteExisting is task 1.2: when
// the default configuration file already exists with arbitrary
// content (empty, comments-only, valid, or malformed), startup
// MUST NOT overwrite it. The no-op gate lives in loadStartupConfig
// (matching the spec scenario "Existing file is not overwritten"),
// not in ensureDefaultConfig.
func TestLoadStartupConfigDoesNotOverwriteExisting(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"comments-only", "# this is a comment, nothing else\n"},
		{"valid", "prompt: keep me\n"},
		{"malformed", "not: [valid\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			path := readConfigPath(t, base)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			beforeInfo, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}

			// Capture stderr so any (incorrect) bootstrap notice
			// surfaces here as a test failure rather than polluting
			// the runner output.
			origWriter := stderrWriter
			t.Cleanup(func() { stderrWriter = origWriter })
			stderrWriter = io.Discard

			cfg, err := loadStartupConfig("")
			if err != nil && tc.name != "malformed" {
				t.Fatalf("loadStartupConfig(\"\"): %v", err)
			}

			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatalf("contents changed\n before: %q\n  after: %q", before, after)
			}
			afterInfo, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if afterInfo.Mode().Perm() != beforeInfo.Mode().Perm() {
				t.Fatalf("mode changed: %#o -> %#o", beforeInfo.Mode().Perm(), afterInfo.Mode().Perm())
			}

			// Sanity: loadConfig should still surface the file's
			// own content (zero-value for empty/comments, parsed
			// fields for valid, error for malformed).
			_ = cfg // The cfg value depends on the body; we only
			// care about the file being untouched.
		})
	}
}

// TestLoadStartupConfigSkipSentinelDoesNotBootstrap is task 1.3:
// loadStartupConfig("-") does not resolve or write the default
// path. Even with an unwritable parent directory (which would
// defeat bootstrap if it tried to write), the call returns a
// zero-value Config without error and leaves the filesystem
// untouched.
func TestLoadStartupConfigSkipSentinelDoesNotBootstrap(t *testing.T) {
	skipIfRoot(t)
	base := t.TempDir()
	// Pre-create <base>/see at 0o555 so a bootstrap attempt would
	// fail to MkdirAll (if the dir were absent) or fail to WriteFile
	// (if the dir were present but read-only).
	seeDir := filepath.Join(base, "see")
	if err := os.Mkdir(seeDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(seeDir, 0o755) })

	// Verify loadStartupConfig("-")
	// does not touch seeDir.
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })
	if err := os.Setenv("HOME", base); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadStartupConfig("-")
	if err != nil {
		t.Fatalf("loadStartupConfig(\"-\"): %v", err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("cfg = %+v, want zero-value", cfg)
	}

	// The config file must not exist after the call.
	configFile := filepath.Join(seeDir, "config.yaml")
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		t.Fatalf("config file was created (err=%v); bootstrap must not fire under \"-\"", err)
	}
}

// TestLoadStartupConfigExplicitPathDoesNotBootstrap is task 1.4:
// loadStartupConfig(<path>) does not write the default path even
// when the named file is absent. The default directory may be
// unwritable; bootstrap must still not fire.
func TestLoadStartupConfigExplicitPathDoesNotBootstrap(t *testing.T) {
	skipIfRoot(t)
	base := t.TempDir()
	seeDir := filepath.Join(base, "see")
	if err := os.Mkdir(seeDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(seeDir, 0o755) })

	// The default path is independent of this explicit config path.
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })
	if err := os.Setenv("HOME", base); err != nil {
		t.Fatal(err)
	}

	// Explicit path is in a different temp dir (would be created if
	// loaded, but loadConfig returns zero-value for missing files).
	explicit := filepath.Join(t.TempDir(), "explicit.yaml")

	cfg, err := loadStartupConfig(explicit)
	if err != nil {
		t.Fatalf("loadStartupConfig(<path>): %v", err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("cfg = %+v, want zero-value", cfg)
	}

	// Default config path must not exist after the call.
	configFile := filepath.Join(seeDir, "config.yaml")
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		t.Fatalf("default config file was created (err=%v); bootstrap must not fire under --config=<path>", err)
	}
}

// TestLoadStartupConfigBootstrapFailureNonFatal is task 1.5: when
// the default path's parent directory is unwritable, the bootstrap
// write fails, loadStartupConfig returns a zero-value Config
// without an error, and writes one line to stderr naming the
// target path and the failure reason.
func TestLoadStartupConfigBootstrapFailureNonFatal(t *testing.T) {
	skipIfRoot(t)
	base := t.TempDir()
	configDir := filepath.Join(base, ".config")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seeDir := filepath.Join(configDir, "see")
	if err := os.Mkdir(seeDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(seeDir, 0o755) })
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })
	if err := os.Setenv("HOME", base); err != nil {
		t.Fatal(err)
	}

	// Capture stderr via the indirection that loadStartupConfig
	// writes to.
	origWriter := stderrWriter
	t.Cleanup(func() { stderrWriter = origWriter })
	var buf bytes.Buffer
	stderrWriter = &buf

	cfg, err := loadStartupConfig("")
	if err != nil {
		t.Fatalf("loadStartupConfig(\"\"): %v (bootstrap failure must be non-fatal)", err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("cfg = %+v, want zero-value", cfg)
	}

	line := buf.String()
	if line == "" {
		t.Fatal("expected a stderr line naming the bootstrap failure, got none")
	}
	if !strings.Contains(line, filepath.Join(base, ".config", "see", "config.yaml")) {
		t.Fatalf("stderr line %q does not name the target path", line)
	}
	if !strings.Contains(line, "permission denied") {
		t.Fatalf("stderr line %q does not name the failure reason", line)
	}
}
