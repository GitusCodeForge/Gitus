package gitlib

import (
	"os"
	"path"
	"strings"
	"testing"
)

// mergeTestRepos creates a bare "upstream" and a bare "fork" that both start
// from the same single commit, with the fork registered as a remote of the
// upstream (the same layout Gitus creates with LocalForkTo).
func mergeTestRepos(t *testing.T) (*LocalGitRepository, string, string) {
	t.Helper()
	dir := t.TempDir()
	work := path.Join(dir, "work")
	if err := os.MkdirAll(work, 0755); err != nil { t.Fatal(err) }
	runGit(t, work, "init", "-q", "-b", "master")
	runGit(t, work, "config", "user.email", "a@example.com")
	runGit(t, work, "config", "user.name", "A")
	commitFile(t, work, "a.txt", "one")

	upstreamPath := path.Join(dir, "upstream")
	forkPath := path.Join(dir, "fork")
	runGit(t, dir, "clone", "-q", "--bare", work, upstreamPath)
	runGit(t, dir, "clone", "-q", "--bare", work, forkPath)
	runGit(t, upstreamPath, "remote", "add", "ns/fork", forkPath)
	return NewLocalGitRepository(upstreamPath), upstreamPath, forkPath
}

// mergeTestForkCommit adds one commit to the fork branch.
func mergeTestForkCommit(t *testing.T, forkPath string, name string, content string) {
	t.Helper()
	dir := t.TempDir()
	work := path.Join(dir, "work")
	runGit(t, dir, "clone", "-q", forkPath, work)
	runGit(t, work, "config", "user.email", "f@example.com")
	runGit(t, work, "config", "user.name", "F")
	if err := os.WriteFile(path.Join(work, name), []byte(content), 0644); err != nil { t.Fatal(err) }
	runGit(t, work, "add", name)
	runGit(t, work, "commit", "-q", "-m", "fork: "+name)
	runGit(t, work, "push", "-q", "origin", "master")
}

func TestMergeMethodValidation(t *testing.T) {
	for _, m := range MergeMethods {
		if !ValidMergeMethod(m) { t.Errorf("%q rejected", m) }
	}
	if ValidMergeMethod("octopus") { t.Error("unknown method accepted") }
}

func TestMergeCreatesMergeCommit(t *testing.T) {
	upstream, upstreamPath, forkPath := mergeTestRepos(t)
	mergeTestForkCommit(t, forkPath, "b.txt", "fork")
	if err := upstream.Merge("ns/fork", "master", "master", MERGE_METHOD_MERGE, "M", "m@example.com", ""); err != nil {
		t.Fatal(err)
	}
	parents := strings.Fields(runGit(t, upstreamPath, "log", "-1", "--format=%P", "master"))
	if len(parents) != 2 { t.Fatalf("merge commit parents = %v", parents) }
	// both sides are present in the merged tree.
	runGit(t, upstreamPath, "cat-file", "-p", "master:a.txt")
	runGit(t, upstreamPath, "cat-file", "-p", "master:b.txt")
	if got := strings.TrimSpace(runGit(t, upstreamPath, "log", "-1", "--format=%an <%ae>", "master")); got != "M <m@example.com>" {
		t.Errorf("merger identity = %q", got)
	}
}

func TestMergeSquashCreatesOneCommit(t *testing.T) {
	upstream, upstreamPath, forkPath := mergeTestRepos(t)
	before := strings.TrimSpace(runGit(t, upstreamPath, "rev-parse", "master"))
	mergeTestForkCommit(t, forkPath, "b.txt", "fork")
	const subject = "Squashed pull request"
	if err := upstream.Merge("ns/fork", "master", "master", MERGE_METHOD_SQUASH, "M", "m@example.com", subject); err != nil {
		t.Fatal(err)
	}
	parents := strings.Fields(runGit(t, upstreamPath, "log", "-1", "--format=%P", "master"))
	if len(parents) != 1 { t.Fatalf("squash commit parents = %v", parents) }
	if parents[0] != before { t.Errorf("squash parent = %s; want %s", parents[0], before) }
	if got := strings.TrimSpace(runGit(t, upstreamPath, "log", "-1", "--format=%s", "master")); got != subject {
		t.Errorf("squash subject = %q", got)
	}
	runGit(t, upstreamPath, "cat-file", "-p", "master:b.txt")
	// a squash must not pull the fork's history in.
	if got := strings.TrimSpace(runGit(t, upstreamPath, "rev-list", "--count", "master")); got != "2" {
		t.Errorf("squash history length = %s", got)
	}
}

