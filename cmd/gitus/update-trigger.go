package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/log"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/routes"
)

// NOTE THAT even if any error happens at this part we still need to
// let the whole program return a success exit code. we can retrigger
// failed cicd later, but whatever pushed to the depot should be accepted
// properly.
func HandleUpdateTrigger(ctx *routes.RouterContext, repoFullName string, refFullName string, newrevType string, oldRev string, newRev string) {
	ctx.Config.RecalculateProperPath()
	ns, name := splitRepoFullName(ctx, repoFullName)
	repo, err := ctx.DatabaseInterface.GetRepositoryByName(ns, name)
	if err != nil {
		printGitError(fmt.Sprintf("Failed to get repository: %s", err))
		return
	}
	// Everything below is best effort.
	//
	// The last-commit listing of a commit is memoized by commit id and therefore
	// never goes stale: a message that does not get through only means the first
	// visitor of the new commit computes the listing instead of finding it warm.
	// Warming it up first also means it has happened before the webhook call,
	// which may involve a slow endpoint.
	err = sendIPCCommand(ctx, IPC_COMMAND_WARM_LAST_COMMIT_CACHE, repoFullName, newRev)
	if err != nil {
		log.Warnf("Failed to warm the last-commit cache for %s@%s: %s", repoFullName, newRev, err.Error())
	}
	// The fork page compares a fork against the upstream repository without
	// fetching (P13). Refresh the upstream's remote-tracking ref for the branch
	// that was just pushed, so the next page view has a fresh comparison.
	refreshForkComparison(ctx, repo, refFullName, newrevType)
	// The webhook is the notification channel for repositories that enabled one;
	// HandleWebHook returns immediately when it is disabled.
	HandleWebHook(ctx, repoFullName, refFullName, newrevType, oldRev, newRev)
}

// refreshForkComparison updates the upstream repository's remote-tracking ref
// for a branch that was just pushed to a fork. It is best effort: the page view
// only needs the ref, and a failure here just means the comparison stays as it
// was until the next push.
func refreshForkComparison(ctx *routes.RouterContext, repo *model.Repository, refFullName string, newrevType string) {
	if repo == nil || newrevType == "delete" { return }
	if !strings.HasPrefix(refFullName, "refs/heads/") { return }
	if repo.ForkOriginName == "" && repo.ForkOriginNamespace == "" { return }
	upstreamPath := path.Join(ctx.Config.GitRoot, repo.ForkOriginNamespace, repo.ForkOriginName)
	if !gitlib.IsValidGitDirectory(upstreamPath) {
		log.Warnf("Cannot refresh fork comparison: %s is not a git repository", upstreamPath)
		return
	}
	upstream, err := model.CreateLocalRepository(model.REPO_TYPE_GIT, repo.ForkOriginNamespace, repo.ForkOriginName, upstreamPath)
	if err != nil {
		log.Warnf("Cannot refresh fork comparison for %s: %s", repo.FullName(), err.Error())
		return
	}
	gr, ok := upstream.(*gitlib.LocalGitRepository)
	if !ok { return }
	remoteName := fmt.Sprintf("%s/%s", repo.Namespace, repo.Name)
	branch := strings.TrimPrefix(refFullName, "refs/heads/")
	if err := gr.FetchRemoteBranch(remoteName, branch); err != nil {
		log.Warnf("Failed to refresh fork comparison for %s/%s: %s", repo.FullName(), branch, err.Error())
	}
}
