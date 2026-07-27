package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// defaultConfigTemplate is the commented YAML file `see` writes to
// the default configuration path on first run when the path is
// absent and `--config` is unset. It is kept as a checked-in file
// at the repository root next to `prompt.md` and `main.go` so
// editors reviewing the file see prose in pull requests and the
// template is editable without touching Go. `//go:embed` fails the
// build if `config.example.yaml` is missing at the repository root
// alongside `config.go`, so a deleted template becomes a compile
// error rather than a silent regression. The body is pure comments
// so the strict loader decodes it to a zero-value Config — bootstrap
// has zero behavioral effect, only a discoverable file on disk.
//
//go:embed config.example.yaml
var defaultConfigTemplate string

// stderrWriter is the destination for one-line startup notices
// emitted when the bootstrap write fails. Indirected through a var
// so tests can capture the output without touching the real
// os.Stderr (which is shared with other goroutines and processes).
var stderrWriter io.Writer = os.Stderr

// userHomeDir is replaceable in tests so the otherwise platform-dependent
// failure path remains covered.
var userHomeDir = os.UserHomeDir

// configPath returns the absolute path to the user's config.yaml.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("see: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "see", "config.yaml"), nil
}

// expandTilde replaces a leading "~" or "~/" with the user's home
// directory. "~foo" (without slash) is treated as a literal path;
// only the bare "~" and "~/" forms expand. Env-var expansion is
// deliberately not performed — the config holds paths, not shell
// scripts.
func expandTilde(p string) (string, error) {
	if p == "" || p[0] != '~' {
		return p, nil
	}
	if p != "~" && p[1] != '/' && p[1] != filepath.Separator {
		return p, nil
	}
	home := os.Getenv("HOME")
	if home == "" {
		h, err := userHomeDir()
		if err != nil {
			return "", fmt.Errorf("see: expand ~: %w", err)
		}
		home = h
	}
	if p == "~" {
		return home, nil
	}
	return filepath.Join(home, p[2:]), nil
}

// validateTildePath normalizes one tilde-expandable path field shared
// by workflows_dir and log_dir: whitespace-only is cleared to "unset",
// '**' is rejected, and a leading ~ is expanded in place. name is the
// field path used in errors (e.g. "log_dir"), matching validateConfig's
// other branches. The disk-existence verdict is left to callers (the
// field may name a directory that is created later).
func validateTildePath(name string, p *string) error {
	if strings.TrimSpace(*p) == "" {
		*p = ""
		return nil
	}
	if strings.Contains(*p, "**") {
		return fmt.Errorf("%s: '**' is not supported", name)
	}
	expanded, err := expandTilde(*p)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*p = expanded
	return nil
}

// Config is the decoded contents of config.yaml. All fields are optional:
// a blank RootDir preserves the current-working-directory fallback, empty
// Include / Exclude slices include every immediate child and exclude none,
// and omitting Workflows keeps the OpenSpec compatibility path active.
//
// Workflows is the multi-workflow schema introduced by the
// `support-multiple-workflows` change. When non-empty, every entry
// must carry nonblank Name, Prompt, Condition, and Commit; names must
// be unique. The previous top-level `prompt`, `condition`, and
// `commit` fields are rejected by the strict decoder: migrate each
// old custom configuration into one named workflow under
// `workflows` (see AGENTS.md for the schema and the migration path).
type Config struct {
	RootDir      string           `yaml:"root_dir"`
	Include      []string         `yaml:"include"`
	Exclude      []string         `yaml:"exclude"`
	Workflows    []WorkflowConfig `yaml:"workflows"`
	WorkflowsDir string           `yaml:"workflows_dir"`
	// Worktree selects worktree lane isolation (the agent runs in a
	// git worktree linked to the operator's checkout) instead of the
	// default branch mode. Default false (zero value) selects branch
	// mode, matching the historical contract.
	Worktree bool `yaml:"worktree"`
	// AutoMerge is a pointer so the loader can distinguish "unset,
	// use the runtime default true" from "explicitly false". The
	// runtime default is true (the lane is rebased and fast-forward
	// merged); an explicit false leaves the rebased lane for manual
	// review. It is only consulted in worktree mode.
	AutoMerge *bool `yaml:"auto_merge"`
	// WorktreeRoot is the parent directory for new worktrees. Empty
	// means "use the default"; the default is resolved in main() to
	// ~/.cache/see/worktrees only when worktree mode is active, so an
	// empty value stays empty (and harmless) in branch mode.
	WorktreeRoot string `yaml:"worktree_root"`
	// LogDir is the directory `see` writes its batch-level event log
	// and per-invocation agent logs to. Empty means "use the default";
	// the default is ~/.cache/see/logs (see defaultLogDir). Resolution
	// precedence is SEE_LOG_DIR > LogDir > defaultLogDir (see
	// resolveLogDir). A whitespace-only value is treated as unset.
	LogDir string `yaml:"log_dir"`
}

