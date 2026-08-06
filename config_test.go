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
	path := writeConfigYAML(t, base, "root_dir: \""+base+"\"\ninclude:\n  - playground-*\nexclude:\n  - playground-old\n")
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

// --- legacy top-level field rejection: support-multiple-workflows (1.1) --

// TestLoadConfigRejectsLegacyPromptField proves the top-level
// `prompt` is no longer accepted: the strict decoder must surface
// an unknown-field error so a migrated config that still carries
// the old key fails fast instead of silently ignoring it.
func TestLoadConfigRejectsLegacyPromptField(t *testing.T) {
	path := writeConfigYAML(t, t.TempDir(), "prompt: \"Apply {change}\"\n")
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "prompt") {
		t.Fatalf("err = %v, want unknown prompt field", err)
	}
}

// TestLoadConfigRejectsLegacyConditionField proves the top-level
// `condition` is no longer accepted; the new schema requires the
// field inside a named workflows entry.
func TestLoadConfigRejectsLegacyConditionField(t *testing.T) {
	path := writeConfigYAML(t, t.TempDir(), "condition: \"echo add-dark-mode\"\n")
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "condition") {
		t.Fatalf("err = %v, want unknown condition field", err)
	}
}

// TestLoadConfigRejectsLegacyCommitField proves the top-level
// `commit` is no longer accepted; the new schema requires the
// field inside a named workflows entry.
func TestLoadConfigRejectsLegacyCommitField(t *testing.T) {
	path := writeConfigYAML(t, t.TempDir(), "commit: \"see: apply {change}\"\n")
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "commit") {
		t.Fatalf("err = %v, want unknown commit field", err)
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
	if err := os.WriteFile(path, []byte("root_dir: \""+base+"\"\n"), 0o644); err != nil {
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
	if err := os.WriteFile(explicit, []byte("root_dir: \""+explicitRoot+"\"\n"), 0o644); err != nil {
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
	if err := os.WriteFile(explicit, []byte("root_dir: \"~\"\n"), 0o644); err != nil {
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
}

// --- custom workflow fields: add-custom-workflows (task 1.1) -------------

// TestLoadConfigWithConditionAndCommit proves that the strict schema
// --- multi-workflow schema: support-multiple-workflows (task 1.2) -------

// --- multi-workflow schema: support-multiple-workflows (task 1.2) -------

// TestLoadConfigWithWorkflows proves the new `workflows` sequence
// decodes in order and every per-workflow field round-trips. The
// schema is opt-in: when the key is absent, the slice is nil and
// OpenSpec compatibility remains in effect.
func TestLoadConfigWithWorkflows(t *testing.T) {
	base := t.TempDir()
	body := `workflows:
  - name: openspec
    prompt: "Apply {change}"
    condition: "echo openspec-change"
    commit: "see: apply openspec {change}"
  - name: update
    prompt: "Bump {change}"
    condition: "echo package-update"
    commit: "see: bump {change}"
    model: "openai/gpt-5-mini"
`
	path := writeConfigYAML(t, base, body)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(cfg.Workflows) != 2 {
		t.Fatalf("len(Workflows) = %d, want 2", len(cfg.Workflows))
	}
	if cfg.Workflows[0] != (WorkflowConfig{
		Name: "openspec", Prompt: "Apply {change}",
		Condition: "echo openspec-change", Commit: "see: apply openspec {change}",
	}) {
		t.Fatalf("Workflows[0] = %+v, want openspec entry", cfg.Workflows[0])
	}
	if cfg.Workflows[1] != (WorkflowConfig{
		Name: "update", Prompt: "Bump {change}",
		Condition: "echo package-update", Commit: "see: bump {change}",
		Model: "openai/gpt-5-mini",
	}) {
		t.Fatalf("Workflows[1] = %+v, want update entry", cfg.Workflows[1])
	}
}

// TestLoadConfigWorkflowModelBlankIsUnset proves omitted and whitespace-only
// model values decode without adding a required-field validation rule.
func TestLoadConfigWorkflowModelBlankIsUnset(t *testing.T) {
	base := t.TempDir()
	body := `workflows:
  - name: omitted
    prompt: "Apply {change}"
    condition: "echo omitted"
    commit: "see: apply {change}"
  - name: blank
    prompt: "Apply {change}"
    condition: "echo blank"
    commit: "see: apply {change}"
    model: "  "
`
	cfg, err := loadConfig(writeConfigYAML(t, base, body))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got := cfg.Workflows[0].Model; got != "" {
		t.Fatalf("omitted model = %q, want empty", got)
	}
	if got := cfg.Workflows[1].Model; got != "  " {
		t.Fatalf("blank model = %q, want decoded whitespace for call-site trimming", got)
	}
}

// TestLoadConfigWorkflowDisableRoundTrip proves the disable field
// round-trips through the strict decoder onto WorkflowConfig.Disable,
// and that an omitted disable decodes to false (enabled), identical to
// the pre-field behavior.
func TestLoadConfigWorkflowDisableRoundTrip(t *testing.T) {
	base := t.TempDir()
	body := `workflows:
  - name: parked
    prompt: "Apply {change}"
    condition: "echo parked"
    commit: "see: apply {change}"
    disable: true
  - name: live
    prompt: "Apply {change}"
    condition: "echo live"
    commit: "see: apply {change}"
`
	cfg, err := loadConfig(writeConfigYAML(t, base, body))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !cfg.Workflows[0].Disable {
		t.Fatalf("Workflows[0].Disable = false, want true")
	}
	if cfg.Workflows[1].Disable {
		t.Fatalf("Workflows[1].Disable = true, want false for omitted field")
	}
}

// TestLoadConfigWorkflowsMultilinePrompt pins the literal-block-scalar
// behavior for per-workflow prompts so multiline prompts survive the
// decoder round-trip.
func TestLoadConfigWorkflowsMultilinePrompt(t *testing.T) {
	base := t.TempDir()
	body := "workflows:\n  - name: openspec\n    prompt: |-\n      First line\n      Second line\n    condition: \"echo x\"\n    commit: \"see: {change}\"\n"
	path := writeConfigYAML(t, base, body)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got, want := cfg.Workflows[0].Prompt, "First line\nSecond line"; got != want {
		t.Fatalf("Workflows[0].Prompt = %q, want %q", got, want)
	}
}

// TestLoadConfigWorkflowsWrongType proves the strict schema rejects
// non-string fields inside a workflow entry.
func TestLoadConfigWorkflowsWrongType(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "workflows:\n  - name: openspec\n    prompt: [not, a, string]\n    condition: \"echo x\"\n    commit: \"see: {change}\"\n")
	if _, err := loadConfig(path); err == nil {
		t.Fatal("err = nil, want non-nil for wrong prompt type")
	}
}

// TestLoadConfigWorkflowsCheckRoundTrip: the optional `check`
// field round-trips through the strict decoder onto WorkflowConfig.Check,
// and an absent check decodes to "". Blank (absent or whitespace-only)
// is the "no check gate" case; workflows behave identically to before
// the field existed.
func TestLoadConfigWorkflowsCheckRoundTrip(t *testing.T) {
	base := t.TempDir()
	body := `workflows:
  - name: gated
    prompt: "Apply {change}"
    condition: "echo gated"
    commit: "see: apply {change}"
    check: "go test ./..."
  - name: ungated
    prompt: "Apply {change}"
    condition: "echo ungated"
    commit: "see: apply {change}"
  - name: blank
    prompt: "Apply {change}"
    condition: "echo blank"
    commit: "see: apply {change}"
    check: "  "
`
	cfg, err := loadConfig(writeConfigYAML(t, base, body))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got := cfg.Workflows[0].Check; got != "go test ./..." {
		t.Fatalf("Workflows[0].Check = %q, want rendered value", got)
	}
	if got := cfg.Workflows[1].Check; got != "" {
		t.Fatalf("Workflows[1].Check = %q, want empty for omitted field", got)
	}
	// Blank check is the "no gate" case, identical to absent; it must
	// not fail validation or decoding.
	if got := cfg.Workflows[2].Check; got != "  " {
		t.Fatalf("Workflows[2].Check = %q, want raw whitespace (call site trims)", got)
	}
}

// TestLoadConfigWorkflowsUnknownField proves per-workflow entries
// inherit the strict known-field contract: an unknown sub-field is
// rejected so a typo does not silently disable a workflow.
func TestLoadConfigWorkflowsUnknownField(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "workflows:\n  - name: openspec\n    promt: \"oops\"\n    condition: \"echo x\"\n    commit: \"see: {change}\"\n")
	if _, err := loadConfig(path); err == nil {
		t.Fatal("err = nil, want non-nil for unknown workflow sub-field")
	}
}

// TestLoadConfigWorkflowsEmptyIsCompatibility proves omitting the
// `workflows` key leaves the slice nil so OpenSpec compatibility
// remains active.
func TestLoadConfigWorkflowsEmptyIsCompatibility(t *testing.T) {
	base := t.TempDir()
	path := writeConfigYAML(t, base, "root_dir: \""+base+"\"\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if cfg.Workflows != nil {
		t.Fatalf("Workflows = %+v, want nil for omitted workflows key", cfg.Workflows)
	}
}

// TestValidateWorkflowsAcceptsComplete proves a non-empty workflows
// slice with valid entries passes validation.
func TestValidateWorkflowsAcceptsComplete(t *testing.T) {
	cfg := Config{Workflows: []WorkflowConfig{
		{Name: "openspec", Prompt: "Apply {change}", Condition: "echo x", Commit: "see: {change}"},
		{Name: "update", Prompt: "Bump {change}", Condition: "echo y", Commit: "see: bump {change}"},
	}}
	if err := validateWorkflows(cfg); err != nil {
		t.Fatalf("err = %v, want nil for complete workflows", err)
	}
}

// TestValidateWorkflowsRejectsBlankField proves every required
// field is enforced; the error names the offending index and field.
func TestValidateWorkflowsRejectsBlankField(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*WorkflowConfig)
		wantErr string
	}{
		{"blank name", func(w *WorkflowConfig) { w.Name = "  " }, "name"},
		{"blank prompt", func(w *WorkflowConfig) { w.Prompt = "" }, "prompt"},
		{"blank condition", func(w *WorkflowConfig) { w.Condition = "\n\t" }, "condition"},
		{"blank commit", func(w *WorkflowConfig) { w.Commit = "" }, "commit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{Workflows: []WorkflowConfig{
				{Name: "openspec", Prompt: "Apply {change}", Condition: "echo x", Commit: "see: {change}"},
			}}
			tc.mutate(&cfg.Workflows[0])
			err := validateWorkflows(cfg)
			if err == nil {
				t.Fatal("err = nil, want non-nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %q, want it to mention %q", err.Error(), tc.wantErr)
			}
			if !strings.Contains(err.Error(), "workflows[0]") {
				t.Fatalf("err = %q, want it to identify workflows[0]", err.Error())
			}
		})
	}
}

