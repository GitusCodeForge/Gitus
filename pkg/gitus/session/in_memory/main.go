package in_memory

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session"
	"github.com/GitusCodeForge/Gitus/pkg/tcache"
)

type GitusInMemorySessionStore struct {
	config *gitus.GitusConfig
	cache *tcache.TCache
}

func NewGitusInMemorySessionStore(cfg *gitus.GitusConfig) (*GitusInMemorySessionStore, error) {
	c := tcache.NewTCache(24 * time.Hour)
	return &GitusInMemorySessionStore{
		config: cfg,
		cache: c,
	}, nil
}

func (ssif *GitusInMemorySessionStore) Install() error {
	return nil
}

func (ssif *GitusInMemorySessionStore) IsSessionStoreUsable() (bool, error) {
	return true, nil
}

func (ssif *GitusInMemorySessionStore) Dispose() error {
	return nil
}

func insertSet(set string, s string) string {
	if inSet(set, s) {
		return set
	} else {
		return fmt.Sprintf("%s{%s}", set, s)
	}
}
func inSet(set string, s string) bool {
	return strings.Contains(set, fmt.Sprintf("{%s}", s))
}
func removeFromSet(set string, s string) string {
	ss := fmt.Sprintf("{%s}", s)
	i := strings.Index(set, ss)
	if i <= -1 { return set }
	return set[0:i] + set[i+len(ss):]
}

func (ssif *GitusInMemorySessionStore) RegisterSession(name string, session_id string) (*session.GitusSession, error) {
	key := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, name)
	i, _ := ssif.cache.Get(key)
	ssif.cache.Register(key, insertSet(i, session_id), 24*time.Hour)
	key2 := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, name, session_id)
	timestamp := time.Now().Unix()
	tExp := timestamp + int64(ssif.config.MaxSessionLifetime)
	csrf := auxfuncs.CryptoGenSym(64)
	s := &session.GitusSession{
		Username: name,
		Id: session_id,
		Timestamp: timestamp,
		ExpireTimestamp: tExp,
		CSRFToken: csrf,
	}
	sstr, err := json.Marshal(s)
	if err != nil { return nil, err }
	// this is possibly the only case where a sessionid register twice
	// makes sense:
	// + we plan to support multiple session, which we may make into
	//   not having an expiration datetime a la github;
	// + memcached does not support sets like redis, so the way is
	//   to store all of them as a long string, which would subject us
	//   to the size limit of values, which is 1MB, which considering
	//   the length of each session key and how many sessions there
	//   *typically* will be, should be plenty enough.
	// + we still want easy check for each session key instead of
	//   deserializing the long string every time.
	ssif.cache.Register(key2, string(sstr), 24*time.Hour)
	return s, nil
}

func (ssif *GitusInMemorySessionStore) RetrieveSession(name string) ([]*session.GitusSession, error) {
	groupKey := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, name)
	i, _ := ssif.cache.Get(groupKey)
	newset := ""
	res := make([]*session.GitusSession, 0)
	for k := range strings.SplitSeq(string(i[1:]), "}{") {
		kk := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, name, k)
		v, _ := ssif.cache.Get(kk)
		var r *session.GitusSession
		err := json.Unmarshal([]byte(v), &r)
		if err != nil { return nil, err }
		if v != "" {
			if time.Now().Unix() >= r.ExpireTimestamp {
				ssif.cache.Delete(kk)
				continue
			} else {
				res = append(res, r)
				newset = fmt.Sprintf("%s{%s}", newset, k)
			}
		}
	}
	if newset == "" {
		ssif.cache.Delete(groupKey)
	} else {
		ssif.cache.Register(groupKey, newset, 24*time.Hour)
	}
	return res, nil
}

func (ssif *GitusInMemorySessionStore) RetrieveSessionByKey(username string, sessionid string) (*session.GitusSession, error) {
	key := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, username, sessionid)
	i, _ := ssif.cache.Get(key)
	var r *session.GitusSession
	err := json.Unmarshal([]byte(i), r)
	if err != nil { return nil, err }
	if time.Now().Unix() >= r.ExpireTimestamp {
		ssif.cache.Delete(key)
		return nil, nil
	}
	return r, nil
}

func (ssif *GitusInMemorySessionStore) RevokeSession(username string, target string) error {
	// NOTE: we don't have transaction semantics here, which could be
	// a problem down the line.
	// TODO: attempt to fix this.
	key := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, username, target)
	ssif.cache.Delete(key)
	sessionSetKey := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, username)
	i, ok := ssif.cache.Get(sessionSetKey)
	if !ok { return nil }
	i = removeFromSet(i, target)
	ssif.cache.Register(sessionSetKey, i, 24*time.Hour)
	return nil
}

func (ssif *GitusInMemorySessionStore) RevokeAllSession(username string) error {
	groupKey := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, username)
	i, _ := ssif.cache.Get(groupKey)
	for k := range strings.SplitSeq(string(i[1:]), "}{") {
		kk := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, username, k)
		ssif.cache.Delete(kk)
	}
	ssif.cache.Delete(groupKey)
	return nil
}

func (ssif *GitusInMemorySessionStore) VerifySessionExist(name string, target string) (bool, error) {
	key1 := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, name)
	s, ok := ssif.cache.Get(key1)
	if !ok { return false, nil }
	if !inSet(s, target) { return false, nil }
	key := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, name, target)
	i, _ := ssif.cache.Get(key)
	if i == "" { return false, nil }
	var r *session.GitusSession
	err := json.Unmarshal([]byte(i), &r)
	if err != nil { return false, err }
	if time.Now().Unix() > r.ExpireTimestamp {
		ssif.cache.Delete(key)
		return false, nil
	}
	return true, nil
}

func (ssif *GitusInMemorySessionStore) VerifySessionFull(name string, target string, csrf string) (bool, error) {
	key1 := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, name)
	s, ok := ssif.cache.Get(key1)
	if !ok { return false, nil }
	if !inSet(s, target) { return false, nil }
	key := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, name, target)
	i, _ := ssif.cache.Get(key)
	if i == "" { return false, nil }
	var r *session.GitusSession
	err := json.Unmarshal([]byte(i), &r)
	if err != nil { return false, err }
	if time.Now().Unix() > r.ExpireTimestamp {
		ssif.cache.Delete(key)
		return false, nil
	}
	if subtle.ConstantTimeCompare([]byte(r.CSRFToken), []byte(csrf)) == 0 { return false, nil }
	return true, nil
}

