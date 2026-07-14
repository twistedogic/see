package main

import (
	"encoding/json"
	"fmt"
	"io"
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

// logFilename builds the `<stem>--<utc-20060102T150405>--<pid>.jsonl`
// filename shared by the batch-level event log and the per-invocation
// agent log. Two files with the same PID in the same UTC second are
// distinct by stem; concurrent runs from the same process get
// distinct filenames by stem ("see" vs the repo basename).
func logFilename(stem string) string {
	return fmt.Sprintf("%s--%s--%d.jsonl",
		stem,
		time.Now().UTC().Format("20060102T150405"),
		os.Getpid(),
	)
}

// eventLogPath returns the filename for the batch-level event log.
// One file per process; UTC timestamp + PID make concurrent runs
// from the same PID distinguishable by second, and concurrent runs
// from different PIDs always distinct.
func eventLogPath() string {
	return logFilename("see")
}

// eventLogger writes every event the watcher emits to a single
// batch-level JSONL file and, when a secondary observer or mirror
// is attached, forwards each event after the disk write:
//   - `secondary` is the TUI's fan-out target (the bubbletea
//     ChanObserver via tuiObserver). nil in log mode.
//   - `mirror` is the optional stream observer (typically
//     os.Stdout) that receives the same JSONL bytes as the file,
//     so `see --mode=log | jq` works. nil when stdout is a TTY
//     or in TUI mode.
//
// Encoding happens once per event: the marshalled bytes land on
// every sink instead of paying encoder state across them. Best-
// effort writes: a marshal failure or sink write error is
// swallowed because the JSONL is observability, not correctness.
type eventLogger struct {
	f *os.File
	// ponytail: mirror is the optional stream observer (typically
	// os.Stdout in --mode=log when stdout is not a TTY). nil in TUI
	// mode and in --mode=log when stdout is a TTY. Writes share the
	// same JSONL encoding as the file, so `see --mode=log | jq`
	// parses identically to `cat <jsonl-file>`.
	mirror    io.Writer
	secondary Observer
}

func openEventLogger(dir string) (*eventLogger, error) {
	f, err := os.Create(filepath.Join(dir, eventLogPath()))
	if err != nil {
		return nil, fmt.Errorf("event log: %w", err)
	}
	return &eventLogger{f: f}, nil
}

// Attach wires a secondary observer (typically the TUI adapter) to
// receive every event after it has been written to the JSONL.
func (l *eventLogger) Attach(obs Observer) { l.secondary = obs }

// SetMirror wires an extra sink (typically os.Stdout in --mode=log
// when stdout is not a TTY) to receive the same JSONL bytes as
// the on-disk file. nil clears the mirror.
func (l *eventLogger) SetMirror(w io.Writer) { l.mirror = w }

func (l *eventLogger) Observe(e Event) {
	payload, err := json.Marshal(e)
	if err != nil {
		return
	}
	// ponytail: envelope pattern — each line is `{"ts": <observed-
	// at RFC3339Nano UTC>, "event": <original payload>}`. The
	// timestamp pins event order down to nanoseconds without
	// relying on the file name's UTC-second granularity; two
	// events sharing the same second stay time-ordered.
	line, err := json.Marshal(struct {
		Ts    string          `json:"ts"`
		Event json.RawMessage `json:"event"`
	}{
		Ts:    time.Now().UTC().Format(time.RFC3339Nano),
		Event: payload,
	})
	if err != nil {
		return
	}
	line = append(line, '\n')
	_, _ = l.f.Write(line)
	if l.mirror != nil {
		_, _ = l.mirror.Write(line)
	}
	if l.secondary != nil {
		l.secondary.Observe(e)
	}
}

func (l *eventLogger) Close() error { return l.f.Close() }