// TestValidateWorkflowsRejectsDuplicateName proves uniqueness is
// enforced so two workflows with the same name cannot produce the
// same branch and log identity.
func TestValidateWorkflowsRejectsDuplicateName(t *testing.T) {
	cfg := Config{Workflows: []WorkflowConfig{
		{Name: "openspec", Prompt: "Apply {change}", Condition: "echo x", Commit: "see: {change}"},
		{Name: "openspec", Prompt: "Bump {change}", Condition: "echo y", Commit: "see: bump {change}"},
	}}
	err := validateWorkflows(cfg)
	if err == nil {
		t.Fatal("err = nil, want non-nil for duplicate workflow names")
	}
	if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "openspec") {
		t.Fatalf("err = %q, want it to mention duplicate and the name", err.Error())
	}
}

// TestValidateWorkflowsEmptyIsCompatibility proves an empty slice
// is the OpenSpec compatibility path; validation returns nil so
// startup proceeds without forcing the operator to declare a
// workflow.
func TestValidateWorkflowsEmptyIsCompatibility(t *testing.T) {
	for _, c := range []Config{
		{},
		{Workflows: nil},
		{Workflows: []WorkflowConfig{}},
	} {
		if err := validateWorkflows(c); err != nil {
			t.Fatalf("err = %v, want nil for empty workflows", err)
		}
	}
}

