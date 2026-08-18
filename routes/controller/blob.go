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


func bindBlobController(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/blob/{blobId}/", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, GlobalVisibility, ErrorGuard},
		ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			if !model.ValidRepositoryName(rfn) {
				ctx.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			_, _, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err == routes.ErrNotFound {
				ctx.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if repo.Type != model.REPO_TYPE_GIT {
				ctx.ReportNormalError("The repository you have requested isn't a Git repository.", w, r)
				return
			}
			if !ctx.Config.IsInBrowseOnlyMode() {
				rc.LoginInfo.IsOwner = (repo.Owner == rc.LoginInfo.UserName) || (ns.Owner == rc.LoginInfo.UserName)
				rc.LoginInfo.IsStrictOwner = repo.Owner == rc.LoginInfo.UserName
			}
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
			
			blobId := r.PathValue("blobId")

			repoHeaderInfo := GenerateRepoHeader("blob", blobId)

			rr := repo.Repository.(*gitlib.LocalGitRepository)
			gobj, err := rr.ReadObject(blobId)
			if err != nil {
				ctx.ReportObjectReadFailure(blobId, err.Error(), w, r)
				return
			}
			if gobj.Type() != gitlib.BLOB {
				ctx.ReportObjectTypeMismatch(gobj.ObjectId(), "BLOB", gobj.Type().String(), w, r)
				return
			}

			// NOTE THAT we don't know the path with blob so we can't predict what kind of
			// file it is unless we look at its content and hope that we can make a good
			// assumption without calculating too much. the current behaviour is thus
			// intentional and we shall come back to this in the future...
			templateType := "file-text"
			bobj := gobj.(*gitlib.BlobObject)
			if r.URL.Query().Has("raw") || r.URL.Query().Has("snapshot") {
				w.Write(bobj.Data)
				return
			}
			str := string(bobj.Data)
			coloredStr, err := colorSyntax("", str)
			if err == nil { str = coloredStr }
			permaLink := fmt.Sprintf("/repo/%s/blob/%s", rfn, blobId)

			LogTemplateError(ctx.LoadTemplate(templateType).Execute(w, templates.FileTemplateModel{
				RepoHeaderInfo: *repoHeaderInfo,
				File: templates.BlobTextTemplateModel{
					FileLineCount: strings.Count(str, "\n"),
					FileContent: str,
				},
				Repository: repo,
				PermaLink: permaLink,
				TreePath: nil,
				CommitInfo: nil,
				TagInfo: nil,
				LoginInfo: rc.LoginInfo,
			}))
		},
	))
}


