package memcached

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session"
	"github.com/bradfitz/gomemcache/memcache"
)

/* NOTE(2026.4.16: memcached is a string kv store. here we do things similar
   to redis but we'll use JSON.
*/

type GitusMemcachedSessionStore struct {
	config *gitus.GitusConfig
	connection *memcache.Client
}

func NewGitusMemcachedSessionStore(cfg *gitus.GitusConfig) (*GitusMemcachedSessionStore, error) {
	c := memcache.New(cfg.Session.Host)
	return &GitusMemcachedSessionStore{
		config: cfg,
		connection: c,
	}, nil
}

func (ssif *GitusMemcachedSessionStore) Install() error {
	return nil
}

func (ssif *GitusMemcachedSessionStore) IsSessionStoreUsable() (bool, error) {
	return true, nil
}

func (ssif *GitusMemcachedSessionStore) Dispose() error {
	return ssif.connection.Close()
}

func insertSet(set []byte, s string) []byte {
	if inSet(set, s) {
		return set
	} else {
		if len(set) <= 0 { return []byte(s) }
		return fmt.Appendf(set, ",%s", s)
	}
}
func inSet(set []byte, s string) bool {
	if len(set) <= 0 { return false }
	for item := range strings.SplitSeq(string(set), ",") {
		if item == s { return true }
	}
	return false
}
func removeFromSet(set []byte, s string) []byte {
	ss := strings.Split(string(set), ",")
	targetI := -1
	for i, k := range ss {
		if k == s { targetI = i; break }
	}
	if targetI == -1 { return set }
	ress := slices.Delete(ss, targetI, targetI+1)
	return []byte(strings.Join(ress, ","))
}

func (ssif *GitusMemcachedSessionStore) RegisterSession(name string, session_id string) (*session.GitusSession, error) {
	sessionSetKey := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, name)
	i, err := ssif.connection.Get(sessionSetKey)
	if err != nil {
		// cache miss is memcached's way of saying the key not found...
		if err != memcache.ErrCacheMiss { return nil, err }
		i = &memcache.Item{
			Key: sessionSetKey,
			Value: []byte(session_id),
			Flags: 0,
			Expiration: 0,
		}
	} else {
		i.Value = insertSet(i.Value, session_id)
	}
	err = ssif.connection.Set(i)
	if err != nil { return nil, err }
	
	key := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, name, session_id)
	csrf := auxfuncs.CryptoGenSym(64)
	t := time.Now().Unix()
	tExp := t + int64(ssif.config.MaxSessionLifetime)
	ss := &session.GitusSession{
		Username: name,
		Id: session_id,
		Timestamp: t,
		ExpireTimestamp: tExp,
		CSRFToken: csrf,
	}
	ssstr, err := json.Marshal(ss)
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
	err = ssif.connection.Set(&memcache.Item{
		Key: key,
		Value: ssstr,
		Flags: 0,
		Expiration: 0,
	})
	if err != nil { return nil, err }
	return ss, nil
}

func (ssif *GitusMemcachedSessionStore) RetrieveSession(name string) ([]*session.GitusSession, error) {
	key := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, name)
	i, err := ssif.connection.Get(key)
	iSet := i.Value
	res := make([]*session.GitusSession, 0)
	if err == memcache.ErrCacheMiss { return res, nil }
	if err != nil { return nil, err }
	for k := range strings.SplitSeq(string(i.Value), ",") {
		kk := fmt.Sprintf("%s:session:%s:%s", key, name, k)
		v, err := ssif.connection.Get(kk)
		var val []byte
		if err != nil {
			val = []byte("{}")
		} else {
			val = v.Value
		}
		var ss *session.GitusSession
		err = json.Unmarshal(val, ss)
		if err != nil { return nil, err }
		if time.Now().Unix() >=  ss.ExpireTimestamp {
			iSet = removeFromSet(iSet, k)
			ssif.connection.Delete(kk)
			continue
		}
		ss.Username = name
		if ss.Id != "" { res = append(res, ss) }
	}
	// NOTE: this (together w/ the removeFromSet call) is meant to be
	// an automated bookkeeping action...
	// TODO: check if this is actually safe
	i.Value = iSet
	ssif.connection.Set(i)
	return res, nil
}

func (ssif *GitusMemcachedSessionStore) RetrieveSessionByKey(username string, sessionid string) (*session.GitusSession, error) {
	key := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, username, sessionid)
	i, err := ssif.connection.Get(key)
	if err == memcache.ErrCacheMiss { return nil, nil }
	if err != nil { return nil, err }
	if len(i.Value) <= 0 { return nil, nil }
	var ss *session.GitusSession
	err = json.Unmarshal(i.Value, ss)
	if err != nil { return nil, err }
	if time.Now().Unix() >= ss.ExpireTimestamp {
		ssif.connection.Delete(key)
		return nil, nil
	}
	return ss, nil
}

func (ssif *GitusMemcachedSessionStore) RevokeSession(username string, target string) error {
	// NOTE: we don't have transaction semantics here, which could be
	// a problem down the line.
	// TODO: attempt to fix this.
	sessionSetKey := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, username)
	i, err := ssif.connection.Get(sessionSetKey)
	if err == memcache.ErrCacheMiss { return nil }
	if err != nil { return err }
	i.Value = removeFromSet(i.Value, target)
	err = ssif.connection.Set(i)
	if err != nil { return err }
	key := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, username, target)
	err = ssif.connection.Delete(key)
	if err != nil && err != memcache.ErrCacheMiss { return err }
	return nil
}

func (ssif *GitusMemcachedSessionStore) RevokeAllSession(username string) error {
	// NOTE: we don't have transaction semantics here, which could be
	// a problem down the line.
	// TODO: attempt to fix this.
	key := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, username)
	i, err := ssif.connection.Get(key)
	if err == memcache.ErrCacheMiss { return nil }
	if err != nil { return err }
	for k := range strings.SplitSeq(string(i.Value), ",") {
		kk := fmt.Sprintf("%s:session:%s:%s", key, username, k)
		err := ssif.connection.Delete(kk)
		if err != nil && err != memcache.ErrCacheMiss { return err }
	}
	err = ssif.connection.Delete(key)
	if err != nil && err != memcache.ErrCacheMiss { return err }
	return nil
}


func (ssif *GitusMemcachedSessionStore) VerifySessionExist(name string, target string) (bool, error) {
	key := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, name, target)
	i, err := ssif.connection.Get(key)
	if err == memcache.ErrCacheMiss { return false, nil }
	if err != nil { return false, err }
	if len(i.Value) <= 0 { return false, nil }
	var ss *session.GitusSession
	err = json.Unmarshal(i.Value, ss)
	if err != nil { return false, err }
	if time.Now().Unix() >= ss.ExpireTimestamp {
		ssif.connection.Delete(key)
		return false, nil
	}
	return true, nil
}

func (ssif *GitusMemcachedSessionStore) VerifySessionFull(username string, session_id string, csrf string) (bool, error) {
	key := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, username, session_id)
	i, err := ssif.connection.Get(key)
	if err == memcache.ErrCacheMiss { return false, nil }
	if err != nil { return false, err }
	if len(i.Value) <= 0 { return false, nil }
	var ss *session.GitusSession
	err = json.Unmarshal(i.Value, ss)
	if err != nil { return false, err }
	if time.Now().Unix() >= ss.ExpireTimestamp {
		ssif.connection.Delete(key)
		return false, nil
	}
	if subtle.ConstantTimeCompare([]byte(ss.CSRFToken), []byte(csrf)) == 0 { return false, nil }
	return true, nil
}









































