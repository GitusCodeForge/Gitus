package gitlib

import (
	"errors"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

func TestInstallManagedHook(t *testing.T) {
	gr, dir := newTestRepo(t)
	hookPath := path.Join(gr.GitDirectoryPath, "hooks", "update")

	if gr.HasManagedHook() {
		t.Fatal("a fresh repository should not report a managed hook")
	}

	params := ManagedHookParams{ConfigPath: "/etc/gitus/config.json", RepoFullName: "ns:repo"}
	if err := gr.InstallManagedHook(params); err != nil { t.Fatal(err) }
	if !gr.HasManagedHook() { t.Fatal("the hook was not installed") }

	content, err := os.ReadFile(hookPath)
	if err != nil { t.Fatal(err) }
	for _, want := range []string{
		managedHookMarker,
		"'/etc/gitus/config.json'",
		"'ns:repo'",
		"update-trigger",
		"exit 0",
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("the hook does not contain %q:\n%s", want, string(content))
		}
	}

	// It has to be executable, and it has to be valid shell.
	info, err := os.Stat(hookPath)
	if err != nil { t.Fatal(err) }
	if info.Mode()&0111 == 0 { t.Fatal("the hook is not executable") }
	out, err := exec.Command("sh", "-n", hookPath).CombinedOutput()
	if err != nil {
		t.Fatalf("the hook does not parse: %s\n%s", err.Error(), string(out))
	}

	// Installing again must be a no-op.
	err = gr.InstallManagedHook(params)
	if err != nil { t.Fatalf("reinstalling failed: %s", err.Error()) }
	again, err := os.ReadFile(hookPath)
	if err != nil { t.Fatal(err) }
	if string(again) != string(content) {
		t.Fatal("reinstalling rewrote an up to date hook")
	}

	// A changed repository name has to be picked up.
	err = gr.InstallManagedHook(ManagedHookParams{ConfigPath: "/etc/gitus/config.json", RepoFullName: "ns:other"})
	if err != nil { t.Fatal(err) }
	content, err = os.ReadFile(hookPath)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(content), "'ns:other'") {
		t.Fatal("the hook was not refreshed")
	}
	if strings.Contains(string(content), "'ns:repo'") {
		t.Fatal("the hook still mentions the old repository name")
	}
	_ = dir
}

func TestInstallManagedHookRefusesForeignHook(t *testing.T) {
	gr, _ := newTestRepo(t)
	hookPath := path.Join(gr.GitDirectoryPath, "hooks", "update")
	foreign := "#!/bin/sh\n# somebody else's hook\nexit 0\n"
	if err := os.MkdirAll(path.Dir(hookPath), 0755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(hookPath, []byte(foreign), 0755); err != nil { t.Fatal(err) }

	err := gr.InstallManagedHook(ManagedHookParams{ConfigPath: "/etc/gitus/config.json", RepoFullName: "ns:repo"})
	if !errors.Is(err, ErrHookNotManaged) {
		t.Fatalf("expected ErrHookNotManaged, got %v", err)
	}
	content, err := os.ReadFile(hookPath)
	if err != nil { t.Fatal(err) }
	if string(content) != foreign {
		t.Fatal("a foreign hook was overwritten")
	}
	if gr.HasManagedHook() { t.Fatal("a foreign hook must not be reported as managed") }
}

func TestInstallManagedHookQuoting(t *testing.T) {
	gr, _ := newTestRepo(t)
	// A quote in the path has to survive shell quoting; this is why the hook is
	// generated through shellQuote rather than by pasting the value in.
	err := gr.InstallManagedHook(ManagedHookParams{
		ConfigPath: "/tmp/it's here/config.json",
		RepoFullName: "ns:repo",
	})
	if err != nil { t.Fatal(err) }
	hookPath := path.Join(gr.GitDirectoryPath, "hooks", "update")
	content, err := os.ReadFile(hookPath)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(content), `'/tmp/it'\''s here/config.json'`) {
		t.Fatalf("the path was not quoted correctly:\n%s", string(content))
	}
	if out, err := exec.Command("sh", "-n", hookPath).CombinedOutput(); err != nil {
		t.Fatalf("the hook does not parse: %s\n%s", err.Error(), string(out))
	}
}

func TestInstallManagedHookUpgradesLegacyHook(t *testing.T) {
	gr, _ := newTestRepo(t)
	hookPath := path.Join(gr.GitDirectoryPath, "hooks", "update")
	// What the old EnableWebHook wrote: no marker, no path, no config.
	legacy := "#!/bin/sh\n...\ngitus web-hooks send \"repo\" \"$refname\" commit \"$oldrev\" \"$newrev\"\n"
	if err := os.MkdirAll(path.Dir(hookPath), 0755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(hookPath, []byte(legacy), 0755); err != nil { t.Fatal(err) }
	if !gr.HasManagedHook() { t.Fatal("the legacy hook should be recognized as ours") }
	err := gr.InstallManagedHook(ManagedHookParams{ConfigPath: "/etc/gitus/config.json", RepoFullName: "ns:repo"})
	if err != nil { t.Fatalf("the legacy hook was not upgraded: %s", err.Error()) }
	content, err := os.ReadFile(hookPath)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(content), managedHookMarker) || strings.Contains(string(content), "web-hooks send") {
		t.Fatal("the legacy hook was not replaced")
	}
}
