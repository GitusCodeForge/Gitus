// Package log writes timestamped, leveled log lines.
//
// There are three levels, from least to most severe: Info, Warn and Error.
// A line is written only when its level is at or above the configured level,
// so SetLevel(LevelWarn) hides Info lines and SetLevel(LevelNone) turns the
// output off entirely. See docs/logging.org for the recommended usage.
package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Level is the severity of a log line.
type Level int

const (
	// LevelNone disables all log output.
	LevelNone Level = iota
	// LevelError is for errors that cannot be handled in a non-failing way
	// (a failed database commit, a repository that cannot be opened).
	LevelError
	// LevelWarn is for errors that are handled with a fallback, and for
	// unexpected states that do not stop the operation.
	LevelWarn
	// LevelInfo is for the normal happenings of the program. It is also the
	// level of debugging aid messages.
	LevelInfo
)

// String returns the name used in a log line ("ERROR", "WARN", "INFO").
func (l Level) String() string {
	switch l {
	case LevelError:
		return "ERROR"
	case LevelWarn:
		return "WARN"
	case LevelInfo:
		return "INFO"
	default:
		return "NONE"
	}
}

var (
	lock     sync.Mutex
	out      io.Writer = os.Stderr
	minLevel           = LevelInfo
)

// ParseLevel parses a level name. An empty string is Info, so that an older
// configuration file without the setting behaves like the old build.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error", "err":
		return LevelError, nil
	case "none", "off", "silent", "quiet":
		return LevelNone, nil
	}
	return LevelInfo, fmt.Errorf("unknown log level %q (expected info, warn, error or none)", s)
}

// SetLevel sets the minimum severity that is written. LevelNone disables the
// output. The default is LevelInfo.
func SetLevel(l Level) {
	lock.Lock()
	minLevel = l
	lock.Unlock()
}

// GetLevel returns the current minimum severity.
func GetLevel() Level {
	lock.Lock()
	defer lock.Unlock()
	return minLevel
}

// SetOutput sets where log lines are written. The default is os.Stderr.
// Passing nil discards all output, which is useful in tests.
func SetOutput(w io.Writer) {
	if w == nil {
		w = io.Discard
	}
	lock.Lock()
	out = w
	lock.Unlock()
}

// Enabled reports whether a line of level l would be written.
func Enabled(l Level) bool {
	lock.Lock()
	defer lock.Unlock()
	return enabledLocked(l)
}

func enabledLocked(l Level) bool {
	// Levels are ordered from most severe (Error, 1) to least (Info, 3).
	// LevelNone is 0, so nothing passes when it is the minimum.
	return l <= minLevel && l > LevelNone
}

// Logf writes one line at level l. The level of a call is normally part of
// the function name; this is for the rare caller that picks a level from
// configuration.
func Logf(l Level, format string, args ...any) {
	lock.Lock()
	defer lock.Unlock()
	if !enabledLocked(l) {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	fmt.Fprintf(out, "%s %s %s\n", time.Now().Format(time.RFC3339), l.String(), msg)
}

// Infof writes an INFO line using fmt.Sprintf formatting.
func Infof(format string, args ...any) { Logf(LevelInfo, format, args...) }

// Warnf writes a WARN line using fmt.Sprintf formatting.
func Warnf(format string, args ...any) { Logf(LevelWarn, format, args...) }

// Errorf writes an ERROR line using fmt.Sprintf formatting.
func Errorf(format string, args ...any) { Logf(LevelError, format, args...) }

// Info writes an INFO line from the operands of fmt.Sprint.
func Info(args ...any) { Logf(LevelInfo, "%s", fmt.Sprint(args...)) }

// Warn writes a WARN line from the operands of fmt.Sprint.
func Warn(args ...any) { Logf(LevelWarn, "%s", fmt.Sprint(args...)) }

// Error writes an ERROR line from the operands of fmt.Sprint.
func Error(args ...any) { Logf(LevelError, "%s", fmt.Sprint(args...)) }
