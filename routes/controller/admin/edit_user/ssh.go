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

func bindAdminEditUserSSHController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/user/{username}/ssh", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := r.PathValue("username")
			if !model.ValidUserName(un) { FoundAt(w, "/") }
			u, err := ctx.DatabaseInterface.GetUserByName(un)
			if err != nil {
				ctx.ReportRedirect("/admin/user-list", 0, "Internal Error", fmt.Sprintf("Failed to fetch user %s: %s", un, err.Error()), w, r)
				return
			}
			if !rc.LoginInfo.IsSuperAdmin && u.Status == model.SUPER_ADMIN {
				ctx.ReportRedirect("/admin/user-list", 3, "Error", "Your account does not have enough privilege for this action.", w, r)
				return
			}
			s, err := ctx.DatabaseInterface.GetAllAuthKeyByUsername(un)
			if err != nil {
				ctx.ReportRedirect("/admin/user-list", 0, "Internal Error", fmt.Sprintf("Failed to fetch SSH keys of user %s: %s", un, err.Error()), w, r)
				return
			}
			LogTemplateError(ctx.LoadTemplate("admin/user/ssh-key").Execute(w, &templates.AdminUserSSHKeyTemplateModel{
				Config: ctx.Config,
				LoginInfo: rc.LoginInfo,
				User: u,
				KeyList: s,
			}))
		},
	))
	
	http.HandleFunc("GET /admin/user/{username}/ssh/{keyName}/edit", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := r.PathValue("username")
			if !model.ValidUserName(un) { FoundAt(w, "/") }
			u, err := ctx.DatabaseInterface.GetUserByName(un)
			if err != nil {
				ctx.ReportRedirect("/admin/user-list", 0, "Internal Error", fmt.Sprintf("Failed to fetch user %s: %s", un, err.Error()), w, r)
				return
			}
			if !rc.LoginInfo.IsSuperAdmin && u.Status == model.SUPER_ADMIN {
				ctx.ReportRedirect("/admin/user-list", 3, "Error", "Your account does not have enough privilege for this action.", w, r)
				return
			}
			kn := strings.TrimSpace(r.PathValue("keyName"))
			k, err := ctx.DatabaseInterface.GetAuthKeyByName(un, kn)
			if err != nil {
				ctx.ReportRedirect("/admin/user-list", 0, "Internal Error", fmt.Sprintf("Failed to fetch SSH key of user %s: %s", un, err.Error()), w, r)
				return
			}
			LogTemplateError(ctx.LoadTemplate("admin/user/edit-ssh-key").Execute(w, &templates.AdminUserEditSSHKeyTemplateModel{
				Config: ctx.Config,
				LoginInfo: rc.LoginInfo,
				User: u,
				Key: k,
			}))
		},
	))
	
	http.HandleFunc("POST /admin/user/{username}/ssh/{keyName}/edit", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {		un := r.PathValue("username")
			if !model.ValidUserName(un) { FoundAt(w, "/") }
			u, err := ctx.DatabaseInterface.GetUserByName(un)
			if err != nil {
				ctx.ReportRedirect("/admin/user-list", 0, "Internal Error", fmt.Sprintf("Failed to fetch user %s: %s", un, err.Error()), w, r)
				return
			}
			if !rc.LoginInfo.IsSuperAdmin && u.Status == model.SUPER_ADMIN {
				ctx.ReportRedirect("/admin/user-list", 3, "Error", "Your account does not have enough privilege for this action.", w, r)
				return
			}
			err = r.ParseForm()
			if err != nil {
				ctx.ReportNormalError("Invalid request", w, r)
				return
			}
			kn := strings.TrimSpace(r.PathValue("keyName"))
			ktext := r.Form.Get("key-text")
			err = ctx.DatabaseInterface.UpdateAuthKey(un, kn, ktext)
			if err != nil {
				ctx.ReportRedirect("/admin/user-list", 0, "Internal Error", fmt.Sprintf("Failed to update SSH key of user %s: %s", un, err.Error()), w, r)
				return
			}
			ctx.ReportRedirect(fmt.Sprintf("/admin/user/%s/ssh", un), 3, "Updated", "The specified SSH key has been updated.", w, r)
		},
	))
	
	http.HandleFunc("GET /admin/user/{username}/ssh/{keyname}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			ctx.ReportSingleButtonCallback(
				fmt.Sprintf("/admin/user/%s/ssh/%s/delete",
					r.PathValue("username"),
					r.PathValue("keyname"),
				),
				"Delete SSH Key",
				fmt.Sprintf("Click the following button to delete key named <code>%s</code> from user <code>%s</code>", r.PathValue("keyname"), r.PathValue("username")),
				"Delete",
				nil,
				w, r,
			)
		},
	))
	http.HandleFunc("POST /admin/user/{username}/ssh/{keyname}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, ValidPOSTRequestRequired,
			CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := r.PathValue("username")
			if !model.ValidUserName(un) { FoundAt(w, "/") }
			u, err := ctx.DatabaseInterface.GetUserByName(un)
			if err != nil {
				ctx.ReportRedirect("/admin/user-list", 0, "Internal Error", fmt.Sprintf("Failed to fetch user %s: %s", un, err.Error()), w, r)
				return
			}
			if !rc.LoginInfo.IsSuperAdmin && u.Status == model.SUPER_ADMIN {
				ctx.ReportRedirect("/admin/user-list", 3, "Error", "Your account does not have enough privilege for this action.", w, r)
				return
			}
			keyname := r.PathValue("keyname")
			err = ctx.DatabaseInterface.RemoveAuthKey(un, keyname)
			if err != nil {
				ctx.ReportRedirect(fmt.Sprintf("/admin/user/%s/ssh", un), 0, "Internal Error", fmt.Sprintf("Failed to delete SSH keys of user %s: %s", un, err.Error()), w, r)
				return
			}
			ctx.SSHKeyManagingContext.RemoveAuthorizedKey(un, keyname)
			err = ctx.SSHKeyManagingContext.Sync()
			if err != nil {
				ctx.ReportRedirect(fmt.Sprintf("/admin/user/%s/ssh", un), 0, "Internal Error", fmt.Sprintf("Failed to delete SSH keys of user %s: %s", un, err.Error()), w, r)
				return
			}
			ctx.ReportRedirect(fmt.Sprintf("/admin/user/%s/ssh", un), 3, "Deleted", "The specified SSH key has been deleted.", w, r)
		},
	))

	http.HandleFunc("POST /admin/user/{username}/ssh", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := r.PathValue("username")
			if !model.ValidUserName(un) { FoundAt(w, "/") }
			keyText := strings.TrimSpace(r.Form.Get("key-text"))
			if len(strings.TrimSpace(keyText)) <= 0 {
				ctx.ReportRedirect(fmt.Sprintf("/admin/user/%s/ssh", un), 3, "Invalid Key Format", "The key text cannot be empty.", w, r)
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
			err := ctx.DatabaseInterface.RegisterAuthKey(un, keyName, keyText)
			if err != nil {
				ctx.ReportInternalError(fmt.Sprintf("Failed to register authentication key: %s", err), w, r)
				return
			}
			ctx.SSHKeyManagingContext.AddAuthorizedKey(un, keyName, keyText)
			err = ctx.SSHKeyManagingContext.Sync()
			if err != nil {
				ctx.ReportInternalError(fmt.Sprintf("Failed to update SSH key store: %s", err), w, r)
				return
			}
			ctx.ReportRedirect(fmt.Sprintf("/admin/user/%s/ssh", un), 3, "Updated", "The key you've provided has been added to the database.", w, r)
		},
	))
}

