package main

import (
	"context"
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

func NewPi(binary string) Agent {
	return PiAgent{Binary: binary}
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

func IsGitRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func gitHardReset(path, commit string) error {
	return exec.Command("git", "-C", path, "reset", "--hard", commit).Run()
}

func retryN(n int, f func() (bool, error)) error {
	var err error
	for range n {
		complete, e := f()
		if e == nil && complete {
			return nil
		}
		err = e
	}
	return err
}

type Watcher struct {
	agent Agent

	RetyCount int
}

func NewWatcher(n int) Watcher {
	return Watcher{agent: NewPi("pi"), RetyCount: n}
}

func (w Watcher) work(ctx context.Context, path string) (bool, error) {
	current, err := GetCurrentCommit(path)
	if err != nil {
		return false, err
	}
	changes := ListActiveOpenSpecChanges(path)
	if len(changes) == 0 {
		log.Printf("no active change on %s", path)
		return true, nil
	}
	change := changes[0]
	log.Printf("working %q on %s", change, path)
	if err := w.agent.Run(ctx, path, applyPrompt(change)); err != nil {
		log.Printf("failed %q on %s: %v", change, path, err)
		if rerr := gitHardReset(path, current); rerr != nil {
			return false, rerr
		}
		return false, err
	}
	done := !slices.Contains(ListActiveOpenSpecChanges(path), change)
	if done {
		log.Printf("completed %q on %s", change, path)
	}
	return done, nil
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
			if !IsGitRepo(repo) {
				continue
			}
			if err := retryN(w.RetyCount, func() (bool, error) {
				return w.work(ctx, repo)
			}); err != nil {
				return err
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
	w := NewWatcher(3)
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
