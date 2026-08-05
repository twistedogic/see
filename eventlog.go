package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// resolveLogDir returns the directory under which all log files for
// this `see` process are created, applying a three-source precedence:
// SEE_LOG_DIR (non-empty) wins; otherwise a non-blank cfg.LogDir wins;
// otherwise defaultLogDir (~/.cache/see/logs). The chosen candidate is
// tilde-expanded using the same rule as root_dir, so SEE_LOG_DIR=~/logs
// resolves to <home>/logs rather than a literal '~' directory. The
// directory is created (MkdirAll 0o755) before return. Failure to
// create the directory is a fatal startup error — the watcher must not
// run, because per-invocation JSONL files have nowhere to land.
func resolveLogDir(cfg Config) (string, error) {
	var candidate string
	if env := os.Getenv("SEE_LOG_DIR"); env != "" {
		candidate = env
	} else {
		candidate = cfg.LogDir
	}
	if strings.TrimSpace(candidate) == "" {
		candidate = defaultLogDir
	}
	dir, err := expandTilde(candidate)
	if err != nil {
		return "", fmt.Errorf("log dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("log dir: %w", err)
	}
	return dir, nil
}

// maxInvocLogsPerStem bounds the per-invocation JSONL files that
// share a stem to the newest N; older files in the same stem are
// removed by rotateLogs after each PiAgent.Run. The value is a fixed
// implementation constant because "JSONL is observability, not
// data" — add an operator knob only when a real need appears.
const maxInvocLogsPerStem = 5

// invocStem returns the filename stem shared by pathFor (which
// builds the per-invocation filename) and rotateLogs (which bounds
// its population). Extracting it ensures the write path and the
// rotation selector cannot drift: they agree on what "the same
// stream" means. The digest spelling in custom mode is supplied by
// the caller, so the helper has no knowledge of change vs digest.
func invocStem(repo, identity string) string {
	return filepath.Base(repo) + "--" + identity
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

// rotateLogs trims the population of per-invocation JSONL files that
// share stem inside dir to keep newest entries. Selection uses the
// exact prefix stem+"--" plus the ".jsonl" suffix so the trailing
// "--" prevents a stem like "myproj--add" from matching files for
// "myproj--add-dark-mode". Recency is determined by lexicographic
// filename sort: the embedded fixed-width YYYYMMDDTHHMMSS timestamp
// makes that order chronological at second granularity, with no
// os.Stat needed. After sorting ascending, the oldest files sit
// at the front and are deleted; the newest keep files at the back
// are retained. Each removal is best-effort; a failure to delete
// one file does not stop the others and is silently swallowed,
// consistent with "JSONL is observability, not data".
func rotateLogs(dir, stem string, keep int) {
	prefix := stem + "--"
	suffix := ".jsonl"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, prefix) && strings.HasSuffix(n, suffix) {
			names = append(names, n)
		}
	}
	if len(names) <= keep {
		return
	}
	slices.Sort(names) // ascending; oldest files sit at the front
	for _, n := range names[:len(names)-keep] {
		_ = os.Remove(filepath.Join(dir, n))
	}
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
