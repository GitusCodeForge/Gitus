package admin

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)


func bindAdminEditNamespaceController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/namespace/{name}/edit", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			nsn := r.PathValue("name")
			if !model.ValidNamespaceName(nsn) { FoundAt(w, "/") }
			ns, err := rc.DatabaseInterface.GetNamespaceByName(nsn)
			if err != nil {
				rc.ReportRedirect("/admin/namespace-list", 0, "Error",
					fmt.Sprintf("Failed to fetch namespace: %s", err.Error()),
					w, r,
				)
				return
			}
			LogTemplateError(rc.LoadTemplate("admin/namespace-edit").Execute(w, &templates.AdminNamespaceEditTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				Namespace: ns,
			}))
		},
	))
	
 	http.HandleFunc("POST /admin/namespace/{name}/edit", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			nsn := r.PathValue("name")
			if !model.ValidNamespaceName(nsn) { FoundAt(w, "/") }
			ns, err := rc.DatabaseInterface.GetNamespaceByName(nsn)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to fetch namespace: %s", err), w, r)
				return
			}
			title := r.Form.Get("title")
			owner := r.Form.Get("owner")
			email := r.Form.Get("email")
			i, err := strconv.Atoi(r.Form.Get("status"))
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			description := r.Form.Get("description")
			ns.Title = title
			ns.Email = email
			ns.Owner = owner
			ns.Status = model.GitusNamespaceStatus(i)
			ns.Description = description
			err = rc.DatabaseInterface.UpdateNamespaceInfo(nsn, ns)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to update namespace info: %s", err), w, r)
				return
			}
			rc.ReportRedirect("/admin/db-setting", 5, "Setting Updated", "Your setting for this namespace has been updated.", w, r)
		},
	))
}

