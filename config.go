package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// userConfigDir is the lookup used by configPath. Indirected through
// a var so tests can pin the config dir without juggling environment
// variables cross-platform (os.UserConfigDir ignores XDG_CONFIG_HOME
// on macOS, so setting XDG_CONFIG_HOME alone is not enough to
// redirect the config file under test).
var userConfigDir = os.UserConfigDir

// configPath returns the absolute path to the user's config.yaml.
// os.UserConfigDir() honours $XDG_CONFIG_HOME on Linux/macOS and
// falls back to ~/.config; on Windows it honours %AppData% —
// sufficient for the v1 surface (no per-OS overrides needed).
func configPath() (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("see: resolve config dir: %w", err)
	}
	return filepath.Join(base, "see", "config.yaml"), nil
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
		h, err := os.UserHomeDir()
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

// Config is the decoded contents of config.yaml. Both fields are
// optional: an empty Watches preserves the current-working-directory
// fallback and an empty Prompt falls through to the embedded default.
type Config struct {
	Watches []string `yaml:"watches"`
	Prompt  string   `yaml:"prompt"`
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
	return cfg, nil
}

// configPathNone is the sentinel value for --config that means "do
// not load any configuration file". The POSIX "-" convention (tar x
// -, git log -) keeps it familiar. ponytail: this collides with a
// literal file named "-"; upgrade path is a separate boolean or an
// env var if it ever bites.
const configPathNone = "-"

// loadStartupConfig applies --config at the loader boundary. Three
// modes:
//
//   "-"              → zero-value Config; the file is not resolved or read.
//   "" (no flag)     → load the default configPath().
//   "/path/to/foo"   → tilde-expand and load that file.
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
