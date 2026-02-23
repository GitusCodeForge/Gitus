package controller

import (
	"net/http"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)


func bindLogoutController(ctx *RouterContext) {
	http.HandleFunc("GET /logout", UseMiddleware(
		[]Middleware{Logged, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			if ctx.Config.GlobalVisibility == gitus.GLOBAL_VISIBILITY_MAINTENANCE {
				FoundAt(w, "/maintenance-notice")
				return
			}
			if rc.LoginInfo == nil || !rc.LoginInfo.LoggedIn { FoundAt(w, "/") }
			LogTemplateError(rc.LoadTemplate("logout").Execute(w, templates.LogoutTemplateModel{
				Config: rc.Config,
				ErrorMsg: "",
			}))
		},
	))

	http.HandleFunc("POST /logout", UseMiddleware(
		[]Middleware{Logged, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			if ctx.Config.GlobalVisibility == gitus.GLOBAL_VISIBILITY_MAINTENANCE {
				FoundAt(w, "/maintenance-notice")
				return
			}
			sk, err := r.Cookie(COOKIE_KEY_SESSION)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			un, err := r.Cookie(COOKIE_KEY_USERNAME)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			err = ctx.SessionInterface.RevokeSession(un.Value, sk.Value)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			w.Header().Add("Set-Cookie", (&http.Cookie{
				Name: COOKIE_KEY_SESSION,
				Value: "",
				Path: "/",
				MaxAge: -1,
				HttpOnly: true,
				Secure: true,
				SameSite: http.SameSiteDefaultMode,
			}).String())
			w.Header().Add("Set-Cookie", (&http.Cookie{
				Name: COOKIE_KEY_USERNAME,
				Value: "",
				Path: "/",
				MaxAge: -1,
				HttpOnly: true,
				Secure: true,
				SameSite: http.SameSiteDefaultMode,
			}).String())
			FoundAt(w, "/")
		},
	))
}


