package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindAdminSessionSettingController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/session-setting", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			LogTemplateError(rc.LoadTemplate("admin/session-setting").Execute(w, &templates.AdminConfigTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "",
			}))
		},
	))
	
	http.HandleFunc("POST /admin/session-setting", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rc.Config.LockForSync()
			defer rc.Config.Unlock()
			rc.Config.Session.Type = r.Form.Get("type")
			rc.Config.Session.Path = r.Form.Get("path")
			rc.Config.Session.TablePrefix = r.Form.Get("table-prefix")
			rc.Config.Session.Host = r.Form.Get("host")
			rc.Config.Session.UserName = r.Form.Get("user-name")
			rc.Config.Session.Password = r.Form.Get("password")
			dbnStr := strings.TrimSpace(r.Form.Get("database-number"))
			if dbnStr == "" { dbnStr = "0" }
			dbn, err := strconv.Atoi(dbnStr)
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			rc.Config.Session.DatabaseNumber = dbn
			err = rc.Config.Sync()
			if err != nil {
				rc.ReportRedirect("/admin/session-setting", 0, "Internal Error", fmt.Sprintf("Error while saving config: %s. Please contact site owner for this...", err.Error()), w, r)
				return
			}
			rc.ReportRedirect("/admin/session-setting", 3, "Updated", "Configuration updated.", w, r)
		},
	))
}