func TestMergeRebaseReplaysForkCommits(t *testing.T) {
	upstream, upstreamPath, forkPath := mergeTestRepos(t)
	// a commit on the upstream side, so the replay is not a fast-forward.
	upstreamWork := t.TempDir()
	runGit(t, upstreamWork, "clone", "-q", upstreamPath, path.Join(upstreamWork, "work"))
	work := path.Join(upstreamWork, "work")
	runGit(t, work, "config", "user.email", "a@example.com")
	runGit(t, work, "config", "user.name", "A")
	if err := os.WriteFile(path.Join(work, "c.txt"), []byte("upstream"), 0644); err != nil { t.Fatal(err) }
	runGit(t, work, "add", "c.txt")
	runGit(t, work, "commit", "-q", "-m", "upstream: c.txt")
	runGit(t, work, "push", "-q", "origin", "master")

	mergeTestForkCommit(t, forkPath, "b.txt", "fork")
	if err := upstream.Merge("ns/fork", "master", "master", MERGE_METHOD_REBASE, "M", "m@example.com", ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(runGit(t, upstreamPath, "log", "-1", "--format=%P", "master")); len(got) != 1 {
		t.Fatalf("rebase head is not a single-parent commit: %v", got)
	}
	if got := strings.TrimSpace(runGit(t, upstreamPath, "rev-list", "--count", "master")); got != "3" {
		t.Errorf("rebase history length = %s; want 3", got)
	}
	if got := strings.TrimSpace(runGit(t, upstreamPath, "log", "-1", "--format=%an <%ae>", "master")); got != "F <f@example.com>" {
		t.Errorf("replayed author = %q", got)
	}
	runGit(t, upstreamPath, "cat-file", "-p", "master:b.txt")
	runGit(t, upstreamPath, "cat-file", "-p", "master:c.txt")
}

func TestMergeRebaseConflictLeavesBranchAlone(t *testing.T) {
	upstream, upstreamPath, forkPath := mergeTestRepos(t)
	before := strings.TrimSpace(runGit(t, upstreamPath, "rev-parse", "master"))

	upstreamWork := t.TempDir()
	runGit(t, upstreamWork, "clone", "-q", upstreamPath, path.Join(upstreamWork, "work"))
	work := path.Join(upstreamWork, "work")
	runGit(t, work, "config", "user.email", "a@example.com")
	runGit(t, work, "config", "user.name", "A")
	if err := os.WriteFile(path.Join(work, "a.txt"), []byte("upstream"), 0644); err != nil { t.Fatal(err) }
	runGit(t, work, "commit", "-q", "-am", "upstream: a.txt")
	runGit(t, work, "push", "-q", "origin", "master")
	after := strings.TrimSpace(runGit(t, upstreamPath, "rev-parse", "master"))
	if after == before { t.Fatal("upstream commit was not pushed") }

	dir := t.TempDir()
	work2 := path.Join(dir, "work")
	runGit(t, dir, "clone", "-q", forkPath, work2)
	runGit(t, work2, "config", "user.email", "f@example.com")
	runGit(t, work2, "config", "user.name", "F")
	if err := os.WriteFile(path.Join(work2, "a.txt"), []byte("fork"), 0644); err != nil { t.Fatal(err) }
	runGit(t, work2, "commit", "-q", "-am", "fork: a.txt")
	runGit(t, work2, "push", "-q", "origin", "master")

	err := upstream.Merge("ns/fork", "master", "master", MERGE_METHOD_REBASE, "M", "m@example.com", "")
	if err == nil { t.Fatal("rebase with a conflict succeeded") }
	if !strings.Contains(err.Error(), "cannot rebase commit") {
		t.Errorf("rebase error = %v", err)
	}
	if got := strings.TrimSpace(runGit(t, upstreamPath, "rev-parse", "master")); got != after {
		t.Errorf("branch moved despite the failed rebase: %s -> %s", after, got)
	}
}
