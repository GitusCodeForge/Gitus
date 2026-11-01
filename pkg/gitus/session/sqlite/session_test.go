package sqlite

import (
	"database/sql"
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session"
)

func newSessionTestStore(t *testing.T) (*GitusSqliteSessionStore, *gitus.GitusConfig) {
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
		"maxSessionLifetime": 3600,
	})
	cfgPath := path.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, cfgJSON, 0644); err != nil { t.Fatal(err) }
	cfg, err := gitus.LoadConfigFile(cfgPath)
	if err != nil { t.Fatal(err) }
	ss, err := NewGitusSqliteSessionStore(cfg)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { ss.Dispose() })
	return ss, cfg
}

func TestSqliteSessionMetadata(t *testing.T) {
	ss, _ := newSessionTestStore(t)
	if err := ss.Install(); err != nil { t.Fatal(err) }
	// EnsureSchema must be a no-op on a freshly installed table.
	if err := ss.EnsureSchema(); err != nil { t.Fatal(err) }
	if err := ss.EnsureSchema(); err != nil { t.Fatal(err) }

	meta := session.GitusSessionMetadata{IP: "203.0.113.9", UserAgent: "Mozilla/5.0 Firefox/130.0"}
	created, err := ss.RegisterSession("alice", "abc", meta)
	if err != nil { t.Fatal(err) }
	if created.IP != meta.IP || created.UserAgent != meta.UserAgent {
		t.Fatalf("registered session = %+v", created)
	}

	list, err := ss.RetrieveSession("alice")
	if err != nil { t.Fatal(err) }
	if len(list) != 1 { t.Fatalf("session list = %+v", list) }
	if list[0].IP != meta.IP || list[0].UserAgent != meta.UserAgent || list[0].Id != "abc" {
		t.Fatalf("retrieved session = %+v", list[0])
	}

	byKey, err := ss.RetrieveSessionByKey("alice", "abc")
	if err != nil { t.Fatal(err) }
	if byKey == nil || byKey.IP != meta.IP || byKey.UserAgent != meta.UserAgent {
		t.Fatalf("session by key = %+v", byKey)
	}

	if err := ss.RevokeSession("alice", "abc"); err != nil { t.Fatal(err) }
	list, err = ss.RetrieveSession("alice")
	if err != nil { t.Fatal(err) }
	if len(list) != 0 { t.Fatalf("sessions after revoke = %+v", list) }
}

func TestSqliteSessionEnsureSchemaMigratesOldTable(t *testing.T) {
	ss, cfg := newSessionTestStore(t)
	// An instance installed before the session metadata existed.
	raw, err := sql.Open("sqlite3", cfg.ProperSessionPath())
	if err != nil { t.Fatal(err) }
	_, err = raw.Exec(`CREATE TABLE s_session (
		user_name TEXT, value TEXT, reg_timestamp INTEGER,
		expire_timestamp INTEGER, csrf TEXT)`)
	if err != nil { t.Fatal(err) }
	_, err = raw.Exec(`INSERT INTO s_session(user_name, value, reg_timestamp, expire_timestamp, csrf)
		VALUES ('alice', 'old', 100, 9999999999, 'csrf')`)
	if err != nil { t.Fatal(err) }
	raw.Close()

	if err := ss.EnsureSchema(); err != nil { t.Fatal(err) }
	// Running it again must not fail on the columns that now exist.
	if err := ss.EnsureSchema(); err != nil { t.Fatal(err) }

	list, err := ss.RetrieveSession("alice")
	if err != nil { t.Fatal(err) }
	if len(list) != 1 || list[0].Id != "old" {
		t.Fatalf("old session did not survive the migration: %+v", list)
	}
	if list[0].IP != "" || list[0].UserAgent != "" {
		t.Fatalf("old session has unexpected metadata: %+v", list[0])
	}

	if _, err := ss.RegisterSession("alice", "new", session.GitusSessionMetadata{IP: "198.51.100.1", UserAgent: "curl/8"}); err != nil {
		t.Fatal(err)
	}
	list, err = ss.RetrieveSession("alice")
	if err != nil { t.Fatal(err) }
	found := false
	for _, s := range list {
		if s.Id == "new" {
			found = true
			if s.IP != "198.51.100.1" || s.UserAgent != "curl/8" {
				t.Errorf("new session metadata = %+v", s)
			}
		}
	}
	if !found { t.Error("new session is missing from the list") }
}
