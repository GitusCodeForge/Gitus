package sqlite

import (
	"crypto/subtle"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session"
	_ "github.com/mattn/go-sqlite3"
)

type GitusSqliteSessionStore struct {
	config *gitus.GitusConfig
	connection *sql.DB
}

func NewGitusSqliteSessionStore(cfg *gitus.GitusConfig) (*GitusSqliteSessionStore, error) {
	p := cfg.ProperSessionPath()
	r, _ := url.Parse(p)
	q := r.Query()
	q.Set("cache", "shared")
	q.Set("mode", "rwc")
	q.Set("_journal_mode", "WAL")
	r.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", r.String())
	if err != nil { return nil, err }
	return &GitusSqliteSessionStore{
		config: cfg,
		connection: db,
	}, nil
}

func (ss *GitusSqliteSessionStore) Dispose() error {
	return ss.connection.Close()
}

// NOTE: all queries below go through connection/tx directly instead of
// Prepare; statements prepared here are one-shot and would have to be
// finalized explicitly, which is easy to forget and leaks in the driver.
func (ss *GitusSqliteSessionStore) IsSessionStoreUsable() (bool, error) {
	tableName := fmt.Sprintf("%ssession", ss.config.Session.TablePrefix)
	r := ss.connection.QueryRow("SELECT 1 FROM sqlite_schema WHERE type = 'table' AND name = ?", tableName)
	if r.Err() != nil { return false, r.Err() }
	var x string
	err := r.Scan(&x)
	if err == sql.ErrNoRows { return false, nil }
	if err != nil { return false, err }
	if len(x) <= 0 { return false, nil }
	return true, nil
}

func (ss *GitusSqliteSessionStore) RegisterSession(name string, s string) (*session.GitusSession, error) {
	tx, err := ss.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	var t int64 = time.Now().UTC().Unix()
	var tExp int64 = t + int64(ss.config.MaxSessionLifetime)
	var csrf = auxfuncs.CryptoGenSym(64)
	_, err = tx.Exec(fmt.Sprintf("INSERT INTO %ssession(user_name, value, reg_timestamp, expire_timestamp, csrf) VALUES (?,?,?,?,?)", ss.config.Session.TablePrefix), name, s, t, tExp, csrf)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &session.GitusSession{
		Username: name,
		Id: s,
		Timestamp: t,
		ExpireTimestamp: tExp,
		CSRFToken: csrf,
	}, nil
}

func (ss *GitusSqliteSessionStore) RetrieveSession(name string) ([]*session.GitusSession, error) {
	res := make([]*session.GitusSession, 0)
	r, err := ss.connection.Query(fmt.Sprintf("SELECT value, reg_timestamp, expire_timestamp, csrf FROM %ssession WHERE user_name = ?", ss.config.Session.TablePrefix), name)
	if err != nil { return nil, err }
	defer r.Close()
	for r.Next() {
		var id string
		var timestamp int64
		var tExp int64
		var csrf string
		err = r.Scan(&id, &timestamp, &tExp, &csrf)
		if err != nil { return nil, err }
		res = append(res, &session.GitusSession{
			Username: name,
			Id: id,
			Timestamp: timestamp,
			ExpireTimestamp: tExp,
			CSRFToken: csrf,
		})
	}
	if r.Err() != nil { return nil, r.Err() }
	return res, nil
}

func (ss *GitusSqliteSessionStore) RetrieveSessionByKey(username string, key string) (*session.GitusSession, error) {
	r := ss.connection.QueryRow(fmt.Sprintf("SELECT reg_timestamp, expire_timestamp, csrf FROM %ssession WHERE user_name = ? AND value = ?", ss.config.Session.TablePrefix), username, key)
	if r.Err() != nil { return nil, r.Err() }
	var timestamp int64
	var tExp int64
	var csrf string
	err := r.Scan(&timestamp, &tExp, &csrf)
	if err != nil { return nil, err }
	if time.Now().Unix() >= tExp {
		ss.RevokeSession(username, key)
		return nil, nil
	}
	return &session.GitusSession{
		Username: username,
		Id: key,
		Timestamp: timestamp,
		ExpireTimestamp: tExp,
		CSRFToken: csrf,
	}, nil
}

func (ss *GitusSqliteSessionStore) RevokeSession(username string, target string) error {
	tx, err := ss.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	_, err = tx.Exec(fmt.Sprintf("DELETE FROM %ssession WHERE user_name = ? AND value = ?", ss.config.Session.TablePrefix), username, target)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (ss *GitusSqliteSessionStore) RevokeAllSession(username string) error {
	tx, err := ss.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	_, err = tx.Exec(fmt.Sprintf("DELETE FROM %ssession WHERE user_name = ?", ss.config.Session.TablePrefix), username)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (ss *GitusSqliteSessionStore) VerifySessionExist(name string, target string) (bool, error) {
	var s int64
	err := ss.connection.QueryRow(fmt.Sprintf("SELECT expire_timestamp FROM %ssession WHERE user_name = ? AND value = ?", ss.config.Session.TablePrefix), name, target).Scan(&s)
	if err == sql.ErrNoRows { return false, nil }
	if err != nil { return false, err }
	if time.Now().Unix() >= s {
		ss.RevokeSession(name, target)
		return false, nil
	}
	return true, nil
}

func (ss *GitusSqliteSessionStore) VerifySessionFull(username string, session string, csrf string) (bool, error) {
	r := ss.connection.QueryRow(fmt.Sprintf("SELECT reg_timestamp, expire_timestamp, csrf FROM %ssession WHERE user_name = ? AND value = ?", ss.config.Session.TablePrefix), username, session)
	if r.Err() != nil { return false, r.Err() }
	var timestamp int64
	var tExp int64
	var sessionCsrf string
	err := r.Scan(&timestamp, &tExp, &sessionCsrf)
	if err != nil { return false, err }
	if time.Now().Unix() >= tExp {
		ss.RevokeSession(username, session)
		return false, nil
	}
	if subtle.ConstantTimeCompare([]byte(sessionCsrf), []byte(csrf)) == 0 { return false, nil }
	return true, nil
}
