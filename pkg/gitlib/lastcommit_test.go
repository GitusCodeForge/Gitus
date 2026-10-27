package gitlib

import (
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %s", strings.Join(args, " "), err.Error(), string(out))
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, dir string, name string, content string) {
	t.Helper()
	p := path.Join(dir, name)
	if err := os.MkdirAll(path.Dir(p), 0755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(p, []byte(content), 0644); err != nil { t.Fatal(err) }
}

// newTestRepo builds a repository whose history exercises the cases the
// resolver has to get right: entries that were not touched for a while, a
// rename, a change that only becomes reachable through a merge, a directory
// name that looks like a glob, and file names with "hostile" characters.
func newTestRepo(t *testing.T) (*LocalGitRepository, string) {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main", ".")

	// c1
	writeFile(t, dir, "a.txt", "1")
	writeFile(t, dir, "中文 名.txt", "1")
	writeFile(t, dir, "weird\nname.txt", "1")
	writeFile(t, dir, "st*r/inner.txt", "1")
	writeFile(t, dir, "sub/b.txt", "1")
	writeFile(t, dir, "sub/deep/c.txt", "1")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", "c1")

	// c2: only touches sub/b.txt.
	writeFile(t, dir, "sub/b.txt", "2")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", "c2")

	// c3: adds a new root entry.
	writeFile(t, dir, "top.txt", "1")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", "c3")

	// c4: renames a.txt away.
	gitRun(t, dir, "mv", "a.txt", "d.txt")
	gitRun(t, dir, "commit", "-qm", "c4")

	// c5/c6: a change that only becomes reachable through a merge commit.
	gitRun(t, dir, "checkout", "-q", "-b", "side")
	writeFile(t, dir, "side/only.txt", "1")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", "c5")
	gitRun(t, dir, "checkout", "-q", "main")
	writeFile(t, dir, "main-only.txt", "1")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", "c6")
	gitRun(t, dir, "merge", "-q", "--no-ff", "-m", "merge side", "side")

	// The application points LocalGitRepository at the git directory itself; for
	// a non-bare repository that is <worktree>/.git.
	return NewLocalGitRepository(path.Join(dir, ".git")), dir
}

func headCommit(t *testing.T, gr *LocalGitRepository, dir string) *CommitObject {
	t.Helper()
	o, err := gr.ReadObject(gitRun(t, dir, "rev-parse", "HEAD"))
	if err != nil { t.Fatal(err) }
	cobj, ok := o.(*CommitObject)
	if !ok { t.Fatal("HEAD is not a commit") }
	return cobj
}

func TestResolveLastCommitAtDir(t *testing.T) {
	gr, dir := newTestRepo(t)
	cobj := headCommit(t, gr, dir)

	// The oracle is the old approach - one `git rev-list -1` per entry - which
	// is slow but unambiguous. The single walk has to agree with it on every
	// entry of every directory.
	for _, dirPath := range []string{"", "sub", "sub/deep", "st*r"} {
		got, err := gr.ResolveLastCommitAtDir(cobj, dirPath)
		if err != nil { t.Fatalf("%q: %s", dirPath, err.Error()) }
		names, err := gr.lastCommitChildren(cobj, dirPath)
		if err != nil { t.Fatal(err) }
		if len(got) != len(names) {
			t.Fatalf("%q: resolved %d entries, the directory has %d (%v)", dirPath, len(got), len(names), got)
		}
		prefix := ""
		if dirPath != "" { prefix = dirPath + "/" }
		for _, name := range names {
			want := gitRun(t, dir, "rev-list", "-1", cobj.Id, "--", prefix+name)
			if got[name] != want {
				t.Fatalf("%q: entry %q resolves to %s, git says %s", dirPath, name, got[name], want)
			}
		}
	}

	// A change that only arrives through a merge commit belongs to the side
	// commit, not to the merge itself.
	root, err := gr.ResolveLastCommitAtDir(cobj, "")
	if err != nil { t.Fatal(err) }
	if root["side"] != gitRun(t, dir, "rev-list", "-1", cobj.Id, "--", "side/only.txt") {
		t.Fatal("a change merged in from a side branch is attributed to the wrong commit")
	}
}

func TestResolveLastCommitAtDirCache(t *testing.T) {
	ClearLastCommitCache()
	defer ClearLastCommitCache()
	gr, dir := newTestRepo(t)
	cobj := headCommit(t, gr, dir)

	first, err := gr.ResolveLastCommitAtDir(cobj, "")
	if err != nil { t.Fatal(err) }
	if len(first) <= 0 { t.Fatal("empty result") }

	// The returned map must not be the cached one: a caller that mutates it
	// must not corrupt the cache.
	first["sub"] = "definitely-not-a-commit"
	second, err := gr.ResolveLastCommitAtDir(cobj, "")
	if err != nil { t.Fatal(err) }
	if second["sub"] == "definitely-not-a-commit" {
		t.Fatal("the cache handed out its own map to the caller")
	}
	if second["sub"] != gitRun(t, dir, "rev-list", "-1", cobj.Id, "--", "sub/b.txt") {
		t.Fatal("cached value is wrong")
	}

	// Concurrent callers of a cold entry must all get the same answer.
	ClearLastCommitCache()
	var wg sync.WaitGroup
	results := make([]map[string]string, 8)
	errs := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = gr.ResolveLastCommitAtDir(cobj, "sub")
		}(i)
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil { t.Fatal(errs[i]) }
		if len(results[i]) != 2 {
			t.Fatalf("unexpected result for sub: %v", results[i])
		}
	}
}

