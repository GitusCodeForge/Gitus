package otp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// Recovery codes are the way back in when the phone with the authenticator app
// is gone. They are shown once, stored as digests, and each one works only
// once.
const (
	// RecoveryCodeCount is how many codes a set contains.
	RecoveryCodeCount = 10
	// recoveryCodeLength is the number of characters per code; with this
	// alphabet that is a bit under 60 bits of entropy.
	recoveryCodeLength = 12
	// recoveryAlphabet leaves out 0/o, 1/i/l and u/v to make the codes easier
	// to read from the screen and to type.
	recoveryAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"
)

// NormalizeRecoveryCode lower cases a code and drops the separators, so
// "abcd-efgh-ijkl", "ABCD EFGH IJKL" and "abcdefghijkl" are the same code.
func NormalizeRecoveryCode(s string) string {
	return strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(strings.ToLower(strings.TrimSpace(s)))
}

// HashRecoveryCode returns the digest that is stored in the user row. The codes
// have enough entropy that a fast hash is enough; they are single use and the
// input is rate limited at login.
func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(NormalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}

// GenerateRecoveryCodes returns count codes, formatted as xxxx-xxxx-xxxx, and
// their digests ready to be stored.
func GenerateRecoveryCodes(count int) ([]string, []string, error) {
	if count <= 0 { count = RecoveryCodeCount }
	plain := make([]string, 0, count)
	hashes := make([]string, 0, count)
	alphabetLen := big.NewInt(int64(len(recoveryAlphabet)))
	for range count {
		var b strings.Builder
		for i := range recoveryCodeLength {
			if i > 0 && i%4 == 0 { b.WriteByte('-') }
			n, err := rand.Int(rand.Reader, alphabetLen)
			if err != nil { return nil, nil, err }
			b.WriteByte(recoveryAlphabet[n.Int64()])
		}
		code := b.String()
		plain = append(plain, code)
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return plain, hashes, nil
}

// VerifyRecoveryCode returns the digest of the code when it matches one of the
// stored digests.
func VerifyRecoveryCode(hashes []string, code string) (string, bool) {
	normalized := NormalizeRecoveryCode(code)
	if len(normalized) <= 0 { return "", false }
	h := HashRecoveryCode(normalized)
	var matched string
	found := 0
	for _, stored := range hashes {
		if subtle.ConstantTimeCompare([]byte(stored), []byte(h)) == 1 {
			matched = stored
			found = 1
		}
	}
	return matched, found == 1
}

// RemoveRecoveryCode returns the list without the used digest.
func RemoveRecoveryCode(hashes []string, used string) []string {
	res := make([]string, 0, len(hashes))
	for _, h := range hashes {
		if h == used { continue }
		res = append(res, h)
	}
	return res
}

// FormatRecoveryCode is a convenience for tests and callers that build codes by
// hand.
func FormatRecoveryCode(s string) (string, error) {
	normalized := NormalizeRecoveryCode(s)
	if len(normalized) != recoveryCodeLength {
		return "", fmt.Errorf("a recovery code has %d characters", recoveryCodeLength)
	}
	return normalized[0:4] + "-" + normalized[4:8] + "-" + normalized[8:12], nil
}