// boolPtr returns a pointer to b; a small helper for tests and for
// encoding an explicitly-set bool into a Config.
func boolPtr(b bool) *bool { return &b }

// WorkflowConfig is one named entry in the ordered workflows slice.
// Every field except Model is required when the workflows block is
// configured; validateWorkflows enforces the nonblank, unique-name
// contract before the watcher starts. Model is optional: a blank or
// absent value is treated as "unset" at the call site (the agent's
// default model is used), and the strict decoder accepts the string
// silently so existing configurations without the field decode to
// the zero value.
type WorkflowConfig struct {
	Name      string `yaml:"name"`
	Prompt    string `yaml:"prompt"`
	Condition string `yaml:"condition"`
	Commit    string `yaml:"commit"`
	Model     string `yaml:"model"`
}

func validateConfig(cfg *Config) error {
	if strings.TrimSpace(cfg.RootDir) == "" {
		cfg.RootDir = ""
	} else {
		if strings.Contains(cfg.RootDir, "**") {
			return fmt.Errorf("root_dir: '**' is not supported")
		}
		expanded, err := expandTilde(cfg.RootDir)
		if err != nil {
			return fmt.Errorf("root_dir: %w", err)
		}
		info, err := os.Stat(expanded)
		if err != nil {
			return fmt.Errorf("root_dir %q: %w", expanded, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("root_dir %q: not a directory", expanded)
		}
		cfg.RootDir = expanded
	}
	for _, group := range []struct {
		field    string
		patterns *[]string
	}{
		{"include", &cfg.Include},
		{"exclude", &cfg.Exclude},
	} {
		for i, pattern := range *group.patterns {
			path := fmt.Sprintf("%s[%d]", group.field, i)
			if strings.Contains(pattern, "**") {
				return fmt.Errorf("%s: '**' is not supported", path)
			}
			if _, err := filepath.Match(pattern, "test"); err != nil {
				return fmt.Errorf("%s: invalid glob pattern: %w", path, err)
			}
			expanded, err := expandTilde(pattern)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			(*group.patterns)[i] = expanded
		}
	}
	if err := validateTildePath("workflows_dir", &cfg.WorkflowsDir); err != nil {
		return err
	}
	if err := validateTildePath("log_dir", &cfg.LogDir); err != nil {
		return err
	}
	return nil
}

// defaultWorkflowsDir is the directory `see` reads workflow `.md`
// files from when no explicit `workflows_dir` is configured. Tilde
// is literal here; callers go through expandTilde when they resolve
// the default.
const defaultWorkflowsDir = "~/.config/see/workflows/"

// defaultLogDir is the directory `see` writes its batch-level event
// log and per-invocation agent logs to when neither SEE_LOG_DIR nor a
// configured log_dir supplies a value. It mirrors defaultWorktreeRoot
// so ~/.cache/see/ is the single root for `see`'s ephemeral artifacts.
// Tilde is literal here; resolveLogDir expands it at resolution time.
const defaultLogDir = "~/.cache/see/logs"

// resolveWorkflowsDir returns the effective directory `see` reads
// workflow `.md` files from, plus a stat-based verdict on whether
// the directory is usable. cfg.WorkflowsDir wins when nonblank;
// otherwise the default ~/.config/see/workflows/ is tilde-expanded
// and returned. The path is always returned; the error is the
// verdict:
//
//	nil         → path does not exist (silent no-op) or is a
//	              usable directory
//	non-nil     → path exists but is not a directory, or stat
//	              failed for some other reason; the error names
//	              the path so the operator can fix it
//
// `**` rejection and the initial tilde-expansion of a configured
// value happen earlier in validateConfig; resolveWorkflowsDir only
// handles the fallback and the disk-side checks described in the
// spec scenarios.
func resolveWorkflowsDir(cfg Config) (string, error) {
	var dir string
	if strings.TrimSpace(cfg.WorkflowsDir) == "" {
		expanded, err := expandTilde(defaultWorkflowsDir)
		if err != nil {
			return "", fmt.Errorf("workflows_dir default %q: %w", defaultWorkflowsDir, err)
		}
		dir = expanded
	} else {
		dir = cfg.WorkflowsDir
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return dir, nil
		}
		return dir, fmt.Errorf("workflows_dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return dir, fmt.Errorf("workflows_dir %q: not a directory", dir)
	}
	return dir, nil
}

// validateWorkflows enforces the multi-workflow contract for the
// Workflows slice: an empty slice is the OpenSpec compatibility
// path and returns nil; a non-empty slice requires every entry to
// have nonblank Name / Prompt / Condition / Commit, and every Name
// to be unique within the slice. The first failure wins; the error
// names the offending workflow index and field so the operator can
// fix the configuration without reading the source.
func validateWorkflows(cfg Config) error {
	if len(cfg.Workflows) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(cfg.Workflows))
	for i, wf := range cfg.Workflows {
		path := fmt.Sprintf("workflows[%d]", i)
		for _, f := range []struct {
			field, value string
		}{
			{"name", wf.Name},
			{"prompt", wf.Prompt},
			{"condition", wf.Condition},
			{"commit", wf.Commit},
		} {
			if strings.TrimSpace(f.value) == "" {
				return fmt.Errorf("%s: %s is required", path, f.field)
			}
		}
		name := strings.TrimSpace(wf.Name)
		if _, dup := seen[name]; dup {
			return fmt.Errorf("%s: duplicate workflow name %q", path, wf.Name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// validateWorktreeSettings enforces the lane-isolation config
// contract: auto_merge and worktree_root are meaningful only in
// worktree mode. An explicitly-true auto_merge without worktree, and
// any non-empty worktree_root without worktree, are rejected with an
// error naming the offending field so the operator can fix the
// configuration. auto_merge: false in branch mode is accepted as a
// harmless explicit no-op (the field is simply not consulted). The
// caller surfaces the error via os.Stderr and exits with status 2,
// consistent with validateWorkflows.
func validateWorktreeSettings(cfg *Config) error {
	if cfg.Worktree {
		return nil
	}
	if cfg.AutoMerge != nil && *cfg.AutoMerge {
		return errors.New("auto_merge requires worktree: true")
	}
	if strings.TrimSpace(cfg.WorktreeRoot) != "" {
		return errors.New("worktree_root requires worktree: true")
	}
	return nil
}

// resolveLaneIsolation combines the loaded config with the parsed CLI
// flags into the effective lane-isolation triple (worktree, auto_merge,
// worktree_root) and validates it. Precedence is CLI flag > config field
// > default. explicitFlags records the flags that were passed on the
// command line (built via flag.Visit) for two reasons: (1) the
// default-true --auto-merge must not reject a default branch-mode run,
// and (2) an explicitly passed --auto-merge (or --auto-merge=false)
// without --worktree is rejected per the lane-isolation contract.
// worktree_root is tilde-expanded; the default ~/.cache/see/worktrees
// applies only when worktree mode is active and no root was set, so an
// empty root stays empty (and harmless) in branch mode. An invalid
// combination returns an error suitable for an exit-status-2 startup
// failure.
func resolveLaneIsolation(cfg Config, explicitFlags map[string]bool, worktreeFlag, autoMergeFlag bool, worktreeRootFlag string) (worktree, autoMerge bool, worktreeRoot string, err error) {
	worktree = cfg.Worktree
	if explicitFlags["worktree"] {
		worktree = worktreeFlag
	}
	switch {
	case explicitFlags["auto-merge"]:
		autoMerge = autoMergeFlag
	case cfg.AutoMerge != nil:
		autoMerge = *cfg.AutoMerge
	default:
		autoMerge = true
	}
	if explicitFlags["worktree-root"] {
		worktreeRoot = worktreeRootFlag
	} else if cfg.WorktreeRoot != "" {
		worktreeRoot = cfg.WorktreeRoot
	}
	if worktreeRoot != "" {
		if worktreeRoot, err = expandTilde(worktreeRoot); err != nil {
			return false, false, "", err
		}
	}
	if worktree && worktreeRoot == "" {
		if worktreeRoot, err = expandTilde(defaultWorktreeRoot); err != nil {
			return false, false, "", err
		}
	}
	valCfg := Config{Worktree: worktree, WorktreeRoot: worktreeRoot}
	if !worktree && ((cfg.AutoMerge != nil && *cfg.AutoMerge) || explicitFlags["auto-merge"]) {
		valCfg.AutoMerge = boolPtr(true)
	}
	if verr := validateWorktreeSettings(&valCfg); verr != nil {
		return false, false, "", verr
	}
	return worktree, autoMerge, worktreeRoot, nil
}

// loadConfig reads path and returns a strict-decoded Config. Missing
// or empty files return a zero-value Config without error; malformed
// YAML, unknown fields, wrong field types, and additional documents
// all return errors that name the offending file so startup can
// surface an actionable message.
func loadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("see: read config %s: %w", path, err)
	}
	// Treat empty content as "no configuration" rather than an error:
	// a freshly-created empty config file is a valid way to override
	// the cwd fallback explicitly.
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if err == io.EOF {
			return cfg, nil
		}
		return cfg, fmt.Errorf("see: parse config %s: %w", path, err)
	}
	// Reject a second document so accidental duplication or a stray
	// appended config does not silently hide configuration.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return cfg, fmt.Errorf("see: parse config %s: only one YAML document is allowed", path)
	}
	if err := validateConfig(&cfg); err != nil {
		return Config{}, fmt.Errorf("see: validate config %s: %w", path, err)
	}
	return cfg, nil
}