// TestValidateWorkflowsRejectsBlankMeasure proves the optional
// `measure` field follows the "present blank is fatal, absent is
// fine" contract: a workflow entry that explicitly sets measure to
// a blank/whitespace-only value fails startup with an error naming
// the workflow and the field. Absent measure (nil) passes through
// because the validator uses a pointer to distinguish "absent"
// from "present blank".
func TestValidateWorkflowsRejectsBlankMeasure(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"empty string", ""},
		{"whitespace only", "  \t\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blank := tc.value
			cfg := Config{Workflows: []WorkflowConfig{
				{Name: "openspec", Prompt: "Apply {change}", Condition: "echo x", Commit: "see: {change}", Measure: &blank},
			}}
			err := validateWorkflows(cfg)
			if err == nil {
				t.Fatal("err = nil, want non-nil for blank measure")
			}
			if !strings.Contains(err.Error(), "workflows[0]") {
				t.Fatalf("err = %q, want it to identify workflows[0]", err.Error())
			}
			if !strings.Contains(err.Error(), "measure") {
				t.Fatalf("err = %q, want it to name the measure field", err.Error())
			}
			if !strings.Contains(err.Error(), "openspec") {
				t.Fatalf("err = %q, want it to name the workflow 'openspec'", err.Error())
			}
		})
	}
	t.Run("absent measure is accepted", func(t *testing.T) {
		cfg := Config{Workflows: []WorkflowConfig{
			{Name: "openspec", Prompt: "Apply {change}", Condition: "echo x", Commit: "see: {change}"},
		}}
		if err := validateWorkflows(cfg); err != nil {
			t.Fatalf("err = %v, want nil for absent measure", err)
		}
	})
	t.Run("nonblank measure is accepted", func(t *testing.T) {
		cmd := "./bench.sh"
		cfg := Config{Workflows: []WorkflowConfig{
			{Name: "openspec", Prompt: "Apply {change}", Condition: "echo x", Commit: "see: {change}", Measure: &cmd},
		}}
		if err := validateWorkflows(cfg); err != nil {
			t.Fatalf("err = %v, want nil for nonblank measure", err)
		}
	})
}

