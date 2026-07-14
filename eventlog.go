package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ensureLogDir returns the directory under which all log files for
// this `see` process should be created. It honors SEE_LOG_DIR when
// set; otherwise it falls back to os.UserCacheDir()/see/logs/. The
// directory is created (MkdirAll 0o755) before return. Failure to
// create the directory is a fatal startup error — the watcher must
// not run, because per-invocation JSONL files have nowhere to land.
func ensureLogDir() (string, error) {
	dir := os.Getenv("SEE_LOG_DIR")
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("log dir: %w", err)
		}
		dir = filepath.Join(base, "see", "logs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("log dir: %w", err)
	}
	return dir, nil
}

// eventLogPath returns the filename for the batch-level event log.
// One file per process; UTC timestamp + PID make concurrent runs
// from the same PID distinguishable by second, and concurrent runs
// from different PIDs always distinct.
func eventLogPath() string {
	return fmt.Sprintf("see--%s--%d.jsonl",
		time.Now().UTC().Format("20060102T150405"),
		os.Getpid(),
	)
}

// eventLogger writes every event the watcher emits to a single
// batch-level JSONL file and, when a secondary observer is
// attached, forwards each event to that observer after the disk
// write. In TUI mode the secondary observer is the bubbletea
// ChanObserver (via the tuiObserver adapter); in log mode the
// secondary observer is nil and the JSONL is the only sink.
type eventLogger struct {
	f   *os.File
	enc *json.Encoder
	// ponytail: secondary is the TUI's fan-out target. nil in log
	// mode. Best-effort write: a JSON encode failure is swallowed
	// (the JSONL is observability, not correctness — losing a
	// trailing event should not crash the watcher).
	secondary Observer
}

func openEventLogger(dir string) (*eventLogger, error) {
	f, err := os.Create(filepath.Join(dir, eventLogPath()))
	if err != nil {
		return nil, fmt.Errorf("event log: %w", err)
	}
	return &eventLogger{f: f, enc: json.NewEncoder(f)}, nil
}

// Attach wires a secondary observer (typically the TUI adapter) to
// receive every event after it has been written to the JSONL.
func (l *eventLogger) Attach(obs Observer) { l.secondary = obs }

func (l *eventLogger) Observe(e Event) {
	_ = l.enc.Encode(e)
	if l.secondary != nil {
		l.secondary.Observe(e)
	}
}

func (l *eventLogger) Close() error { return l.f.Close() }
