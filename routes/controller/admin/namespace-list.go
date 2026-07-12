package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

// /admin/namespace-list?p={pagenum}&s={pagesize}&q={query}
func bindAdminNamespaceListController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/namespace-list", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			i, err := rc.DatabaseInterface.CountAllNamespace()
			p := r.URL.Query().Get("p")
			if len(p) <= 0 { p = "1" }
			s := r.URL.Query().Get("s")
			if len(s) <= 0 { s = "50" }
			q := strings.TrimSpace(r.URL.Query().Get("q"))
			pageNum, err := strconv.ParseInt(p, 10, 64)
			pageSize, err := strconv.ParseInt(s, 10, 64)
			totalPage := i / pageSize
			if i % pageSize != 0 { totalPage += 1 }
			if pageNum > totalPage { pageNum = totalPage }
			if pageNum <= 1 { pageNum = 1 }
			var namespaceList map[string]*model.Namespace
			if len(q) > 0 {
				namespaceList, err = rc.DatabaseInterface.SearchForNamespace(q, pageNum-1, pageSize)
			} else {
				namespaceList, err = rc.DatabaseInterface.GetAllNamespaces(pageNum-1, pageSize)
			}
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to load namespace list: %s", err), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("admin/namespace-list").Execute(w, &templates.AdminNamespaceListTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "",
				NamespaceList: namespaceList,
				Query: q,
				PageInfo: &templates.PageInfoModel{
					PageNum: pageNum,
					PageSize: pageSize,
					TotalPage: totalPage,
				},
			}))
			
		},
	))
	
	http.HandleFunc("POST /admin/namespace-list", UseMiddleware(
		[]Middleware{Logged, LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			owner := r.Form.Get("owner")
			name := r.Form.Get("name")
			if !model.ValidNamespaceName(name) {
				rc.ReportRedirect("/admin/namespace-list", 5, "Invalid Namespace Name", "Invalid namespace name; namespace name must only contains uppercase & lowercase letters (a-z, A-Z), 0-9, underscore and hyphen.\n", w, r)
				return
			}
			title := r.Form.Get("title")
			email := r.Form.Get("email")
			description := r.Form.Get("description")
			i, err := strconv.ParseInt(r.Form.Get("status"), 10, 32)
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			ns, err := rc.DatabaseInterface.RegisterNamespace(name, owner)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to register namespace: %s", err), w, r)
				return
			}
			ns.Status = model.GitusNamespaceStatus(i)
			ns.Title = title
			ns.Email = email
			ns.Owner = owner
			ns.Description = description
			err = rc.DatabaseInterface.UpdateNamespaceInfo(name, ns)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to update namespace info: %s", err), w, r)
				return
			}
			FoundAt(w, "/admin/namespace-list")
		},
	))

	http.HandleFunc("GET /admin/namespace/{name}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			ctx.ReportSingleButtonCallback(
				fmt.Sprintf("/admin/namespace/{name}/delete", r.PathValue("name")),
				"Delete User",
				fmt.Sprintf("Click the following button to delete namespace <code>%s</code>", r.PathValue("name")),
				"Delete",
				nil,
				w, r,
			)
		},
	))
	http.HandleFunc("POST /admin/namespace/{name}/delete", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			CSRFCheck, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			nsn := r.PathValue("name")
			if !model.ValidUserName(nsn) { FoundAt(w, "/") }
			err := rc.DatabaseInterface.HardDeleteNamespaceByName(nsn)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to delete namespace by name: %s", err), w, r)
				return
			}
			FoundAt(w, "/admin/namespace-list")
		},
	))
}

