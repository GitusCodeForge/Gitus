package redis_like

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strconv"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session"
	"github.com/redis/go-redis/v9"
)

/* NOTE: currently (2026.4.16) a user's sessions exist in two parts in redis:
   + `{prefix}:session_list:{user_name}`
     + this one is a hashmap that stores `{session_id}: {timestamp}`
   + `{prefix}:session:{user_name}:{session_id}`
     + this one is a hashmap that stores everything else, including csrf token
*/

type GitusRedisLikeSessionStore struct {
	config *gitus.GitusConfig
	connection *redis.Client
}

func NewGitusRedisLikeSessionStore(cfg *gitus.GitusConfig) (*GitusRedisLikeSessionStore, error) {
	c := redis.NewClient(&redis.Options{
		Addr: cfg.Session.Host,
		Username: cfg.Session.UserName,
		Password: cfg.Session.Password,
		DB: cfg.Session.DatabaseNumber,
	})
	return &GitusRedisLikeSessionStore{
		config: cfg,
		connection: c,
	}, nil
}

func (ssif *GitusRedisLikeSessionStore) Install() error {
	return nil
}

func (ssif *GitusRedisLikeSessionStore) IsSessionStoreUsable() (bool, error) {
	return true, nil
}

func (ssif *GitusRedisLikeSessionStore) Dispose() error {
	return ssif.connection.Close()
}

func (ssif *GitusRedisLikeSessionStore) RegisterSession(name string, s string) (*session.GitusSession, error) {
	lkey := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, name)
	skey := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, name, s)
	ctx := context.TODO()
	t := time.Now().Unix()
	tExp := t + int64(ssif.config.MaxSessionLifetime)
	timestampStr := fmt.Sprintf("%d", t)
	tExpStr := fmt.Sprintf("%d", tExp)
	csrf := auxfuncs.CryptoGenSym(64)
	r1 := ssif.connection.HSet(ctx, lkey, s, timestampStr)
	if r1.Err() != nil { return nil, r1.Err() }
	r2 := ssif.connection.HSet(ctx, skey, "csrf", csrf)
	if r2.Err() != nil { return nil, r2.Err() }
	r3 := ssif.connection.HSet(ctx, skey, "timestamp", timestampStr)
	if r3.Err() != nil { return nil, r3.Err() }
	r4 := ssif.connection.HSet(ctx, skey, "expire_timestamp", tExpStr)
	if r4.Err() != nil { return nil, r4.Err() }
	return &session.GitusSession{
		Username: name,
		Id: s,
		Timestamp: t,
		ExpireTimestamp: tExp,
		CSRFToken: csrf,
	}, nil
}

func (ssif *GitusRedisLikeSessionStore) RetrieveSession(name string) ([]*session.GitusSession, error) {
	key := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, name)
	// NOTE (for people who're unfamiliar with redis): *SCAN commands (as
	// told by Redis's documentations) does not guarantee the number of
	// values and you "should not consider the iteration complete as long
	// as the returned cursor is not zero".
	res := make([]*session.GitusSession, 0)
	removePending := make([]string, 0)
	lastCursor := uint64(0)
	for {
		cmd := ssif.connection.HScan(context.TODO(), key, uint64(lastCursor), "*", 0)
		keys, cursor, err := cmd.Result()
		if err != nil { return nil, err }
		i := 0
		lenk := len(keys)
		for i < lenk {
			sk := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, name, keys[i])
			cmd2 := ssif.connection.HGet(context.TODO(), sk, "csrf")
			csrf, err := cmd2.Result()
			if err != nil { return nil, err }
			cmd3 := ssif.connection.HGet(context.TODO(), sk, "timestamp")
			tsstr, err := cmd3.Result()
			if err != nil { return nil, err }
			timestamp, _ := strconv.ParseInt(tsstr, 10, 64)
			cmd4 := ssif.connection.HGet(context.TODO(), sk, "expire_timestamp")
			testr, err := cmd4.Result()
			if err != nil { return nil, err }
			tExp, _ := strconv.ParseInt(testr, 10, 64)
			if time.Now().Unix() >= tExp {
				// we don't directly call RevokeSession here because
				// i'm afraid of messing up the cursor and the loop...
				ssif.connection.Del(context.TODO(), sk)
				removePending = append(removePending, keys[i])
				continue
			}
			res = append(res, &session.GitusSession{
				Username: name,
				Id: keys[i],
				Timestamp: timestamp,
				ExpireTimestamp: tExp,
				CSRFToken: csrf,
			})
			i += 2
		}
		if cursor == 0 { break}
		lastCursor = cursor
	}
	for _, k := range removePending {
		ssif.connection.HDel(context.TODO(), key, k)
	}
	return res, nil
}

