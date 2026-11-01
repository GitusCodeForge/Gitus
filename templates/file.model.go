//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitus"
import "github.com/GitusCodeForge/Gitus/pkg/gitlib"

type FileTemplateModel struct {
	Config *gitus.GitusConfig
	Repository *model.Repository
	RepoHeaderInfo RepoHeaderTemplateModel
	File BlobTextTemplateModel
	PermaLink string

	ComparisonInfo *gitlib.BranchComparisonInfo
	AllowBlame bool
	// AllowEdit is true only where the file view has an edit route; the blob
	// permalink has no ?edit handler.
	AllowEdit bool
	// SVGSymbols is set when the file is an SVG icon sprite. Such a document
	// draws nothing on its own, so the page lists the defined symbols.
	SVGSymbols []gitlib.SVGSymbolInfo
	TreeFileList *TreeFileListTemplateModel
	TreePath *TreePathTemplateModel
	CommitInfo *CommitInfoTemplateModel
	TagInfo *TagInfoTemplateModel
	
	LoginInfo *LoginInfoModel
}

