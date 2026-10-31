package log

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func withBuffer(t *testing.T, l Level) *bytes.Buffer {
	t.Helper()
	buf := new(bytes.Buffer)
	SetOutput(buf)
	SetLevel(l)
	t.Cleanup(func() {
		SetOutput(os.Stderr)
		SetLevel(LevelInfo)
	})
	return buf
}

func TestLevelsAreFiltered(t *testing.T) {
	buf := withBuffer(t, LevelWarn)
	Infof("this is hidden")
	Warnf("this is shown: %d", 1)
	Errorf("this is shown too: %s", "x")
	got := buf.String()
	if strings.Contains(got, "hidden") {
		t.Fatalf("INFO line was written at WARN level: %q", got)
	}
	if !strings.Contains(got, " WARN this is shown: 1\n") {
		t.Fatalf("WARN line missing: %q", got)
	}
	if !strings.Contains(got, " ERROR this is shown too: x\n") {
		t.Fatalf("ERROR line missing: %q", got)
	}
}

func TestNoneDisablesEverything(t *testing.T) {
	buf := withBuffer(t, LevelNone)
	Infof("a")
	Warnf("b")
	Errorf("c")
	if buf.Len() != 0 {
		t.Fatalf("output was not disabled: %q", buf.String())
	}
	if Enabled(LevelError) {
		t.Fatal("Enabled(ERROR) is true at LevelNone")
	}
}

func TestInfoLevelShowsEverything(t *testing.T) {
	buf := withBuffer(t, LevelInfo)
	Info("plain ", "info")
	if !strings.Contains(buf.String(), " INFO plain info\n") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}

func TestParseLevel(t *testing.T) {
	for s, want := range map[string]Level{
		"":      LevelInfo,
		"info":  LevelInfo,
		"INFO":  LevelInfo,
		"warn":  LevelWarn,
		"error": LevelError,
		"none":  LevelNone,
	} {
		got, err := ParseLevel(s)
		if err != nil || got != want {
			t.Fatalf("ParseLevel(%q) = %v, %v; want %v", s, got, err, want)
		}
	}
	if _, err := ParseLevel("verbose"); err == nil {
		t.Fatal("ParseLevel accepted an unknown level")
	}
}

func TestLineStartsWithTimestampAndLevel(t *testing.T) {
	buf := withBuffer(t, LevelInfo)
	Warnf("something")
	line := buf.String()
	// 2026-10-31T18:08:29+08:00 WARN something
	if len(line) < len("2026-10-31T18:08:29+08:00 WARN ") {
		t.Fatalf("line is too short: %q", line)
	}
	if line[4] != '-' || line[7] != '-' || line[10] != 'T' || line[16] != ':' {
		t.Fatalf("line does not start with an RFC3339 timestamp: %q", line)
	}
	if !strings.HasSuffix(line, " WARN something\n") {
		t.Fatalf("unexpected line: %q", line)
	}
}
