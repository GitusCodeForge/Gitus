package edit_user

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
	"golang.org/x/crypto/bcrypt"
)

func bindAdminEditUserInfoController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/user/{username}/edit", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := r.PathValue("username")
			if !model.ValidUserName(un) { FoundAt(w, "/") }
			u, err := rc.DatabaseInterface.GetUserByName(un)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to fetch user %s: %s", un, err.Error()), w, r)
				return
			}
			if !rc.LoginInfo.IsSuperAdmin && u.Status == model.SUPER_ADMIN {
				ctx.ReportRedirect("/admin/user-list", 3, "Error", "Your account does not have enough privilege for this action.", w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("admin/user/edit").Execute(w, &templates.AdminUserEditTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				User: u,
			}))
		},
	))
	
 	http.HandleFunc("POST /admin/user/{username}/edit", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				ctx.ReportNormalError("Invalid request.", w, r)
				return
			}
			un := r.PathValue("username")
			if !model.ValidUserName(un) { FoundAt(w, "/") }
			user, err := ctx.DatabaseInterface.GetUserByName(un)
			if err != nil {
				ctx.ReportInternalError(fmt.Sprintf("Failed to fetch user %s: %s", un, err), w, r)
				return
			}
			if !rc.LoginInfo.IsSuperAdmin && user.Status == model.SUPER_ADMIN {
				ctx.ReportRedirect("/admin/user-list", 3, "Error", "Your account does not have enough privilege for this action.", w, r)
				return
			}
			switch r.Form.Get("type") {
			case "info":
				if len(r.Form.Get("title")) > 0 { user.Title = r.Form.Get("title") }
				if len(r.Form.Get("email")) > 0 { user.Email = r.Form.Get("email") }
				if len(r.Form.Get("website")) > 0 { user.Website = r.Form.Get("website") }
				if len(r.Form.Get("bio")) > 0 { user.Bio = r.Form.Get("bio") }
				i, err := strconv.ParseInt(r.Form.Get("status"), 10, 32)
				if err != nil {
					rc.ReportNormalError("Invalid request", w, r)
					return
				}
				if !rc.LoginInfo.IsSuperAdmin && model.GitusUserStatus(i) != model.SUPER_ADMIN {
					rc.ReportRedirect("/admin/user-list", 0, "Error", "Not enough permission.", w, r)
					return
				}
				user.Status = model.GitusUserStatus(i)
				err = rc.DatabaseInterface.UpdateUserInfo(un, user)
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to update user info: %s", err.Error()), w, r)
					return
				}
			case "password":
				// we will have confirm check at the frontend; this is
				// here for the people who disabled javascript.
				if r.Form.Get("new-password") != r.Form.Get("confirm-new-password") {
					LogTemplateError(ctx.LoadTemplate("setting-user-info").Execute(w, templates.AdminUserEditTemplateModel{
						User: user,
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: struct{Type string; Message string}{
							Type: r.Form.Get("type"),
							Message: "New password mismatch",
						},
					}))
					return
				}
				err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(r.Form.Get("old-password")))
				if err == bcrypt.ErrMismatchedHashAndPassword {
					LogTemplateError(rc.LoadTemplate("setting-user-info").Execute(w, templates.AdminUserEditTemplateModel{
						User: user,
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: struct{Type string; Message string}{
							Type: r.Form.Get("type"),
							Message: "Wrong old password",
						},
					}))
					return
				}
				newpwh, err := bcrypt.GenerateFromPassword([]byte(r.Form.Get("new-password")), bcrypt.DefaultCost)
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to hash password: %s", err.Error()), w, r)
					return
				}
				rc.DatabaseInterface.UpdateUserPassword(un, string(newpwh))
			}
			rc.ReportRedirect(fmt.Sprintf("/admin/user/%s/edit", un), 3, "Updated", "Your setting for this user has been updated.", w, r)
		},
	))
}

