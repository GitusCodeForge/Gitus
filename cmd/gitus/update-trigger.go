package main

import (
	"fmt"
	"os"

	"github.com/GitusCodeForge/Gitus/routes"
)

// NOTE THAT even if any error happens at this part we still need to
// let the whole program return a success exit code. we can retrigger
// failed cicd later, but whatever pushed to the depot should be accepted
// properly.
func HandleUpdateTrigger(ctx *routes.RouterContext, repoFullName string, refFullName string, newrevType string, oldRev string, newRev string) {
	ctx.Config.RecalculateProperPath()
	ns, name := splitRepoFullName(ctx, repoFullName)
	_, err := ctx.DatabaseInterface.GetRepositoryByName(ns, name)
	if err != nil {
		printGitError(fmt.Sprintf("Failed to get repository: %s", err))
		return
	}
	// Everything below is best effort. The last-commit listing of a commit is
	// memoized by commit id and therefore never goes stale: a message that does
	// not get through only means the first visitor of the new commit computes
	// the listing instead of finding it warm.
	err = sendIPCCommand(ctx, IPC_COMMAND_WARM_LAST_COMMIT_CACHE, repoFullName, newRev)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to warm the last-commit cache for %s@%s: %s\n", repoFullName, newRev, err.Error())
	}
}
