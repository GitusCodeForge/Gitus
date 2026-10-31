package main

import (
	"fmt"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/log"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/routes"
)

// installHooksForRepository installs (or refreshes) the git hook Gitus manages
// in a repository.
func installHooksForRepository(ctx *routes.RouterContext, repo *model.Repository) error {
	if repo.Type != model.REPO_TYPE_GIT {
		return fmt.Errorf("not a git repository")
	}
	return model.InstallManagedHook(repo.Repository, ctx.Config.FilePath, repo.FullName())
}

// HandleInstallHooks installs the managed git hook in one repository, or in
// every repository when repoFullName is empty. It returns the number of
// repositories that could not be handled.
func HandleInstallHooks(ctx *routes.RouterContext, repoFullName string) int {
	if ctx.DatabaseInterface == nil {
		log.Errorf("No database interface available; cannot enumerate repositories.")
		return 1
	}
	if len(repoFullName) > 0 {
		ns, name := splitRepoFullName(ctx, repoFullName)
		repo, err := ctx.DatabaseInterface.GetRepositoryByName(ns, name)
		if err != nil {
			log.Warnf("%s: %s", repoFullName, err.Error())
			return 1
		}
		err = installHooksForRepository(ctx, repo)
		if err != nil {
			log.Warnf("%s: %s", repo.FullName(), err.Error())
			return 1
		}
		log.Infof("%s: ok", repo.FullName())
		return 0
	}

	const pageSize int64 = 100
	count, err := ctx.DatabaseInterface.CountAllRepositories()
	if err != nil {
		log.Errorf("Failed to count repositories: %s", err.Error())
		return 1
	}
	failures := 0
	for page := int64(0); page*pageSize < count; page++ {
		repositories, err := ctx.DatabaseInterface.GetAllRepositories(page, pageSize)
		if err != nil {
			log.Errorf("Failed to list repositories: %s", err.Error())
			return failures + 1
		}
		for _, repo := range repositories {
			err := installHooksForRepository(ctx, repo)
			if err != nil {
				log.Warnf("%s: %s", repo.FullName(), err.Error())
				failures++
				continue
			}
			log.Infof("%s: ok", repo.FullName())
		}
	}
	if failures > 0 {
		log.Errorf("%d repositories could not be handled.", failures)
	}
	return failures
}
