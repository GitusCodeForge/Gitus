package otp

import (
	"strings"
	"testing"
)

func TestGenerateRecoveryCodes(t *testing.T) {
	plain, hashes, err := GenerateRecoveryCodes(RecoveryCodeCount)
	if err != nil { t.Fatal(err) }
	if len(plain) != RecoveryCodeCount || len(hashes) != RecoveryCodeCount {
		t.Fatalf("got %d codes and %d hashes", len(plain), len(hashes))
	}
	seen := make(map[string]bool)
	for i, c := range plain {
		if len(c) != 14 { t.Fatalf("code %q has length %d; want 14 (xxxx-xxxx-xxxx)", c, len(c)) }
		if strings.Count(c, "-") != 2 { t.Fatalf("code %q is not grouped", c) }
		if seen[c] { t.Fatalf("duplicate code %q", c) }
		seen[c] = true
		if hashes[i] != HashRecoveryCode(c) { t.Fatalf("hash %d does not match its code", i) }
		if _, ok := VerifyRecoveryCode(hashes, c); !ok { t.Fatalf("code %q does not verify", c) }
	}
	for _, c := range []string{"", "0000-0000-0000", "not a code"} {
		if _, ok := VerifyRecoveryCode(hashes, c); ok { t.Fatalf("junk %q verified", c) }
	}
}

func TestRecoveryCodeNormalization(t *testing.T) {
	code := "abcd-efgh-jkmn"
	if _, ok := VerifyRecoveryCode([]string{HashRecoveryCode(code)}, "ABCD EFGH JKMN"); !ok {
		t.Error("upper case code with spaces did not verify")
	}
	if _, ok := VerifyRecoveryCode([]string{HashRecoveryCode(code)}, " abcdefghjkmn "); !ok {
		t.Error("code without separators did not verify")
	}
	if NormalizeRecoveryCode("ABCD-EFGH-JKMN") != "abcdefghjkmn" {
		t.Errorf("NormalizeRecoveryCode = %q", NormalizeRecoveryCode("ABCD-EFGH-JKMN"))
	}
}

func TestRecoveryCodeSingleUse(t *testing.T) {
	_, hashes, err := GenerateRecoveryCodes(3)
	if err != nil { t.Fatal(err) }
	plain := []string{"2345-6789-abcd", "efgh-jkmn-pqrs", "tuvw-xyz2-3456"}
	hashes = hashes[:0]
	for _, c := range plain { hashes = append(hashes, HashRecoveryCode(c)) }

	used, ok := VerifyRecoveryCode(hashes, plain[1])
	if !ok { t.Fatal("the code did not verify") }
	hashes = RemoveRecoveryCode(hashes, used)
	if _, ok := VerifyRecoveryCode(hashes, plain[1]); ok {
		t.Error("a used code still verifies")
	}
	if _, ok := VerifyRecoveryCode(hashes, plain[0]); !ok {
		t.Error("removing one code removed another")
	}
	if got := RemoveRecoveryCode(nil, "x"); len(got) != 0 {
		t.Errorf("RemoveRecoveryCode(nil) = %v", got)
	}
}

func TestFormatRecoveryCode(t *testing.T) {
	if got, err := FormatRecoveryCode("abcdefghjkmn"); err != nil || got != "abcd-efgh-jkmn" {
		t.Fatalf("FormatRecoveryCode = %q, %v", got, err)
	}
	if _, err := FormatRecoveryCode("short"); err == nil {
		t.Error("a short code was formatted")
	}
}
