package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeConfigYAML drops a config.yaml at <base>/see/config.yaml with the
// given body and returns the absolute path. The helper mirrors the
// legacy writeWatchConfig shape so the file location matches what
// watchConfigPath resolves to in tests that pin userConfigDir.
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

// TestLoadConfigValid proves the happy path: a single YAML document with
// both watches and a multiline prompt decodes into the struct.
func TestLoadConfigValid(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "watches:\n  - /repos/alpha\n  - /repos/beta\nprompt: |\n  Apply the change {change}.\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	wantWatches := []string{"/repos/alpha", "/repos/beta"}
	if !reflect.DeepEqual(cfg.Watches, wantWatches) {
		t.Fatalf("Watches = %v, want %v", cfg.Watches, wantWatches)
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
	if cfg.Watches != nil {
		t.Fatalf("Watches = %v, want nil", cfg.Watches)
	}
	if cfg.Prompt != "" {
		t.Fatalf("Prompt = %q, want empty", cfg.Prompt)
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
	if cfg.Watches != nil {
		t.Fatalf("Watches = %v, want nil", cfg.Watches)
	}
	if cfg.Prompt != "" {
		t.Fatalf("Prompt = %q, want empty", cfg.Prompt)
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

// TestLoadConfigWrongTypeWatches proves the schema rejects wrong field
// types. watches must be a sequence of strings; a mapping is wrong.
func TestLoadConfigWrongTypeWatches(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "watches: {not: a sequence}\n")
	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("err = nil, want non-nil for wrong watches type")
	}
}

// TestLoadConfigMalformed covers unparseable YAML — a trailing `[`
// without a closing bracket.
func TestLoadConfigMalformed(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "watches: [\n")
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
	path := writeConfigYAML(t, base, "watches:\n  - /repos/a\n---\nwatches:\n  - /repos/b\n")
	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("err = nil, want non-nil for multi-document YAML")
	}
}

// TestLoadConfigIgnoresLegacyWatchesFile pins the breaking change:
// the legacy watches file is not consulted. With only the legacy file
// present and no config.yaml, the loader returns a zero-value Config.
func TestLoadConfigIgnoresLegacyWatchesFile(t *testing.T) {
	base := t.TempDir()
	seeDir := filepath.Join(base, "see")
	if err := os.MkdirAll(seeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(seeDir, "watches")
	if err := os.WriteFile(legacy, []byte("/legacy/path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(filepath.Join(seeDir, "config.yaml"))
	if err != nil {
		t.Fatalf("err = %v, want nil (legacy file must not be read)", err)
	}
	if cfg.Watches != nil {
		t.Fatalf("Watches = %v, want nil (legacy file must not contribute)", cfg.Watches)
	}
	if cfg.Prompt != "" {
		t.Fatalf("Prompt = %q, want empty", cfg.Prompt)
	}
}

// --- loadStartupConfig / --config: task 3.x -------------------------------

// TestLoadStartupConfigUnsetLoadsDefaultPath proves that an empty
// configFlag (the common case: no --config passed) loads the file at
// the resolved default config path.
func TestLoadStartupConfigUnsetLoadsDefaultPath(t *testing.T) {
	base := t.TempDir()
	writeConfigYAML(t, base, "watches:\n  - /repos/default\nprompt: from default\n")
	orig := userConfigDir
	t.Cleanup(func() { userConfigDir = orig })
	userConfigDir = func() (string, error) { return base, nil }

	cfg, err := loadStartupConfig("")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !reflect.DeepEqual(cfg.Watches, []string{"/repos/default"}) {
		t.Fatalf("Watches = %v, want [/repos/default]", cfg.Watches)
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
	if err := os.WriteFile(explicit, []byte("watches:\n  - /repos/explicit\nprompt: from explicit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := userConfigDir
	t.Cleanup(func() { userConfigDir = orig })
	userConfigDir = func() (string, error) { return base, nil }

	cfg, err := loadStartupConfig(explicit)
	if err != nil {
		t.Fatalf("err = %v, want nil (default path must be bypassed)", err)
	}
	if !reflect.DeepEqual(cfg.Watches, []string{"/repos/explicit"}) {
		t.Fatalf("Watches = %v, want [/repos/explicit]", cfg.Watches)
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
	orig := userConfigDir
	t.Cleanup(func() { userConfigDir = orig })
	userConfigDir = func() (string, error) { return base, nil }

	cfg, err := loadStartupConfig("-")
	if err != nil {
		t.Fatalf("err = %v, want nil (sentinel must not read the file)", err)
	}
	if cfg.Watches != nil || cfg.Prompt != "" {
		t.Fatalf("cfg = %+v, want zero-value Config", cfg)
	}
}

// TestLoadStartupConfigTildeExpansion proves that a leading "~/" in
// the configFlag value is expanded against $HOME, consistent with
// --watch.
func TestLoadStartupConfigTildeExpansion(t *testing.T) {
	home := t.TempDir()
	explicit := filepath.Join(home, "see.yaml")
	if err := os.WriteFile(explicit, []byte("watches:\n  - /repos/tilde\nprompt: from tilde\n"), 0o644); err != nil {
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
	if !reflect.DeepEqual(cfg.Watches, []string{"/repos/tilde"}) {
		t.Fatalf("Watches = %v, want [/repos/tilde]", cfg.Watches)
	}
	if cfg.Prompt != "from tilde" {
		t.Fatalf("Prompt = %q, want %q", cfg.Prompt, "from tilde")
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
