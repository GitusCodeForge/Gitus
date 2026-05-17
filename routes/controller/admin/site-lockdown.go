package admin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindAdminSiteLockdownController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/site-lockdown", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			LogTemplateError(rc.LoadTemplate("admin/site-lockdown").Execute(w, &templates.AdminSiteLockdownTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				CurrentMode: rc.Config.GlobalVisibility,
				PrivateNoticeMessage: rc.Config.PrivateNoticeMessage,
				ShutdownNoticeMessage: rc.Config.ShutdownMessage,
				FullAccessUser: strings.Join(rc.Config.FullAccessUser, ","),
				MaintenanceNoticeMessage: rc.Config.MaintenanceMessage,
			}))
		},
	))
	
	http.HandleFunc("POST /admin/site-lockdown", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,

		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rc.Config.LockForSync()
			defer rc.Config.Unlock()
			t := strings.TrimSpace(r.Form.Get("type"))
			switch t {
			case "public":
				rc.Config.GlobalVisibility = gitus.GLOBAL_VISIBILITY_PUBLIC
			case "private":
				rc.Config.GlobalVisibility = gitus.GLOBAL_VISIBILITY_PRIVATE
				rc.Config.PrivateNoticeMessage = r.Form.Get("private-notice-message")
			case "shutdown":
				rc.Config.GlobalVisibility = gitus.GLOBAL_VISIBILITY_SHUTDOWN
				ul := make([]string, 0)
				for k := range strings.SplitSeq(r.Form.Get("full-access-user"), ",") {
					ul = append(ul, strings.TrimSpace(k))
				}
				rc.Config.FullAccessUser = ul
				rc.Config.ShutdownMessage = r.Form.Get("shutdown-notice-message")
			case "maintenance":
				rc.Config.GlobalVisibility = gitus.GLOBAL_VISIBILITY_MAINTENANCE
				rc.Config.MaintenanceMessage = r.Form.Get("maintenance-notice-message")
			}
			err := rc.Config.Sync()
			if err != nil {
				rc.ReportRedirect("/admin/site-lockdown", 0, "Internal Error", fmt.Sprintf("Failed to save config due to error: %s", err.Error()), w, r)
				return
			}
			rc.ReportRedirect("/admin/site-lockdown", 3, "Configuration Saved", "Configuration saved.", w, r)
		},
	))
}

