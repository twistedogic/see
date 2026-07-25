package main

import (
	"os"
	"path/filepath"
	"sort"
)

// resolveConfiguredTargets builds the repository list from the configured root
// and its optional basename filters. Config validation has already checked the
// root and pattern syntax; errors are still returned for direct callers.
func resolveConfiguredTargets(cfg Config) (repos []string, warnings []Warning, err error) {
	if cfg.RootDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, nil, err
		}
		cfg.RootDir = cwd
		cfg.Include = []string{"."}
	}

	var candidates []string
	if len(cfg.Include) == 0 {
		entries, err := os.ReadDir(cfg.RootDir)
		if err != nil {
			return nil, nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				candidates = append(candidates, filepath.Join(cfg.RootDir, entry.Name()))
			}
		}
	} else {
		for _, pattern := range cfg.Include {
			matches, err := filepath.Glob(filepath.Join(cfg.RootDir, pattern))
			if err != nil {
				return nil, nil, err
			}
			candidates = append(candidates, matches...)
		}
	}

	for _, candidate := range candidates {
		excluded := false
		for _, pattern := range cfg.Exclude {
			match, err := filepath.Match(pattern, filepath.Base(candidate))
			if err != nil {
				return nil, nil, err
			}
			if match {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			warnings = append(warnings, Warning{Path: candidate, Msg: err.Error()})
			continue
		}
		classified, targetWarnings, err := classifyTarget(abs)
		if err != nil {
			return nil, nil, err
		}
		repos = append(repos, classified...)
		warnings = append(warnings, targetWarnings...)
	}
	return dedupeAndSort(repos), warnings, nil
}

// classifyTarget returns the repo paths that p resolves to, plus any
// warnings. p is expected to be absolute; the caller has done abs
// conversion already.
func classifyTarget(p string) ([]string, []Warning, error) {
	gitPath := filepath.Join(p, ".git")
	if _, err := os.Stat(gitPath); err == nil {
		return []string{p}, nil, nil
	}
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, []Warning{{Path: p, Msg: "path does not exist"}}, nil
		}
		return nil, []Warning{{Path: p, Msg: err.Error()}}, nil
	}
	if !info.IsDir() {
		return nil, []Warning{{Path: p, Msg: "not a directory"}}, nil
	}
	// Directory without .git: parent-of-repos.
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, []Warning{{Path: p, Msg: err.Error()}}, nil
	}
	var repos []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(p, e.Name())
		if _, err := os.Stat(filepath.Join(child, ".git")); err == nil {
			repos = append(repos, child)
		}
	}
	if len(repos) == 0 {
		return nil, []Warning{{Path: p, Msg: "no .git children"}}, nil
	}
	return repos, nil, nil
}

// dedupeAndSort folds p into a sorted, deduplicated slice of
// absolute paths. The watcher relies on the order being stable so
// the JSONL and TUI render the same scan each pass.
func dedupeAndSort(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		cleaned := filepath.Clean(p)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		out = append(out, cleaned)
	}
	sort.Strings(out)
	return out
}
