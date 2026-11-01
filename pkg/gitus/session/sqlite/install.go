package sqlite

import (
	"database/sql"
	"fmt"
)

func (ss *GitusSqliteSessionStore) Install() error {
	tx, err := ss.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %ssession (
    user_name TEXT,
    value TEXT,
    reg_timestamp INTEGER,
    expire_timestamp INTEGER,
    csrf TEXT,
    ip TEXT,
    user_agent TEXT
)`, ss.config.Session.TablePrefix))
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}


// EnsureSchema adds the columns that were introduced after the session table
// was created. Install() only creates a missing table, so an instance upgraded
// in place would otherwise keep the old set of columns.
func (ss *GitusSqliteSessionStore) EnsureSchema() error {
	pfx := ss.config.Session.TablePrefix
	for _, column := range []struct{ name string; kind string }{
		{"ip", "TEXT"},
		{"user_agent", "TEXT"},
	} {
		ok, err := ss.hasColumn(fmt.Sprintf("%ssession", pfx), column.name)
		if err != nil { return err }
		if ok { continue }
		_, err = ss.connection.Exec(fmt.Sprintf(
			"ALTER TABLE %ssession ADD COLUMN %s %s", pfx, column.name, column.kind))
		if err != nil { return err }
	}
	return nil
}

func (ss *GitusSqliteSessionStore) hasColumn(table string, column string) (bool, error) {
	r, err := ss.connection.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil { return false, err }
	defer r.Close()
	for r.Next() {
		var cid int
		var name, kind string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := r.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column { return true, nil }
	}
	return false, r.Err()
}
