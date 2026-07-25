package main

import (
	"bytes"
	_ "embed"
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

// Config is the decoded contents of config.yaml. All fields are optional:
// a blank RootDir preserves the current-working-directory fallback, empty
// Include / Exclude slices include every immediate child and exclude none,
// an empty Prompt falls through to the embedded default, and Condition /
// Commit belong to the custom workflow.
type Config struct {
	RootDir   string   `yaml:"root_dir"`
	Include   []string `yaml:"include"`
	Exclude   []string `yaml:"exclude"`
	Prompt    string   `yaml:"prompt"`
	Condition string   `yaml:"condition"`
	Commit    string   `yaml:"commit"`
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
	return nil
}

// validateCustomConfig checks that custom workflow mode (triggered
// by a nonblank Condition) has both an effective prompt template
// and a nonblank Commit template before the watcher starts. The
// effective prompt is the post-precedence value computed from
// cliPrompt (the --prompt flag) and cfg.Prompt using the same
// rule as selectPromptTemplate. Returns nil for compatibility mode
// (blank Condition) regardless of the other fields — the OpenSpec
// resolver does not need a commit template. Errors name the missing
// field so the operator can fix the configuration without reading
// the source.
func validateCustomConfig(cfg Config, cliPrompt string) error {
	if strings.TrimSpace(cfg.Condition) == "" {
		return nil
	}
	if strings.TrimSpace(selectPromptTemplate(cliPrompt, cfg.Prompt)) == "" {
		return fmt.Errorf("see: custom condition requires a prompt (configure `prompt` or pass --prompt)")
	}
	if strings.TrimSpace(cfg.Commit) == "" {
		return fmt.Errorf("see: custom condition requires a `commit` template in config.yaml")
	}
	return nil
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
		return loadConfig(path)
	}
	expanded, err := expandTilde(configFlag)
	if err != nil {
		return Config{}, err
	}
	return loadConfig(expanded)
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
