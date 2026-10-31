package gitlib

import (
	"os"
	"os/exec"
	"path"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil { t.Fatalf("git %v in %s: %s: %s", args, dir, err, out) }
	return string(out)
}

func commitFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(path.Join(dir, name), []byte(content), 0644); err != nil { t.Fatal(err) }
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-q", "-m", "add "+name)
}

// TestCompareBranchWithRemoteDoesNotFetch checks the P13 property: comparing a
// fork against its upstream uses the remote-tracking ref as-is. A new commit on
// the fork is only visible after the ref is refreshed explicitly (which the
// update hook does on a push).
func TestCompareBranchWithRemoteDoesNotFetch(t *testing.T) {
	dir := t.TempDir()
	work := path.Join(dir, "work")
	os.MkdirAll(work, 0755)
	runGit(t, work, "init", "-q", "-b", "master")
	runGit(t, work, "config", "user.email", "a@example.com")
	runGit(t, work, "config", "user.name", "A")
	commitFile(t, work, "a.txt", "one")

	upstreamPath := path.Join(dir, "upstream")
	forkPath := path.Join(dir, "fork")
	runGit(t, dir, "clone", "-q", "--bare", work, upstreamPath)
	runGit(t, dir, "clone", "-q", "--bare", work, forkPath)

	upstream := NewLocalGitRepository(upstreamPath)
	runGit(t, upstreamPath, "remote", "add", "ns/fork", forkPath)

	// No remote-tracking ref yet: there is nothing to compare and the method
	// must not fetch to create one.
	if info, err := upstream.CompareBranchWithRemote("master", "ns/fork"); err != nil || info != nil {
		t.Fatalf("comparison without a fetched ref = %+v, %v", info, err)
	}

	if err := upstream.FetchRemoteBranch("ns/fork", "master"); err != nil { t.Fatal(err) }
	info, err := upstream.CompareBranchWithRemote("master", "ns/fork")
	if err != nil { t.Fatal(err) }
	if info == nil || len(info.ARevList) != 0 || len(info.BRevList) != 0 {
		t.Fatalf("initial comparison = %+v", info)
	}

	// A new commit in the fork.
	work2 := path.Join(dir, "work2")
	runGit(t, dir, "clone", "-q", forkPath, work2)
	runGit(t, work2, "config", "user.email", "a@example.com")
	runGit(t, work2, "config", "user.name", "A")
	commitFile(t, work2, "b.txt", "two")
	runGit(t, work2, "push", "-q", "origin", "master")

	// The page view must still see the old remote-tracking ref.
	info, err = upstream.CompareBranchWithRemote("master", "ns/fork")
	if err != nil { t.Fatal(err) }
	if info == nil || len(info.BRevList) != 0 {
		t.Fatalf("comparison fetched behind our back: %+v", info)
	}

	// The hook-side refresh makes the new commit visible.
	if err = upstream.FetchRemoteBranch("ns/fork", "master"); err != nil { t.Fatal(err) }
	info, err = upstream.CompareBranchWithRemote("master", "ns/fork")
	if err != nil { t.Fatal(err) }
	if info == nil || len(info.BRevList) != 1 {
		t.Fatalf("comparison after refresh = %+v", info)
	}
}

func TestLocalForkToPopulatesComparison(t *testing.T) {
	dir := t.TempDir()
	work := path.Join(dir, "work")
	os.MkdirAll(work, 0755)
	runGit(t, work, "init", "-q", "-b", "master")
	runGit(t, work, "config", "user.email", "a@example.com")
	runGit(t, work, "config", "user.name", "A")
	commitFile(t, work, "a.txt", "one")
	upstreamPath := path.Join(dir, "upstream")
	runGit(t, dir, "clone", "-q", "--bare", work, upstreamPath)
	upstream := NewLocalGitRepository(upstreamPath)
	forkPath := path.Join(dir, "fork")
	if err := upstream.LocalForkTo("ns/fork", forkPath); err != nil { t.Fatal(err) }
	if info, err := upstream.CompareBranchWithRemote("master", "ns/fork"); err != nil || info == nil {
		t.Fatalf("LocalForkTo did not populate the comparison ref: %+v, %v", info, err)
	}
}
