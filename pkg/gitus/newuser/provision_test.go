package newuser

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"testing"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db/sqlite"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
)

func provisionTestInstance(t *testing.T) (*sqlite.SqliteGitusDatabaseInterface, *gitus.GitusConfig) {
	t.Helper()
	dir := t.TempDir()
	gitRoot := path.Join(dir, "repos")
	if err := os.MkdirAll(gitRoot, 0755); err != nil {
		t.Fatal(err)
	}
	cfgJSON, _ := json.Marshal(map[string]any{
		"version": 0, "root": gitRoot, "gitUser": "tester", "hostName": "localhost",
		"bindAddress": "127.0.0.1", "bindPort": 8000,
		"enableNamespace": true,
		"operationMode":   "forge",
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
	dbif, err := sqlite.NewSqliteGitusDatabaseInterface(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbif.Dispose() })
	if err := dbif.InstallTables(); err != nil {
		t.Fatalf("InstallTables: %s", err)
	}
	if _, err := dbif.RegisterUser("admin", "admin@example.com", "hash", model.ADMIN); err != nil {
		t.Fatalf("RegisterUser(admin): %s", err)
	}
	if _, err := dbif.RegisterNamespace("acme", "admin"); err != nil {
		t.Fatalf("RegisterNamespace(acme): %s", err)
	}
	return dbif, cfg
}

func namespaceACL(t *testing.T, dbif db.GitusDatabaseInterface, nsName string, username string) *model.ACLTuple {
	t.Helper()
	ns, err := dbif.GetNamespaceByName(nsName)
	if err != nil {
		t.Fatalf("GetNamespaceByName(%s): %s", nsName, err)
	}
	if ns.ACL == nil {
		return nil
	}
	return ns.ACL.GetUserPrivilege(username)
}

func TestFinalStatus(t *testing.T) {
	cfg := &gitus.GitusConfig{}
	cases := map[model.GitusUserStatus]model.GitusUserStatus{
		0:                                  model.NORMAL_USER,
		model.NORMAL_USER:                  model.NORMAL_USER,
		model.NORMAL_USER_APPROVAL_NEEDED:  model.NORMAL_USER,
		model.NORMAL_USER_CONFIRM_NEEDED:   model.NORMAL_USER,
		model.NORMAL_USER_NO_NEW_NAMESPACE: model.NORMAL_USER_NO_NEW_NAMESPACE,
		model.BANNED:                       model.BANNED,
	}
	for in, want := range cases {
		cfg.DefaultNewUserStatus = in
		if got := FinalStatus(cfg); got != want {
			t.Errorf("FinalStatus(default=%d) = %d; want %d", in, got, want)
		}
	}
}

func TestValidDefaultStatus(t *testing.T) {
	for _, s := range []model.GitusUserStatus{
		model.NORMAL_USER, model.NORMAL_USER_APPROVAL_NEEDED,
		model.NORMAL_USER_CONFIRM_NEEDED, model.NORMAL_USER_NO_NEW_NAMESPACE, model.BANNED,
	} {
		if !ValidDefaultStatus(s) {
			t.Errorf("status %d should be a valid default", s)
		}
	}
	for _, s := range []model.GitusUserStatus{model.ADMIN, model.SUPER_ADMIN, 6, 0} {
		if ValidDefaultStatus(s) {
			t.Errorf("status %d should not be a valid default", s)
		}
	}
}

func TestProvisionNewUser(t *testing.T) {
	dbif, cfg := provisionTestInstance(t)
	cfg.DefaultNewUserNamespace = "acme"

	// an existing member must survive the new memberships
	if err := dbif.SetNamespaceACL("acme", "carol", &model.ACLTuple{AddMember: true}); err != nil {
		t.Fatal(err)
	}

	// a normal user gets a personal namespace and joins the default one.
	if _, err := dbif.RegisterUser("alice", "alice@example.com", "hash", model.NORMAL_USER); err != nil {
		t.Fatal(err)
	}
	if err := ProvisionNewUser(dbif, cfg, "alice", model.NORMAL_USER); err != nil {
		t.Fatal(err)
	}
	ns, err := dbif.GetNamespaceByName("alice")
	if err != nil {
		t.Fatalf("alice has no personal namespace: %s", err)
	}
	if ns.Owner != "alice" {
		t.Errorf("alice's namespace owner = %q", ns.Owner)
	}
	alice := namespaceACL(t, dbif, "acme", "alice")
	if alice == nil || !alice.AddRepository {
		t.Errorf("alice is not a member that can add repositories: %+v", alice)
	}
	if alice.AddMember || alice.DeleteRepository || alice.EditInfo {
		t.Errorf("alice got more than the intended privileges: %+v", alice)
	}
	if carol := namespaceACL(t, dbif, "acme", "carol"); carol == nil || !carol.AddMember {
		t.Errorf("provisioning removed carol's membership: %+v", carol)
	}

	// a no-new-namespace user joins the default namespace but gets no
	// personal namespace.
	if _, err := dbif.RegisterUser("bob", "bob@example.com", "hash", model.NORMAL_USER_NO_NEW_NAMESPACE); err != nil {
		t.Fatal(err)
	}
	if err := ProvisionNewUser(dbif, cfg, "bob", model.NORMAL_USER_NO_NEW_NAMESPACE); err != nil {
		t.Fatal(err)
	}
	if _, err := dbif.GetNamespaceByName("bob"); !errors.Is(err, db.ErrEntityNotFound) {
		t.Errorf("bob should not have a personal namespace, got %v", err)
	}
	bob := namespaceACL(t, dbif, "acme", "bob")
	if bob == nil || !bob.AddRepository {
		t.Errorf("bob is not a member that can add repositories: %+v", bob)
	}
}

func TestAddToDefaultNamespaceMissingNamespace(t *testing.T) {
	dbif, cfg := provisionTestInstance(t)
	cfg.DefaultNewUserNamespace = "does-not-exist"
	if err := AddToDefaultNamespace(dbif, cfg, "dave"); err != nil {
		t.Fatalf("a missing default namespace should not fail: %s", err)
	}
}

func TestProvisionWithoutNamespaceMode(t *testing.T) {
	dbif, cfg := provisionTestInstance(t)
	cfg.UseNamespace = false
	cfg.DefaultNewUserNamespace = "acme"
	if _, err := dbif.RegisterUser("eve", "eve@example.com", "hash", model.NORMAL_USER); err != nil {
		t.Fatal(err)
	}
	if err := ProvisionNewUser(dbif, cfg, "eve", model.NORMAL_USER); err != nil {
		t.Fatal(err)
	}
	if _, err := dbif.GetNamespaceByName("eve"); !errors.Is(err, db.ErrEntityNotFound) {
		t.Errorf("a personal namespace was created although namespaces are off: %v", err)
	}
	// the default namespace is part of namespace mode, so it is not used either.
	if eve := namespaceACL(t, dbif, "acme", "eve"); eve != nil {
		t.Errorf("eve was added to the default namespace although namespaces are off: %+v", eve)
	}
}
