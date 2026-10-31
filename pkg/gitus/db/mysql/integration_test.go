package mysql

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	_ "github.com/go-sql-driver/mysql"
)

func loadTestConfig(t *testing.T) *gitus.GitusConfig {
	t.Helper()
	dir := t.TempDir()
	root := path.Join(dir, "repos")
	snip := path.Join(dir, "snippets")
	if err := os.MkdirAll(root, 0755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(snip, 0755); err != nil { t.Fatal(err) }
	cfgJSON, _ := json.Marshal(map[string]any{
		"version": 0, "root": root, "gitUser": "tester", "hostName": "localhost",
		"bindAddress": "127.0.0.1", "bindPort": 8000,
		"database": map[string]any{
			"type": "mysql", "url": "127.0.0.1:53307",
			"userName": "gitus", "password": "gitus", "databaseName": "gitus",
			"tablePrefix": "t_",
		},
		"session":       map[string]any{"type": "sqlite", "path": path.Join(dir, "s.db"), "tablePrefix": "s_"},
		"receiptSystem": map[string]any{"type": "sqlite", "path": path.Join(dir, "r.db"), "tablePrefix": "r_"},
		"snippetRoot":   snip,
		"frontPage":     map[string]any{"path": "/index.md"},
	})
	cfgPath := path.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, cfgJSON, 0644); err != nil { t.Fatal(err) }
	cfg, err := gitus.LoadConfigFile(cfgPath)
	if err != nil { t.Fatal(err) }
	return cfg
}

