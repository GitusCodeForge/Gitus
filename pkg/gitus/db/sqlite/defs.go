package sqlite

import (
	"database/sql"
	"net/url"
	"sync"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	_ "github.com/mattn/go-sqlite3"
)

type SqliteGitusDatabaseInterface struct {
	config *gitus.GitusConfig
	connection *sql.DB
	stmtCacheLock sync.RWMutex
	stmtCache map[string]*sql.Stmt
}

var requiredTableList = []string{
	"user_authkey",
	"user_signkey",
	"user_email",
	"user_reg_request",
	"user",
	"namespace",
	"repository",
	"issue",
	"issue_event",
	"pull_request",
	"pull_request_event",
	"snippet",
	"webhook_log",
}

// prepare hands out a prepared statement for the given query, reusing an
// earlier one whenever possible.
//
// Prepared statements are expensive to set up and, more importantly, hold a
// sqlite3_stmt handle until they are finalized; preparing a new one on every
// call and never closing it leaks memory and eventually exhausts the driver.
// Statements are therefore cached per interface instance, keyed by the full
// query string (which already embeds the table prefix).
//
// Statements returned here are owned by the cache. Callers must not close
// them.
func (dbif *SqliteGitusDatabaseInterface) prepare(query string) (*sql.Stmt, error) {
	dbif.stmtCacheLock.RLock()
	stmt, ok := dbif.stmtCache[query]
	dbif.stmtCacheLock.RUnlock()
	if ok { return stmt, nil }
	dbif.stmtCacheLock.Lock()
	defer dbif.stmtCacheLock.Unlock()
	if stmt, ok := dbif.stmtCache[query]; ok { return stmt, nil }
	stmt, err := dbif.connection.Prepare(query)
	if err != nil { return nil, err }
	if dbif.stmtCache == nil {
		dbif.stmtCache = make(map[string]*sql.Stmt)
	}
	dbif.stmtCache[query] = stmt
	return stmt, nil
}

// prepareTx hands out a statement that executes inside tx. The statement is
// attached to the transaction and is finalized together with it, so callers
// must not close it either. See prepare for the caching rationale.
func (dbif *SqliteGitusDatabaseInterface) prepareTx(tx *sql.Tx, query string) (*sql.Stmt, error) {
	stmt, err := dbif.prepare(query)
	if err != nil { return nil, err }
	return tx.Stmt(stmt), nil
}

func (dbif *SqliteGitusDatabaseInterface) IsDatabaseUsable() (bool, error) {
	stmt, err := dbif.prepare("SELECT 1 FROM sqlite_schema WHERE type = 'table' AND name = ?")
	if err != nil { return false, err }
	for _, item := range requiredTableList {
		tableName := dbif.config.Database.TablePrefix + "_" + item
		r := stmt.QueryRow(tableName)
		if r.Err() != nil { return false, r.Err() }
		var a string
		err := r.Scan(&a)
		if err == sql.ErrNoRows { return false, nil }
		if err != nil { return false, err }
		if len(a) <= 0 { return false, nil }
	}
	return true, nil
}

func NewSqliteGitusDatabaseInterface(cfg *gitus.GitusConfig) (*SqliteGitusDatabaseInterface, error) {
	p := cfg.ProperDatabasePath()
	r, _ := url.Parse(p)
	q := r.Query()
	q.Set("cache", "shared")
	q.Set("mode", "rwc")
	q.Set("_journal_mode", "WAL")
	r.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", r.String())
	if err != nil { return nil, err }
	return &SqliteGitusDatabaseInterface{
		config: cfg,
		connection: db,
		stmtCache: make(map[string]*sql.Stmt),
	}, nil
}
