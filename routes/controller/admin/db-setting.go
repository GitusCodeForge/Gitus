package admin

import (
	"fmt"
	"net/http"

	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindAdminDatabaseSettingController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/db-setting", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			LogTemplateError(rc.LoadTemplate("admin/db-setting").Execute(w, &templates.AdminConfigTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "",
			}))
		},
	))
	
	http.HandleFunc("POST /admin/db-setting", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			rc.Config.LockForSync()
			defer rc.Config.Unlock()
			rc.Config.Database.Type = r.Form.Get("type")
			rc.Config.Database.Path = r.Form.Get("path")
			rc.Config.Database.URL = r.Form.Get("url")
			rc.Config.Database.DatabaseName = r.Form.Get("name")
			rc.Config.Database.UserName = r.Form.Get("user")
			rc.Config.Database.Password = r.Form.Get("password")
			rc.Config.Database.TablePrefix = r.Form.Get("table-prefix")
			err = rc.Config.Sync()
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Error while saving config: %s", err), w, r)
				return
			}
			rc.ReportRedirect("/admin/db-setting", 3, "Setting Updated", "Your setting for main database has been updated.", w, r)
		},
	))
}

