//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitus"
import "github.com/GitusCodeForge/Gitus/pkg/gitus/model"

type RepositorySingleIssueTemplateModel struct {
	Config *gitus.GitusConfig
	Repository *model.Repository
	RepoHeaderInfo *RepoHeaderTemplateModel
	LoginInfo *LoginInfoModel
	ErrorMsg string
	Issue *model.Issue
	IssueEventList []*model.IssueEvent
	// CanManageLabels is whether the visitor may attach and remove labels.
	CanManageLabels bool
	// IssueLabelVocabulary is the repository's issue label list, i.e. the
	// labels that can be attached to its issues. It is separate from the
	// repository's own labels.
	IssueLabelVocabulary []string
	// AvailableLabelList is the vocabulary without the labels the issue
	// already carries.
	AvailableLabelList []string
}

