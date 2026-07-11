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


func bindRegisterController(ctx *RouterContext) {
	http.HandleFunc("GET /reg", UseMiddleware(
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
			if !rc.Config.AllowRegistration { FoundAt(w, "/"); return }
			LogTemplateError(rc.LoadTemplate("registration").Execute(w, templates.RegistrationTemplateModel{
				Config: rc.Config,
				ErrorMsg: "",
				LoginInfo: rc.LoginInfo,
			}))
		},
	))

	http.HandleFunc("POST /reg", UseMiddleware(
		[]Middleware{Logged, RateLimit, ValidPOSTRequestRequired, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			switch rc.Config.GlobalVisibility {
			case gitus.GLOBAL_VISIBILITY_MAINTENANCE:
				FoundAt(w, "/maintenance-notice")
				return
			case gitus.GLOBAL_VISIBILITY_SHUTDOWN:
				FoundAt(w, "/shutdown-notice")
				return
			}
			if !rc.Config.AllowRegistration {
				rc.ReportNormalError("Registration not allowed on this instance.", w, r)
				return
			}
			userName := r.Form.Get("username")
			if !model.ValidUserName(userName) {
				rc.ReportRedirect("/reg", 5, "Invalid User Name", "User name must consists of only upper & lowercase letters (a-z, A-Z), 0-9, underscore and hyphen.", w, r)
				return
			}
			email := r.Form.Get("email")

			// username & ns name check.
			_, err := rc.DatabaseInterface.GetUserByName(userName)
			if err == nil {
				LogTemplateError(rc.LoadTemplate("registration").Execute(w, &templates.RegistrationTemplateModel{
					Config: rc.Config,
					LoginInfo: nil,
					ErrorMsg: "Username/Namespace name already exists. Please try another name.",
				}))
				return
			}
			_, err = rc.DatabaseInterface.GetNamespaceByName(userName)
			if err == nil {
				LogTemplateError(rc.LoadTemplate("registration").Execute(w, &templates.RegistrationTemplateModel{
					Config: rc.Config,
					LoginInfo: nil,
					ErrorMsg: "Username/Namespace name already exists. Please try another name.",
				}))
				return
			}
			if rc.Config.ReadingRequiredDocument != nil {
				for i := range rc.Config.ReadingRequiredDocument {
					if len(strings.TrimSpace(r.Form.Get(fmt.Sprintf("req-%d", i)))) <= 0 {
						rc.ReportRedirect("/reg", 5, "Field Required", "You must consent to all the required documents.", w, r)
						return
					}
				}
			}
			
			password := r.Form.Get("password")
			passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), ctx.Config.PasswordHashStrength)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to hash the provided password: %s. Please try again.", err.Error()), w, r)
				return
			}

			var succeedMsg string
			passwordHashStr := string(passwordHash)
			newUserStatus := model.NORMAL_USER
			if rc.Config.DefaultNewUserStatus != 0 {
				newUserStatus = rc.Config.DefaultNewUserStatus
			}
			
			if rc.Config.ManualApproval || newUserStatus == model.NORMAL_USER_APPROVAL_NEEDED {
				err = rc.DatabaseInterface.InsertRegistrationRequest(userName, email, passwordHashStr, strings.TrimSpace(r.Form.Get("reason")))
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to submit registration request: %s. Please contact the site owner.", err.Error()), w, r)
					return
				}
				_, err = rc.DatabaseInterface.RegisterUser(userName, email, passwordHashStr, model.NORMAL_USER_APPROVAL_NEEDED)
				if err != nil {
					LogTemplateError(rc.LoadTemplate("registration").Execute(w, &templates.RegistrationTemplateModel{
						Config: rc.Config,
						LoginInfo: nil,
						ErrorMsg: fmt.Sprintf("Error while registering: %s. Please try again.", err.Error()),
					}))
					return
				}
				msg := "Your registration request has been submitted. "
				if rc.Config.EmailConfirmationRequired {
					msg += " You will receive the confirmation email after the administrators approved your request."
				} else {
					msg += " Your account would be usable after the administrators approved your request."
				}
				rc.ReportRedirect("/", 0, "Request Submitted", msg, w, r)
				return
			}
			
			if rc.Config.EmailConfirmationRequired || newUserStatus == model.NORMAL_USER_CONFIRM_NEEDED {
				_, err = rc.DatabaseInterface.RegisterUser(userName, email, passwordHashStr, model.NORMAL_USER_CONFIRM_NEEDED)
				if err != nil {
					LogTemplateError(rc.LoadTemplate("registration").Execute(w, &templates.RegistrationTemplateModel{
						Config: rc.Config,
						LoginInfo: nil,
						ErrorMsg: fmt.Sprintf("Error while registering: %s. Please try again.", err.Error()),
					}))
					return
				}
				command := make([]string, 3)
				command[0] = receipt.CONFIRM_REGISTRATION
				command[1] = userName
				command[2] = email
				rid, err := rc.ReceiptSystem.IssueReceipt(24*60, command)
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to issue receipt for registration: %s", err.Error()), w, r)
					return
				}
				go func() {
					email := r.Form.Get("email")
					title := fmt.Sprintf("Confirmation of registering on %s", rc.Config.DepotName)
					body := fmt.Sprintf(`
This email is used to register on %s, a code repository hosting platform.

If this isn't you, you don't need to do anything about it, as the registration
request expires after 24 hours; but if this is you, please copy & open the
following link to confirm your registration:

    %s/receipt?id=%s

We wish you all the best in your future endeavours.

%s
`, rc.Config.DepotName, rc.Config.ProperHTTPHostName(), rid, rc.Config.DepotName)
					err = rc.Mailer.SendPlainTextMail(email, title, body)
				}()
				succeedMsg = "A confirmation email has been sent to the email address you have specified. Please proceed from there."
				rc.ReportRedirect("/", 0, "Request Submitted", succeedMsg, w, r)
				return
			}
			
			_, err = rc.DatabaseInterface.RegisterUser(userName, email, passwordHashStr, newUserStatus)
			if err != nil {
				LogTemplateError(rc.LoadTemplate("registration").Execute(w, &templates.RegistrationTemplateModel{
					Config: rc.Config,
					LoginInfo: nil,
					ErrorMsg: fmt.Sprintf("Error while registering: %s. Please try again.", err.Error()),
				}))
				return
			}
			if rc.Config.UseNamespace {
				if newUserStatus == model.NORMAL_USER {
					_, err = rc.DatabaseInterface.RegisterNamespace(userName, userName)
					if err != nil {
						rc.ReportInternalError(
							fmt.Sprintf("Failed at registering namespace %s. Please contact site admin for this issue.", err.Error()),
							w, r,
						)
						return
					}
				}
				if len(rc.Config.DefaultNewUserNamespace) > 0 {
					ns, err := rc.DatabaseInterface.GetNamespaceByName(rc.Config.DefaultNewUserNamespace)
					if err != nil {
						rc.ReportInternalError(
							fmt.Sprintf("Failed at getting default new user namespace: %s. Please contact site admin for this issue.", err.Error()),
							w, r,
						)
						return
					}
					ns.ACL.ACL[userName] = &model.ACLTuple{
						AddMember: false,
						DeleteMember: false,
						EditMember: false,
						EditInfo: false,
						AddRepository: true,
						PushToRepository: false,
						ArchiveRepository: false,
						DeleteRepository: false,
						EditHooks: false,
						EditWebHooks: false,
					}
					err = rc.DatabaseInterface.UpdateNamespaceInfo(ns.Name, ns)
					if err != nil {
						rc.ReportInternalError(
							fmt.Sprintf("Failed when updating namespace info: %s. Please contact site admin for this issue.", err.Error()),
							w, r,
						)
						return
					}
				}
			}
			succeedMsg = "Registration complete. You can now login."
			loginInfo, _ := GenerateLoginInfoModel(ctx, r)
			LogTemplateError(rc.LoadTemplate("error").Execute(w, &templates.ErrorTemplateModel{
				Config: rc.Config,
				ErrorCode: 200,
				ErrorMessage: succeedMsg,
				LoginInfo: loginInfo,
			}))
		},
	))
}

	
