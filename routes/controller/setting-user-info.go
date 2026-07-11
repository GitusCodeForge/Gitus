package controller

import (
	"net/http"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
	"golang.org/x/crypto/bcrypt"
)


func bindSettingController(ctx *RouterContext) {
	http.HandleFunc("GET /setting", UseMiddleware(
		[]Middleware{Logged, LoginRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := rc.LoginInfo.UserName
			if !model.ValidUserName(un) {
				ctx.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			user, err := ctx.DatabaseInterface.GetUserByName(un)
			if err != nil {
				if err == db.ErrEntityNotFound {
					ctx.ReportNotFound(un, "User", "depot", w, r)
					return
				}
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			LogTemplateError(ctx.LoadTemplate("setting/user-info").Execute(w, templates.SettingUserInfoTemplateModel{
				User: user,
				Config: ctx.Config,
				LoginInfo: rc.LoginInfo,
			}))
		},
	))

	http.HandleFunc("POST /setting", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			// GenerateLoginInfoModel also checks the validity of the
			// session stored in cookie.  if the session is invalid it's
			// not considered logged in.
			targetUsername := r.Form.Get("username")
			if !model.ValidUserName(targetUsername) {
				ctx.ReportNotFound(targetUsername, "User", "Depot", w, r)
				return
			}
			if rc.LoginInfo.UserName != r.Form.Get("username") {
				// at this point we can at least be more sure that the
				// info we get from cookie is valid and it *should* be
				// safe to assume that we can go ahead and use the
				// username from cookie, but since this branch is a sign
				// of tampering i suppose it's better to just rollback.
				un := rc.LoginInfo.UserName
				if !model.ValidUserName(un) {
					ctx.ReportNotFound(un, "User", "Depot", w, r)
					return
				}
				user, err := ctx.DatabaseInterface.GetUserByName(un)
				if err != nil {
					ctx.ReportInternalError(err.Error(), w, r)
					return
				}
				LogTemplateError(ctx.LoadTemplate("setting/user-info").Execute(w, templates.SettingUserInfoTemplateModel{
					User: user,
					Config: ctx.Config,
					LoginInfo: rc.LoginInfo,
					ErrorMsg: struct{Type string; Message string}{
						Type: r.Form.Get("type"),
						Message: "Invalid state",
					},
				}))
				return
			}
			user, err := ctx.DatabaseInterface.GetUserByName(targetUsername)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			switch r.Form.Get("type") {
			case "info":
				if len(r.Form.Get("title")) > 0 { user.Title = r.Form.Get("title") }
				if len(r.Form.Get("email")) > 0 { user.Email = r.Form.Get("email") }
				if len(r.Form.Get("website")) > 0 { user.Website = r.Form.Get("website") }
				if len(r.Form.Get("bio")) > 0 { user.Bio = r.Form.Get("bio") }
				err = ctx.DatabaseInterface.UpdateUserInfo(targetUsername, user)
				if err != nil {
					ctx.ReportInternalError(err.Error(), w, r)
					return
				}
			case "website-preference":
				user.WebsitePreference.ForegroundColor = strings.TrimSpace(r.Form.Get("foreground-color"))
				user.WebsitePreference.BackgroundColor = strings.TrimSpace(r.Form.Get("background-color"))
				user.WebsitePreference.UseSiteWideThemeConfig = r.Form.Has("use-sitewide-theme-config") && len(r.Form.Get("use-sitewide-theme-config")) > 0
				user.WebsitePreference.UseJavascript = r.Form.Has("use-javascript") && len(r.Form.Get("use-javascript")) > 0
				err = rc.DatabaseInterface.UpdateUserInfo(user.Name, user)
				if err != nil {
					ctx.ReportInternalError(err.Error(), w, r)
					return
				}
			case "password":
				// we will have confirm check at the frontend; this is
				// here for the people who disabled javascript.
				if r.Form.Get("new-password") != r.Form.Get("confirm-new-password") {
					LogTemplateError(ctx.LoadTemplate("setting/user-info").Execute(w, templates.SettingUserInfoTemplateModel{
						User: user,
						Config: ctx.Config,
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
					LogTemplateError(ctx.LoadTemplate("setting/user-info").Execute(w, templates.SettingUserInfoTemplateModel{
						User: user,
						Config: ctx.Config,
						LoginInfo: rc.LoginInfo,
						ErrorMsg: struct{Type string; Message string}{
							Type: r.Form.Get("type"),
							Message: "Wrong old password",
						},
					}))
					return
				}
				if err != nil {
					ctx.ReportInternalError(err.Error(), w, r)
					return
				}
				newpwh, err := bcrypt.GenerateFromPassword([]byte(r.Form.Get("new-password")), ctx.Config.PasswordHashStrength)
				if err != nil {
					ctx.ReportInternalError(err.Error(), w, r)
					return
				}
				ctx.DatabaseInterface.UpdateUserPassword(targetUsername, string(newpwh))
			}
			ctx.SessionInterface.RevokeAllSession(targetUsername)
			LogTemplateError(ctx.LoadTemplate("setting/user-info").Execute(w, templates.SettingUserInfoTemplateModel{
				User: user,
				Config: ctx.Config,
				LoginInfo: rc.LoginInfo,
				ErrorMsg: struct{Type string; Message string}{
					Type: r.Form.Get("type"),
					Message: "Updated.",
				},
			}))
		},
	))
}