// TestValidateWorkflowsChecksDisabledEntries proves the load-time filter
// runs AFTER validation: a disabled entry with a blank required field,
// and a disabled duplicate name, both still fail validateWorkflows on
// the full merged list before any entry is removed. This is the
// validate-then-filter ordering invariant.
func TestValidateWorkflowsChecksDisabledEntries(t *testing.T) {
	t.Run("blank field on disabled entry", func(t *testing.T) {
		cfg := Config{Workflows: []WorkflowConfig{
			{Name: "parked", Prompt: "Apply {change}", Condition: "", Commit: "see: {change}", Disable: true},
		}}
		err := validateWorkflows(cfg)
		if err == nil || !strings.Contains(err.Error(), "workflows[0]") || !strings.Contains(err.Error(), "condition") {
			t.Fatalf("err = %v, want workflows[0] condition required", err)
		}
	})
	t.Run("disabled duplicate name", func(t *testing.T) {
		cfg := Config{Workflows: []WorkflowConfig{
			{Name: "openspec", Prompt: "Apply {change}", Condition: "echo x", Commit: "see: {change}"},
			{Name: "openspec", Prompt: "Bump {change}", Condition: "echo y", Commit: "see: bump {change}", Disable: true},
		}}
		err := validateWorkflows(cfg)
		if err == nil || !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "openspec") {
			t.Fatalf("err = %v, want duplicate openspec", err)
		}
	})
}

// TestFilterDisabledWorkflowsKeepsEnabled proves the load-time filter drops
// only Disable==true entries and preserves the enabled entries in their
// original relative order.
func TestFilterDisabledWorkflowsKeepsEnabled(t *testing.T) {
	in := []WorkflowConfig{
		{Name: "live-1", Condition: "echo a"},
		{Name: "parked", Condition: "echo b", Disable: true},
		{Name: "live-2", Condition: "echo c"},
	}
	got := filterDisabledWorkflows(in)
	if len(got) != 2 || got[0].Name != "live-1" || got[1].Name != "live-2" {
		t.Fatalf("got = %+v, want [live-1, live-2]", got)
	}
}