func TestResolveLastCommitAtDirStore(t *testing.T) {
	ClearLastCommitCache()
	defer ClearLastCommitCache()
	gr, dir := newTestRepo(t)
	cobj := headCommit(t, gr, dir)

	want, err := gr.ResolveLastCommitAtDir(cobj, "")
	if err != nil { t.Fatal(err) }

	storePath := gr.lastCommitStorePath(cobj.Id, "")
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("no store file at %s: %s", storePath, err.Error())
	}

	// After dropping the in-memory cache the listing has to come from disk.
	// Hiding git makes sure it is not simply recomputed.
	ClearLastCommitCache()
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer t.Setenv("PATH", oldPath)
	got, err := gr.ResolveLastCommitAtDir(cobj, "")
	if err != nil {
		t.Fatalf("the listing was not read from disk: %s", err.Error())
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v { t.Fatalf("entry %q: got %s, want %s", k, got[k], v) }
	}
	t.Setenv("PATH", oldPath)

	// A file that does not belong to the lookup, or is not readable as one, is
	// ignored instead of being used.
	os.WriteFile(storePath, []byte("{not json"), 0644)
	ClearLastCommitCache()
	got, err = gr.ResolveLastCommitAtDir(cobj, "")
	if err != nil { t.Fatal(err) }
	if len(got) != len(want) { t.Fatalf("corrupt store file was used: %v", got) }

	// An id that cannot be a commit id must not be turned into a file name.
	if _, ok := gr.loadLastCommitFromDisk("../../escape", ""); ok {
		t.Fatal("a bogus id was accepted")
	}
	gr.saveLastCommitToDisk("../../escape", "", map[string]string{"a": "b"})

	// ClearLastCommitStore drops both halves.
	if err = gr.ClearLastCommitStore(); err != nil { t.Fatal(err) }
	if _, err = os.Stat(storePath); !os.IsNotExist(err) {
		t.Fatal("store file survived ClearLastCommitStore")
	}
}

func TestPruneLastCommitStore(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 10; i++ {
		p := path.Join(dir, "f"+strconv.Itoa(i)+".json")
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil { t.Fatal(err) }
		when := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(p, when, when); err != nil { t.Fatal(err) }
	}
	pruneLastCommitStore(dir, 4)
	entries, err := os.ReadDir(dir)
	if err != nil { t.Fatal(err) }
	if len(entries) != 4 { t.Fatalf("kept %d files, want 4", len(entries)) }
	// the four newest survive
	for i := 6; i < 10; i++ {
		if _, err := os.Stat(path.Join(dir, "f"+strconv.Itoa(i)+".json")); err != nil {
			t.Fatalf("f%d should have survived: %s", i, err.Error())
		}
	}
}
