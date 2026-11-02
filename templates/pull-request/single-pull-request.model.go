//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitus"
import "github.com/GitusCodeForge/Gitus/pkg/gitus/model"

type RepositorySinglePullRequestTemplateModel struct {
	Config *gitus.GitusConfig
	Repository *model.Repository
	RepoHeaderInfo *RepoHeaderTemplateModel
	LoginInfo *LoginInfoModel
	ErrorMsg string
	PullRequest *model.PullRequest
	PullRequestEventList []*model.PullRequestEvent
	PageNum int64
	// Reviews is the current state of every assigned reviewer.
	Reviews []*model.PullRequestReview
	CanManageReviewers bool
	CanRemoveReviewers bool
	CanReview bool
	// ApprovalsSatisfied is true when the pull request has no reviewers or
	// every reviewer approved CurrentProviderCommit.
	ApprovalsSatisfied bool
	CurrentProviderCommit string
}

