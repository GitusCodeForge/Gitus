package init

import (
	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session/in_memory"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session/memcached"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session/redis_like"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session/sqlite"
)


func InitializeDatabase(cfg *gitus.GitusConfig) (session.GitusSessionStore, error) {
	switch cfg.Session.Type {
	case "sqlite": return sqlite.NewGitusSqliteSessionStore(cfg)
	case "valkey": fallthrough
	case "keydb": fallthrough
	case "redis":
		return redis_like.NewGitusRedisLikeSessionStore(cfg)
	case "memcached":
		return memcached.NewGitusMemcachedSessionStore(cfg)
	case "in-memory":
		return in_memory.NewGitusInMemorySessionStore(cfg)
	}
	return nil, db.ErrDatabaseNotSupported
}

