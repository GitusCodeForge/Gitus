//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitlib"
import "github.com/GitusCodeForge/Gitus/pkg/gitus"
import "github.com/GitusCodeForge/Gitus/pkg/gitus/model"

type SubmoduleTemplateModel struct {
	Config *gitus.GitusConfig
	Repository *model.Repository
	RepoHeaderInfo RepoHeaderTemplateModel
	TreePath *TreePathTemplateModel
	Submodule *gitlib.SubmoduleConfig
	SubmodulePath string
	CommitId string
	RawURL string
	ResolvedFullName string
	ResolvedURL string
	External bool
	GitusResolved bool
	SuggestedURL string
	LocalRepository *model.Repository
	LocalCommitExists bool
	CloneURL string
	LoginInfo *LoginInfoModel
}
