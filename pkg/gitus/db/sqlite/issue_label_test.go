package sqlite

import (
	"encoding/json"
	"os"
	"path"
	"slices"
	"testing"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
)

func issueTestDB(t *testing.T) *SqliteGitusDatabaseInterface {
	t.Helper()
	dir := t.TempDir()
	cfgJSON, _ := json.Marshal(map[string]any{
		"version": 0, "root": path.Join(dir, "repos"), "gitUser": "tester",
		"hostName": "localhost", "bindAddress": "127.0.0.1", "bindPort": 8000,
		"database": map[string]any{
			"type": "sqlite", "path": path.Join(dir, "gitus.db"), "tablePrefix": "gitus",
		},
		"session": map[string]any{
			"type": "sqlite", "path": path.Join(dir, "session.db"), "tablePrefix": "s_",
		},
		"receiptSystem": map[string]any{
			"type": "sqlite", "path": path.Join(dir, "receipt.db"), "tablePrefix": "r_",
		},
	})
	cfgPath := path.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, cfgJSON, 0644); err != nil { t.Fatal(err) }
	cfg, err := gitus.LoadConfigFile(cfgPath)
	if err != nil { t.Fatal(err) }
	dbif, err := NewSqliteGitusDatabaseInterface(cfg)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { dbif.Dispose() })
	if err = dbif.InstallTables(); err != nil { t.Fatal(err) }
	if _, err = dbif.RegisterUser("alice", "alice@example.com", "hash", model.NORMAL_USER); err != nil { t.Fatal(err) }
	if _, err = dbif.RegisterNamespace("acme", "alice"); err != nil { t.Fatal(err) }
	if _, err = dbif.CreateRepository("acme", "proj", model.REPO_TYPE_GIT, "alice"); err != nil { t.Fatal(err) }
	return dbif
}

func TestRepositoryIssueLabels(t *testing.T) {
	dbif := issueTestDB(t)
	// the repository's own labels and the issue labels are two vocabularies.
	if err := dbif.AddRepositoryLabel("acme", "proj", "xorg"); err != nil { t.Fatal(err) }
	if err := dbif.AddRepositoryIssueLabel("acme", "proj", "bug"); err != nil { t.Fatal(err) }
	if err := dbif.AddRepositoryIssueLabel("acme", "proj", "feature request"); err != nil { t.Fatal(err) }
	// adding the same one twice must not duplicate it.
	if err := dbif.AddRepositoryIssueLabel("acme", "proj", "bug"); err != nil { t.Fatal(err) }

	issueLabels, err := dbif.GetRepositoryIssueLabel("acme", "proj")
	if err != nil { t.Fatal(err) }
	if !slices.Equal(issueLabels, []string{"bug", "feature request"}) {
		t.Fatalf("issue labels = %v", issueLabels)
	}
	repoLabels, err := dbif.GetRepositoryLabel("acme", "proj")
	if err != nil { t.Fatal(err) }
	if !slices.Equal(repoLabels, []string{"xorg"}) {
		t.Fatalf("repository labels = %v", repoLabels)
	}
	repo, err := dbif.GetRepositoryByName("acme", "proj")
	if err != nil { t.Fatal(err) }
	if !slices.Equal(repo.RepoLabelList, []string{"xorg"}) {
		t.Fatalf("repository model labels = %v", repo.RepoLabelList)
	}

	if err = dbif.RemoveRepositoryIssueLabel("acme", "proj", "bug"); err != nil { t.Fatal(err) }
	if err = dbif.RemoveRepositoryIssueLabel("acme", "proj", "nope"); err != nil { t.Fatal(err) }
	issueLabels, err = dbif.GetRepositoryIssueLabel("acme", "proj")
	if err != nil { t.Fatal(err) }
	if !slices.Equal(issueLabels, []string{"feature request"}) {
		t.Fatalf("issue labels after remove = %v", issueLabels)
	}
	if repoLabels, err = dbif.GetRepositoryLabel("acme", "proj"); err != nil || !slices.Equal(repoLabels, []string{"xorg"}) {
		t.Fatalf("repository labels changed: %v %v", repoLabels, err)
	}
}

