package controller

import (
	"fmt"
	"net/http"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindRepositoryForkController(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/fork", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			_, _, _, s, err := rc.ResolveRepositoryFullName(rfn)
			if err == ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			fr, err := rc.DatabaseInterface.GetForkRepositoryOfUser(rc.LoginInfo.UserName, s.Namespace, s.Name)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			if (!rc.Config.UseNamespace) {
				if len(fr) > 0 {
					FoundAt(w, fmt.Sprintf("/repo/%s", fr[0].FullName()))
					return
				}
			}
			var l map[string]*model.Namespace = nil
			if (rc.Config.UseNamespace) {
				l, err = rc.DatabaseInterface.GetAllComprisingNamespace(rc.LoginInfo.UserName)
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				for k := range l {
					if !rc.LoginInfo.IsAdmin && l[k].Owner != rc.LoginInfo.UserName {
						acl := l[k].ACL.GetUserPrivilege(rc.LoginInfo.UserName)
						if acl == nil || !acl.AddRepository {
							delete(l, k)
						}
					}
				}
			}
			LogTemplateError(rc.LoadTemplate("fork").Execute(w, templates.ForkTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				SourceRepository: s,
				ForkedRepoList: fr,
				NamespaceList: l,
			}))
		},
	))
	
	http.HandleFunc("POST /repo/{repoName}/fork", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			originNs, originName, _, _, err := rc.ResolveRepositoryFullName(rfn)
			if err == ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			namespace := r.Form.Get("namespace")
			name := r.Form.Get("name")
			if !model.ValidStrictRepositoryName(name){
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/fork", rfn), 0, "Invalid Repository Name", "Repository name must consists of only upper & lowercase letters (a-z, A-Z), 0-9, underscore and hyphen.", w, r)
				return
			}
			rp, err := rc.DatabaseInterface.SetUpCloneRepository(originNs, originName, namespace, name, rc.LoginInfo.UserName)
			if err == db.ErrEntityAlreadyExists {
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/fork", rfn), 0, "Already Exists", fmt.Sprintf("The repository %s:%s already exists. Please choose a different name or namespace.", namespace, name), w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			FoundAt(w, fmt.Sprintf("/repo/%s", rp.FullName()))
		},
	))
}

