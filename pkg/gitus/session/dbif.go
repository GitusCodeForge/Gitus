package session

import (
	"crypto/rand"
	"math/big"
	"strings"
)

type GitusSession struct {
	Username string `json:"username"`
	Id string `json:"id"`
	Timestamp int64 `json:"timestamp"`
	ExpireTimestamp int64 `json:"expireTimestamp"`
	CSRFToken string `json:"csrf_token"`
	// IP and UserAgent describe the client that created the session. They
	// are recorded once and never updated; see GitusSessionMetadata.
	IP string `json:"ip"`
	UserAgent string `json:"userAgent"`
}

// GitusSessionMetadata describes the client that asked for a session. It is
// recorded when the session is registered, so the user can tell their active
// sessions apart later.
type GitusSessionMetadata struct {
	// IP is the address the request came from, as resolved by
	// routes.ResolveMostPossibleIP. It can be a proxy address.
	IP string
	// UserAgent is the raw User-Agent header. It is rendered through
	// DescribeUserAgent.
	UserAgent string
}

type GitusSessionStore interface {
	Install() error
	// EnsureSchema applies the changes made to the schema after the store was
	// installed. It is idempotent and runs once at startup, so an instance
	// upgraded in place does not lose its sessions.
	EnsureSchema() error
	Dispose() error
	IsSessionStoreUsable() (bool, error)
	RegisterSession(username string, session string, metadata GitusSessionMetadata) (*GitusSession, error)
	RetrieveSession(username string) ([]*GitusSession, error)
	RetrieveSessionByKey(username string, session string) (*GitusSession, error)
	RevokeSession(username string, target string) error
	RevokeAllSession(username string) error
	VerifySessionExist(username string, target string) (bool, error)
	VerifySessionFull(username string, session_id string, csrf string) (bool, error)
}

const passchdict = "abcdefghijklmnopqrstuvwxyz0123456789"
func NewSessionString() (string, error) {
	res := make([]byte, 0)
	rmax := big.NewInt(int64(len(passchdict)))
	for range 48 {
		n, err := rand.Int(rand.Reader, rmax)
		if err != nil { return "", err }
		res = append(res, passchdict[n.Uint64()])
	}
	return string(res), nil
}

// DescribeUserAgent returns a short description of the client a User-Agent
// belongs to, e.g. "Firefox on Linux". It is a small heuristic meant to help
// telling sessions apart, not a full User-Agent database; an unknown or empty
// agent is shown as-is (truncated) or as "unknown client".
func DescribeUserAgent(ua string) string {
	s := strings.ToLower(ua)
	browser := ""
	switch {
	case strings.Contains(s, "ladybird"): browser = "Ladybird"
	case strings.Contains(s, "k-meleon"): browser = "K-Meleon"
	case strings.Contains(s, "netsurf"): browser = "NetSurf"
	case strings.Contains(s, "lynx"): browser = "Lynx"
	case strings.Contains(s, "w3m"): browser = "w3m"
	case strings.Contains(s, "links"): browser = "Links"
	case strings.Contains(s, "edg/"): browser = "Edge"
	case strings.Contains(s, "firefox/"): browser = "Firefox"
	// Chrome's User-Agent contains "Chrome/" and "Safari/", and Edge's
	// contains both as well; the order above handles that.
	case strings.Contains(s, "chrome/") || strings.Contains(s, "chromium/"): browser = "Chrome"
	case strings.Contains(s, "safari/"): browser = "Safari"
	case strings.Contains(s, "curl/"): browser = "curl"
	case strings.Contains(s, "wget/"): browser = "wget"
	}
	platform := ""
	switch {
	case strings.Contains(s, "android"): platform = "Android"
	case strings.Contains(s, "iphone"), strings.Contains(s, "ipad"), strings.Contains(s, "ipod"): platform = "iOS"
	case strings.Contains(s, "windows"): platform = "Windows"
	case strings.Contains(s, "mac os"), strings.Contains(s, "macintosh"): platform = "macOS"
	case strings.Contains(s, "linux"): platform = "Linux"
	}
	switch {
	case len(browser) > 0 && len(platform) > 0: return browser + " on " + platform
	case len(browser) > 0: return browser
	case len(platform) > 0: return platform
	}
	if len(strings.TrimSpace(ua)) <= 0 { return "unknown client" }
	if len(ua) > 60 { return ua[:60] + "..." }
	return ua
}
