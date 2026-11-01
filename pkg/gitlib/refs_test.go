package gitlib

import (
	"os"
	"path"
	"strings"
	"testing"
)

func refsTestRepo(t *testing.T) (*LocalGitRepository, string) {
	t.Helper()
	dir := t.TempDir()
	work := path.Join(dir, "work")
	if err := os.MkdirAll(work, 0755); err != nil { t.Fatal(err) }
	runGit(t, work, "init", "-q", "-b", "master")
	runGit(t, work, "config", "user.email", "a@example.com")
	runGit(t, work, "config", "user.name", "A")
	commitFile(t, work, "a.txt", "one")
	commitFile(t, work, "b.txt", "two")

	bare := path.Join(dir, "repo.git")
	runGit(t, dir, "clone", "-q", "--bare", work, bare)
	return NewLocalGitRepository(bare), bare
}

func TestHeadBranch(t *testing.T) {
	gr, bare := refsTestRepo(t)
	head, err := gr.HeadBranch()
	if err != nil { t.Fatal(err) }
	if head != "master" { t.Fatalf("HeadBranch = %q", head) }
	// a detached HEAD has no branch.
	runGit(t, bare, "symbolic-ref", "HEAD", "refs/heads/master")
	if err := os.WriteFile(path.Join(bare, "HEAD"), []byte(strings.Repeat("a", 40)+"\n"), 0644); err != nil { t.Fatal(err) }
	head, err = gr.HeadBranch()
	if err != nil { t.Fatal(err) }
	if head != "" { t.Fatalf("detached HeadBranch = %q", head) }
}

func TestCreateBranch(t *testing.T) {
	gr, bare := refsTestRepo(t)
	if err := gr.CreateBranch("feature", "master"); err != nil { t.Fatal(err) }
	if got := strings.TrimSpace(runGit(t, bare, "rev-parse", "refs/heads/feature")); got != strings.TrimSpace(runGit(t, bare, "rev-parse", "master")) {
		t.Fatalf("feature = %s", got)
	}
	// creating it twice is an error and must not move the branch.
	runGit(t, bare, "update-ref", "refs/heads/feature", "master~1")
	before := strings.TrimSpace(runGit(t, bare, "rev-parse", "refs/heads/feature"))
	if err := gr.CreateBranch("feature", "master"); err == nil {
		t.Fatal("existing branch was overwritten")
	}
	if got := strings.TrimSpace(runGit(t, bare, "rev-parse", "refs/heads/feature")); got != before {
		t.Errorf("branch moved despite the error: %s -> %s", before, got)
	}
}

func TestCreateBranchFromTagAndCommit(t *testing.T) {
	gr, bare := refsTestRepo(t)
	older := strings.TrimSpace(runGit(t, bare, "rev-parse", "master~1"))
	runGit(t, bare, "-c", "user.name=A", "-c", "user.email=a@example.com", "tag", "-a", "-m", "release", "v1.0", older)
	if err := gr.CreateBranch("from-tag", "v1.0"); err != nil { t.Fatal(err) }
	if got := strings.TrimSpace(runGit(t, bare, "rev-parse", "refs/heads/from-tag")); got != older {
		t.Errorf("branch from an annotated tag = %s; want %s", got, older)
	}
	if err := gr.CreateBranch("from-commit", older[:12]); err != nil { t.Fatal(err) }
	if got := strings.TrimSpace(runGit(t, bare, "rev-parse", "refs/heads/from-commit")); got != older {
		t.Errorf("branch from a commit = %s; want %s", got, older)
	}
}

func TestCreateRefRejectsBadInput(t *testing.T) {
	gr, bare := refsTestRepo(t)
	// "a/b" is a valid ref name for git (a hierarchical branch); whether the
	// web interface accepts it is a separate decision.
	for _, name := range []string{"foo..bar", "foo bar", "-x", "", "x.lock"} {
		if err := gr.CreateBranch(name, "master"); err == nil {
			t.Errorf("branch %q was accepted", name)
		}
		if err := gr.CreateTag(name, "master"); err == nil {
			t.Errorf("tag %q was accepted", name)
		}
	}
	if err := gr.CreateBranch("nope", "does-not-exist"); err == nil {
		t.Error("branch from an unknown revision was created")
	}
	if got := strings.TrimSpace(runGit(t, bare, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/tags")); got != "refs/heads/master" {
		t.Errorf("refs after the rejected operations = %q", got)
	}
}

func TestDeleteBranch(t *testing.T) {
	gr, bare := refsTestRepo(t)
	runGit(t, bare, "branch", "tmp", "master~1")
	if err := gr.DeleteBranch("tmp"); err != nil { t.Fatal(err) }
	if out := runGit(t, bare, "for-each-ref", "--format=%(refname)", "refs/heads"); strings.Contains(out, "refs/heads/tmp") {
		t.Error("branch still exists after delete")
	}
	if err := gr.DeleteBranch("tmp"); err == nil {
		t.Error("deleting a missing branch succeeded")
	}
	if err := gr.DeleteBranch("master"); err == nil {
		t.Fatal("the HEAD branch was deleted")
	}
	if got := strings.TrimSpace(runGit(t, bare, "rev-parse", "refs/heads/master")); len(got) != 40 {
		t.Errorf("master after the refused delete = %q", got)
	}
}

func TestCreateAndDeleteTag(t *testing.T) {
	gr, bare := refsTestRepo(t)
	if err := gr.CreateTag("v0.1", "master~1"); err != nil { t.Fatal(err) }
	if got := strings.TrimSpace(runGit(t, bare, "rev-parse", "refs/tags/v0.1")); got != strings.TrimSpace(runGit(t, bare, "rev-parse", "master~1")) {
		t.Errorf("tag = %s", got)
	}
	if err := gr.CreateTag("v0.1", "master"); err == nil {
		t.Error("existing tag was overwritten")
	}
	if err := gr.DeleteTag("v0.1"); err != nil { t.Fatal(err) }
	if out := runGit(t, bare, "for-each-ref", "--format=%(refname)", "refs/tags"); strings.Contains(out, "v0.1") {
		t.Error("tag still exists after delete")
	}
	if err := gr.DeleteTag("v0.1"); err == nil {
		t.Error("deleting a missing tag succeeded")
	}
}
