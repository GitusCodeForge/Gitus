package global_cache

import (
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
)

type GitusGlobalCacheInterface interface {
	Read(key string) (string, bool)
	Write(key string, value string, t time.Duration) error
}

func NewGitusGlobalCache(config *gitus.GitusConfig) (*GitusGlobalCacheInterface, error) {
	switch config.GlobalCache.Type {
	case "":
		fallthrough
		
	default:
		return nil, ErrGlobalCacheTypeNotSupported
	}
}


