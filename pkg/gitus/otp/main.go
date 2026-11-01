// Package otp implements the one-time passwords used by authenticator apps:
// HOTP (RFC 4226) and TOTP (RFC 6238), both with SHA-1, which is what every
// authenticator app supports.
package otp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	TYPE_TOTP = "totp"
	TYPE_HOTP = "hotp"

	DefaultDigits = 6
	DefaultPeriod = 30

	// SecretBytes is the size of the shared secret. RFC 4226 requires at
	// least 128 bits and recommends 160, which is also the HMAC-SHA1 block
	// size.
	SecretBytes = 20
)

var base32NoPadding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a new random secret, base32 encoded the way
// authenticator apps expect it (upper case, no padding).
func GenerateSecret() (string, error) {
	b := make([]byte, SecretBytes)
	if _, err := rand.Read(b); err != nil { return "", err }
	return base32NoPadding.EncodeToString(b), nil
}

// decodeSecret accepts the secret as it is stored or as a user typed it:
// lower case, with spaces or dashes, with or without padding.
func decodeSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(secret))
	s = strings.NewReplacer(" ", "", "-", "").Replace(s)
	if len(s) <= 0 { return nil, fmt.Errorf("empty secret") }
	if m := len(s) % 8; m != 0 { s = s + strings.Repeat("=", 8-m) }
	return base32.StdEncoding.DecodeString(s)
}

// NormalizeCode strips the separators people put in codes, so "123 456" and
// "123-456" are the same code as "123456".
func NormalizeCode(s string) string {
	return strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(strings.TrimSpace(s))
}

func normalizeDigits(digits int) (int, error) {
	if digits <= 0 { return DefaultDigits, nil }
	if digits < 6 || digits > 10 { return 0, fmt.Errorf("unsupported digit count %d", digits) }
	return digits, nil
}

func normalizePeriod(period int) int {
	if period <= 0 { return DefaultPeriod }
	return period
}

func hotpCode(key []byte, counter uint64, digits int) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	// dynamic truncation, RFC 4226 section 5.3.
	offset := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(1)
	for range digits { mod *= 10 }
	return fmt.Sprintf("%0*d", digits, bin%mod)
}

// Code returns the HOTP code for counter.
func Code(secret string, counter uint64, digits int) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil { return "", err }
	d, err := normalizeDigits(digits)
	if err != nil { return "", err }
	return hotpCode(key, counter, d), nil
}

// TOTPCode returns the TOTP code for the time step t falls in.
func TOTPCode(secret string, t time.Time, period int, digits int) (string, error) {
	return Code(secret, TimeCounter(t, period), digits)
}

// TimeCounter returns the TOTP counter of the time step t falls in.
func TimeCounter(t time.Time, period int) uint64 {
	return uint64(t.Unix() / int64(normalizePeriod(period)))
}

// VerifyCode checks a HOTP code against the counters starting at counter and
// looking window steps ahead. It returns the counter that matched.
func VerifyCode(secret string, code string, counter uint64, digits int, window int) (uint64, bool, error) {
	key, err := decodeSecret(secret)
	if err != nil { return 0, false, err }
	d, err := normalizeDigits(digits)
	if err != nil { return 0, false, err }
	code = NormalizeCode(code)
	if len(code) != d { return 0, false, nil }
	if window < 0 { window = 0 }
	for i := uint64(0); i <= uint64(window); i++ {
		if subtle.ConstantTimeCompare([]byte(hotpCode(key, counter+i, d)), []byte(code)) == 1 {
			return counter + i, true, nil
		}
	}
	return 0, false, nil
}

// VerifyTOTP checks a TOTP code against the time steps within window of now
// (backwards and forwards, so a slightly wrong clock still works). It returns
// the counter that matched.
func VerifyTOTP(secret string, code string, now time.Time, period int, digits int, window int) (uint64, bool, error) {
	key, err := decodeSecret(secret)
	if err != nil { return 0, false, err }
	d, err := normalizeDigits(digits)
	if err != nil { return 0, false, err }
	code = NormalizeCode(code)
	if len(code) != d { return 0, false, nil }
	if window < 0 { window = 0 }
	counter := TimeCounter(now, period)
	for i := -window; i <= window; i++ {
		c := int64(counter) + int64(i)
		if c < 0 { continue }
		if subtle.ConstantTimeCompare([]byte(hotpCode(key, uint64(c), d)), []byte(code)) == 1 {
			return uint64(c), true, nil
		}
	}
	return 0, false, nil
}

// ProvisioningURL returns the otpauth:// URL an authenticator app understands.
// Gitus does not render the QR code itself, so the URL (or the secret) is what
// the user copies into the app.
func ProvisioningURL(issuer string, account string, typ string, secret string, digits int, period int, counter uint64) string {
	if typ != TYPE_HOTP { typ = TYPE_TOTP }
	d, err := normalizeDigits(digits)
	if err != nil { d = DefaultDigits }
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", strconv.Itoa(d))
	if typ == TYPE_HOTP {
		v.Set("counter", strconv.FormatUint(counter, 10))
	} else {
		v.Set("period", strconv.Itoa(normalizePeriod(period)))
	}
	return "otpauth://" + typ + "/" + label + "?" + v.Encode()
}
