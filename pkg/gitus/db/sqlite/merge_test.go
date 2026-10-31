package sqlite

import (
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
)

func mergeTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil { t.Fatalf("git %v in %s: %s: %s", args, dir, err, out) }
	return string(out)
}

func mergeTestWrite(t *testing.T, dir string, name string, content string) {
	t.Helper()
	if err := os.WriteFile(path.Join(dir, name), []byte(content), 0644); err != nil { t.Fatal(err) }
}

// setupMergeFixture builds a throw-away sqlite instance with one user, a bare
// receiver repository and a bare provider (fork) repository registered as a
// remote of the receiver, plus an open pull request from the fork.
func setupMergeFixture(t *testing.T, title string) (*SqliteGitusDatabaseInterface, string, string, int64) {
	t.Helper()
	dir := t.TempDir()
	gitRoot := path.Join(dir, "repos")
	if err := os.MkdirAll(gitRoot, 0755); err != nil { t.Fatal(err) }
	cfgJSON, _ := json.Marshal(map[string]any{
		"version": 0, "root": gitRoot, "gitUser": "tester", "hostName": "localhost",
		"bindAddress": "127.0.0.1", "bindPort": 8000,
		"database": map[string]any{
			"type": "sqlite", "path": path.Join(dir, "gitus.db"), "tablePrefix": "gitus",
		},
		"session":       map[string]any{"type": "sqlite", "path": path.Join(dir, "s.db"), "tablePrefix": "s_"},
		"receiptSystem": map[string]any{"type": "sqlite", "path": path.Join(dir, "r.db"), "tablePrefix": "r_"},
		"snippetRoot":   path.Join(dir, "snippets"),
		"frontPage":     map[string]any{"path": "/index.md"},
	})
	cfgPath := path.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, cfgJSON, 0644); err != nil { t.Fatal(err) }
	cfg, err := gitus.LoadConfigFile(cfgPath)
	if err != nil { t.Fatal(err) }

	dbif, err := NewSqliteGitusDatabaseInterface(cfg)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { dbif.Dispose() })
	if err = dbif.InstallTables(); err != nil { t.Fatalf("InstallTables: %s", err) }
	if _, err = dbif.RegisterUser("alice", "alice@example.com", "hash", model.NORMAL_USER); err != nil {
		t.Fatalf("RegisterUser: %s", err)
	}

	// one commit, then a bare receiver and a bare fork that both start there.
	work := path.Join(dir, "work")
	if err = os.MkdirAll(work, 0755); err != nil { t.Fatal(err) }
	mergeTestGit(t, work, "init", "-q", "-b", "master")
	mergeTestGit(t, work, "config", "user.email", "a@example.com")
	mergeTestGit(t, work, "config", "user.name", "A")
	mergeTestWrite(t, work, "a.txt", "one")
	mergeTestGit(t, work, "add", "a.txt")
	mergeTestGit(t, work, "commit", "-q", "-m", "initial")

	receiverPath := path.Join(gitRoot, "acme", "hello")
	forkPath := path.Join(gitRoot, "bob", "hello")
	if err = os.MkdirAll(path.Dir(receiverPath), 0755); err != nil { t.Fatal(err) }
	if err = os.MkdirAll(path.Dir(forkPath), 0755); err != nil { t.Fatal(err) }
	mergeTestGit(t, dir, "clone", "-q", "--bare", work, receiverPath)
	mergeTestGit(t, dir, "clone", "-q", "--bare", work, forkPath)
	mergeTestGit(t, receiverPath, "remote", "add", "bob/hello", forkPath)

	// one commit on the fork.
	forkWork := path.Join(dir, "fork-work")
	mergeTestGit(t, dir, "clone", "-q", forkPath, forkWork)
	mergeTestGit(t, forkWork, "config", "user.email", "b@example.com")
	mergeTestGit(t, forkWork, "config", "user.name", "B")
	mergeTestWrite(t, forkWork, "b.txt", "fork")
	mergeTestGit(t, forkWork, "add", "b.txt")
	mergeTestGit(t, forkWork, "commit", "-q", "-m", "fork: b.txt")
	mergeTestGit(t, forkWork, "push", "-q", "origin", "master")

	absId, err := dbif.NewPullRequest("alice", title, "acme", "hello", "master", "bob", "hello", "master")
	if err != nil { t.Fatalf("NewPullRequest: %s", err) }
	return dbif, receiverPath, forkPath, absId
}

