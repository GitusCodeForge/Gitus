package gitlib

import (
	"errors"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

func hookPaths(gr *LocalGitRepository) (update string, preReceive string) {
	return path.Join(gr.GitDirectoryPath, "hooks", "update"),
		path.Join(gr.GitDirectoryPath, "hooks", "pre-receive")
}

func readHook(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertShellParses(t *testing.T, p string) {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("%s is not executable", p)
	}
	if out, err := exec.Command("sh", "-n", p).CombinedOutput(); err != nil {
		t.Fatalf("%s does not parse: %s\n%s", p, err.Error(), string(out))
	}
}

func TestInstallManagedHook(t *testing.T) {
	gr, _ := newTestRepo(t)
	updatePath, preReceivePath := hookPaths(gr)

	if gr.HasManagedHook() {
		t.Fatal("a fresh repository should not report a managed hook")
	}

	params := ManagedHookParams{ConfigPath: "/etc/gitus/config.json", RepoFullName: "ns:repo"}
	if err := gr.InstallManagedHook(params); err != nil {
		t.Fatal(err)
	}
	if !gr.HasManagedHook() {
		t.Fatal("the hook was not installed")
	}

	update := readHook(t, updatePath)
	for _, want := range []string{
		managedHookMarker,
		"'/etc/gitus/config.json'",
		"'ns:repo'",
		"update-trigger",
		"exit 0",
	} {
		if !strings.Contains(update, want) {
			t.Fatalf("the update hook does not contain %q:\n%s", want, update)
		}
	}
	preReceive := readHook(t, preReceivePath)
	for _, want := range []string{
		managedHookMarker,
		"'/etc/gitus/config.json'",
		"'ns:repo'",
		"storage-check",
		`[ "$has_objects" = "1" ]`,
		"exit 0",
	} {
		if !strings.Contains(preReceive, want) {
			t.Fatalf("the pre-receive hook does not contain %q:\n%s", want, preReceive)
		}
	}
	assertShellParses(t, updatePath)
	assertShellParses(t, preReceivePath)

	// Installing again must be a no-op.
	if err := gr.InstallManagedHook(params); err != nil {
		t.Fatalf("reinstalling failed: %s", err.Error())
	}
	if got := readHook(t, updatePath); got != update {
		t.Fatal("reinstalling rewrote an up to date update hook")
	}
	if got := readHook(t, preReceivePath); got != preReceive {
		t.Fatal("reinstalling rewrote an up to date pre-receive hook")
	}

	// A changed repository name has to be picked up.
	if err := gr.InstallManagedHook(ManagedHookParams{ConfigPath: "/etc/gitus/config.json", RepoFullName: "ns:other"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{updatePath, preReceivePath} {
		content := readHook(t, p)
		if !strings.Contains(content, "'ns:other'") {
			t.Fatalf("%s was not refreshed", p)
		}
		if strings.Contains(content, "'ns:repo'") {
			t.Fatalf("%s still mentions the old repository name", p)
		}
	}
}

func TestInstallManagedHookRefusesForeignHook(t *testing.T) {
	for _, hookName := range []string{"update", "pre-receive"} {
		t.Run(hookName, func(t *testing.T) {
			gr, _ := newTestRepo(t)
			updatePath, preReceivePath := hookPaths(gr)
			foreignPath := updatePath
			if hookName == "pre-receive" {
				foreignPath = preReceivePath
			}
			foreign := "#!/bin/sh\n# somebody else's hook\nexit 0\n"
			if err := os.MkdirAll(path.Dir(foreignPath), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(foreignPath, []byte(foreign), 0755); err != nil {
				t.Fatal(err)
			}

			err := gr.InstallManagedHook(ManagedHookParams{ConfigPath: "/etc/gitus/config.json", RepoFullName: "ns:repo"})
			if !errors.Is(err, ErrHookNotManaged) {
				t.Fatalf("expected ErrHookNotManaged, got %v", err)
			}
			if got := readHook(t, foreignPath); got != foreign {
				t.Fatal("a foreign hook was overwritten")
			}
			// the other hook must not have been written either
			other := updatePath
			if hookName == "update" {
				other = preReceivePath
			}
			if _, err := os.Stat(other); !os.IsNotExist(err) {
				t.Fatalf("%s was written although %s is foreign", other, foreignPath)
			}
		})
	}
}

func TestInstallManagedHookQuoting(t *testing.T) {
	gr, _ := newTestRepo(t)
	updatePath, preReceivePath := hookPaths(gr)
	// A quote in the path has to survive shell quoting; this is why the hook is
	// generated through shellQuote rather than by pasting the value in.
	err := gr.InstallManagedHook(ManagedHookParams{
		ConfigPath:   "/tmp/it's here/config.json",
		RepoFullName: "ns:repo",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{updatePath, preReceivePath} {
		content := readHook(t, p)
		if !strings.Contains(content, `'/tmp/it'\''s here/config.json'`) {
			t.Fatalf("the path was not quoted correctly in %s:\n%s", p, content)
		}
		assertShellParses(t, p)
	}
}

func TestInstallManagedHookUpgradesLegacyHook(t *testing.T) {
	gr, _ := newTestRepo(t)
	updatePath, _ := hookPaths(gr)
	legacy := "#!/bin/sh\ngitus web-hooks send\n"
	if err := os.MkdirAll(path.Dir(updatePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(updatePath, []byte(legacy), 0755); err != nil {
		t.Fatal(err)
	}
	if !gr.HasManagedHook() {
		t.Fatal("the legacy hook should be recognized as ours")
	}
	err := gr.InstallManagedHook(ManagedHookParams{ConfigPath: "/etc/gitus/config.json", RepoFullName: "ns:repo"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readHook(t, updatePath), "web-hooks send") {
		t.Fatal("the legacy hook was not upgraded")
	}
}
