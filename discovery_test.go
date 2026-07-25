package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mkRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestExpandTilde(t *testing.T) {
	t.Setenv("HOME", "/home/alice")
	for _, tc := range []struct{ in, want string }{
		{"~/work", "/home/alice/work"},
		{"~", "/home/alice"},
		{"/abs/path", "/abs/path"},
		{"relative/path", "relative/path"},
		{"~other", "~other"},
		{"", ""},
	} {
		got, err := expandTilde(tc.in)
		if err != nil {
			t.Fatalf("expandTilde(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("expandTilde(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveConfiguredTargetsFallsBackToCWD(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root)
	t.Chdir(root)

	got, warnings, err := resolveConfiguredTargets(Config{})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("resolve: warnings=%v err=%v", warnings, err)
	}
	if !reflect.DeepEqual(got, []string{root}) {
		t.Fatalf("got %v, want [%q]", got, root)
	}
}

func TestResolveConfiguredTargetsIncludesLiteralAndWildcard(t *testing.T) {
	root := t.TempDir()
	playRust := filepath.Join(root, "playground-rust")
	playGo := filepath.Join(root, "playground-go")
	mkRepo(t, playRust)
	mkRepo(t, playGo)
	mkRepo(t, filepath.Join(root, "notes"))

	for _, tc := range []struct {
		name    string
		include []string
		want    []string
	}{
		{"literal", []string{"playground-rust"}, []string{playRust}},
		{"wildcard", []string{"playground-*"}, []string{playGo, playRust}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, warnings, err := resolveConfiguredTargets(Config{RootDir: root, Include: tc.include})
			if err != nil || len(warnings) != 0 {
				t.Fatalf("resolve: warnings=%v err=%v", warnings, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveConfiguredTargetsEmptyIncludeUsesEveryDirectory(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mkRepo(t, repo)
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, warnings, err := resolveConfiguredTargets(Config{RootDir: root})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("resolve: warnings=%v err=%v", warnings, err)
	}
	if !reflect.DeepEqual(got, []string{repo}) {
		t.Fatalf("got %v, want [%q]", got, repo)
	}
}

func TestResolveConfiguredTargetsExcludesBasenames(t *testing.T) {
	root := t.TempDir()
	notes := filepath.Join(root, "notes")
	mkRepo(t, filepath.Join(root, "bin"))
	mkRepo(t, filepath.Join(root, "playground-rust"))
	mkRepo(t, notes)

	got, warnings, err := resolveConfiguredTargets(Config{
		RootDir: root,
		Include: []string{"*"},
		Exclude: []string{"bin", "playground*"},
	})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("resolve: warnings=%v err=%v", warnings, err)
	}
	if !reflect.DeepEqual(got, []string{notes}) {
		t.Fatalf("got %v, want [%q]", got, notes)
	}
}

func TestResolveConfiguredTargetsEmptyExcludeDropsNothing(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mkRepo(t, repo)

	got, warnings, err := resolveConfiguredTargets(Config{RootDir: root, Include: []string{"repo"}, Exclude: []string{}})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("resolve: warnings=%v err=%v", warnings, err)
	}
	if !reflect.DeepEqual(got, []string{repo}) {
		t.Fatalf("got %v, want [%q]", got, repo)
	}
}

func TestResolveConfiguredTargetsClassifiesParentOfRepos(t *testing.T) {
	root := t.TempDir()
	direct := filepath.Join(root, "direct")
	parent := filepath.Join(root, "parent")
	child := filepath.Join(parent, "child")
	mkRepo(t, direct)
	mkRepo(t, child)

	got, warnings, err := resolveConfiguredTargets(Config{RootDir: root})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("resolve: warnings=%v err=%v", warnings, err)
	}
	if want := []string{direct, child}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolveConfiguredTargetsPassesThroughClassificationWarnings(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, warnings, err := resolveConfiguredTargets(Config{RootDir: root})
	if err != nil || len(got) != 0 {
		t.Fatalf("resolve: got=%v err=%v", got, err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Msg, "no .git children") {
		t.Fatalf("warnings=%v, want one classification warning", warnings)
	}
}

func TestDedupeAndSort(t *testing.T) {
	got := dedupeAndSort([]string{"/repos/zeta", "/repos/alpha", "/repos/mu", "/repos/alpha/."})
	want := []string{"/repos/alpha", "/repos/mu", "/repos/zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := dedupeAndSort(nil); got != nil {
		t.Fatalf("empty got %v, want nil", got)
	}
}
