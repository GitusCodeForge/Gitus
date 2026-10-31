package gitlib

import (
	"os"
	"path"
	"strings"
	"testing"
)

func TestResolveSubmoduleURL(t *testing.T) {
	cases := []struct {
		ns, raw, full string
		gitus, ext    bool
	}{
		{"abc", "../ghi.git", "abc:ghi", true, false},
		{"abc", "../other:ghi.git", "other:ghi", true, false},
		{"abc", "../ghi", "abc:ghi", true, false},
		{"abc", "./ghi.git", "abc:ghi", true, false},
		{"abc", "ghi.git", "abc:ghi", true, false},
		{"abc", "abc:ghi", "abc:ghi", true, false},
		{"", "../ghi.git", "ghi", true, false},
		{"abc", "../ghi.git?x#y", "abc:ghi", true, false},
		{"abc", "https://github.com/x/y.git", "", false, true},
		{"abc", "git@github.com:x/y.git", "", false, true},
		{"abc", "ssh://git@example.com/x/y.git", "", false, true},
		{"abc", "", "", false, false},
		{"abc", "../", "", false, false},
	}
	for _, c := range cases {
		got := ResolveSubmoduleURL(c.ns, c.raw)
		if got.FullName != c.full || got.GitusResolved != c.gitus || got.External != c.ext {
			t.Errorf("ResolveSubmoduleURL(%q, %q) = %+v; want full=%q gitus=%v external=%v",
				c.ns, c.raw, got, c.full, c.gitus, c.ext)
		}
	}
}

func TestSuggestedGitSubmoduleURL(t *testing.T) {
	got := ResolveSubmoduleURL("abc", "../ghi.git")
	if got.SuggestedGitURL() != "../abc:ghi" {
		t.Fatalf("SuggestedGitURL = %q", got.SuggestedGitURL())
	}
	if u := ResolveSubmoduleURL("abc", "https://example.com/x.git"); u.SuggestedGitURL() != "" {
		t.Fatalf("external URL suggested %q", u.SuggestedGitURL())
	}
}

func TestParseSubmoduleConfig(t *testing.T) {
	src := `[submodule "dotfiles"]
	path = sub/dotfiles
	url = ../acme:dotfiles
	branch = main
	update = checkout
[submodule "no-path"]
	url = ../other
`
	subs, err := ParseSubmoduleConfig(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("got %d submodules", len(subs))
	}
	sm := subs["sub/dotfiles"]
	if sm == nil || sm.Url != "../acme:dotfiles" || sm.Branch != "main" || sm.Update != "checkout" {
		t.Fatalf("sub/dotfiles = %+v", sm)
	}
	if subs["no-path"] == nil {
		t.Fatal("a section without a path should fall back to the section name")
	}
}

func TestReadSubmoduleConfigFromTree(t *testing.T) {
	dir := t.TempDir()
	work := path.Join(dir, "work")
	if err := os.MkdirAll(work, 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, work, "init", "-q", "-b", "main")
	runGit(t, work, "config", "user.email", "a@example.com")
	runGit(t, work, "config", "user.name", "A")
	commitFile(t, work, ".gitmodules", `[submodule "dotfiles"]
	path = sub/dotfiles
	url = ../acme:dotfiles
	branch = main
`)
	commitFile(t, work, "README.md", "demo\n")

	bare := path.Join(dir, "bare.git")
	runGit(t, dir, "clone", "-q", "--bare", work, bare)
	gr := NewLocalGitRepository(bare)
	head := strings.TrimSpace(runGit(t, bare, "rev-parse", "HEAD"))
	obj, err := gr.ReadObject(head)
	if err != nil {
		t.Fatal(err)
	}
	commit, ok := obj.(*CommitObject)
	if !ok {
		t.Fatalf("%s is not a commit", head)
	}
	treeObj, err := gr.ReadObject(commit.TreeObjId)
	if err != nil {
		t.Fatal(err)
	}
	root, ok := treeObj.(*TreeObject)
	if !ok {
		t.Fatal("tree is not a tree")
	}
	subs, err := gr.ReadSubmoduleConfigFromTree(root)
	if err != nil {
		t.Fatal(err)
	}
	sm := subs["sub/dotfiles"]
	if sm == nil || sm.Url != "../acme:dotfiles" || sm.Branch != "main" {
		t.Fatalf("submodule config = %+v", sm)
	}
}
