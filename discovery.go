package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// hasGlob reports whether p contains a shell-glob metacharacter.
// Patterns without metacharacters are treated as literal paths by
// resolveTargets; the matching is then a single Stat check.
func hasGlob(p string) bool {
	return strings.ContainsAny(p, "*?[")
}

// resolveTargets turns a list of patterns (already tilde-expanded)
// into a deduplicated, sorted, absolute list of repo paths.
// Classification:
//
//   - stat(p/.git) succeeds → repo, single entry
//   - p is a directory without .git → parent-of-repos; iterate its
//     immediate children for entries with .git
//   - anything else (file, missing, parent with no .git children) →
//     a Warning event and skip
//
// Patterns containing "**" are rejected at startup with a clear error
// because filepath.Match does not support recursive globs and the
// project chooses not to add a dependency for the feature.
func resolveTargets(patterns []string) (repos []string, warnings []Warning, err error) {
	for _, p := range patterns {
		if strings.Contains(p, "**") {
			return nil, nil, fmt.Errorf(
				"'**' is not supported in watch patterns (no recursive glob); list the paths you care about explicitly: %s",
				p,
			)
		}
		expanded, err := expandTilde(p)
		if err != nil {
			return nil, nil, err
		}
		matches, err := globMatches(expanded)
		if err != nil {
			return nil, nil, err
		}
		if len(matches) == 0 {
			warnings = append(warnings, Warning{
				Path:   p,
				Change: "",
				Msg:    fmt.Sprintf("no matches for %q", p),
			})
			continue
		}
		for _, m := range matches {
			abs, err := filepath.Abs(m)
			if err != nil {
				warnings = append(warnings, Warning{Path: m, Msg: err.Error()})
				continue
			}
			classified, w, err := classifyTarget(abs)
			if err != nil {
				return nil, nil, err
			}
			repos = append(repos, classified...)
			warnings = append(warnings, w...)
		}
	}
	repos = dedupeAndSort(repos)
	return repos, warnings, nil
}

// globMatches expands a single pattern. A pattern without glob
// metacharacters is treated as a literal path; a pattern with
// metacharacters is expanded by Glob. A literal path that does not
// exist yields zero matches (the caller will emit a Warning).
func globMatches(p string) ([]string, error) {
	if !hasGlob(p) {
		return []string{p}, nil
	}
	return filepath.Glob(p)
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