// TestFilterDisabledWorkflowsAllDisabledEmpty proves disabling every
// workflow collapses the evaluated list to empty (the watcher then runs
// in OpenSpec compatibility mode), which is the documented tail of
// "disabled means not present".
func TestFilterDisabledWorkflowsAllDisabledEmpty(t *testing.T) {
	in := []WorkflowConfig{
		{Name: "a", Condition: "echo a", Disable: true},
		{Name: "b", Condition: "echo b", Disable: true},
	}
	got := filterDisabledWorkflows(in)
	if len(got) != 0 {
		t.Fatalf("got = %+v, want empty slice", got)
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
		{"valid", "include:\n  - playground-*\n"},
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

// TestConfigRejectsAutoMergeWithoutWorktree: a config that sets
// auto_merge: true without worktree: true is invalid in branch mode.
// The error names auto_merge so the operator can fix the field.
func TestConfigRejectsAutoMergeWithoutWorktree(t *testing.T) {
	cfg := Config{Worktree: false, AutoMerge: boolPtr(true)}
	err := validateWorktreeSettings(&cfg)
	if err == nil {
		t.Fatal("validateWorktreeSettings returned nil; want error")
	}
	if !strings.Contains(err.Error(), "auto_merge") {
		t.Fatalf("err = %v, want 'auto_merge' in message", err)
	}
	if !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("err = %v, want 'worktree' in message", err)
	}
}

// TestConfigRejectsWorktreeRootWithoutWorktree: a non-empty
// worktree_root without worktree: true is rejected; the error names
// worktree_root.
func TestConfigRejectsWorktreeRootWithoutWorktree(t *testing.T) {
	cfg := Config{Worktree: false, WorktreeRoot: "/somewhere"}
	err := validateWorktreeSettings(&cfg)
	if err == nil {
		t.Fatal("validateWorktreeSettings returned nil; want error")
	}
	if !strings.Contains(err.Error(), "worktree_root") {
		t.Fatalf("err = %v, want 'worktree_root' in message", err)
	}
}

// TestConfigAcceptsValidCombinations: the three valid shapes the
// contract must accept. (a) branch mode with auto_merge explicitly
// false is a harmless no-op, not an error. (b) worktree mode with
// auto_merge true and the default (empty) root. (c) worktree mode
// with manual-merge and a custom root.
func TestConfigAcceptsValidCombinations(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{
			name: "branch_mode_auto_merge_false",
			cfg:  Config{Worktree: false, AutoMerge: boolPtr(false)},
		},
		{
			name: "worktree_auto_merge_default_root",
			cfg:  Config{Worktree: true, AutoMerge: boolPtr(true), WorktreeRoot: ""},
		},
		{
			name: "worktree_manual_merge_custom_root",
			cfg:  Config{Worktree: true, AutoMerge: boolPtr(false), WorktreeRoot: "~/custom"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateWorktreeSettings(&tc.cfg); err != nil {
				t.Fatalf("validateWorktreeSettings returned %v; want nil", err)
			}
		})
	}
}

// TestConfigAcceptsAbsentAutoMerge: a fully default branch-mode config
// (no auto_merge pointer at all) is valid; the absent field must not
// trip the validator.
func TestConfigAcceptsAbsentAutoMerge(t *testing.T) {
	cfg := Config{Worktree: false, AutoMerge: nil}
	if err := validateWorktreeSettings(&cfg); err != nil {
		t.Fatalf("validateWorktreeSettings returned %v; want nil", err)
	}
}

// TestLoadConfigParsesWorktreeFields: the strict decoder accepts the
// three new top-level fields. auto_merge: false decodes into a non-nil
// *bool so the resolver can distinguish explicit false from unset.
func TestLoadConfigParsesWorktreeFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "worktree: true\nauto_merge: false\nworktree_root: ~/wt\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !cfg.Worktree {
		t.Fatalf("Worktree = false, want true")
	}
	if cfg.AutoMerge == nil {
		t.Fatal("AutoMerge = nil, want non-nil *bool")
	}
	if *cfg.AutoMerge {
		t.Fatalf("*AutoMerge = true, want false")
	}
	if cfg.WorktreeRoot != "~/wt" {
		t.Fatalf("WorktreeRoot = %q, want ~/wt", cfg.WorktreeRoot)
	}
}

func TestLoadStartupConfigMergesWorkflowFilesBeforeConfig(t *testing.T) {
	dir := t.TempDir()
	makeWorkflowFile(t, dir, "02-deps.md")
	makeWorkflowFile(t, dir, "01-openspec.md")
	config := filepath.Join(t.TempDir(), "config.yaml")
	body := "workflows_dir: \"" + dir + "\"\nworkflows:\n" +
		"  - name: release\n" +
		"    prompt: Release {change}\n" +
		"    condition: echo release\n" +
		"    commit: see-release-{change}\n"
	if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadStartupConfig(config)
	if err != nil {
		t.Fatalf("loadStartupConfig: %v", err)
	}
	if len(cfg.Workflows) != 3 {
		t.Fatalf("len(Workflows) = %d, want 3", len(cfg.Workflows))
	}
	if got, want := []string{cfg.Workflows[0].Name, cfg.Workflows[1].Name, cfg.Workflows[2].Name}, []string{"01-openspec", "02-deps", "release"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("workflow names = %v, want %v", got, want)
	}
	if err := validateWorkflows(cfg); err != nil {
		t.Fatalf("validateWorkflows: %v", err)
	}
}

