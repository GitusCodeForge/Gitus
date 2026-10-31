package routes

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRRDocPath(t *testing.T) {
	base := t.TempDir()
	static := filepath.Join(base, "static")

	ok := map[string]string{
		"guide.md":       filepath.Join(static, "_rrdoc", "guide.md"),
		"sub/guide.md":   filepath.Join(static, "_rrdoc", "sub", "guide.md"),
		// An absolute name is kept below the document directory instead of
		// replacing it.
		"/etc/passwd":    filepath.Join(static, "_rrdoc", "etc", "passwd"),
		"sub/../guide.md": filepath.Join(static, "_rrdoc", "guide.md"),
	}
	for name, want := range ok {
		got, err := ResolveRRDocPath(static, name)
		if err != nil { t.Errorf("ResolveRRDocPath(%q): %s", name, err); continue }
		if got != want { t.Errorf("ResolveRRDocPath(%q) = %q, want %q", name, got, want) }
	}

	bad := []string{
		"..",
		"../secret",
		"sub/../../secret",
		"../_rrdoc/../secret",
	}
	for _, name := range bad {
		if got, err := ResolveRRDocPath(static, name); err == nil {
			t.Errorf("ResolveRRDocPath(%q) = %q, want an error", name, got)
		} else if !strings.Contains(err.Error(), "outside") {
			t.Errorf("ResolveRRDocPath(%q): unexpected error %v", name, err)
		}
	}
}
