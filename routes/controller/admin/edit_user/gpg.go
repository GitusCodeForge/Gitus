package edit_user

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)


func bindAdminEditUserGPGController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/user/{username}/gpg", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := r.PathValue("username")
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			u, err := rc.DatabaseInterface.GetUserByName(un)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get user %s by name: %s", un, err), w, r)
				return
			}
			if !rc.LoginInfo.IsSuperAdmin && u.Status == model.SUPER_ADMIN {
				rc.ReportRedirect("/admin/user-list", 3, "Error", "Your account does not have enough privilege for this action.", w, r)
				return
			}
			s, err := rc.DatabaseInterface.GetAllSignKeyByUsername(un)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to retrieve SSH keys of user: %s", err), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("admin/user/gpg-key").Execute(w, &templates.AdminUserGPGKeyTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				User: u,
				KeyList: s,
			}))
		},
	))

	http.HandleFunc("GET /admin/user/{username}/gpg/{keyName}/edit", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := r.PathValue("username")
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			u, err := rc.DatabaseInterface.GetUserByName(un)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get user %s: %s", un, err), w, r)
				return
			}
			if !rc.LoginInfo.IsSuperAdmin && u.Status == model.SUPER_ADMIN {
				rc.ReportRedirect("/admin/user-list", 3, "Error", "Your account does not have enough privilege for this action.", w, r)
				return
			}
			kn := strings.TrimSpace(r.PathValue("keyName"))
			k, err := rc.DatabaseInterface.GetSignKeyByName(un, kn)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get GPG keys of user: %s", err), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("admin/user/edit-gpg-key").Execute(w, &templates.AdminUserEditGPGKeyTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				User: u,
				Key: k,
			}))
		},
	))

	http.HandleFunc("POST /admin/user/{username}/gpg/{keyName}/edit", UseMiddleware(
		[]Middleware{Logged, LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			un := r.PathValue("username")
			if !model.ValidUserName(un) { FoundAt(w, "/") }
			u, err := rc.DatabaseInterface.GetUserByName(un)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get user %s: %s", un, err), w, r)
				return
			}
			if !rc.LoginInfo.IsSuperAdmin && u.Status == model.SUPER_ADMIN {
				rc.ReportRedirect("/admin/user-list", 3, "Error", "Your account does not have enough privilege for this action.", w, r)
				return
			}
			kn := strings.TrimSpace(r.PathValue("keyName"))
			ktext := r.Form.Get("key-text")
			err = rc.DatabaseInterface.UpdateSignKey(un, kn, ktext)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to update signing key: %s", err), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/admin/user/%s/gpg", un), 3, "Updated", "The specified GPG key has been updated.", w, r)
		},
	))
	
	http.HandleFunc("GET /admin/user/{username}/gpg/{keyname}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			ctx.ReportSingleButtonCallback(
				fmt.Sprintf("/admin/user/%s/gpg/%s/delete",
					r.PathValue("username"),
					r.PathValue("keyname"),
				),
				"Delete GPG Key",
				fmt.Sprintf("Click the following button to delete key named <code>%s</code> from user <code>%s</code>", r.PathValue("keyname"), r.PathValue("username")),
				"Delete",
				nil,
				w, r,
			)
		},
	))
	http.HandleFunc("POST /admin/user/{username}/gpg/{keyname}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, ValidPOSTRequestRequired,
			CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := r.PathValue("username")
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			u, err := rc.DatabaseInterface.GetUserByName(un)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get user %s by name: %s", un, err), w, r)
				return
			}
			if !rc.LoginInfo.IsSuperAdmin && u.Status == model.SUPER_ADMIN {
				rc.ReportRedirect("/admin/user-list", 3, "Error", "Your account does not have enough privilege for this action.", w, r)
				return
			}
			keyname := r.PathValue("keyname")
			err = rc.DatabaseInterface.RemoveSignKey(un, keyname)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to delete signing key: %s", err), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/admin/user/%s/gpg", un), 3, "Deleted", "The specified GPG key has been deleted.", w, r)
		},
	))
	
	http.HandleFunc("POST /admin/user/{username}/gpg", UseMiddleware(
		[]Middleware{Logged, LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			un := r.PathValue("username")
			if !model.ValidUserName(un) { FoundAt(w, "/") }
			keyText := strings.TrimSpace(r.Form.Get("key-text"))
			if len(strings.TrimSpace(keyText)) <= 0 {
				rc.ReportRedirect(fmt.Sprintf("/admin/user/%s/gpg", un), 3, "Invalid Key Format", "The key text cannot be empty.", w,  r)
				return
			}
			s := strings.Split(keyText, " ")
			keyName := ""
			if len(s) < 3 {
				keyName = "key_" + auxfuncs.GenSym(8)
			} else {
				keyName = s[2]
			}
			keyName = strings.TrimSpace(keyName)
			err = rc.DatabaseInterface.RegisterSignKey(un, keyName, keyText)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/admin/user/%s/gpg", un), 3, "Updated", "The key you've provided has been added to the database.", w, r)
		},
	))
}

