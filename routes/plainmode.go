package routes

import (
	"sync"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
)

// PlainModeListingCache memoizes the directory scans that instances without a
// database (browse-only mode, host mode) perform for namespaces and
// repositories.
//
// Without it every request that resolves a repository, and every listing page,
// walks GitRoot again and builds a LocalGitRepository per repository found. The
// cache is shared by all requests of the process (RouterContext.NewLocal copies
// the pointer) and guarded by a mutex, which also removes the data race the
// shared GitNamespaceList map used to have.
//
// Entries go stale after ttl; browse-only mode is read-mostly and does not need
// to see a change the instant it happens on disk.
type PlainModeListingCache struct {
	mu sync.Mutex
	ttl time.Duration

	namespaces map[string]*model.Namespace
	namespacesAt time.Time

	repositories map[string]map[string]*model.Repository
	repositoriesAt map[string]time.Time
}

const plainModeListingCacheTTL = 2 * time.Second

func NewPlainModeListingCache() *PlainModeListingCache {
	return &PlainModeListingCache{
		ttl: plainModeListingCacheTTL,
		repositories: make(map[string]map[string]*model.Repository),
		repositoriesAt: make(map[string]time.Time),
	}
}

// copyNamespaceListing hands out copies so that callers can attach their own
// repository lists (which they do) without writing into the cached entries.
func copyNamespaceListing(m map[string]*model.Namespace) map[string]*model.Namespace {
	res := make(map[string]*model.Namespace, len(m))
	for k, v := range m {
		cp := *v
		if v.RepositoryList != nil {
			cp.RepositoryList = make(map[string]*model.Repository, len(v.RepositoryList))
			for rk, rv := range v.RepositoryList {
				cp.RepositoryList[rk] = rv
			}
		}
		res[k] = &cp
	}
	return res
}

func copyRepositoryListing(m map[string]*model.Repository) map[string]*model.Repository {
	res := make(map[string]*model.Repository, len(m))
	for k, v := range m {
		res[k] = v
	}
	return res
}
