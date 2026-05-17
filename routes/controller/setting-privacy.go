package controller

import (
	"fmt"
	"net/http"
	"strings"

	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)


func bindSettingPrivacyController(ctx *RouterContext) {
	http.HandleFunc("GET /setting/privacy", UseMiddleware(
		[]Middleware{Logged, LoginRequired, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			user, err := rc.DatabaseInterface.GetUserByName(rc.LoginInfo.UserName)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while retrieving user: %s\n", err), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("setting/privacy").Execute(w, &templates.SettingPrivacyTemplateModel{
				Config: rc.Config,
				User: user,
				LoginInfo: rc.LoginInfo,
			}))
		},
	))
	
	http.HandleFunc("POST /setting/privacy", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			user, err := rc.DatabaseInterface.GetUserByName(rc.LoginInfo.UserName)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while retrieving user: %s\n", err), w, r)
				return
			}
			switch r.Form.Get("section") {
			case "2fa":
				switch r.Form.Get("type") {
				case "email":
					enable := len(strings.TrimSpace(r.Form.Get("email-enable"))) > 0
					user.TFAConfig.Email.Enable = enable
					err = rc.DatabaseInterface.UpdateUserInfo(rc.LoginInfo.UserName, user)
					if err != nil {
						rc.ReportInternalError(err.Error(), w, r)
						return
					}
					rc.ReportRedirect("/setting/privacy", 5, "Setting Updated", "Your configuration about two-factor authentication has been updated.", w, r)
					return
				default:
					rc.ReportNormalError("Invalid request", w, r)
					return
				}
			default:
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
		},
	))
}

