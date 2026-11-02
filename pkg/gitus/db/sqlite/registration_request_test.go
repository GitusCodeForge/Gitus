package sqlite

import (
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
)

func setupRegistrationRequestTest(t *testing.T) (*SqliteGitusDatabaseInterface, *gitus.GitusConfig) {
	t.Helper()
	dir := t.TempDir()
	gitRoot := path.Join(dir, "repos")
	if err := os.MkdirAll(gitRoot, 0755); err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(cfgPath, cfgJSON, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := gitus.LoadConfigFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	dbif, err := NewSqliteGitusDatabaseInterface(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbif.Dispose() })
	if err := dbif.InstallTables(); err != nil {
		t.Fatalf("InstallTables: %s", err)
	}
	return dbif, cfg
}

// The first CREATE TABLE for the registration request table was missing the
// comma before timestamp, so existing databases have no timestamp column.
// EnsureSchema has to add it, otherwise every insert fails.
func TestRegistrationRequestTimestampMigration(t *testing.T) {
	dbif, _ := setupRegistrationRequestTest(t)
	if _, err := dbif.connection.Exec(`
DROP TABLE gitus_user_reg_request;
CREATE TABLE gitus_user_reg_request (
    username TEXT,
    email TEXT,
    password_hash TEXT,
    reason TEXT
    timestamp INTEGER
)`); err != nil {
		t.Fatal(err)
	}
	if err := dbif.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %s", err)
	}
	if err := dbif.InsertRegistrationRequest("alice", "alice@example.com", "hash", "please"); err != nil {
		t.Fatalf("InsertRegistrationRequest after migration: %s", err)
	}
}

func TestRegistrationRequestQueries(t *testing.T) {
	dbif, _ := setupRegistrationRequestTest(t)
	if _, err := dbif.RegisterUser("alice", "alice@example.com", "hash", model.NORMAL_USER_APPROVAL_NEEDED); err != nil {
		t.Fatal(err)
	}
	if _, err := dbif.RegisterUser("bob", "bob@example.com", "hash", model.NORMAL_USER_APPROVAL_NEEDED); err != nil {
		t.Fatal(err)
	}
	if err := dbif.InsertRegistrationRequest("alice", "alice@example.com", "hash", "first"); err != nil {
		t.Fatal(err)
	}
	if err := dbif.InsertRegistrationRequest("bob", "bob@example.com", "hash", "second"); err != nil {
		t.Fatal(err)
	}

	list, err := dbif.GetRegistrationRequestPaginated(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d requests", len(list))
	}
	for _, r := range list {
		if r.AbsId == 0 {
			t.Errorf("request for %s has no id", r.Username)
		}
		if len(r.Reason) == 0 {
			t.Errorf("request for %s has no reason", r.Username)
		}
	}

	mine, err := dbif.GetRequestOfUsernamePaginated("alice", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].Username != "alice" || mine[0].AbsId == 0 || mine[0].Reason != "first" {
		t.Fatalf("alice's requests = %+v", mine)
	}

	one, err := dbif.GetRegistrationRequestByAbsId(mine[0].AbsId)
	if err != nil {
		t.Fatal(err)
	}
	if one.Username != "alice" || one.Email != "alice@example.com" || one.Reason != "first" {
		t.Fatalf("request = %+v", one)
	}

	search, err := dbif.SearchRegistrationRequestPaginated("alice", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(search) != 1 || search[0].Reason != "first" {
		t.Fatalf("search result = %+v", search)
	}

	// approving uses the id and only moves the already-registered user on.
	if err := dbif.ApproveRegistrationRequest(one.AbsId); err != nil {
		t.Fatalf("ApproveRegistrationRequest: %s", err)
	}
	u, err := dbif.GetUserByName("alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.Status != model.NORMAL_USER {
		t.Errorf("alice's status after approval = %d", u.Status)
	}
	if _, err := dbif.GetRegistrationRequestByAbsId(one.AbsId); err == nil {
		t.Error("the approved request is still there")
	}
}
