package admin

import (
	"fmt"
	"net/http"

	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindAdminReceiptSystemSettingController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/rs-setting", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			LogTemplateError(rc.LoadTemplate("admin/rs-setting").Execute(w, &templates.AdminConfigTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "",
			}))
		},
	))

	http.HandleFunc("POST /admin/rs-setting", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rc.Config.LockForSync()
			defer rc.Config.Unlock()
			rc.Config.ReceiptSystem.Type = r.Form.Get("type")
			rc.Config.ReceiptSystem.Path = r.Form.Get("path")
			rc.Config.ReceiptSystem.URL = r.Form.Get("url")
			rc.Config.ReceiptSystem.UserName = r.Form.Get("username")
			rc.Config.ReceiptSystem.Password = r.Form.Get("password")
			rc.Config.ReceiptSystem.TablePrefix = r.Form.Get("table-prefix")
			err := rc.Config.Sync()
			if err != nil {
				rc.ReportRedirect("/admin/rs-setting", 0, "Internal Error", fmt.Sprintf("Error while saving config: %s. Please contact site owner for this...", err.Error()), w, r)
				return
			}
			rc.ReportRedirect("/admin/rs-setting", 3, "Updated", "Configuration is updated.", w, r)

		},
	))
}

