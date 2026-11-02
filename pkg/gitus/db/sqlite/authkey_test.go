package sqlite

import (
	"testing"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
)

func TestGetAllAuthKey(t *testing.T) {
	dbif, _ := setupRegistrationRequestTest(t)
	for _, name := range []string{"alice", "bob"} {
		if _, err := dbif.RegisterUser(name, name+"@example.com", "hash", model.NORMAL_USER); err != nil {
			t.Fatal(err)
		}
	}
	if err := dbif.RegisterAuthKey("alice", "laptop", "ssh-ed25519 AAAA alice"); err != nil {
		t.Fatal(err)
	}
	if err := dbif.RegisterAuthKey("bob", "desktop", "ssh-ed25519 BBBB bob"); err != nil {
		t.Fatal(err)
	}
	keys, err := dbif.GetAllAuthKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys: %+v", len(keys), keys)
	}
	found := map[string]bool{}
	for _, k := range keys {
		found[k.UserName+":"+k.KeyName] = true
	}
	for _, want := range []string{"alice:laptop", "bob:desktop"} {
		if !found[want] {
			t.Errorf("missing key %s in %+v", want, keys)
		}
	}
}