func TestLoadStartupConfigRejectsWorkflowFileCollision(t *testing.T) {
	dir := t.TempDir()
	makeWorkflowFile(t, dir, "openspec.md")
	config := filepath.Join(t.TempDir(), "config.yaml")
	body := "workflows_dir: \"" + dir + "\"\nworkflows:\n" +
		"  - name: openspec\n" +
		"    prompt: Apply {change}\n" +
		"    condition: echo openspec\n" +
		"    commit: see-apply-{change}\n"
	if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadStartupConfig(config)
	if err == nil {
		t.Fatal("loadStartupConfig: want collision error")
	}
	if !strings.Contains(err.Error(), filepath.Join(dir, "openspec.md")) || !strings.Contains(err.Error(), "workflows[0]") {
		t.Fatalf("error = %q, want file path and workflows[0]", err)
	}
}

// TestLoadStartupConfigFiltersDisabledFileWorkflow proves a disabled .md
// workflow is absent from the evaluated list after the load-time filter,
// while an enabled config.yaml entry survives the merge. File workflows
// run before config entries, so the disabled file workflow is first and
// is removed, leaving only the enabled config entry in relative order.
func TestLoadStartupConfigFiltersDisabledFileWorkflow(t *testing.T) {
	dir := t.TempDir()
	fileBody := strings.Join([]string{
		"---",
		"condition: \"echo file-work\"",
		"commit: \"see-file\"",
		"disable: true",
		"---",
		"Apply the change.",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "openspec.md"), []byte(fileBody), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "config.yaml")
	cfgBody := "workflows_dir: \"" + dir + "\"\nworkflows:\n" +
		"  - name: release\n" +
		"    prompt: Release {change}\n" +
		"    condition: echo release\n" +
		"    commit: see-release-{change}\n"
	if err := os.WriteFile(config, []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadStartupConfig(config)
	if err != nil {
		t.Fatalf("loadStartupConfig: %v", err)
	}
	if err := validateWorkflows(cfg); err != nil {
		t.Fatalf("validateWorkflows: %v", err)
	}
	got := filterDisabledWorkflows(cfg.Workflows)
	if len(got) != 1 || got[0].Name != "release" {
		t.Fatalf("evaluated = %+v, want only [release]", got)
	}
}

// TestLoadStartupConfigRejectsDisabledFileWithBlankField proves a disabled
// .md workflow is still validated at parse time: a blank commit fails load,
// naming the file path and the missing field. Disabling does not bypass the
// per-file completeness contract.
func TestLoadStartupConfigRejectsDisabledFileWithBlankField(t *testing.T) {
	dir := t.TempDir()
	fileBody := strings.Join([]string{
		"---",
		"condition: \"echo file-work\"",
		"commit: \"   \"",
		"disable: true",
		"---",
		"Apply the change.",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "openspec.md"), []byte(fileBody), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(config, []byte("workflows_dir: \""+dir+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadStartupConfig(config)
	if err == nil || !strings.Contains(err.Error(), filepath.Join(dir, "openspec.md")) || !strings.Contains(err.Error(), "commit") {
		t.Fatalf("err = %v, want file path and commit", err)
	}
}

func TestLoadStartupConfigKeepsConfigWorkflowsWithoutFiles(t *testing.T) {
	for _, tc := range []struct {
		name         string
		workflowsDir string
		includeDir   bool
		createDir    bool
	}{
		{name: "empty", workflowsDir: filepath.Join(t.TempDir(), "workflows"), includeDir: true, createDir: true},
		{name: "missing", workflowsDir: filepath.Join(t.TempDir(), "workflows"), includeDir: true},
		{name: "absent", includeDir: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.createDir {
				if err := os.Mkdir(tc.workflowsDir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			config := filepath.Join(t.TempDir(), "config.yaml")
			body := ""
			if tc.includeDir {
				body += "workflows_dir: \"" + tc.workflowsDir + "\"\n"
			}
			body += "workflows:\n" +
				"  - name: release\n" +
				"    prompt: Release {change}\n" +
				"    condition: echo release\n" +
				"    commit: see-release-{change}\n"
			if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if !tc.includeDir {
				t.Setenv("HOME", t.TempDir())
			}
			cfg, err := loadStartupConfig(config)
			if err != nil {
				t.Fatalf("loadStartupConfig: %v", err)
			}
			if len(cfg.Workflows) != 1 || cfg.Workflows[0].Name != "release" {
				t.Fatalf("Workflows = %+v, want config workflow only", cfg.Workflows)
			}
		})
	}
}

func TestLoadConfigParsesWorkflowsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("workflows_dir: ~/workflows\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	want := filepath.Join(home, "workflows")
	if cfg.WorkflowsDir != want {
		t.Fatalf("WorkflowsDir = %q, want %q", cfg.WorkflowsDir, want)
	}
}

func TestLoadConfigRejectsWorkflowsDirDoubleStar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("workflows_dir: /tmp/**\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadConfig(path)
	if err == nil {
		t.Fatalf("loadConfig: want '**' rejection, got nil")
	}
	if !strings.Contains(err.Error(), "workflows_dir") || !strings.Contains(err.Error(), "**") {
		t.Fatalf("loadConfig error = %q, want it to name workflows_dir and **", err)
	}
}

func TestValidateConfigExpandsWorkflowsDirTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := Config{WorkflowsDir: "~/wf"}
	if err := validateConfig(&cfg); err != nil {
		t.Fatalf("validateConfig: %v", err)
	}
	if cfg.WorkflowsDir != filepath.Join(home, "wf") {
		t.Fatalf("WorkflowsDir = %q, want %q", cfg.WorkflowsDir, filepath.Join(home, "wf"))
	}
}

func TestValidateConfigLogDirBlankIsUntreated(t *testing.T) {
	cfg := Config{LogDir: ""}
	if err := validateConfig(&cfg); err != nil {
		t.Fatalf("validateConfig: %v", err)
	}
	if cfg.LogDir != "" {
		t.Fatalf("LogDir = %q, want blank left untouched", cfg.LogDir)
	}
}

func TestValidateConfigLogDirWhitespaceOnlyIsBlank(t *testing.T) {
	cfg := Config{LogDir: "   "}
	if err := validateConfig(&cfg); err != nil {
		t.Fatalf("validateConfig: %v", err)
	}
	if cfg.LogDir != "" {
		t.Fatalf("LogDir = %q, want whitespace treated as blank", cfg.LogDir)
	}
}

func TestValidateConfigExpandsLogDirTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := Config{LogDir: "~/logs"}
	if err := validateConfig(&cfg); err != nil {
		t.Fatalf("validateConfig: %v", err)
	}
	if cfg.LogDir != filepath.Join(home, "logs") {
		t.Fatalf("LogDir = %q, want %q", cfg.LogDir, filepath.Join(home, "logs"))
	}
}

func TestValidateConfigRejectsDoubleStarLogDir(t *testing.T) {
	cfg := Config{LogDir: "/var/**"}
	err := validateConfig(&cfg)
	if err == nil || !strings.Contains(err.Error(), "log_dir: '**' is not supported") {
		t.Fatalf("err = %v, want log_dir double-star error", err)
	}
}

func TestResolveWorkflowsDirFallsBackToDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err := resolveWorkflowsDir(Config{})
	if err != nil {
		t.Fatalf("resolveWorkflowsDir: %v", err)
	}
	want := filepath.Join(home, ".config", "see", "workflows")
	if dir != want {
		t.Fatalf("resolveWorkflowsDir = %q, want %q", dir, want)
	}
}

func TestResolveWorkflowsDirReturnsConfiguredWhenSet(t *testing.T) {
	cfg := Config{WorkflowsDir: "/etc/see/workflows"}
	dir, err := resolveWorkflowsDir(cfg)
	if err != nil {
		t.Fatalf("resolveWorkflowsDir: %v", err)
	}
	if dir != "/etc/see/workflows" {
		t.Fatalf("resolveWorkflowsDir = %q, want /etc/see/workflows", dir)
	}
}

func TestResolveWorkflowsDirMissingIsNoOp(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	dir, err := resolveWorkflowsDir(Config{WorkflowsDir: missing})
	if err != nil {
		t.Fatalf("resolveWorkflowsDir: %v", err)
	}
	if dir != missing {
		t.Fatalf("resolveWorkflowsDir = %q, want %q", dir, missing)
	}
}

func TestResolveWorkflowsDirErrorsWhenPathIsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := resolveWorkflowsDir(Config{WorkflowsDir: file})
	if err == nil {
		t.Fatalf("resolveWorkflowsDir: want not-a-directory error, got nil")
	}
	if !strings.Contains(err.Error(), file) || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("resolveWorkflowsDir error = %q, want it to name %q and 'not a directory'", err, file)
	}
}

func TestResolveWorkflowsDirHappyPathDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveWorkflowsDir(Config{WorkflowsDir: dir})
	if err != nil {
		t.Fatalf("resolveWorkflowsDir: %v", err)
	}
	if got != dir {
		t.Fatalf("resolveWorkflowsDir = %q, want %q", got, dir)
	}
}

// --- startup-level workflow_files tests: tasks 5.1, 5.2, 5.3 -------

// TestLoadStartupConfigMergesSingleWorkflowFile asserts the full
// loader path: a t.TempDir containing one .md workflow with
// frontmatter and a body is picked up by loadStartupConfig and the
// merged slice carries the body's Prompt and the frontmatter's
// Model through. The config.yaml has no workflows: of its own, so
// the file workflow is the only entry.
func TestLoadStartupConfigMergesSingleWorkflowFile(t *testing.T) {
	dir := t.TempDir()
	body := strings.Join([]string{
		"---",
		"condition: \"echo dark-mode\"",
		"commit: \"see: apply {change}\"",
		"model: \"openai/gpt-5-mini\"",
		"---",
		"Apply the dark-mode change.",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "dark-mode.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(config, []byte("workflows_dir: \""+dir+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadStartupConfig(config)
	if err != nil {
		t.Fatalf("loadStartupConfig: %v", err)
	}
	if len(cfg.Workflows) != 1 {
		t.Fatalf("len(Workflows) = %d, want 1", len(cfg.Workflows))
	}
	wf := cfg.Workflows[0]
	if wf.Name != "dark-mode" {
		t.Fatalf("Name = %q, want dark-mode", wf.Name)
	}
	if wf.Prompt != "Apply the dark-mode change." {
		t.Fatalf("Prompt = %q, want %q", wf.Prompt, "Apply the dark-mode change.")
	}
	if wf.Model != "openai/gpt-5-mini" {
		t.Fatalf("Model = %q, want openai/gpt-5-mini", wf.Model)
	}
	if wf.Condition != "echo dark-mode" {
		t.Fatalf("Condition = %q, want %q", wf.Condition, "echo dark-mode")
	}
	if wf.Commit != "see: apply {change}" {
		t.Fatalf("Commit = %q, want %q", wf.Commit, "see: apply {change}")
	}
	if err := validateWorkflows(cfg); err != nil {
		t.Fatalf("validateWorkflows: %v", err)
	}
}

// TestLoadStartupConfigMissingWorkflowsDirIsNoOpWhenConfigHasWorkflows
// asserts that a missing workflows_dir does not affect config.yaml
// workflows: the configured slice is preserved as-is when the
// directory does not exist.
func TestLoadStartupConfigMissingWorkflowsDirIsNoOpWhenConfigHasWorkflows(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.yaml")
	body := "workflows_dir: \"" + filepath.Join(t.TempDir(), "does-not-exist") + "\"\n" +
		"workflows:\n" +
		"  - name: release\n" +
		"    prompt: Release {change}\n" +
		"    condition: echo release\n" +
		"    commit: see-release-{change}\n"
	if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadStartupConfig(config)
	if err != nil {
		t.Fatalf("loadStartupConfig: %v", err)
	}
	if len(cfg.Workflows) != 1 {
		t.Fatalf("len(Workflows) = %d, want 1", len(cfg.Workflows))
	}
	if cfg.Workflows[0].Name != "release" {
		t.Fatalf("Workflows[0].Name = %q, want release", cfg.Workflows[0].Name)
	}
}

// TestLoadStartupConfigRejectsNonDirectoryWorkflowsDir asserts that a
// configured workflows_dir pointing at a regular file produces an
// actionable error naming the path; the loader must surface the
// failure rather than fall through silently.
func TestLoadStartupConfigRejectsNonDirectoryWorkflowsDir(t *testing.T) {
	dir := t.TempDir()
	notADir := filepath.Join(dir, "regular-file")
	if err := os.WriteFile(notADir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "config.yaml")
	body := "workflows_dir: \"" + notADir + "\"\nworkflows:\n" +
		"  - name: release\n" +
		"    prompt: Release {change}\n" +
		"    condition: echo release\n" +
		"    commit: see-release-{change}\n"
	if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadStartupConfig(config)
	if err == nil {
		t.Fatal("loadStartupConfig: want non-directory error, got nil")
	}
	if !strings.Contains(err.Error(), notADir) {
		t.Fatalf("error = %q, want it to name %s", err, notADir)
	}
}
