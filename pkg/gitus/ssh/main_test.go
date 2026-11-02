package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func testPublicKey(t *testing.T) (string, gossh.PublicKey) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pub := signer.PublicKey()
	return strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pub))), pub
}

func TestMatchAuthorizedKey(t *testing.T) {
	line, pub := testPublicKey(t)
	otherLine, _ := testPublicKey(t)

	if !MatchAuthorizedKey(line+" user@example", line) {
		t.Error("the same key with a comment did not match")
	}
	if !MatchAuthorizedKey(line, gossh.FingerprintSHA256(pub)) {
		t.Error("the SHA256 fingerprint did not match")
	}
	if !MatchAuthorizedKey(line, "MD5:"+gossh.FingerprintLegacyMD5(pub)) {
		t.Error("the MD5 fingerprint did not match")
	}
	if !MatchAuthorizedKey(line, gossh.FingerprintLegacyMD5(pub)) {
		t.Error("the bare legacy MD5 fingerprint did not match")
	}
	if MatchAuthorizedKey(line, otherLine) {
		t.Error("a different key matched")
	}
	for _, junk := range []string{"", "not a key", "SHA256:not-a-fingerprint"} {
		if MatchAuthorizedKey(junk, line) || MatchAuthorizedKey(line, junk) {
			t.Errorf("junk %q matched", junk)
		}
	}
}

func TestAuthorizedKeyLine(t *testing.T) {
	line, _ := testPublicKey(t)
	got := AuthorizedKeyLine("/etc/gitus/config.json", "alice", "laptop", line)
	for _, want := range []string{
		"gitus -config",
		"ssh alice laptop",
		"ssh-ed25519",
		line,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("line does not contain %q:\n%s", want, got)
		}
	}
}
