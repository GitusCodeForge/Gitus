package main

import (
	"fmt"
	"os"

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
		fmt.Fprintf(os.Stderr, "No database interface available; cannot enumerate repositories.\n")
		return 1
	}
	if len(repoFullName) > 0 {
		ns, name := splitRepoFullName(ctx, repoFullName)
		repo, err := ctx.DatabaseInterface.GetRepositoryByName(ns, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", repoFullName, err.Error())
			return 1
		}
		err = installHooksForRepository(ctx, repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", repo.FullName(), err.Error())
			return 1
		}
		fmt.Printf("%s: ok\n", repo.FullName())
		return 0
	}

	const pageSize int64 = 100
	count, err := ctx.DatabaseInterface.CountAllRepositories()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to count repositories: %s\n", err.Error())
		return 1
	}
	failures := 0
	for page := int64(0); page*pageSize < count; page++ {
		repositories, err := ctx.DatabaseInterface.GetAllRepositories(page, pageSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list repositories: %s\n", err.Error())
			return failures + 1
		}
		for _, repo := range repositories {
			err := installHooksForRepository(ctx, repo)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %s\n", repo.FullName(), err.Error())
				failures++
				continue
			}
			fmt.Printf("%s: ok\n", repo.FullName())
		}
	}
	if failures > 0 {
		fmt.Fprintf(os.Stderr, "%d repositories could not be handled.\n", failures)
	}
	return failures
}
