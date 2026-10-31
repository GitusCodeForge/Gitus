package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

// resolveTreePathAllowSubmodule is ResolveTreePath with one difference: if the
// final component is a gitlink (mode 160000), it returns the tree item instead
// of trying to read the submodule commit from the parent repository, where it
// does not exist.
func resolveTreePathAllowSubmodule(rr *gitlib.LocalGitRepository, root *gitlib.TreeObject, p string) (gitlib.GitObject, *gitlib.TreeObjectItem, error) {
	if root == nil {
		return nil, nil, gitlib.ErrObjectNotFound
	}
	parts := make([]string, 0)
	for _, item := range strings.Split(p, "/") {
		if item == "" || item == "." {
			continue
		}
		parts = append(parts, item)
	}
	if len(parts) <= 0 {
		return root, nil, nil
	}

	tobj := root
	for i, name := range parts {
		var item *gitlib.TreeObjectItem
		for j := range tobj.ObjectList {
			if tobj.ObjectList[j].Name == name {
				item = &tobj.ObjectList[j]
				break
			}
		}
		if item == nil {
			return nil, nil, gitlib.ErrObjectNotFound
		}
		if i == len(parts)-1 {
			if item.Mode == gitlib.TREE_SUBMODULE {
				return nil, item, nil
			}
			obj, err := rr.ReadObject(item.Hash)
			return obj, nil, err
		}
		if item.Mode != gitlib.TREE_TREE_OBJECT {
			return nil, nil, gitlib.ErrObjectNotFound
		}
		obj, err := rr.ReadObject(item.Hash)
		if err != nil {
			return nil, nil, err
		}
		sub, ok := obj.(*gitlib.TreeObject)
		if !ok {
			return nil, nil, gitlib.ErrObjectNotFound
		}
		tobj = sub
	}
	return tobj, nil, nil
}

func buildSubmoduleTreePath(rootFullName, rootPath, treePath string) *templates.TreePathTemplateModel {
	tp1 := make([]string, 0)
	treePathSegmentList := make([]struct {
		Name    string
		RelPath string
	}, 0)
	for item := range strings.SplitSeq(treePath, "/") {
		if len(item) <= 0 {
			continue
		}
		tp1 = append(tp1, item)
		treePathSegmentList = append(treePathSegmentList, struct {
			Name    string
			RelPath string
		}{
			Name: item, RelPath: strings.Join(tp1, "/"),
		})
	}
	return &templates.TreePathTemplateModel{
		RootFullName:        rootFullName,
		RootPath:            rootPath,
		TreePath:            treePath,
		TreePathSegmentList: treePathSegmentList,
	}
}

// renderSubmoduleView renders the page for a gitlink entry. It never fetches
// the submodule URL: it only reads the parent commit's .gitmodules, resolves
// the URL for display, and links to a repository on this instance when the URL
// points at one.
func renderSubmoduleView(
	rc *routes.RouterContext, w http.ResponseWriter, r *http.Request,
	repo *model.Repository, rootTree *gitlib.TreeObject, item *gitlib.TreeObjectItem,
	treePath, rootPath, rootFullName string, headerType, headerNode string,
) {
	rr := repo.Repository.(*gitlib.LocalGitRepository)
	subs, err := rr.ReadSubmoduleConfigFromTree(rootTree)
	if err != nil {
		rc.ReportInternalError(fmt.Sprintf("Failed to read .gitmodules: %s", err.Error()), w, r)
		return
	}
	cleanPath := strings.Trim(treePath, "/")
	sub := subs[cleanPath]
	rawURL := ""
	if sub != nil {
		rawURL = sub.Url
	}
	resolved := gitlib.ResolveSubmoduleURL(repo.Namespace, rawURL)

	modelValue := &templates.SubmoduleTemplateModel{
		Config:     rc.Config,
		Repository: repo,
		RepoHeaderInfo: templates.RepoHeaderTemplateModel{
			TypeStr:  headerType,
			NodeName: headerNode,
		},
		TreePath:         buildSubmoduleTreePath(rootFullName, rootPath, cleanPath),
		Submodule:        sub,
		SubmodulePath:    cleanPath,
		CommitId:         item.Hash,
		RawURL:           rawURL,
		ResolvedFullName: resolved.FullName,
		External:         resolved.External,
		GitusResolved:    resolved.GitusResolved,
		SuggestedURL: func() string {
			suggested := resolved.SuggestedGitURL()
			if strings.TrimSpace(rawURL) == suggested {
				return ""
			}
			return suggested
		}(),
		CloneURL:  fmt.Sprintf("%s/repo/%s", rc.Config.ProperHTTPHostName(), repo.FullName()),
		LoginInfo: rc.LoginInfo,
	}

	if resolved.FullName != "" {
		if _, _, _, localRepo, err := rc.ResolveRepositoryFullName(resolved.FullName); err == nil {
			modelValue.LocalRepository = localRepo
			modelValue.ResolvedURL = fmt.Sprintf("/repo/%s", resolved.FullName)
			if lr, ok := localRepo.Repository.(*gitlib.LocalGitRepository); ok {
				if _, err := lr.ReadObject(item.Hash); err == nil {
					modelValue.LocalCommitExists = true
				}
			}
		}
	} else if resolved.External {
		if u, err := url.Parse(rawURL); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
			modelValue.ResolvedURL = rawURL
		}
	}

	routes.LogTemplateError(rc.LoadTemplate("submodule").Execute(w, modelValue))
}
