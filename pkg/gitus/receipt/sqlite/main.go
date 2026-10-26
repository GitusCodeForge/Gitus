package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/receipt"
	_ "github.com/mattn/go-sqlite3"
)

type GitusSqliteReceiptSystemInterface struct {
	config *gitus.GitusConfig
	connection *sql.DB
}

var requiredTableList = []string{
	"receipt",
}

func NewSqliteReceiptSystemInterface(cfg *gitus.GitusConfig) (*GitusSqliteReceiptSystemInterface, error) {
	p := cfg.ProperReceiptSystemPath()
	r, _ := url.Parse(p)
	q := r.Query()
	q.Set("cache", "shared")
	q.Set("mode", "rwc")
	q.Set("_journal_mode", "WAL")
	r.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", r.String())
	if err != nil { return nil, err }
	return &GitusSqliteReceiptSystemInterface{
		config: cfg,
		connection: db,
	}, nil
}

func (rsif *GitusSqliteReceiptSystemInterface) Dispose() error {
	return rsif.connection.Close()
}

// NOTE: all queries below go through connection/tx directly instead of
// Prepare; statements prepared here are one-shot and would have to be
// finalized explicitly, which is easy to forget and leaks in the driver.
func (rsif *GitusSqliteReceiptSystemInterface) IsReceiptSystemUsable() (bool, error) {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	for _, item := range requiredTableList {
		r := rsif.connection.QueryRow("SELECT 1 FROM sqlite_schema WHERE type = 'table' AND name = ?", fmt.Sprintf("%s_%s", pfx, item))
		if r.Err() != nil { return false, r.Err() }
		var a string
		err := r.Scan(&a)
		if err == sql.ErrNoRows { return false, nil }
		if err != nil { return false, err }
		if len(a) <= 0 { return false, nil }
	}
	return true, nil
}

func (rsif *GitusSqliteReceiptSystemInterface) Install() error {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	tx, err := rsif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_receipt (
    id TEXT UNIQUE,
    command TEXT,
    issue_time INT,
    timeout_minute INT
)`, pfx))
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (rsif *GitusSqliteReceiptSystemInterface) RetrieveReceipt(rid string) (*receipt.Receipt, error) {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	r := rsif.connection.QueryRow(fmt.Sprintf(`
SELECT command, issue_time, timeout_minute
FROM %s_receipt
WHERE id = ?
`, pfx), rid)
	if r.Err() != nil { return nil, r.Err() }
	var cmd string
	var issueTime, timeoutMinute int64
	err := r.Scan(&cmd, &issueTime, &timeoutMinute)
	if err != nil { return nil, err }
	return &receipt.Receipt{
		Id: rid,
		Command: receipt.ParseReceiptCommand(cmd),
		IssueTime: issueTime,
		TimeoutMinute: timeoutMinute,
	}, nil
}

func (rsif *GitusSqliteReceiptSystemInterface) IssueReceipt(timeoutMinute int64, command []string) (string, error) {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	tx, err := rsif.connection.Begin()
	if err != nil { return "", err }
	defer tx.Rollback()
	rid := receipt.NewReceiptId()
	_, err = tx.Exec(fmt.Sprintf(`
INSERT INTO %s_receipt(id, command, issue_time, timeout_minute)
VALUES (?,?,?,?)
`, pfx), rid, receipt.SerializeReceiptCommand(command), time.Now().Unix(), timeoutMinute)
	if err != nil { return "", err }
	err = tx.Commit()
	if err != nil { return "", err }
	return rid, nil
}

func (rsif *GitusSqliteReceiptSystemInterface) CancelReceipt(rid string) error {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	tx, err := rsif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	_, err = tx.Exec(fmt.Sprintf(`DELETE FROM %s_receipt WHERE id = ?`, pfx), rid)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (rsif *GitusSqliteReceiptSystemInterface) GetAllReceipt(pageNum int, pageSize int) ([]*receipt.Receipt, error) {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	r, err := rsif.connection.Query(fmt.Sprintf(`
SELECT id, command, issue_time, timeout_minute
FROM %s_receipt
ORDER BY rowid ASC
LIMIT ? OFFSET ?`, pfx), pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*receipt.Receipt, 0)
	var id, command string
	var issueTime, timeoutMinute int64
	for r.Next() {
		err = r.Scan(&id, &command, &issueTime, &timeoutMinute)
		if err != nil { return nil, err }
		res = append(res, &receipt.Receipt{
			Id: id,
			Command: receipt.ParseReceiptCommand(command),
			IssueTime: issueTime,
			TimeoutMinute: timeoutMinute,
		})
	}
	if r.Err() != nil { return nil, r.Err() }
	return res, nil
}

func (rsif *GitusSqliteReceiptSystemInterface) SearchReceipt(q string, pageNum int, pageSize int) ([]*receipt.Receipt, error) {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	pattern := strings.ReplaceAll(q, "\\", "\\\\")
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	pattern = "%" + pattern + "%"
	r, err := rsif.connection.Query(fmt.Sprintf(`
SELECT id, command, issue_time, timeout_minute
FROM %s_receipt
WHERE id LIKE ? ESCAPE ? OR command LIKE ? ESCAPE ?
ORDER BY rowid ASC LIMIT ? OFFSET ?`, pfx), pattern, "\\", pattern, "\\", pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*receipt.Receipt, 0)
	for r.Next() {
		var id, command string
		var issueTime, timeoutMinute int64
		err = r.Scan(&id, &command, &issueTime, &timeoutMinute)
		if err != nil { return nil, err }
		res = append(res, &receipt.Receipt{
			Id: id,
			Command: receipt.ParseReceiptCommand(command),
			IssueTime: issueTime,
			TimeoutMinute: timeoutMinute,
		})
	}
	if r.Err() != nil { return nil, r.Err() }
	return res, nil
}

func (rsif *GitusSqliteReceiptSystemInterface) EditReceipt(id string, robj *receipt.Receipt) error {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	tx, err := rsif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	_, err = tx.Exec(fmt.Sprintf(`
UPDATE %s_receipt
SET command = ?, issue_time = ?, timeout_minute = ?
WHERE id = ?
`, pfx), receipt.SerializeReceiptCommand(robj.Command), robj.IssueTime, robj.TimeoutMinute, robj.Id)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}
