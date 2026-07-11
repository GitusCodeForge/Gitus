package controller

import (
	"fmt"
	"net/http"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/receipt"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
	"golang.org/x/crypto/bcrypt"
)

func bindConfirmRegistrationController(ctx *RouterContext) {
	http.HandleFunc("GET /confirm-registration", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			if ctx.Config.GlobalVisibility == gitus.GLOBAL_VISIBILITY_SHUTDOWN {
				FoundAt(w, "/shutdown-notice")
				return
			}
			if ctx.Config.GlobalVisibility == gitus.GLOBAL_VISIBILITY_MAINTENANCE {
				FoundAt(w, "/maintenance-notice")
				return
			}
			rid := r.URL.Query().Get("id")
			re, err := ctx.ReceiptSystem.RetrieveReceipt(rid)
			if err != nil { FoundAt(w, "/"); return }
			if re.Expired() {
				ctx.ReceiptSystem.CancelReceipt(rid)
				ctx.ReportRedirect("/", 5, "Receipt Expired", "The confirmation receipt has expired. If you're the owner of the account, please contact site owner.", w, r)
				return
			}
			LogTemplateError(ctx.LoadTemplate("reg-confirm").Execute(w, &templates.RegConfirmTemplateModel{
				Config: ctx.Config,
				LoginInfo: ctx.LoginInfo,
				ReceiptID: rid,
				Username: re.Command[1],
			}))
		},
	))
	http.HandleFunc("POST /confirm-registration", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, ValidPOSTRequestRequired, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			if ctx.Config.GlobalVisibility == gitus.GLOBAL_VISIBILITY_SHUTDOWN {
				FoundAt(w, "/shutdown-notice")
				return
			}
			if ctx.Config.GlobalVisibility == gitus.GLOBAL_VISIBILITY_MAINTENANCE {
				FoundAt(w, "/maintenance-notice")
				return
			}
			rid := r.Form.Get("rid")
			re, err := ctx.ReceiptSystem.RetrieveReceipt(rid)
			if err != nil { FoundAt(w, "/"); return }
			if re.Expired() {
				ctx.ReceiptSystem.CancelReceipt(rid)
				ctx.ReportRedirect("/", 5, "Receipt Expired", "The confirmation receipt has expired. If you're the owner of the account, please contact site owner.", w, r)
				return
			}
			if len(re.Command) != 3 || re.Command[0] != receipt.CONFIRM_REGISTRATION {
				// invalid receipt command...
				FoundAt(w, "/")
				return
			}
			username := re.Command[1]
			u, err := ctx.DatabaseInterface.GetUserByName(username)
			if err != nil {
				ctx.ReportInternalError(
					fmt.Sprintf("Failed while confirming: %s. Please contact site admin for this issue.", err.Error()),
					w, r,
				)
				return
			}
			err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(r.Form.Get("confirmation-password")))
			if err == bcrypt.ErrMismatchedHashAndPassword {
				ctx.ReportRedirect(
					// TODO: fix redirect target
					"/", 5,
					"Receipt Expired",
					"The confirmation receipt has expired. If you're the owner of the account, please contact site owner.", w, r)
				return
			}
			if err != nil {
				ctx.ReportInternalError(
					fmt.Sprintf("Failed while confirming: %s. Please contact site admin for this issue.", err.Error()),
					w, r,
				)
				return
			}
			status := model.NORMAL_USER
			// NOTE: per the reg flow (see
			// routes/controller/register.go), if the instance
			// requires manual approval, that path is always followed
			// through first - i.e. when a user reaches the stage of
			// requiring email confirmation, his registration must've
			// already approved by an admin.  this is the reason we
			// don't check it here.
			ctx.ReceiptSystem.CancelReceipt(rid)
			err = ctx.DatabaseInterface.UpdateUserStatus(username, status)
			if err != nil {
				ctx.ReportInternalError(
					fmt.Sprintf("Failed at confirming: %s. Please contact site admin for this issue.", err.Error()),
					w, r,
				)
				return
			}
			if ctx.Config.UseNamespace {
				_, err = ctx.DatabaseInterface.RegisterNamespace(username, username)
				if err != nil {
					ctx.ReportInternalError(
						fmt.Sprintf("Failed at registering namespace %s. Please contact site admin for this issue.", err.Error()),
						w, r,
					)
					return
				}
			}
			LogTemplateError(ctx.LoadTemplate("error").Execute(w, &templates.ErrorTemplateModel{
				Config: ctx.Config,
				ErrorCode: 200,
				ErrorMessage: "Registration complete. You can try to login now.",
				LoginInfo: nil,
			}))
		},
	))
}








































