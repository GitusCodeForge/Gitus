package init

import (
	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/global_cache"
	gc_tcache "github.com/GitusCodeForge/Gitus/pkg/gitus/global_cache/tcache"
)

func InitializeGlobalCache(cfg *gitus.GitusConfig) (global_cache.GitusGlobalCacheInterface, error) {
	switch cfg.GlobalCache.Type {
	case "in-memory":
		return gc_tcache.NewGitusInMemoryGlobalCache(cfg)
	default:
		return nil, global_cache.ErrGlobalCacheTypeNotSupported
	}
}

