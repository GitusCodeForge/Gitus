package session

import (
	"crypto/rand"
	"math/big"
)

type GitusSession struct {
	Username string `json:"username"`
	Id string `json:"id"`
	Timestamp int64 `json:"timestamp"`
	ExpireTimestamp int64 `json:"expireTimestamp"`
	CSRFToken string `json:"csrf_token"`
}

type GitusSessionStore interface {
	Install() error
	Dispose() error
	IsSessionStoreUsable() (bool, error)
	RegisterSession(username string, session string) (*GitusSession, error)
	RetrieveSession(username string) ([]*GitusSession, error)
	RetrieveSessionByKey(username string, session string) (*GitusSession, error)
	RevokeSession(username string, target string) error
	RevokeAllSession(username string) error
	VerifySessionExist(username string, target string) (bool, error)
	VerifySessionFull(username string, session_id string, csrf string) (bool, error)
}

const passchdict = "abcdefghijklmnopqrstuvwxyz0123456789"
func NewSessionString() string {
	res := make([]byte, 0)
	rmax := big.NewInt(int64(len(passchdict)))
	for range 48 {
		n, _ := rand.Int(rand.Reader, rmax)
		res = append(res, passchdict[n.Uint64()])
	}
	return string(res)
}


