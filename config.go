package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// userConfigDir is the lookup used by watchConfigPath. Indirected
// through a var so tests can pin the config dir without juggling
// environment variables cross-platform (os.UserConfigDir ignores
// XDG_CONFIG_HOME on macOS, so setting XDG_CONFIG_HOME alone is
// not enough to redirect the config file under test).
var userConfigDir = os.UserConfigDir

// watchConfigPath returns the absolute path to the watches config file.
// os.UserConfigDir() honours $XDG_CONFIG_HOME on Linux/macOS and falls
// back to ~/.config; on Windows it honours %AppData% — sufficient for
// the v1 surface (no per-OS overrides needed).
func watchConfigPath() (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("see: resolve config dir: %w", err)
	}
	return filepath.Join(base, "see", "watches"), nil
}

// expandTilde replaces a leading "~" or "~/" with the user's home
// directory. "~foo" (without slash) is treated as a literal path; only
// the bare "~" and "~/" forms expand. Env-var expansion is deliberately
// not performed — the config holds paths, not shell scripts.
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

// loadWatchConfig reads the watches file at path and returns the
// non-blank, non-comment lines with tilde expansion applied.
// Missing file → (nil, nil) — the caller falls back to cwd.
// Malformed line → (nil, error) carrying the line number.
func loadWatchConfig(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("see: read watches config %s: %w", path, err)
	}
	defer f.Close()

	var out []string
	scanner := bufio.NewScanner(f)
	for lineNum := 1; scanner.Scan(); lineNum++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || raw[0] == '#' {
			continue
		}
		expanded, err := expandTilde(raw)
		if err != nil {
			return nil, fmt.Errorf("see: watches config line %d: %w", lineNum, err)
		}
		out = append(out, expanded)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("see: watches config %s: %w", path, err)
	}
	return out, nil
}