func TestIssueLabels(t *testing.T) {
	dbif := issueTestDB(t)
	iid, err := dbif.NewRepositoryIssue("acme", "proj", "alice", "first issue", "body")
	if err != nil { t.Fatal(err) }

	issue, err := dbif.GetRepositoryIssue("acme", "proj", int(iid))
	if err != nil { t.Fatal(err) }
	if len(issue.LabelList) != 0 { t.Fatalf("new issue labels = %v", issue.LabelList) }

	if err = dbif.AddIssueLabel("acme", "proj", iid, "bug"); err != nil { t.Fatal(err) }
	if err = dbif.AddIssueLabel("acme", "proj", iid, "ui"); err != nil { t.Fatal(err) }
	// adding the same label twice must not duplicate it.
	if err = dbif.AddIssueLabel("acme", "proj", iid, "bug"); err != nil { t.Fatal(err) }

	issue, err = dbif.GetRepositoryIssue("acme", "proj", int(iid))
	if err != nil { t.Fatal(err) }
	if !slices.Equal(issue.LabelList, []string{"bug", "ui"}) {
		t.Fatalf("labels = %v", issue.LabelList)
	}

	// the labels also come back with the listing.
	list, err := dbif.SearchIssuePaginated("", "acme", "proj", 0, 0, 10)
	if err != nil { t.Fatal(err) }
	if len(list) != 1 || !slices.Equal(list[0].LabelList, []string{"bug", "ui"}) {
		t.Fatalf("listed issues = %+v", list)
	}
	if err = dbif.RemoveIssueLabel("acme", "proj", iid, "bug"); err != nil { t.Fatal(err) }
	if err = dbif.RemoveIssueLabel("acme", "proj", iid, "nope"); err != nil { t.Fatal(err) }
	issue, err = dbif.GetRepositoryIssue("acme", "proj", int(iid))
	if err != nil { t.Fatal(err) }
	if !slices.Equal(issue.LabelList, []string{"ui"}) {
		t.Fatalf("labels after remove = %v", issue.LabelList)
	}
	if err = dbif.RemoveIssueLabel("acme", "proj", iid, "ui"); err != nil { t.Fatal(err) }
	issue, err = dbif.GetRepositoryIssue("acme", "proj", int(iid))
	if err != nil { t.Fatal(err) }
	if len(issue.LabelList) != 0 { t.Fatalf("labels after removing all = %v", issue.LabelList) }
}

func TestIssueLabelEnsureSchemaMigratesOldTable(t *testing.T) {
	dbif := issueTestDB(t)
	// An instance installed before the issue_label column existed: drop the
	// column by rebuilding the table without it, the way SQLite requires.
	_, err := dbif.connection.Exec(`CREATE TABLE gitus_issue_old AS
		SELECT repo_namespace, repo_name, issue_id, issue_timestamp, issue_author,
			issue_title, issue_content, issue_status, issue_priority FROM gitus_issue`)
	if err != nil { t.Fatal(err) }
	if _, err = dbif.connection.Exec(`DROP TABLE gitus_issue`); err != nil { t.Fatal(err) }
	if _, err = dbif.connection.Exec(`CREATE TABLE gitus_issue (
		repo_namespace TEXT, repo_name TEXT, issue_id INTEGER, issue_timestamp INTEGER,
		issue_author TEXT, issue_title TEXT, issue_content TEXT,
		issue_status INTEGER, issue_priority SMALLINT)`); err != nil { t.Fatal(err) }
	if _, err = dbif.connection.Exec(`INSERT INTO gitus_issue(repo_namespace, repo_name, issue_id,
		issue_timestamp, issue_author, issue_title, issue_content, issue_status, issue_priority)
		VALUES ('acme', 'proj', 1, 100, 'alice', 'old issue', 'body', 1, 0)`); err != nil { t.Fatal(err) }

	if err = dbif.EnsureSchema(); err != nil { t.Fatal(err) }
	// Running it again must not fail on the column that now exists.
	if err = dbif.EnsureSchema(); err != nil { t.Fatal(err) }

	issue, err := dbif.GetRepositoryIssue("acme", "proj", 1)
	if err != nil { t.Fatalf("old issue is unreadable after the migration: %s", err) }
	if issue.IssueTitle != "old issue" { t.Fatalf("issue = %+v", issue) }
	if len(issue.LabelList) != 0 { t.Fatalf("old issue labels = %v", issue.LabelList) }

	if err = dbif.AddIssueLabel("acme", "proj", 1, "bug"); err != nil { t.Fatal(err) }
	issue, err = dbif.GetRepositoryIssue("acme", "proj", 1)
	if err != nil { t.Fatal(err) }
	if !slices.Equal(issue.LabelList, []string{"bug"}) { t.Fatalf("labels after migration = %v", issue.LabelList) }

	// the issue label vocabulary column is added to an old repository table
	// as well.
	if _, err = dbif.connection.Exec(`ALTER TABLE gitus_repository DROP COLUMN repo_issue_label_list`); err != nil {
		t.Fatal(err)
	}
	if err = dbif.EnsureSchema(); err != nil { t.Fatal(err) }
	if err = dbif.AddRepositoryIssueLabel("acme", "proj", "bug"); err != nil { t.Fatal(err) }
	if l, err := dbif.GetRepositoryIssueLabel("acme", "proj"); err != nil || !slices.Equal(l, []string{"bug"}) {
		t.Fatalf("issue labels after the repository migration = %v %v", l, err)
	}
}