// TestMysqlBackend is an end-to-end exercise of the backend. It needs a real
// server, so it only runs when GITUS_MYSQL_TEST=1 is set; the defaults match a
// throw-away server on 127.0.0.1:53307 with database/user/password "gitus"
// (see loadTestConfig). It empties every t_-prefixed table first.
func TestMysqlBackend(t *testing.T) {
	if os.Getenv("GITUS_MYSQL_TEST") != "1" {
		t.Skip("set GITUS_MYSQL_TEST=1 and point the test at a disposable MySQL/MariaDB server to run this")
	}
	cfg := loadTestConfig(t)

	// Start from an empty prefixed schema.
	raw, err := sql.Open("mysql", "gitus:gitus@tcp(127.0.0.1:53307)/gitus?parseTime=true")
	if err != nil { t.Fatal(err) }
	defer raw.Close()
	rows, err := raw.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name LIKE 't!_%' ESCAPE '!'`)
	if err != nil { t.Fatal(err) }
	var tables []string
	for rows.Next() {
		var n string
		if err = rows.Scan(&n); err != nil { t.Fatal(err) }
		tables = append(tables, n)
	}
	rows.Close()
	if _, err = raw.Exec(`SET FOREIGN_KEY_CHECKS = 0`); err != nil { t.Fatal(err) }
	for _, n := range tables {
		if _, err = raw.Exec(fmt.Sprintf("DROP TABLE `%s`", n)); err != nil { t.Fatal(err) }
	}
	if _, err = raw.Exec(`SET FOREIGN_KEY_CHECKS = 1`); err != nil { t.Fatal(err) }

	dbif, err := NewMysqlGitusDatabaseInterface(cfg)
	if err != nil { t.Fatal(err) }
	defer dbif.Dispose()

	if err = dbif.InstallTables(); err != nil { t.Fatalf("InstallTables: %s", err) }
	if err = dbif.EnsureSchema(); err != nil { t.Fatalf("EnsureSchema: %s", err) }
	// Both must be idempotent.
	if err = dbif.InstallTables(); err != nil { t.Fatalf("second InstallTables: %s", err) }
	if err = dbif.EnsureSchema(); err != nil { t.Fatalf("second EnsureSchema: %s", err) }
	if ok, err := dbif.IsDatabaseUsable(); err != nil || !ok {
		t.Fatalf("IsDatabaseUsable = %v, %v", ok, err)
	}

	// ---- users ----
	for _, name := range []string{"alice", "bob"} {
		if _, err = dbif.RegisterUser(name, name+"@example.com", "hash", model.NORMAL_USER); err != nil {
			t.Fatalf("RegisterUser(%s): %s", name, err)
		}
	}
	alice, err := dbif.GetUserByName("alice")
	if err != nil { t.Fatal(err) }
	if alice.Title != "alice" { t.Fatalf("unexpected user: %+v", alice) }
	info, err := dbif.GetUserLoginInfo("alice")
	if err != nil { t.Fatal(err) }
	if info.Email != "alice@example.com" { t.Fatalf("unexpected login info: %+v", info) }
	if err = dbif.UpdateUserStatus("bob", model.ADMIN); err != nil { t.Fatal(err) }
	if bob, err := dbif.GetUserByName("bob"); err != nil || bob.Status != model.ADMIN {
		t.Fatalf("UpdateUserStatus: %+v %v", bob, err)
	}
	if _, err = dbif.GetUserByName("nobody"); err != db.ErrEntityNotFound {
		t.Fatalf("missing user: %v", err)
	}
	if _, err = dbif.RegisterUser("alice", "x@example.com", "h", model.NORMAL_USER); err == nil {
		t.Fatal("duplicate user accepted")
	}

	// ---- emails ----
	if err = dbif.AddEmail("alice", "alice+alt@example.com"); err != nil { t.Fatal(err) }
	if verified, err := dbif.CheckIfEmailVerified("alice", "alice+alt@example.com"); err != nil || verified {
		t.Fatalf("unexpected verified state: %v %v", verified, err)
	}
	if err = dbif.VerifyRegisteredEmail("alice", "alice+alt@example.com"); err != nil { t.Fatal(err) }
	if verified, err := dbif.CheckIfEmailVerified("alice", "alice+alt@example.com"); err != nil || !verified {
		t.Fatalf("email not verified: %v %v", verified, err)
	}
	if u, err := dbif.ResolveEmailToUsername("alice+alt@example.com"); err != nil || u != "alice" {
		t.Fatalf("ResolveEmailToUsername = %q, %v", u, err)
	}
	emails, err := dbif.GetAllRegisteredEmailOfUser("alice")
	if err != nil || len(emails) != 1 { t.Fatalf("emails: %+v %v", emails, err) }

	// ---- namespaces ----
	if _, err = dbif.RegisterNamespace("alice", "alice"); err != nil { t.Fatal(err) }
	if _, err = dbif.RegisterNamespace("bob", "bob"); err != nil { t.Fatal(err) }
	ns, err := dbif.GetNamespaceByName("alice")
	if err != nil { t.Fatal(err) }
	if ns.Owner != "alice" { t.Fatalf("unexpected namespace: %+v", ns) }
	// bob gets a namespace ACL entry on alice, and must then see it.
	if err = dbif.SetNamespaceACL("alice", "bob", &model.ACLTuple{AddRepository: true}); err != nil { t.Fatal(err) }
	if l, err := dbif.GetAllComprisingNamespace("bob"); err != nil || l["alice"] == nil {
		t.Fatalf("GetAllComprisingNamespace did not see the ACL entry: %+v %v", l, err)
	}
	if l, err := dbif.GetAllVisibleNamespace("bob"); err != nil || l["alice"] == nil {
		t.Fatalf("GetAllVisibleNamespace did not see the ACL entry: %+v %v", l, err)
	}
	if err = dbif.UpdateNamespaceStatus("bob", model.NAMESPACE_NORMAL_PRIVATE); err != nil { t.Fatal(err) }
	if n, err := dbif.GetNamespaceByName("bob"); err != nil || n.Status != model.NAMESPACE_NORMAL_PRIVATE {
		t.Fatalf("namespace status: %+v %v", n, err)
	}

	// ---- repositories ----
	if _, err = dbif.CreateRepository("alice", "proj", model.REPO_TYPE_GIT, "alice"); err != nil { t.Fatal(err) }
	if _, err = dbif.CreateRepository("bob", "proj", model.REPO_TYPE_GIT, "bob"); err != nil { t.Fatal(err) }
	repo, err := dbif.GetRepositoryByName("alice", "proj")
	if err != nil { t.Fatal(err) }
	repo.Description = "a project"
	repo.Status = model.REPO_NORMAL_PRIVATE
	repo.WebHookConfig = &model.WebHookConfig{Enable: true, Secret: "0123456789abcdefghij", TargetURL: "https://example.com/h", PayloadType: "json"}
	if err = dbif.UpdateRepositoryInfo("alice", "proj", repo); err != nil { t.Fatal(err) }
	got, err := dbif.GetRepositoryByName("alice", "proj")
	if err != nil { t.Fatal(err) }
	if got.Description != "a project" || got.Status != model.REPO_NORMAL_PRIVATE {
		t.Fatalf("repository metadata was not saved: %+v", got)
	}
	if got.WebHookConfig == nil || !got.WebHookConfig.Enable { t.Fatalf("webhook was not saved: %+v", got.WebHookConfig) }
	if _, _, err = dbif.GetRepositoryWithNamespace("alice", "proj"); err != nil { t.Fatal(err) }
	if _, _, err = dbif.GetRepositoryWithNamespace("alice", "nope"); err != db.ErrEntityNotFound {
		t.Fatalf("missing repository: %v", err)
	}
	// A private repository is visible to its owner and, after an ACL entry, to bob.
	if err = dbif.SetRepositoryACL("alice", "proj", "bob", &model.ACLTuple{PushToRepository: true}); err != nil { t.Fatal(err) }
	if l, err := dbif.GetAllVisibleRepositoryFromNamespace("bob", "alice"); err != nil || len(l) != 1 {
		t.Fatalf("repository ACL not applied: %+v %v", l, err)
	}
	if l, err := dbif.GetAllVisibleRepositoryFromNamespace("", "alice"); err != nil || len(l) != 0 {
		t.Fatalf("anonymous listing must not see the private repository: %+v %v", l, err)
	}
	if n, err := dbif.CountAllRepositories(); err != nil || n != 2 { t.Fatalf("CountAllRepositories = %d, %v", n, err) }
	if l, err := dbif.GetAllRepositories(0, 10); err != nil || len(l) != 2 { t.Fatalf("GetAllRepositories: %+v %v", l, err) }

	// ---- issues ----
	id1, err := dbif.NewRepositoryIssue("alice", "proj", "alice", "first", "content")
	if err != nil { t.Fatal(err) }
	id2, err := dbif.NewRepositoryIssue("alice", "proj", "alice", "second", "content")
	if err != nil { t.Fatal(err) }
	if id1 != 1 || id2 != 2 { t.Fatalf("issue ids: %d, %d", id1, id2) }
	issue, err := dbif.GetRepositoryIssue("alice", "proj", 1)
	if err != nil { t.Fatal(err) }
	if issue.IssueTitle != "first" { t.Fatalf("unexpected issue: %+v", issue) }
	if n, err := dbif.CountAllRepositoryIssue("alice", "proj"); err != nil || n != 2 { t.Fatalf("issue count: %d %v", n, err) }
	if err = dbif.NewRepositoryIssueEvent("alice", "proj", 1, model.EVENT_COMMENT, "bob", "hello"); err != nil { t.Fatal(err) }
	if l, err := dbif.GetAllIssueEvent("alice", "proj", 1); err != nil || len(l) != 1 { t.Fatalf("issue events: %+v %v", l, err) }
	if err = dbif.SetIssuePriority("alice", "proj", 1, 2); err != nil { t.Fatal(err) }
	if l, err := dbif.SearchIssuePaginated("", "alice", "proj", 0, 0, 10); err != nil || len(l) != 2 { t.Fatalf("issue search: %+v %v", l, err) }

	// ---- pull requests ----
	pr1, err := dbif.NewPullRequest("alice", "one", "alice", "proj", "main", "alice", "proj", "feature")
	if err != nil { t.Fatal(err) }
	pr2, err := dbif.NewPullRequest("alice", "two", "alice", "proj", "main", "alice", "proj", "feature")
	if err != nil { t.Fatal(err) }
	if pr1 != 1 || pr2 != 2 { t.Fatalf("pull request ids: %d, %d", pr1, pr2) }
	if p, err := dbif.GetPullRequest("alice", "proj", 1); err != nil || p.Title != "one" { t.Fatalf("pull request: %+v %v", p, err) }
	if l, err := dbif.GetAllPullRequestPaginated("alice", "proj", 0, 10); err != nil || len(l) != 2 { t.Fatalf("pull requests: %+v %v", l, err) }

	// ---- snippets ----
	if _, err = dbif.NewSnippet("alice", "snip", model.SNIPPET_PUBLIC); err != nil { t.Fatal(err) }
	if s, err := dbif.GetSnippet("alice", "snip"); err != nil || s.Name != "snip" { t.Fatalf("snippet: %+v %v", s, err) }
	if l, err := dbif.GetAllSnippet("alice"); err != nil || len(l) != 1 { t.Fatalf("snippets: %+v %v", l, err) }

	// ---- search / counts ----
	if l, err := dbif.SearchForUser("ali", 0, 10); err != nil || len(l) != 1 { t.Fatalf("user search: %+v %v", l, err) }
	if l, err := dbif.SearchForNamespace("ali", 0, 10); err != nil || len(l) != 1 { t.Fatalf("namespace search: %+v %v", l, err) }
	if l, err := dbif.SearchForRepository("proj", 0, 10); err != nil || len(l) != 2 { t.Fatalf("repository search: %+v %v", l, err) }
	if n, err := dbif.CountAllUser(); err != nil || n != 2 { t.Fatalf("CountAllUser = %d, %v", n, err) }

	// ---- webhook log ----
	if err = dbif.RegisterWebhookRequest("00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002", "alice", "proj", "abc"); err != nil {
		t.Fatal(err)
	}
	if err = dbif.UpdateWebhookResult("00000000-0000-0000-0000-000000000001", &model.WebhookResult{UUID: "00000000-0000-0000-0000-000000000001", Status: 1, Message: "ok"}); err != nil {
		t.Fatal(err)
	}

	// ---- visibility listings that go through the query builder ----
	if l, err := dbif.GetAllVisibleNamespacePaginated("bob", 0, 10); err != nil || l["alice"] == nil {
		t.Fatalf("GetAllVisibleNamespacePaginated: %+v %v", l, err)
	}
	if l, err := dbif.SearchAllVisibleNamespacePaginated("bob", "ali", 0, 10); err != nil || l["alice"] == nil {
		t.Fatalf("SearchAllVisibleNamespacePaginated: %+v %v", l, err)
	}
	if l, err := dbif.GetAllVisibleRepositoryPaginated("bob", 0, 10); err != nil || len(l) != 2 {
		t.Fatalf("GetAllVisibleRepositoryPaginated: %+v %v", l, err)
	}
	if l, err := dbif.SearchAllVisibleRepositoryPaginated("bob", "proj", 0, 10); err != nil || len(l) != 2 {
		t.Fatalf("SearchAllVisibleRepositoryPaginated: %+v %v", l, err)
	}
	if l, err := dbif.GetAllVisibleRepositoryPaginated("", 0, 10); err != nil || len(l) != 1 {
		t.Fatalf("anonymous repository listing: %+v %v", l, err)
	}
	if l, err := dbif.GetAllNamespaceByOwner("alice"); err != nil || l["alice"] == nil {
		t.Fatalf("GetAllNamespaceByOwner: %+v %v", l, err)
	}
	if l, err := dbif.GetAllRepositoryFromNamespace("alice"); err != nil || l["proj"] == nil {
		t.Fatalf("GetAllRepositoryFromNamespace: %+v %v", l, err)
	}
	if l, err := dbif.GetAllUsers(0, 10); err != nil || len(l) != 2 {
		t.Fatalf("GetAllUsers: %+v %v", l, err)
	}
	if l, err := dbif.GetAllNamespaces(0, 10); err != nil || len(l) != 2 {
		t.Fatalf("GetAllNamespaces: %+v %v", l, err)
	}
	if l, err := dbif.GetAllBelongingNamespace("bob", "alice"); err != nil || len(l) != 1 {
		t.Fatalf("GetAllBelongingNamespace: %+v %v", l, err)
	}
	if l, err := dbif.GetAllBelongingRepository("bob", "alice", "", 0, 10); err != nil || len(l) != 1 {
		t.Fatalf("GetAllBelongingRepository: %+v %v", l, err)
	}
	if n, err := dbif.CountAllBelongingRepository("bob", "alice", ""); err != nil || n != 1 {
		t.Fatalf("CountAllBelongingRepository = %d, %v", n, err)
	}
	if n, err := dbif.CountAllVisibleNamespace("bob"); err != nil || n != 2 {
		t.Fatalf("CountAllVisibleNamespace = %d, %v", n, err)
	}
	if n, err := dbif.CountAllVisibleRepositories("bob"); err != nil || n != 2 {
		t.Fatalf("CountAllVisibleRepositories = %d, %v", n, err)
	}
	if n, err := dbif.CountAllVisibleNamespaceSearchResult("bob", "ali"); err != nil || n != 1 {
		t.Fatalf("CountAllVisibleNamespaceSearchResult = %d, %v", n, err)
	}
	if n, err := dbif.CountAllVisibleRepositoriesSearchResult("bob", "proj"); err != nil || n != 2 {
		t.Fatalf("CountAllVisibleRepositoriesSearchResult = %d, %v", n, err)
	}
	if n, err := dbif.CountAllRepositoriesSearchResult("proj"); err != nil || n != 2 {
		t.Fatalf("CountAllRepositoriesSearchResult = %d, %v", n, err)
	}
	if n, err := dbif.CountAllNamespace(); err != nil || n != 2 { t.Fatalf("CountAllNamespace = %d, %v", n, err) }

	// ---- user info, keys ----
	alice.Bio = "bio"
	alice.Website = "https://example.com"
	alice.TFAConfig.Email.Enable = true
	alice.WebsitePreference.BackgroundColor = "#123456"
	if err = dbif.UpdateUserInfo("alice", alice); err != nil { t.Fatal(err) }
	if a, err := dbif.GetUserByName("alice"); err != nil || a.Bio != "bio" || !a.TFAConfig.Email.Enable || a.WebsitePreference.BackgroundColor != "#123456" {
		t.Fatalf("UpdateUserInfo did not round-trip: %+v %v", a, err)
	}
	if err = dbif.RegisterAuthKey("alice", "k1", "ssh-ed25519 AAAA"); err != nil { t.Fatal(err) }
	if k, err := dbif.GetAuthKeyByName("alice", "k1"); err != nil || k.KeyText != "ssh-ed25519 AAAA" { t.Fatalf("authkey: %+v %v", k, err) }
	if err = dbif.UpdateAuthKey("alice", "k1", "ssh-ed25519 BBBB"); err != nil { t.Fatal(err) }
	if l, err := dbif.GetAllAuthKeyByUsername("alice"); err != nil || len(l) != 1 || l[0].KeyText != "ssh-ed25519 BBBB" { t.Fatalf("authkeys: %+v %v", l, err) }
	if err = dbif.RemoveAuthKey("alice", "k1"); err != nil { t.Fatal(err) }
	if err = dbif.RegisterSignKey("alice", "s1", "PGP KEY"); err != nil { t.Fatal(err) }
	if k, err := dbif.GetSignKeyByName("alice", "s1"); err != nil || k.KeyText != "PGP KEY" { t.Fatalf("signkey: %+v %v", k, err) }
	if err = dbif.UpdateSignKey("alice", "s1", "PGP KEY 2"); err != nil { t.Fatal(err) }
	if l, err := dbif.GetAllSignKeyByUsername("alice"); err != nil || len(l) != 1 || l[0].KeyText != "PGP KEY 2" { t.Fatalf("signkeys: %+v %v", l, err) }
	if err = dbif.RemoveSignKey("alice", "s1"); err != nil { t.Fatal(err) }

	// ---- registration requests ----
	if err = dbif.InsertRegistrationRequest("carol", "carol@example.com", "hash", "please"); err != nil { t.Fatal(err) }
	if l, err := dbif.GetRegistrationRequestPaginated(0, 10); err != nil || len(l) != 1 { t.Fatalf("registration requests: %+v %v", l, err) }
	if l, err := dbif.GetRequestOfUsernamePaginated("carol", 0, 10); err != nil || len(l) != 1 { t.Fatalf("registration requests by name: %+v %v", l, err) }
	if l, err := dbif.SearchRegistrationRequestPaginated("carol", 0, 10); err != nil || len(l) != 1 { t.Fatalf("registration request search: %+v %v", l, err) }
	if n, err := dbif.CountRegistrationRequest("carol"); err != nil || n != 1 { t.Fatalf("registration request count = %d, %v", n, err) }
	rr, err := dbif.GetRegistrationRequestByAbsId(func() int64 {
		l, _ := dbif.GetRegistrationRequestPaginated(0, 10)
		return l[0].AbsId
	}())
	if err != nil || rr.Username != "carol" { t.Fatalf("registration request by abs id: %+v %v", rr, err) }
	if err = dbif.ApproveRegistrationRequest(rr.AbsId); err != nil { t.Fatal(err) }
	if u, err := dbif.GetUserByName("carol"); err != nil || u.Email != "carol@example.com" { t.Fatalf("approved registration: %+v %v", u, err) }

	// ---- labels ----
	if err = dbif.AddRepositoryLabel("alice", "proj", "bug"); err != nil { t.Fatal(err) }
	if l, err := dbif.GetRepositoryLabel("alice", "proj"); err != nil || len(l) != 1 || l[0] != "bug" { t.Fatalf("labels: %+v %v", l, err) }
	if n, err := dbif.CountRepositoryWithLabel("bob", "bug"); err != nil || n != 1 { t.Fatalf("label count = %d, %v", n, err) }
	if l, err := dbif.GetRepositoryWithLabelPaginated("bob", "bug", 0, 10); err != nil || len(l) != 1 { t.Fatalf("repositories by label: %+v %v", l, err) }
	if err = dbif.RemoveRepositoryLabel("alice", "proj", "bug"); err != nil { t.Fatal(err) }

	// ---- issue deletion / counts ----
	if n, err := dbif.CountIssue("", "alice", "proj", 0); err != nil || n != 2 { t.Fatalf("CountIssue = %d, %v", n, err) }
	if err = dbif.HardDeleteRepositoryIssueEvent(func() int64 {
		l, _ := dbif.GetAllIssueEvent("alice", "proj", 1)
		return l[0].EventAbsId
	}()); err != nil { t.Fatal(err) }
	if err = dbif.HardDeleteRepositoryIssue("alice", "proj", 2); err != nil { t.Fatal(err) }

	// ---- pull request extra ----
	prs, err := dbif.GetAllPullRequestPaginated("alice", "proj", 0, 10)
	if err != nil || len(prs) != 2 { t.Fatalf("pull requests: %+v %v", prs, err) }
	absId := prs[0].PRAbsId
	if p, err := dbif.GetPullRequestByAbsId(absId); err != nil || p.PRId != 1 || p.ReceiverNamespace != "alice" {
		t.Fatalf("GetPullRequestByAbsId: %+v %v", p, err)
	}
	if _, err = dbif.CommentOnPullRequest(absId, "bob", "hi"); err != nil { t.Fatal(err) }
	if l, err := dbif.GetAllPullRequestEventPaginated(absId, 0, 10); err != nil || len(l) != 1 { t.Fatalf("pull request events: %+v %v", l, err) }
	if err = dbif.ClosePullRequestAsNotMerged(absId, "alice"); err != nil { t.Fatal(err) }
	if err = dbif.ReopenPullRequest(absId, "alice"); err != nil { t.Fatal(err) }
	if n, err := dbif.CountPullRequest("", "alice", "proj", 0); err != nil || n != 2 { t.Fatalf("CountPullRequest = %d, %v", n, err) }
	if l, err := dbif.SearchPullRequestPaginated("one", "alice", "proj", 0, 0, 10); err != nil || len(l) != 1 { t.Fatalf("pull request search: %+v %v", l, err) }
	if err = dbif.DeletePullRequest(prs[1].PRAbsId); err != nil { t.Fatal(err) }

	// ---- snippets with shared users ----
	snip, err := dbif.NewSnippet("alice", "shared", model.SNIPPET_SHARED_USER)
	if err != nil { t.Fatal(err) }
	snip.SharedUser = map[string]bool{"bob": true}
	snip.Description = "a shared snippet"
	if err = dbif.SaveSnippetInfo(snip); err != nil { t.Fatal(err) }
	if s, err := dbif.GetSnippet("alice", "shared"); err != nil || !s.SharedUser["bob"] || s.Description != "a shared snippet" {
		t.Fatalf("shared snippet: %+v %v", s, err)
	}
	if l, err := dbif.GetAllVisibleSnippetPaginated("alice", "bob", "", 0, 10); err != nil || len(l) != 2 {
		t.Fatalf("visible snippets for the sharee: %+v %v", l, err)
	}
	if n, err := dbif.CountAllVisibleSnippet("alice", "bob", ""); err != nil || n != 2 {
		t.Fatalf("CountAllVisibleSnippet = %d, %v", n, err)
	}
	if l, err := dbif.GetAllVisibleSnippetPaginated("alice", "", "", 0, 10); err != nil || len(l) != 1 {
		t.Fatalf("anonymous snippets: %+v %v", l, err)
	}

	// ---- webhook result reading ----
	if wr, err := dbif.GetWebhookResultByUUID("00000000-0000-0000-0000-000000000001"); err != nil || wr.Status != 1 {
		t.Fatalf("webhook result: %+v %v", wr, err)
	}

	// ---- ACL removal ----
	if err = dbif.SetRepositoryACL("alice", "proj", "bob", nil); err != nil { t.Fatal(err) }
	if l, err := dbif.GetAllVisibleRepositoryFromNamespace("bob", "alice"); err != nil || len(l) != 0 {
		t.Fatalf("repository ACL was not removed: %+v %v", l, err)
	}
	if err = dbif.SetNamespaceACL("alice", "bob", nil); err != nil { t.Fatal(err) }
	if l, err := dbif.GetAllComprisingNamespace("bob"); err != nil || l["alice"] != nil {
		t.Fatalf("namespace ACL was not removed: %+v %v", l, err)
	}

	// ---- EnsureSchema repairs a database that lost the counter and an index ----
	if _, err = raw.Exec("DROP INDEX idx_repository_repo_owner ON t__repository"); err != nil { t.Fatal(err) }
	if _, err = raw.Exec("DROP TABLE t__id_counter"); err != nil { t.Fatal(err) }
	if err = dbif.EnsureSchema(); err != nil { t.Fatalf("EnsureSchema after dropping: %s", err) }
	var n int
	if err = raw.QueryRow(`SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 't__repository' AND index_name = 'idx_repository_repo_owner'`).Scan(&n); err != nil { t.Fatal(err) }
	if n != 1 { t.Fatal("EnsureSchema did not recreate the index") }
	if err = raw.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 't__id_counter'`).Scan(&n); err != nil { t.Fatal(err) }
	if n != 1 { t.Fatal("EnsureSchema did not recreate id_counter") }

	// ---- deletes ----
	if err = dbif.HardDeleteRepository("alice", "proj"); err != nil { t.Fatal(err) }
	if err = dbif.HardDeleteNamespaceByName("bob"); err != nil { t.Fatal(err) }
	if err = dbif.HardDeleteUserByName("carol"); err != nil { t.Fatal(err) }

}
