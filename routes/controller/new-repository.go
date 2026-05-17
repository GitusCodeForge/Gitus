package controller

import (
	"fmt"
	"net/http"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

// since we've decided that admin have full power, should we list all
// namespace when the user is an admin? the answer should be no -
// admin should go to the respective page to find the new repository
// link when the namespace does not explicitly have them as a member.
func bindNewRepositoryController(ctx *RouterContext) {
	http.HandleFunc("GET /new/repo", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, LoginRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			l, err := rc.DatabaseInterface.GetAllComprisingNamespace(rc.LoginInfo.UserName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("new/repository").Execute(w, templates.NewRepositoryTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				NamespaceList: l,
				Selected: r.URL.Query().Get("ns"),
			}))
		},
	))
	
	http.HandleFunc("POST /new/repo", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			userName := rc.LoginInfo.UserName
			newRepoNS := r.Form.Get("namespace")
			if !model.ValidNamespaceName(newRepoNS) {
				rc.ReportRedirect("/new/repo", 5, "Invalid Namespace Name", "Namespace name must consists of only upper & lowercase letters (a-z, A-Z), 0-9, underscore and hyphen.", w, r)
				return
			}
			ns, err := rc.DatabaseInterface.GetNamespaceByName(newRepoNS)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			isOwner := ns.Owner == userName
			priv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			isPrivilegedMember := priv != nil && priv.AddRepository
			if !rc.LoginInfo.IsAdmin && !isOwner && !isPrivilegedMember {
				rc.ReportForbidden("Not enough privilege", w, r)
				return
			}
			newRepoName := r.Form.Get("name")
			if !model.ValidStrictRepositoryName(newRepoName) {
				rc.ReportRedirect("/new/repo", 5, "Invalid Repository Name", "Repository name must consists of only upper & lowercase letters (a-z, A-Z), 0-9, underscore and hyphen.", w, r)
				return
			}
			newRepoDescription := r.Form.Get("description")
			repo, err := rc.DatabaseInterface.CreateRepository(newRepoNS, newRepoName, model.REPO_TYPE_GIT, userName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			repo.Owner = userName
			repo.Description = newRepoDescription
			// NOTE: we ignore this error since we have the repository already.
			rc.DatabaseInterface.UpdateRepositoryInfo(newRepoNS, newRepoName, repo)
			FoundAt(w, fmt.Sprintf("/repo/%s:%s", newRepoNS, newRepoName))
		},
	))
}

