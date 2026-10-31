package init

import (
	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db/mysql"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db/postgres"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db/sqlite"
)


func InitializeDatabase(cfg *gitus.GitusConfig) (db.GitusDatabaseInterface, error) {
	switch cfg.Database.Type {
	case "sqlite": return sqlite.NewSqliteGitusDatabaseInterface(cfg)
	case "postgres": return postgres.NewPostgresGitusDatabaseInterface(cfg)
	case "mysql", "mariadb": return mysql.NewMysqlGitusDatabaseInterface(cfg)
	}
	return nil, db.ErrDatabaseNotSupported
}