// configPathNone is the sentinel value for --config that means "do
// not load any configuration file". The POSIX "-" convention (tar x
// -, git log -) keeps it familiar. ponytail: this collides with a
// literal file named "-"; upgrade path is a separate boolean or an
// env var if it ever bites.
const configPathNone = "-"

// ensureDefaultConfig materialises a default configuration file at
// path on first run. The parent directory is created with mode
// 0o755 if absent, and the file is written with mode 0o644 using
// the embedded defaultConfigTemplate body. Callers gate on the
// absent-file case before invoking this function so a no-op write
// never fires against an existing configuration. A write failure
// (permission denied, read-only filesystem, parent unwritable)
// returns the underlying error so the caller can decide whether
// startup should continue with a zero-value Config.
func ensureDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfigTemplate), 0o644)
}

// loadStartupConfig applies --config at the loader boundary. Three
// modes:
//
//	"-"              → zero-value Config; the file is not resolved or read.
//	"" (no flag)     → load the default configPath(); if the file is
//	                   absent, bootstrap it with the embedded template
//	                   (best-effort; bootstrap failure is non-fatal).
//	"/path/to/foo"   → tilde-expand and load that file; the default
//	                   path is never written.
//
// A malformed default returns a zero-value Config (so startup
// proceeds with command-line inputs); a malformed explicit path
// returns the loadConfig error so the operator can see what is wrong.
// For every loaded configuration, workflow Markdown files are merged
// ahead of configured workflows before the caller validates the
// resulting ordered slice.
func loadStartupConfig(configFlag string) (Config, error) {
	if configFlag == configPathNone {
		return Config{}, nil
	}
	if configFlag == "" {
		path, err := configPath()
		if err != nil {
			return Config{}, err
		}
		// Bootstrap is best-effort: an unwritable home directory
		// must not block startup. Fall through to loadConfig with
		// a zero-value Config when the write fails so the
		// command-line entries and the cwd fallback still produce
		// a working watch list. The notice to stderr names the
		// target path and the underlying error.
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			if err := ensureDefaultConfig(path); err != nil {
				fmt.Fprintf(stderrWriter,
					"see: bootstrap default config at %s: %v\n",
					path, err)
			}
		}
		cfg, err := loadConfig(path)
		if err != nil {
			return Config{}, err
		}
		return mergeWorkflowFiles(cfg)
	}
	expanded, err := expandTilde(configFlag)
	if err != nil {
		return Config{}, err
	}
	cfg, err := loadConfig(expanded)
	if err != nil {
		return Config{}, err
	}
	return mergeWorkflowFiles(cfg)
}

// selectPromptTemplate picks the effective prompt: a nonblank
// command-line value wins, then a nonblank configured value, then
// "". The embedded default is left to Watcher.SetPromptTemplate, so
// trimming semantics stay in one place. "Blank" means
// whitespace-only; there is no separate sentinel for an
// intentionally empty prompt.
func selectPromptTemplate(cliPrompt, configPrompt string) string {
	if strings.TrimSpace(cliPrompt) != "" {
		return cliPrompt
	}
	if strings.TrimSpace(configPrompt) != "" {
		return configPrompt
	}
	return ""
}
