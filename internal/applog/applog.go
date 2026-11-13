package applog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	keepSessions  = 10
	sessionGlob   = "session-*.log"
	maxRecentCmds = 20
)

var (
	mu         sync.Mutex
	file       *os.File
	dirFn      = defaultDir
	nowFn      = time.Now
	pidFn      = os.Getpid
	version    = "dev"
	recentCmds []string
)

// SetVersion records the app version written into the session header.
func SetVersion(v string) {
	if strings.TrimSpace(v) != "" {
		version = v
	}
}

// SetDirForTest overrides the log directory (tests only).
func SetDirForTest(dir string) {
	dirFn = func() string { return dir }
}

// ResetForTest closes any open log and restores defaults (tests only).
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
		file = nil
	}
	dirFn = defaultDir
	nowFn = time.Now
	pidFn = os.Getpid
	recentCmds = nil
}

func defaultDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return filepath.Join(".", "kube-commander", "logs")
			}
			return filepath.Join(home, "AppData", "Local", "kube-commander", "logs")
		}
		return filepath.Join(base, "kube-commander", "logs")
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".", "kube-commander", "logs")
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "kube-commander", "logs")
}

// Dir returns the active log directory.
func Dir() string {
	return dirFn()
}

// Init creates the log directory, opens a new session file, and prunes old sessions.
// Failures are best-effort: returns an error for the caller to warn, but Infof/Cmd stay no-ops.
func Init() error {
	mu.Lock()
	defer mu.Unlock()

	if file != nil {
		_ = file.Close()
		file = nil
	}

	dir := dirFn()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	name := fmt.Sprintf("session-%s.log", nowFn().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open session log: %w", err)
	}
	file = f

	header := fmt.Sprintf("kube-commander session log\nversion=%s\npid=%d\nstarted=%s\n---\n",
		version, pidFn(), nowFn().Format(time.RFC3339))
	if _, err := file.WriteString(header); err != nil {
		_ = file.Close()
		file = nil
		return fmt.Errorf("write session header: %w", err)
	}

	if err := pruneLocked(dir); err != nil {
		// Non-fatal: session file is usable even if prune fails.
		_, _ = fmt.Fprintf(file, "%s prune warning: %v\n", stamp(), err)
	}
	return nil
}

func pruneLocked(dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, sessionGlob))
	if err != nil {
		return err
	}
	if len(matches) <= keepSessions {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i] > matches[j] // newest names first (timestamp in filename)
	})
	for _, path := range matches[keepSessions:] {
		_ = os.Remove(path)
	}
	return nil
}

// Close flushes and closes the session log.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file == nil {
		return
	}
	_ = file.Close()
	file = nil
}

// Writer returns a writer that appends to the session log (mutex-safe).
// Returns io.Discard when logging is not initialized.
func Writer() io.Writer {
	mu.Lock()
	defer mu.Unlock()
	if file == nil {
		return io.Discard
	}
	return logWriter{}
}

type logWriter struct{}

func (logWriter) Write(p []byte) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	if file == nil {
		return len(p), nil
	}
	return file.Write(p)
}

// Infof writes a timestamped info line.
// Lines that look like shell starts ("starting interactive: …") are also recorded in RecentCmds.
func Infof(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	if strings.HasPrefix(msg, "starting interactive: ") {
		recordCmdLocked(strings.TrimPrefix(msg, "starting interactive: "))
	}
	if file == nil {
		return
	}
	_, _ = file.WriteString(stamp() + " " + msg + "\n")
}

// Cmd logs an external command invocation and its combined output.
func Cmd(name string, args []string, output string, err error) {
	mu.Lock()
	defer mu.Unlock()
	line := name
	if len(args) > 0 {
		line += " " + strings.Join(args, " ")
	}
	recordCmdLocked(line)
	if file == nil {
		return
	}
	var b strings.Builder
	b.WriteString(stamp())
	b.WriteString(" $ ")
	b.WriteString(line)
	b.WriteString("\n")
	out := strings.TrimSpace(output)
	if out != "" {
		b.WriteString(out)
		b.WriteString("\n")
	}
	if err != nil {
		b.WriteString(stamp())
		b.WriteString(" exit: error: ")
		b.WriteString(err.Error())
		b.WriteString("\n")
	} else {
		b.WriteString(stamp())
		b.WriteString(" exit: ok\n")
	}
	_, _ = file.WriteString(b.String())
}

// RecentCmds returns a copy of the recent shell command lines (oldest first).
func RecentCmds() []string {
	mu.Lock()
	defer mu.Unlock()
	out := make([]string, len(recentCmds))
	copy(out, recentCmds)
	return out
}

func recordCmdLocked(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	recentCmds = append(recentCmds, line)
	if len(recentCmds) > maxRecentCmds {
		recentCmds = recentCmds[len(recentCmds)-maxRecentCmds:]
	}
}

func stamp() string {
	return nowFn().Format("15:04:05")
}
