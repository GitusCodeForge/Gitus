//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitus"
import "github.com/GitusCodeForge/Gitus/pkg/gitus/model"
import "github.com/GitusCodeForge/Gitus/pkg/gitlib"

type RepositoryRefsTemplateModel struct {
	Config *gitus.GitusConfig
	Repository *model.Repository
	RepoHeaderInfo *RepoHeaderTemplateModel
	LoginInfo *LoginInfoModel
	BranchList []*gitlib.Branch
	TagList []*gitlib.Tag
	HeadBranch string
	CanModify bool
}
