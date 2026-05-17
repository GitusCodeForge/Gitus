package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindNewNamespaceController(ctx *RouterContext) {
	http.HandleFunc("GET /new/namespace", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, LoginRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			LogTemplateError(rc.LoadTemplate("new/namespace").Execute(w, templates.NewNamespaceTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
			}))
		},
	))

	http.HandleFunc("POST /new/namespace", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			userName := rc.LoginInfo.UserName
			newNamespaceName := r.Form.Get("name")
			if !model.ValidNamespaceName(newNamespaceName) {
				rc.ReportRedirect("/new/namespace", 5, "Invalid Namespace Name", "Namespace name must consists of only upper & lowercase letters (a-z, A-Z), 0-9, underscore and hyphen.", w, r)
				return
			}
			ns, err := rc.DatabaseInterface.RegisterNamespace(newNamespaceName, userName)
			if err != nil {
				if err == db.ErrEntityAlreadyExists {
					rc.ReportRedirect("/new/namespace", 5, "Already Exists", fmt.Sprintf("Namespace \"%s\" already exists; please choose another name.", newNamespaceName) , w, r)
				} else {
					rc.ReportInternalError(err.Error(), w, r)
				}
				return
			}
			newNamespaceTitle := r.Form.Get("title")
			if len(strings.TrimSpace(newNamespaceTitle)) > 0 {
				ns.Title = strings.TrimSpace(newNamespaceTitle)
				// NOTE: we don't care if the title setting failed; we have the
				// namespace already.
				rc.DatabaseInterface.UpdateNamespaceInfo(newNamespaceName, ns)
			}
			FoundAt(w, fmt.Sprintf("/s/%s", newNamespaceName))
		},
	))
}

