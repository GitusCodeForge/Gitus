package in_memory

import (
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/tcache"
)

type GitusInMemoryGlobalCache struct {
	config *gitus.GitusConfig
	cache *tcache.TCache
}

func NewGitusInMemoryGlobalCache(cfg *gitus.GitusConfig) (*GitusInMemoryGlobalCache, error) {
	c := tcache.NewTCache(24 * time.Hour)
	return &GitusInMemoryGlobalCache{
		config: cfg,
		cache: c,
	}, nil
}

func (gc *GitusInMemoryGlobalCache) Read(key string) (string, bool) {
	return gc.cache.Get(key)
}

func (gc *GitusInMemoryGlobalCache) Write(key string, value string, t time.Duration) error {
	gc.cache.Register(key, value, t)
	return nil
}


