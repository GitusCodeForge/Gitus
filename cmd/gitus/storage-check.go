package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/log"
	"github.com/GitusCodeForge/Gitus/routes"
)

// HandleStorageCheck is run from the pre-receive hook before any ref is
// updated. It returns an error only when the repository is over the configured
// limit; that error is printed to the client and the push is refused.
//
// When the size cannot be determined the check fails open: an unreadable
// repository or an unreachable database should not stop people from pushing,
// and the next push will try the check again.
func HandleStorageCheck(ctx *routes.RouterContext, repoFullName string) error {
	limit := ctx.Config.GitConfig.RepositorySizeLimit
	if limit <= 0 {
		return nil
	}
	if ctx.DatabaseInterface == nil {
		log.Warnf("Cannot check the storage limit of %s: no database interface", repoFullName)
		return nil
	}
	ctx.Config.RecalculateProperPath()
	ns, name := splitRepoFullName(ctx, repoFullName)
	repo, err := ctx.DatabaseInterface.GetRepositoryByName(ns, name)
	if err != nil {
		log.Warnf("Cannot check the storage limit of %s: %s", repoFullName, err)
		return nil
	}
	gr, ok := repo.Repository.(*gitlib.LocalGitRepository)
	if !ok {
		log.Warnf("Cannot check the storage limit of %s: not a local git repository", repoFullName)
		return nil
	}
	size, err := gr.SizeInBytes()
	if err != nil {
		log.Warnf("Cannot check the storage limit of %s: %s", repoFullName, err)
		return nil
	}
	// The quarantine directory normally lives under objects/ and is already in
	// the count. A git version that puts it elsewhere would otherwise have the
	// new objects left out, so add it in that case.
	if q := os.Getenv("GIT_QUARANTINE_PATH"); len(q) > 0 && !isInside(gr.GitDirectoryPath, q) {
		qsize, qerr := gitlib.DirectorySizeInBytes(q)
		if qerr != nil {
			log.Warnf("Cannot check the storage limit of %s: %s", repoFullName, qerr)
			return nil
		}
		size += qsize
	}
	if err := gitlib.CheckSizeLimit(size, limit); err != nil {
		return fmt.Errorf("%s: %s", repoFullName, err.Error())
	}
	return nil
}

// isInside reports whether p is the directory dir or below it.
func isInside(dir string, p string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
