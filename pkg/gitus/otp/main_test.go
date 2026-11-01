package otp

import (
	"strings"
	"testing"
	"time"
)

// The RFC 4226 test vectors: the secret is the ASCII string
// "12345678901234567890", base32 encoded.
const rfc4226Secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestHOTPRFC4226(t *testing.T) {
	want := []string{
		"755224", "287082", "359152", "969429", "338314",
		"254676", "287922", "162583", "399871", "520489",
	}
	for counter, w := range want {
		got, err := Code(rfc4226Secret, uint64(counter), 6)
		if err != nil { t.Fatal(err) }
		if got != w {
			t.Errorf("counter %d = %s; want %s", counter, got, w)
		}
	}
}

func TestTOTPRFC6238(t *testing.T) {
	// RFC 6238 uses an 8 digit code and the same SHA-1 secret as RFC 4226.
	cases := []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, c := range cases {
		got, err := TOTPCode(rfc4226Secret, time.Unix(c.unix, 0), 30, 8)
		if err != nil { t.Fatal(err) }
		if got != c.want {
			t.Errorf("t=%d = %s; want %s", c.unix, got, c.want)
		}
	}
}

func TestGenerateSecret(t *testing.T) {
	seen := make(map[string]bool)
	for range 20 {
		s, err := GenerateSecret()
		if err != nil { t.Fatal(err) }
		if len(s) != 32 { t.Fatalf("secret length = %d (%q)", len(s), s) }
		if strings.ContainsAny(s, "=abcdefghijklmnopqrstuvwxyz") {
			t.Fatalf("secret is not upper case base32 without padding: %q", s)
		}
		if seen[s] { t.Fatal("GenerateSecret returned a duplicate") }
		seen[s] = true
		// the secret has to decode, and a code has to come out of it.
		if _, err := Code(s, 0, 6); err != nil { t.Fatalf("generated secret is unusable: %s", err) }
	}
}

func TestVerifyCode(t *testing.T) {
	code, err := Code(rfc4226Secret, 5, 6)
	if err != nil { t.Fatal(err) }
	// the exact counter matches, and reports which one it was.
	if c, ok, err := VerifyCode(rfc4226Secret, code, 5, 6, 1); err != nil || !ok || c != 5 {
		t.Fatalf("exact counter = %d, %v, %v", c, ok, err)
	}
	// looking ahead from an older counter finds it.
	if c, ok, _ := VerifyCode(rfc4226Secret, code, 3, 6, 5); !ok || c != 5 {
		t.Fatalf("look ahead = %d, %v", c, ok)
	}
	// a counter that is already behind is not accepted.
	if _, ok, _ := VerifyCode(rfc4226Secret, code, 6, 6, 5); ok {
		t.Fatal("counter behind the expected one was accepted")
	}
	// window 0 only accepts the exact counter.
	if _, ok, _ := VerifyCode(rfc4226Secret, code, 4, 6, 0); ok {
		t.Fatal("window 0 accepted the next counter")
	}
}

func TestVerifyTOTP(t *testing.T) {
	now := time.Unix(1111111111, 0)
	code, err := TOTPCode(rfc4226Secret, now, 30, 6)
	if err != nil { t.Fatal(err) }
	if _, ok, err := VerifyTOTP(rfc4226Secret, code, now, 30, 6, 1); err != nil || !ok {
		t.Fatalf("current step rejected: %v %v", ok, err)
	}
	// one step later is still accepted with a window of 1 ...
	if _, ok, _ := VerifyTOTP(rfc4226Secret, code, now.Add(30*time.Second), 30, 6, 1); !ok {
		t.Fatal("previous step rejected with window 1")
	}
	// ... but not with a window of 0.
	if _, ok, _ := VerifyTOTP(rfc4226Secret, code, now.Add(30*time.Second), 30, 6, 0); ok {
		t.Fatal("previous step accepted with window 0")
	}
}

func TestCodeInputIsForgiving(t *testing.T) {
	code, err := Code(rfc4226Secret, 0, 6)
	if err != nil { t.Fatal(err) }
	spaced := code[0:3] + " " + code[3:]
	if _, ok, _ := VerifyCode(rfc4226Secret, spaced, 0, 6, 0); !ok {
		t.Errorf("code with a space was rejected: %q", spaced)
	}
	if _, ok, _ := VerifyCode(strings.ToLower(rfc4226Secret), code, 0, 6, 0); !ok {
		t.Error("lower case secret was rejected")
	}
	// a secret with separators is accepted too.
	withDashes := rfc4226Secret[0:8] + "-" + rfc4226Secret[8:16] + "-" + rfc4226Secret[16:]
	if _, ok, _ := VerifyCode(withDashes, code, 0, 6, 0); !ok {
		t.Errorf("secret with dashes was rejected: %q", withDashes)
	}
	if _, ok, _ := VerifyCode(rfc4226Secret, "12345", 0, 6, 0); ok {
		t.Error("short code accepted")
	}
	if _, ok, _ := VerifyCode(rfc4226Secret, code, 0, 6, 0); !ok {
		t.Error("valid code rejected")
	}
}

func TestProvisioningURL(t *testing.T) {
	u := ProvisioningURL("Gitus", "alice@example.com", TYPE_TOTP, "ABCDEF", 6, 30, 0)
	if !strings.HasPrefix(u, "otpauth://totp/Gitus:alice@example.com?") {
		t.Fatalf("totp url = %s", u)
	}
	for _, part := range []string{"secret=ABCDEF", "issuer=Gitus", "algorithm=SHA1", "digits=6", "period=30"} {
		if !strings.Contains(u, part) { t.Errorf("totp url is missing %q: %s", part, u) }
	}
	h := ProvisioningURL("Gitus", "alice", TYPE_HOTP, "ABCDEF", 6, 30, 7)
	if !strings.HasPrefix(h, "otpauth://hotp/Gitus:alice?") {
		t.Fatalf("hotp url = %s", h)
	}
	if !strings.Contains(h, "counter=7") { t.Errorf("hotp url is missing the counter: %s", h) }
	if strings.Contains(h, "period=") { t.Errorf("hotp url should not have a period: %s", h) }
}
