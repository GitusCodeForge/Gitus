// /reset-password/request
// /reset-password/update-password

package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/receipt"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
	"golang.org/x/crypto/bcrypt"
)

func bindResetPasswordController(ctx *RouterContext) {
	http.HandleFunc("GET /reset-password/request", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			switch rc.Config.GlobalVisibility {
			case gitus.GLOBAL_VISIBILITY_MAINTENANCE:
				FoundAt(w, "/maintenance-notice")
				return
			case gitus.GLOBAL_VISIBILITY_SHUTDOWN:
				FoundAt(w, "/shutdown-notice")
				return
			}
			LogTemplateError(rc.LoadTemplate("reset-password-request").Execute(w, struct{
				Config *gitus.GitusConfig
				ErrorMsg string
				LoginInfo *templates.LoginInfoModel
			}{
				Config: rc.Config,
				ErrorMsg: "",
				LoginInfo: rc.LoginInfo,
			}))
		},
	))

	http.HandleFunc("POST /reset-password/request", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			UseLoginInfo, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
		switch rc.Config.GlobalVisibility {
		case gitus.GLOBAL_VISIBILITY_MAINTENANCE:
			FoundAt(w, "/maintenance-notice")
			return
		case gitus.GLOBAL_VISIBILITY_SHUTDOWN:
			FoundAt(w, "/shutdown-notice")
			return
		}
		targetUserName := strings.TrimSpace(r.Form.Get("username"))
		if !model.ValidUserName(targetUserName) {
			rc.ReportNotFound(targetUserName, "User", "Depot", w, r)
			return
		}
		targetEmail := strings.TrimSpace(r.Form.Get("email"))
		user, err := rc.DatabaseInterface.GetUserByName(targetUserName)
		if err != nil {
			rc.ReportRedirect("/reset-password/request", 0,
				"Reset Failed",
				fmt.Sprintf("Failed to initiate password reset request: %s.", err.Error()),
				w, r,
			)
			return
		}
		if user.Email == targetEmail {
			iid, err := rc.ReceiptSystem.IssueReceipt(24*60, []string{
				"reset-password",
				strings.TrimSpace(targetUserName),
			})
			if err != nil {
				rc.ReportRedirect("/reset-password/request", 0,
					"Internal Error",
					fmt.Sprintf("Failed to initiate password reset request: %s. Please contact the site owner for this...", err.Error()),
					w, r,
				)
				return
			}
			go func() {
				rc.Mailer.SendPlainTextMail(
					targetEmail,
					fmt.Sprintf("Reset password instructions from %s", rc.Config.DepotName),
					fmt.Sprintf(`Dear user,

%s has received your request for a password reset. Please visit the following link to proceed with the process:

    %s/receipt?id=%s

This link would become invalid after 24 hours.

If this isn't you, you can simply ignore this message.`,
						rc.Config.DepotName,
						rc.Config.ProperHTTPHostName(),
						iid,
					),
				)
			}()
		}
		rc.ReportRedirect("/", 0, "Request Recieved", "Your request of password reset has been received. If the info matches, an email would be sent to your email address; please proceed from there.", w, r)
		},
	))

	http.HandleFunc("GET /reset-password/update-password", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			switch rc.Config.GlobalVisibility {
			case gitus.GLOBAL_VISIBILITY_MAINTENANCE:
				FoundAt(w, "/maintenance-notice")
				return
			case gitus.GLOBAL_VISIBILITY_SHUTDOWN:
				FoundAt(w, "/shutdown-notice")
				return
			}
			rid := strings.TrimSpace(r.URL.Query().Get("id"))
			if len(rid) <= 0 { FoundAt(w, "/reset-password/request"); return }
			re, err := rc.ReceiptSystem.RetrieveReceipt(rid)
			if err != nil {
				rc.ReportInternalError(
					fmt.Sprintf("Failed while retrieving receipt: %s", err.Error()),
					w, r,
				)
				return
			}
			if re.Expired() {
				rc.ReceiptSystem.CancelReceipt(rid)
				rc.ReportRedirect("/", 5, "Receipt Expired", "The receipt you've received has passed its validity time limit. Please go through the process again.", w, r)
				return
			}
			if re.Command[0] != receipt.RESET_PASSWORD && len(re.Command) != 1 {
				rc.ReceiptSystem.CancelReceipt(rid)
				rc.ReportRedirect("/", 5, "Invalid Receipt", "The receipt you've provided is invalid. Please try again.", w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("reset-password-update").Execute(w, struct {
				Config *gitus.GitusConfig
				ReceiptId string
				LoginInfo *templates.LoginInfoModel
			}{
				Config: rc.Config,
				ReceiptId: rid,
				LoginInfo: nil,
			}))
		},
	))

	http.HandleFunc("POST /reset-password/update-password", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			CSRFCheck, UseLoginInfo, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			switch rc.Config.GlobalVisibility {
			case gitus.GLOBAL_VISIBILITY_MAINTENANCE:
				FoundAt(w, "/maintenance-notice")
				return
			case gitus.GLOBAL_VISIBILITY_SHUTDOWN:
				FoundAt(w, "/shutdown-notice")
				return
			}
			rid := strings.TrimSpace(r.Form.Get("rid"))
			re, err := rc.ReceiptSystem.RetrieveReceipt(rid)
			if err != nil {
				rc.ReportNormalError(
					fmt.Sprintf("Internal error: %s", err.Error()),
					w, r,
				)
				return
			}
			if re.Expired() {
				rc.ReceiptSystem.CancelReceipt(rid)
				rc.ReportRedirect("/", 5, "Receipt Expired", "The receipt you've received has passed its validity time limit. Please go through the process again.", w, r)
				return
			}
			if re.Command[0] != receipt.RESET_PASSWORD && len(re.Command) != 1 {
				rc.ReceiptSystem.CancelReceipt(rid)
				rc.ReportRedirect("/", 5, "Invalid Receipt", "The receipt you've provided is invalid. Please try again.", w, r)
				return
			}
			targetUserName := re.Command[1]
			if !model.ValidUserName(targetUserName) {
				rc.ReportNotFound(targetUserName, "User", "Depot", w, r)
				return
			}
			newPassword := strings.TrimSpace(r.Form.Get("password"))
			confirm := strings.TrimSpace(r.Form.Get("confirm"))
			if newPassword != confirm {
				rc.ReportRedirect(
					fmt.Sprintf("/reset-password/update-password?id=%s", rid),
					3,
					"Password Not Match",
					"The password you entered does not match. Please enter the *same* password in both of the password fields.",
					w, r,
				)
				return
			}
			newPassHashBytes, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
			if err != nil {
				rc.ReportNormalError(
					fmt.Sprintf("Internal error: %s", err.Error()),
					w, r,
				)
				return
			}
			err = rc.DatabaseInterface.UpdateUserPassword(targetUserName, string(newPassHashBytes))
			if err != nil {
				rc.ReportNormalError(
					fmt.Sprintf("Internal error: %s", err.Error()),
					w, r,
				)
				return
			}
			rc.ReceiptSystem.CancelReceipt(rid)
			rc.ReportRedirect("/login", 3, "Password Updated", "Your password has been updated.", w, r)
		},
	))
	
}

