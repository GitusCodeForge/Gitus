package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/receipt"
	pgx "github.com/jackc/pgx/v5"
	pgxpool "github.com/jackc/pgx/v5/pgxpool"
)

type GitusPostgresReceiptSystemInterface struct {
	config *gitus.GitusConfig
	pool *pgxpool.Pool
}

var requiredTableList = []string{
	"receipt",
}

func NewPostgresReceiptSystemInterface(cfg *gitus.GitusConfig) (*GitusPostgresReceiptSystemInterface, error) {
	u := &url.URL{
		Scheme: "postgres",
		User: url.UserPassword(cfg.Database.UserName, cfg.Database.Password),
		Host: cfg.Database.URL,
		Path: cfg.Database.DatabaseName,
	}
	pool, err := pgxpool.New(context.TODO(), u.String())
	if err != nil { return nil, err }
	return &GitusPostgresReceiptSystemInterface{
		config: cfg,
		pool: pool,
	}, nil
}

func (rsif *GitusPostgresReceiptSystemInterface) Dispose() error {
	rsif.pool.Close()
	return nil
}

func (rsif *GitusPostgresReceiptSystemInterface) IsReceiptSystemUsable() (bool, error) {
	ctx := context.Background()
	queryStr := `
SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' AND tablename = $1)
`
	for _, item := range requiredTableList {
		tableName := fmt.Sprintf("%s_%s", rsif.config.ReceiptSystem.TablePrefix, item)
		stmt := rsif.pool.QueryRow(ctx, queryStr, tableName)
		var a bool
		err := stmt.Scan(&a)
		if errors.Is(err, pgx.ErrNoRows) { return false, nil }
		if err != nil { return false, err }
		if (!a) { return false, nil }
	}
	return true, nil
}

func (rsif *GitusPostgresReceiptSystemInterface) Install() error {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	ctx := context.Background()
	tx, err := rsif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_receipt (
    id VARCHAR(256) UNIQUE,
    command TEXT,
    issue_time TIMESTAMP,
    timeout_minute INT
)`, pfx))
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (rsif *GitusPostgresReceiptSystemInterface) RetrieveReceipt(rid string) (*receipt.Receipt, error) {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	ctx := context.Background()
	stmt := rsif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT command, issue_time, timeout_minute
FROM %s_receipt
WHERE id = $1
`, pfx), rid)
	var cmd string
	var issueTime time.Time
	var timeoutMinute int64
	err := stmt.Scan(&cmd, &issueTime, &timeoutMinute)
	if err != nil { return nil, err }
	return &receipt.Receipt{
		Id: rid,
		Command: receipt.ParseReceiptCommand(cmd),
		IssueTime: issueTime.Unix(),
		TimeoutMinute: timeoutMinute,
	}, nil 
}

func (rsif *GitusPostgresReceiptSystemInterface) IssueReceipt(timeoutMinute int64, command []string) (string, error) {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	ctx := context.Background()
	tx, err := rsif.pool.Begin(ctx)
	if err != nil { return "", err }
	defer tx.Rollback(ctx)
	rid := receipt.NewReceiptId()
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_receipt(id, command, issue_time, timeout_minute)
VALUES ($1,$2,$3,$4)
`, pfx), rid, receipt.SerializeReceiptCommand(command), time.Now(), timeoutMinute)
	if err != nil { return "", err }
	err = tx.Commit(ctx)
	if err != nil { return "", err }
	return rid, nil
}

func (rsif *GitusPostgresReceiptSystemInterface) CancelReceipt(rid string) error {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	ctx := context.Background()
	tx, err := rsif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s_receipt WHERE id = $1`, pfx), rid)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (rsif *GitusPostgresReceiptSystemInterface) GetAllReceipt(pageNum int, pageSize int) ([]*receipt.Receipt, error) {
	pfx := rsif.config.ReceiptSystem.TablePrefix
	ctx := context.Background()
	stmt, err := rsif.pool.Query(ctx, fmt.Sprintf(`
SELECT id, command, issue_time, timeout_minute
FROM %s_receipt
ORDER BY issue_time ASC
LIMIT $1 OFFSET $2`, pfx), pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*receipt.Receipt, 0)
	var id, command string
	var issueTime, timeoutMinute int64
	for stmt.Next() {
		err = stmt.Scan(&id, &command, &issueTime, &timeoutMinute)
		if err != nil { return nil, err }
		res = append(res, &receipt.Receipt{
			Id: id,
			Command: receipt.ParseReceiptCommand(command),
			IssueTime: issueTime,
			TimeoutMinute: timeoutMinute,
		})
	}
	return res, nil
}

func (rsif *GitusPostgresReceiptSystemInterface) SearchReceipt(q string, pageNum int, pageSize int) ([]*receipt.Receipt, error) {
	pfx := rsif.config.Database.TablePrefix
	ctx := context.Background()
	pattern := strings.ReplaceAll(q, "\\", "\\\\")
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	pattern = "%" + pattern + "%"
	stmt, err := rsif.pool.Query(ctx, fmt.Sprintf(`
SELECT id, command, issue_time, timeout_minute
FROM %s_receipt
WHERE id LIKE $1 ESCAPE $2 OR command LIKE $1 ESCAPE $2
ORDER BY issue_time ASC LIMIT $3 OFFSET $4`, pfx),
		pattern, "\\", pageSize, pageNum*pageSize,
	)
	if err != nil { return nil, nil }
	defer stmt.Close()
	res := make([]*receipt.Receipt, 0)
	var id, command string
	var timeoutMinute int64
	var issueTime time.Time
	for stmt.Next() {
		err = stmt.Scan(&id, &command, &issueTime, &timeoutMinute)
		if err != nil { return nil, err }
		res = append(res, &receipt.Receipt{
			Id: id,
			Command: receipt.ParseReceiptCommand(command),
			IssueTime: issueTime.Unix(),
			TimeoutMinute: timeoutMinute,
		})
	}
	return res, nil
}

func (rsif *GitusPostgresReceiptSystemInterface) EditReceipt(id string, robj *receipt.Receipt) error {
	pfx := rsif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := rsif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_receipt
SET command = $1, issue_time = $2, timeout_minute = $3
WHERE id = $4
`, pfx), receipt.SerializeReceiptCommand(robj.Command), robj.IssueTime, robj.TimeoutMinute, robj.Id)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

