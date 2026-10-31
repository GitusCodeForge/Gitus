package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/go-sql-driver/mysql"
)

// MysqlGitusDatabaseInterface implements db.GitusDatabaseInterface on top of
// MySQL and MariaDB. Both speak the same protocol and the SQL here sticks to
// what they have in common.
type MysqlGitusDatabaseInterface struct {
	config *gitus.GitusConfig
	connection *sql.DB
}

func NewMysqlGitusDatabaseInterface(cfg *gitus.GitusConfig) (*MysqlGitusDatabaseInterface, error) {
	mc := mysql.NewConfig()
	mc.User = cfg.Database.UserName
	mc.Passwd = cfg.Database.Password
	mc.Net = "tcp"
	mc.Addr = cfg.Database.URL
	if mc.Addr == "" { mc.Addr = "127.0.0.1:3306" }
	mc.DBName = cfg.Database.DatabaseName
	// The columns are DATETIME and the code expects time.Time back, so the
	// driver has to parse them; UTC keeps the values independent of the
	// server's local time zone.
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.Params = map[string]string{"charset": "utf8mb4"}
	conn, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil { return nil, err }
	return &MysqlGitusDatabaseInterface{
		config: cfg,
		connection: conn,
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) Dispose() error {
	return dbif.connection.Close()
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

func (dbif *MysqlGitusDatabaseInterface) IsDatabaseUsable() (bool, error) {
	ctx := context.Background()
	for _, item := range requiredTableList {
		tableName := fmt.Sprintf("%s_%s", dbif.config.Database.TablePrefix, item)
		var n int
		err := dbif.connection.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ?
`, tableName).Scan(&n)
		if err != nil { return false, err }
		if n <= 0 { return false, nil }
	}
	return true, nil
}
