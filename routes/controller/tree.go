package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/routes"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func handleTreeSnapshotRequest(repo *model.Repository, treeId string, obj gitlib.GitObject, w http.ResponseWriter) {
	rr := repo.Repository.(*gitlib.LocalGitRepository)
	filename := fmt.Sprintf(
		"%s-%s-tree-%s",
		repo.Namespace, repo.Name, treeId,
	)
	responseWithTreeZip(rr, obj, filename, w)
}

func bindTreeHandler(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/tree/{treeId}/{treePath...}", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			_, _, ns, repo, err := rc.ResolveRepositoryFullName(rfn)
			if err == routes.ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			if repo.Type != model.REPO_TYPE_GIT {
				rc.ReportNormalError("The repository you have requested isn't a Git repository.", w, r)
				return
			}
			if !rc.Config.IsInBrowseOnlyMode() {
				rc.LoginInfo.IsOwner = repo.Owner == rc.LoginInfo.UserName || ns.Owner == rc.LoginInfo.UserName
			}

			// reject visit if repo is private & user not logged in or not member.
			if !ctx.Config.IsInBrowseOnlyMode() && repo.Status == model.REPO_NORMAL_PRIVATE {
				chk := rc.LoginInfo.IsAdmin || rc.LoginInfo.IsOwner
				if !chk {
					chk = repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName) != nil
				}
				if !chk {
					chk = ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName) != nil
				}
				if !chk {
					rc.ReportNotFound(repo.FullName(), "Repository", "Depot", w, r)
					return
				}
			}
			
			treeId := r.PathValue("treeId")
			treePath := r.PathValue("treePath")

			rr := repo.Repository.(*gitlib.LocalGitRepository)
			repoHeaderInfo := templates.RepoHeaderTemplateModel{
				TypeStr: "tree",
				NodeName: treeId,
			}

			gobj, err := rr.ReadObject(treeId)
			if err != nil {
				rc.ReportObjectReadFailure(treeId, err.Error(), w, r)
				return
			}
			if gobj.Type() != gitlib.TREE {
				rc.ReportObjectTypeMismatch(gobj.ObjectId(), "TREE", gobj.Type().String(), w, r)
				return
			}

			rootFullName := fmt.Sprintf("%s@%s:%s", rfn, "tree", treeId)
			rootPath := fmt.Sprintf("/repo/%s/%s/%s", rfn, "tree", treeId)
			permaLink := fmt.Sprintf("/repo/%s/tree/%s/%s", rfn, treeId, treePath)
			tobj := gobj.(*gitlib.TreeObject)
			target, err := rr.ResolveTreePath(tobj, treePath)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
			}

			if target.Type() == gitlib.TREE && len(treePath) > 0 && !strings.HasSuffix(treePath, "/") {
				FoundAt(w, fmt.Sprintf("%s/%s/", rootPath, treePath))
				return
			}

			isSnapshotRequest :=  r.URL.Query().Has("snapshot")
			if isSnapshotRequest {
				handleTreeSnapshotRequest(repo, treeId, target, w)
				return
			}
			
			tp1 := make([]string, 0)
			treePathSegmentList := make([]struct{Name string;RelPath string}, 0)
			for item := range strings.SplitSeq(treePath, "/") {
				if len(item) <= 0 { continue }
				tp1 = append(tp1, item)
				treePathSegmentList = append(treePathSegmentList, struct{
					Name string; RelPath string
				}{
					Name: item, RelPath: strings.Join(tp1, "/"),
				})
			}
			treePathModelValue := &templates.TreePathTemplateModel{
				RootFullName: rootFullName,
				RootPath: rootPath,
				TreePath: treePath,
				TreePathSegmentList: treePathSegmentList,
			}
			LogTemplateError(rc.LoadTemplate("tree").Execute(w, templates.TreeTemplateModel{
				Repository: repo,
				RepoHeaderInfo: repoHeaderInfo,
				TreeFileList: &templates.TreeFileListTemplateModel{
					ShouldHaveParentLink: len(treePath) > 0,
					RepoPath: fmt.Sprintf("/repo/%s", rfn),
					RootPath: rootPath,
					TreePath: treePath,
					FileList: target.(*gitlib.TreeObject).ObjectList,
				},
				PermaLink: permaLink,
				TreePath: treePathModelValue,
				CommitInfo: nil,
				TagInfo: nil,
				LoginInfo: rc.LoginInfo,
				Config: rc.Config,
			}))
		},
	))
}

