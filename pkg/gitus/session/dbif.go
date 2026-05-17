package session

import (
	"math/rand"
)

type GitusSession struct {
	Username string `json:"username"`
	Id string `json:"id"`
	Timestamp int64 `json:"timestamp"`
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
	VerifySessionExist(username string, target string) (bool, error)
	VerifySessionFull(username string, session_id string, csrf string) (bool, error)
}

const passchdict = "abcdefghijklmnopqrstuvwxyz0123456789"
func NewSessionString() string {
	res := make([]byte, 0)
	for range 48 {
		res = append(res, passchdict[rand.Intn(len(passchdict))])
	}
	return string(res)
}


