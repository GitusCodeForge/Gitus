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

func (ss *GitusSqliteSessionStore) IsSessionStoreUsable() (bool, error) {
	tableName := fmt.Sprintf("%ssession", ss.config.Session.TablePrefix)
	stmt, err := ss.connection.Prepare("SELECT 1 FROM sqlite_schema WHERE type = 'table' AND name = ?")
	if err != nil { return false, err }
 	r := stmt.QueryRow(tableName)
	if r.Err() != nil { return false, r.Err() }
	var x string
	err = r.Scan(&x)
	if err == sql.ErrNoRows { return false, nil }
	if err != nil { return false, err }
	if len(x) <= 0 { return false, nil }
	return true, nil
}

func (ss *GitusSqliteSessionStore) RegisterSession(name string, s string) (*session.GitusSession, error) {
	tx, err := ss.connection.Begin()
	if err != nil { return nil, err }
	stmt, err := tx.Prepare(fmt.Sprintf("INSERT INTO %ssession(user_name, value, reg_timestamp, csrf) VALUES (?,?,?,?)", ss.config.Session.TablePrefix))
	if err != nil { tx.Rollback(); return nil, err }
	var t int64 = time.Now().UTC().Unix()
	var csrf = auxfuncs.CryptoGenSym(64)
	_, err = stmt.Exec(name, s, t, csrf)
	if err != nil { tx.Rollback(); return nil, err }
	err = tx.Commit();
	if err != nil { tx.Rollback(); return nil, err }
	return &session.GitusSession{
		Username: name,
		Id: s,
		Timestamp: t,
		CSRFToken: csrf,
	}, nil
}

func (ss *GitusSqliteSessionStore) RetrieveSession(name string) ([]*session.GitusSession, error) {
	stmt, err := ss.connection.Prepare(fmt.Sprintf("SELECT value, reg_timestamp, csrf FROM %ssession WHERE user_name = ?", ss.config.Session.TablePrefix))
	if err != nil { return nil, err }
	res := make([]*session.GitusSession, 0)
	if err != nil { return nil, err }
	r, err := stmt.Query(name)
	for r.Next() {
		var id string
		var timestamp int64
		var csrf string
		err = r.Scan(&id, &timestamp, &csrf)
		if err != nil { return nil, err }
		res = append(res, &session.GitusSession{
			Username: name,
			Id: id,
			Timestamp: timestamp,
			CSRFToken: csrf,
		})
	}
	return res, nil
}

func (ss *GitusSqliteSessionStore) RetrieveSessionByKey(username string, key string) (*session.GitusSession, error) {
	stmt, err := ss.connection.Prepare(fmt.Sprintf("SELECT reg_timestamp, csrf FROM %ssession WHERE user_name = ? AND value = ?", ss.config.Session.TablePrefix))
	if err != nil { return nil, err }
	r := stmt.QueryRow(username, key)
	if r.Err() != nil { return nil, r.Err() }
	var timestamp int64
	var csrf string
	err = r.Scan(&timestamp, &csrf)
	if err != nil { return nil, err }
	return &session.GitusSession{
		Username: username,
		Id: key,
		Timestamp: timestamp,
		CSRFToken: csrf,
	}, nil
}

func (ss *GitusSqliteSessionStore) RevokeSession(username string, target string) error {
	tx, err := ss.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf("DELETE FROM %ssession WHERE user_name = ? AND value = ?", ss.config.Session.TablePrefix))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(username, target)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (ss *GitusSqliteSessionStore) VerifySessionExist(name string, target string) (bool, error) {
	stmt, err := ss.connection.Prepare(fmt.Sprintf("SELECT 1 FROM %ssession WHERE user_name = ? AND value = ?", ss.config.Session.TablePrefix))
	if err != nil { return false, err }
	s := ""
	err = stmt.QueryRow(name, target).Scan(&s)
	if err == sql.ErrNoRows { return false, nil }
	if err != nil { return false, err }
	return (len(s) > 0), nil
}

func (ss *GitusSqliteSessionStore) VerifySessionFull(username string, session string, csrf string) (bool, error) {
	stmt, err := ss.connection.Prepare(fmt.Sprintf("SELECT reg_timestamp, csrf FROM %ssession WHERE user_name = ? AND value = ?", ss.config.Session.TablePrefix))
	if err != nil { return false, err }
	r := stmt.QueryRow(username, csrf)
	if r.Err() != nil { return false, r.Err() }
	var timestamp int64
	var sessionCsrf string
	err = r.Scan(&timestamp, &csrf)
	if err != nil { return false, err }
	if subtle.ConstantTimeCompare([]byte(sessionCsrf), []byte(csrf)) == 0 { return false, nil }
	return true, nil
}

