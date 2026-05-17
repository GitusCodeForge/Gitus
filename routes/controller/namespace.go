package controller

import (
	"fmt"
	"net/http"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindNamespaceController(ctx *RouterContext) {
	if !ctx.Config.UseNamespace { return }
	http.HandleFunc("GET /s/{namespace}", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			namespaceName := r.PathValue("namespace")
			if !model.ValidNamespaceName(namespaceName) {
				rc.ReportNotFound(namespaceName, "Repository", "Depot", w, r)
				return
			}
			var ns *model.Namespace
			var ok bool
			var err error
			if rc.Config.OperationMode != gitus.OP_MODE_NORMAL {
				ns, ok = rc.GitNamespaceList[namespaceName]
				if !ok {
					err = rc.SyncAllNamespacePlain()
					if err != nil {
						rc.ReportInternalError(err.Error(), w, r)
						return
					}
					ns, ok = rc.GitNamespaceList[namespaceName]
					if !ok {
						rc.ReportNotFound(namespaceName, "Namespace", rc.Config.DepotName, w, r)
						return
					}
				}
				ns.RepositoryList, err = rc.Config.GetAllRepositoryByNamespacePlain(ns.Name)
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to retrieve repository of namespace %s: %s", ns.Name, err), w, r)
					return
				}
				if ns.Status == model.NAMESPACE_NORMAL_PRIVATE {
					rc.ReportNotFound(namespaceName, "Namespace", rc.Config.DepotName, w, r)
					return
				}
				LogTemplateError(rc.LoadTemplate("namespace").Execute(w, templates.NamespaceTemplateModel{
					Namespace: ns,
					Config: rc.Config,
				}))
			} else {
				ns, err = rc.DatabaseInterface.GetNamespaceByName(namespaceName)
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				if ns.Status == model.NAMESPACE_INTERNAL {
					if !rc.LoginInfo.LoggedIn {
						rc.ReportNotFound(namespaceName, "Namespace", "Depot", w, r)
						return
					}
				}
				if ns.Status == model.NAMESPACE_NORMAL_PRIVATE {
					if !rc.LoginInfo.LoggedIn {
						rc.ReportNotFound(namespaceName, "Namespace", "Depot", w, r)
						return
					}
					rc.LoginInfo.IsOwner = ns.Owner == rc.LoginInfo.UserName
					v := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName).HasSettingPrivilege()
					rc.LoginInfo.IsSettingMember = v
					if !rc.LoginInfo.IsAdmin && !rc.LoginInfo.IsOwner && !rc.LoginInfo.IsSettingMember {
						rc.ReportNotFound(namespaceName, "Namespace", "Depot", w, r)
						return
					}
				}
				s, err := rc.DatabaseInterface.GetAllVisibleRepositoryFromNamespace(rc.LoginInfo.UserName, ns.Name)
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				ns.RepositoryList = make(map[string]*model.Repository, 0)
				for _, k := range s {
					ns.RepositoryList[k.Name] = k
				}
				LogTemplateError(rc.LoadTemplate("namespace").Execute(w, templates.NamespaceTemplateModel{
					Namespace: ns,
					LoginInfo: rc.LoginInfo,
					Config: rc.Config,
				}))
			}
		},
	))
	
	http.HandleFunc("GET /s/{namespace}/new-repo", UseMiddleware(
		[]Middleware{Logged, NormalModeRequired, LoginRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			nsName := r.PathValue("namespace")
			if !model.ValidNamespaceName(nsName) {
				rc.ReportNotFound(nsName, "Namespace", "Depot", w, r)
				return
			}
			ns, err := rc.DatabaseInterface.GetNamespaceByName(nsName)
			if err == db.ErrEntityNotFound {
				rc.ReportNotFound(nsName, "Namespace", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to retrieve namespace: %s", err), w, r)
				return
			}
			rc.LoginInfo.IsOwner = ns.Owner == rc.LoginInfo.UserName
			priv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			if !rc.LoginInfo.IsAdmin && !rc.LoginInfo.IsOwner && priv == nil {
				rc.ReportRedirect(fmt.Sprintf("/s/%s", nsName), 5, "Not Member", "You need to be a member of this namespace to create a repository under this namespace.", w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("new/repository").Execute(w, &templates.NewRepositoryTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				PredefinedNamespace: ns.Name,
			}))
		},
	))
	
	http.HandleFunc("POST /s/{namespace}/new-repo", UseMiddleware(
		[]Middleware{Logged, NormalModeRequired, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			nsName := r.PathValue("namespace")
			if !model.ValidNamespaceName(nsName) {
				rc.ReportNotFound(nsName, "Namespace", "Depot", w, r)
				return
			}
			ns, err := rc.DatabaseInterface.GetNamespaceByName(nsName)
			if err == db.ErrEntityNotFound {
				rc.ReportNotFound(nsName, "Namespace", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to retrieve namespace: %s", err), w, r)
				return
			}
			rc.LoginInfo.IsOwner = ns.Owner == rc.LoginInfo.UserName
			priv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			if !rc.LoginInfo.IsAdmin && !rc.LoginInfo.IsOwner && priv == nil {
				rc.ReportRedirect(fmt.Sprintf("/s/%s", nsName), 5, "Not Member", "You need to be a member of this namespace to create a repository under this namespace.", w, r)
				return
			}
			name := r.Form.Get("name")
			repo, err := rc.DatabaseInterface.CreateRepository(nsName, name, model.REPO_TYPE_GIT, rc.LoginInfo.UserName)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to create repository: %s", err), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/repo/%s", repo.FullName()), 5, "Repository Created", fmt.Sprintf("A new repository named %s has been created under namespace %s.", name, nsName), w, r)
		},
	))
}

