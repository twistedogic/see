package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
)

type Agent interface {
	Run(ctx context.Context, path, prompt string) error
}

type PiAgent struct {
	Binary string
}

func (p PiAgent) Run(ctx context.Context, path, prompt string) error {
	cmd := exec.CommandContext(ctx, p.Binary, "--mode", "json", "--no-session", prompt)
	cmd.Dir = path
	return cmd.Run()
}

func applyPrompt(change string) string {
	return fmt.Sprintf(
		"Apply the openspec change %q: read its proposal and tasks, implement them, run the tests, verify, then archive the change. Sync specs if needed.",
		change,
	)
}

func ListActiveOpenSpecChanges(cwd string) []string {
	entries, err := os.ReadDir(filepath.Join(cwd, "openspec", "changes"))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "archive" {
			names = append(names, e.Name())
		}
	}
	return names
}

func GetCurrentCommit(cwd string) (string, error) {
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ponytail: previously took (bool, error); the (false, nil) state could
// clobber a prior error and report silent success. Contract collapsed to
// func() error so the bug class is unreachable.
func retryN(n int, f func() error) error {
	var err error
	for range n {
		if err = f(); err == nil {
			return nil
		}
	}
	return err
}

type Watcher struct {
	agent Agent

	RetyCount int
}

func NewWatcher(binary string, n int) Watcher {
	return Watcher{agent: PiAgent{Binary: binary}, RetyCount: n}
}

func (w Watcher) work(ctx context.Context, path string) error {
	current, err := GetCurrentCommit(path)
	if err != nil {
		return err
	}
	changes := ListActiveOpenSpecChanges(path)
	if len(changes) == 0 {
		log.Printf("no active change on %s", path)
		return nil
	}
	change := changes[0]
	log.Printf("working %q on %s", change, path)
	if err := w.agent.Run(ctx, path, applyPrompt(change)); err != nil {
		log.Printf("failed %q on %s: %v", change, path, err)
		if rerr := exec.Command("git", "-C", path, "reset", "--hard", current).Run(); rerr != nil {
			return rerr
		}
		return err
	}
	done := !slices.Contains(ListActiveOpenSpecChanges(path), change)
	if done {
		log.Printf("completed %q on %s", change, path)
		// ponytail: commit runs inline; if `git add` fails (e.g., dirty submodules) we still try commit so manually-staged work isn't lost.
		add := exec.Command("git", "-C", path, "add", "-A")
		if err := add.Run(); err != nil {
			log.Printf("git add failed %q on %s: %v", change, path, err)
		}
		msg := fmt.Sprintf("see: apply openspec change %s", change)
		if err := exec.Command("git", "-C", path, "commit", "-m", msg).Run(); err != nil {
			log.Printf("git commit failed %q on %s: %v", change, path, err)
		}
	}
	return nil
}

func (w Watcher) runOnce(ctx context.Context, wd string) error {
	entries, err := os.ReadDir(wd)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil
		default:
			if !entry.IsDir() {
				continue
			}
			repo := filepath.Join(wd, entry.Name())
			if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
				continue
			}
			if _, err := GetCurrentCommit(repo); err != nil {
				log.Printf("skipping %s: no commits", repo)
				continue
			}
			if err := retryN(w.RetyCount, func() error {
				return w.work(ctx, repo)
			}); err != nil {
				return fmt.Errorf("%s: %w", repo, err)
			}
		}
	}
	return nil
}

func (w Watcher) Watch(ctx context.Context, wd string) error {
	// ponytail: tight poll loop, add a backoff sleep when watching large trees.
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if err := w.runOnce(ctx, wd); err != nil {
				return err
			}
		}
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	var (
		pi    = flag.String("pi", "pi", "path to the pi binary")
		retry = flag.Int("retry", 3, "retries per repo on failure")
	)
	flag.Parse()
	w := NewWatcher(*pi, *retry)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	path, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if err := w.Watch(ctx, path); err != nil {
		log.Fatal(err)
	}
}