func TestCheckAndMergePullRequestMethods(t *testing.T) {
	cases := []struct {
		method       string
		wantParents  int
		wantCount    string
		wantSubject  string
		wantAuthor   string
	}{
		{gitlib.MERGE_METHOD_MERGE, 2, "3", "merge: from bob/hello/master to master", "alice <alice@example.com>"},
		{gitlib.MERGE_METHOD_SQUASH, 1, "2", "Squash this pull request", "alice <alice@example.com>"},
		{gitlib.MERGE_METHOD_REBASE, 1, "2", "fork: b.txt", "B <b@example.com>"},
	}
	for _, c := range cases {
		t.Run(c.method, func(t *testing.T) {
			dbif, receiverPath, _, absId := setupMergeFixture(t, "Squash this pull request")
			if _, err := dbif.CheckPullRequestMergeConflict(absId); err != nil { t.Fatal(err) }
			if err := dbif.CheckAndMergePullRequest(absId, "alice", c.method); err != nil {
				t.Fatalf("CheckAndMergePullRequest: %s", err)
			}
			parents := strings.Fields(mergeTestGit(t, receiverPath, "log", "-1", "--format=%P", "master"))
			if len(parents) != c.wantParents { t.Errorf("parents = %v", parents) }
			if got := strings.TrimSpace(mergeTestGit(t, receiverPath, "rev-list", "--count", "master")); got != c.wantCount {
				t.Errorf("history length = %s; want %s", got, c.wantCount)
			}
			if got := strings.TrimSpace(mergeTestGit(t, receiverPath, "log", "-1", "--format=%s", "master")); got != c.wantSubject {
				t.Errorf("subject = %q; want %q", got, c.wantSubject)
			}
			// the merged tree contains the fork's file in every case.
			mergeTestGit(t, receiverPath, "cat-file", "-p", "master:b.txt")
			// the fork's own commit stays the author in a rebase.
			if got := strings.TrimSpace(mergeTestGit(t, receiverPath, "log", "-1", "--format=%an <%ae>", "master")); !strings.Contains(got, c.wantAuthor) {
				t.Errorf("author = %q; want to contain %q", got, c.wantAuthor)
			}

			pr, err := dbif.GetPullRequestByAbsId(absId)
			if err != nil { t.Fatal(err) }
			if pr.Status != model.PULL_REQUEST_CLOSED_AS_MERGED { t.Errorf("status = %d", pr.Status) }
			events, err := dbif.GetAllPullRequestEventPaginated(absId, 0, 10)
			if err != nil { t.Fatal(err) }
			found := false
			for _, e := range events {
				if e.EventType == model.PULL_REQUEST_EVENT_CLOSE_AS_MERGED {
					found = true
					if e.EventContent != c.method { t.Errorf("event content = %q; want %q", e.EventContent, c.method) }
				}
			}
			if !found { t.Error("no close-as-merged event") }
		})
	}
}

func TestCheckAndMergePullRequestRejectsConflict(t *testing.T) {
	dbif, receiverPath, forkPath, absId := setupMergeFixture(t, "conflicting")
	before := strings.TrimSpace(mergeTestGit(t, receiverPath, "rev-parse", "master"))

	// make both sides change a.txt so the merge conflicts.
	recvWork := path.Join(t.TempDir(), "recv-work")
	mergeTestGit(t, t.TempDir(), "clone", "-q", receiverPath, recvWork)
	mergeTestGit(t, recvWork, "config", "user.email", "a@example.com")
	mergeTestGit(t, recvWork, "config", "user.name", "A")
	mergeTestWrite(t, recvWork, "a.txt", "upstream")
	mergeTestGit(t, recvWork, "commit", "-q", "-am", "upstream: a.txt")
	mergeTestGit(t, recvWork, "push", "-q", "origin", "master")
	before = strings.TrimSpace(mergeTestGit(t, receiverPath, "rev-parse", "master"))

	forkWork := path.Join(t.TempDir(), "fork-work")
	mergeTestGit(t, t.TempDir(), "clone", "-q", forkPath, forkWork)
	mergeTestGit(t, forkWork, "config", "user.email", "b@example.com")
	mergeTestGit(t, forkWork, "config", "user.name", "B")
	mergeTestWrite(t, forkWork, "a.txt", "fork")
	mergeTestGit(t, forkWork, "commit", "-q", "-am", "fork: a.txt")
	mergeTestGit(t, forkWork, "push", "-q", "origin", "master")

	if err := dbif.CheckAndMergePullRequest(absId, "alice", gitlib.MERGE_METHOD_MERGE); err != gitlib.ErrMergeConflict {
		t.Fatalf("merge of a conflicting pull request = %v", err)
	}
	if got := strings.TrimSpace(mergeTestGit(t, receiverPath, "rev-parse", "master")); got != before {
		t.Errorf("branch moved despite the conflict: %s -> %s", before, got)
	}
	pr, err := dbif.GetPullRequestByAbsId(absId)
	if err != nil { t.Fatal(err) }
	if pr.Status != model.PULL_REQUEST_OPEN { t.Errorf("status = %d; want open", pr.Status) }
}

func TestCheckAndMergePullRequestRejectsUnknownMethod(t *testing.T) {
	dbif, _, _, absId := setupMergeFixture(t, "octopus")
	if err := dbif.CheckAndMergePullRequest(absId, "alice", "octopus"); err == nil {
		t.Fatal("unknown merge method accepted")
	}
}
