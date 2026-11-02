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

// namespaceCreationAllowed refuses the accounts an administrator marked with
// NORMAL_USER_NO_NEW_NAMESPACE. The same check runs on the form and on the
// submission so the page cannot be reached by typing the URL either.
func namespaceCreationAllowed(rc *RouterContext, w http.ResponseWriter, r *http.Request) bool {
	if !rc.Config.IsInForgeMode() {
		FoundAt(w, "/")
		return false
	}
	u, err := rc.DatabaseInterface.GetUserByName(rc.LoginInfo.UserName)
	if err != nil {
		rc.ReportInternalError(fmt.Sprintf("Failed to fetch user %s: %s", rc.LoginInfo.UserName, err), w, r)
		return false
	}
	if u.Status == model.NORMAL_USER_NO_NEW_NAMESPACE {
		rc.ReportRedirect("/", 5, "Not Allowed",
			"Your account is not allowed to create namespaces. Please contact the site administrator.",
			w, r)
		return false
	}
	return true
}

func bindNewNamespaceController(ctx *RouterContext) {
	http.HandleFunc("GET /new/namespace", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, LoginRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			if !namespaceCreationAllowed(rc, w, r) { return }
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
			if !namespaceCreationAllowed(rc, w, r) { return }
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

