package admin

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/mail"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindAdminMailerSettingController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/mailer-setting", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			LogTemplateError(rc.LoadTemplate("admin/mailer-setting").Execute(w, &templates.AdminConfigTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "",
			}))
			
		},
	))
	
	http.HandleFunc("POST /admin/mailer-setting", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			if r.Form.Get("action") == "Test Mailer" {
				port, err := strconv.ParseInt(r.Form.Get("port"), 10, 32)
				if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
					return
				}
				mailer, err := mail.CreateMailerFromMailerConfig(&gitus.GitusMailerConfig{
					Type: r.Form.Get("type"),
					SMTPServer: r.Form.Get("server"),
					SMTPPort: int(port),
					User: r.Form.Get("username"),
					Password: r.Form.Get("password"),
				})
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to create mailer: %s", err), w, r)
					return
				}
				go func(){
					err = mailer.SendPlainTextMail(r.Form.Get("test-email-target"), "Mailer Configuration Test", fmt.Sprintf(`
This is a test email from %s.

If you can see this message it means the mailer configuration can be used normally.
`, rc.Config.DepotName))
				}()
				LogTemplateError(rc.LoadTemplate("admin/mailer-setting").Execute(w, &templates.AdminConfigTemplateModel{
					Config: rc.Config,
					LoginInfo: rc.LoginInfo,
					ErrorMsg: "Test email has been sent. You should be able to see the email if the setup is correct.",
				}))
				return
			}
			
			rc.Config.LockForSync()
			defer rc.Config.Unlock()
			rc.Config.Mailer.Type = r.Form.Get("type")
			rc.Config.Mailer.SMTPServer = r.Form.Get("server")
			i, err := strconv.ParseInt(r.Form.Get("port"), 10, 32)
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			rc.Config.Mailer.SMTPPort = int(i)
			rc.Config.Mailer.User = r.Form.Get("username")
			rc.Config.Mailer.Password = r.Form.Get("password")
			err = rc.Config.Sync()
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to save mailer config: %s", err), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("admin/mailer-setting").Execute(w, &templates.AdminConfigTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "Updated.",
			}))
			
		},
	))
}

