package controller

import (
	"fmt"
	"net/http"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/receipt"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindSettingEmailController(ctx *RouterContext) {
	http.HandleFunc("GET /setting/email", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			e, err := rc.DatabaseInterface.GetAllRegisteredEmailOfUser(rc.LoginInfo.UserName)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while retrieving user email: %s.", err), w, r)
				return
			}
			u, err := rc.DatabaseInterface.GetUserByName(rc.LoginInfo.UserName)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while retrieving user info: %s", err), w, r)
				return
			}
			el := make([]struct{Email string;Verified bool;Primary bool}, 0)
			for _, k := range e {
				el = append(el, struct{Email string;Verified bool;Primary bool}{
					Email: k.Email,
					Verified: k.Verified,
					Primary: u.Email == k.Email,
				})
			}
			LogTemplateError(rc.LoadTemplate("setting/email").Execute(w, templates.SettingEmailTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				EmailList: el,
			}))
		},
	))
	
	http.HandleFunc("POST /setting/email", UseMiddleware(
		[]Middleware{ Logged, ValidPOSTRequestRequired,
			LoginRequired, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			email := r.Form.Get("email")
			if len(email) <= 0 {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			err = rc.DatabaseInterface.AddEmail(rc.LoginInfo.UserName, email)
			if err != nil {
				rc.ReportRedirect("/setting/email", 0, "Internal Error", fmt.Sprintf("Failed while registering email: %s\n", err), w, r)
				return
			}
			rc.ReportRedirect("/setting/email", 3, "Email Added", "The email you specified has been added to your user account. You should verify it, or else it wouldn't be recognized as properly yours.", w, r)
		},
	))
	
	http.HandleFunc("GET /setting/email/verify", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			fmt.Println(ctx)
			fmt.Println(rc)
			email := r.URL.Query().Get("email")
			command := make([]string, 3)
			command[0] = receipt.VERIFY_EMAIL
			command[1] = rc.LoginInfo.UserName
			command[2] = email
			rid, err := rc.ReceiptSystem.IssueReceipt(24*60, command)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to issue receipt: %s\n", err.Error()), w, r)
				return
			}
			title := fmt.Sprintf("Verification of email on %s", rc.Config.DepotName)
			body := fmt.Sprintf(`
This email is registered as being owned by user %s on %s.

If this isn't you, you don't need to do anything about it, as the verification request expires after 24 hours, upon which the verification will not succeed and the email won't be labelled as a valid email of that user; but if this is you, please copy & open the following link to verify this email:

    %s/receipt?id=%s

We wish you all the best in your future endeavours.

%s
`, rc.LoginInfo.UserName, rc.Config.DepotName, rc.Config.ProperHTTPHostName(), rid, rc.Config.DepotName)
			go func() {
				rc.Mailer.SendPlainTextMail(email, title, body)
			}()
			rc.ReportRedirect("/setting/email", 3, "Verification Email Sent", "Please follow the instruction in the email to verify this email.", w, r)
		},
	))
	
	http.HandleFunc("GET /setting/email/primary", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			email := r.URL.Query().Get("email")
			if len(email) <= 0 {
				rc.ReportRedirect("/setting/email", 3, "Invalid Request", "This email is not associated with your user account.", w, r)
				return
			}
			b, err := rc.DatabaseInterface.CheckIfEmailVerified(rc.LoginInfo.UserName, email)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while checking email's verification status: %s\n", err), w, r)
				return
			}
			if !b {
				rc.ReportRedirect("/setting/email", 3, "Email Not Verified", "The specified email has been not been verified. You can only use verified email addresses as your primary email address.", w, r)
				return
			}
			u, err := rc.DatabaseInterface.GetUserByName(rc.LoginInfo.UserName)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while retrieving user: %s", err), w, r)
				return
			}
			u.Email = email
			err = rc.DatabaseInterface.UpdateUserInfo(rc.LoginInfo.UserName, u)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while saving user info: %s", err), w, r)
				return
			}
			rc.ReportRedirect("/setting/email", 3, "Settings Saved", "The specified email is saved as the primary email address.", w, r)
		},
	))
	
	http.HandleFunc("GET /setting/email/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, GlobalVisibility,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			email := r.URL.Query().Get("email")
			if len(email) <= 0 {
				rc.ReportRedirect("/setting/email", 3, "Invalid Request", "This email is not associated with your user account.", w, r)
				return
			}
			err = rc.DatabaseInterface.DeleteRegisteredEmail(rc.LoginInfo.UserName, email)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while deleting registered email: %s\n", err), w, r)
				return
			}
			rc.ReportRedirect("/setting/email", 3, "Email Deleted", "The specified email has been deleted from your user account.", w, r)
		},
	))
}