func (ssif *GitusRedisLikeSessionStore) RetrieveSessionByKey(username string, sessionid string) (*session.GitusSession, error) {
	key := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, username)
	cmd := ssif.connection.HGet(context.TODO(), key, sessionid)
	if cmd.Err() == redis.Nil { return nil, nil }
	if cmd.Err() != nil { return nil, cmd.Err() }
	key2 := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, username, sessionid)
	cmd2 := ssif.connection.HGet(context.TODO(), key2, "csrf")
	if cmd2.Err() != nil { return nil, cmd2.Err() }
	csrf, err := cmd2.Result()
	if err != nil { return nil, err }
	cmd3 := ssif.connection.HGet(context.TODO(), key2, "timestamp")
	if cmd3.Err() != nil { return nil, cmd3.Err() }
	tsStr, err := cmd3.Result()
	if err != nil { return nil, err }
	if len(tsStr) <= 0 { return nil, nil }
	cmd4 := ssif.connection.HGet(context.TODO(), key2, "expire_timestamp")
	testr, err := cmd4.Result()
	if err != nil { return nil, err }
	tExp, _ := strconv.ParseInt(testr, 10, 64)
	if time.Now().Unix() >= tExp {
		ssif.RevokeSession(username, sessionid)
		return nil, nil
	}
	timestamp, _ := strconv.ParseInt(tsStr, 10, 64)
	return &session.GitusSession{
		Username: username,
		Id: sessionid,
		Timestamp: timestamp,
		CSRFToken: csrf,
	}, nil
}

func (ssif *GitusRedisLikeSessionStore) RevokeSession(username string, target string) error {
	key := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, username)
	cmd := ssif.connection.HDel(context.TODO(), key, target)
	if cmd.Err() != nil { return cmd.Err() }
	key = fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, username, target)
	cmd2 := ssif.connection.Del(context.TODO(), key)
	if cmd2.Err() != nil { return cmd.Err() }
	return nil
}

func (ssif *GitusRedisLikeSessionStore) RevokeAllSession(username string) error {
	// NOTE (for people who're unfamiliar with redis): *SCAN commands (as
	// told by Redis's documentations) does not guarantee the number of
	// values and you "should not consider the iteration complete as long
	// as the returned cursor is not zero".
	key := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, username)
	lastCursor := uint64(0)
	for {
		cmd := ssif.connection.HScan(context.TODO(), key, uint64(lastCursor), "*", 0)
		keys, cursor, err := cmd.Result()
		if err != nil { return err }
		i := 0
		lenk := len(keys)
		for i < lenk {
			sk := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, username, keys[i])
			cmd2 := ssif.connection.Del(context.TODO(), sk)
			if cmd2.Err() != nil { return cmd2.Err() }
		}
		if cursor == 0 { break}
		lastCursor = cursor
	}
	cmd := ssif.connection.Del(context.TODO(), key)
	if cmd.Err() != nil { return cmd.Err() }
	return nil
}

func (ssif *GitusRedisLikeSessionStore) VerifySessionExist(name string, target string) (bool, error) {
	key := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, name)
	cmd := ssif.connection.HGet(context.TODO(), key, target)
	if cmd.Err() == redis.Nil { return false, nil }
	if cmd.Err() != nil { return false, cmd.Err() }
	r, err := cmd.Result()
	if err != nil { return false, err }
	if len(r) <= 0 { return false, nil }
	key2 := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, name, target)
	cmd2 := ssif.connection.HGet(context.TODO(), key2, "expire_timestamp")
	if cmd2.Err() == redis.Nil { return false, nil }
	if cmd2.Err() != nil { return false, cmd2.Err() }
	r2, err := cmd2.Result()
	if err != nil { return false, err }
	tExp, _ := strconv.ParseInt(r2, 10, 64)
	if time.Now().Unix() >= tExp {
		ssif.RevokeSession(name, target)
		return false, nil
	}
	return true, nil
}

func (ssif *GitusRedisLikeSessionStore) VerifySessionFull(name string, session_id string, csrf string) (bool, error) {
	key := fmt.Sprintf("%s:session_list:%s", ssif.config.Session.TablePrefix, name)
	cmd := ssif.connection.HGet(context.TODO(), key, session_id)
	if cmd.Err() == redis.Nil { return false, nil }
	if cmd.Err() != nil { return false, cmd.Err() }
	r, err := cmd.Result()
	if err != nil { return false, err }
	if len(r) <= 0 { return false, nil }
	key2 := fmt.Sprintf("%s:session:%s:%s", ssif.config.Session.TablePrefix, name, session_id)
	cmd2 := ssif.connection.HGet(context.TODO(), key2, "expire_timestamp")
	if cmd2.Err() == redis.Nil { return false, nil }
	if cmd2.Err() != nil { return false, cmd2.Err() }
	r2, err := cmd2.Result()
	if err != nil { return false, err }
	tExp, _ := strconv.ParseInt(r2, 10, 64)
	if time.Now().Unix() >= tExp {
		ssif.RevokeSession(name, session_id)
		return false, nil
	}
	cmd3 := ssif.connection.HGet(context.TODO(), key2, "csrf")
	if cmd3.Err() == redis.Nil { return false, nil }
	if cmd3.Err() != nil { return false, cmd2.Err() }
	r3, err := cmd3.Result()
	if err != nil { return false, err }
	if subtle.ConstantTimeCompare([]byte(r3), []byte(csrf)) == 0 { return false, nil }
	return true, nil
}

