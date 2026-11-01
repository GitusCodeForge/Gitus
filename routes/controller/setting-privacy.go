package controller

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/otp"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
	"github.com/skip2/go-qrcode"
)


// renderPrivacyPage renders the privacy settings page. newRecoveryCodes is only
// non-empty right after they were generated, so they are shown once.
func renderPrivacyPage(rc *RouterContext, w http.ResponseWriter, user *model.GitusUser, newRecoveryCodes []string, errMsg string) {
	m := &templates.SettingPrivacyTemplateModel{
		Config: rc.Config,
		User: user,
		LoginInfo: rc.LoginInfo,
		NewRecoveryCodes: newRecoveryCodes,
	}
	if len(errMsg) > 0 {
		m.ErrorMsg.Type = "info"
		m.ErrorMsg.Message = errMsg
	}
	// while a setup is pending the page shows the secret and the URL the
	// authenticator app needs.
	if user.TFAConfig.OTP.Pending && len(user.TFAConfig.OTP.Secret) > 0 {
		typ := otp.TYPE_TOTP
		if !user.TFAConfig.OTPIsTOTP() { typ = otp.TYPE_HOTP }
		m.ProvisioningURL = otp.ProvisioningURL(
			rc.Config.DepotName, user.Name, typ,
			user.TFAConfig.OTP.Secret,
			user.TFAConfig.OTP.Digits,
			user.TFAConfig.OTP.Period,
			user.TFAConfig.OTP.Counter,
		)
	}
	routes.LogTemplateError(rc.LoadTemplate("setting/privacy").Execute(w, m))
}

func bindSettingPrivacyController(ctx *RouterContext) {
	http.HandleFunc("GET /setting/privacy", UseMiddleware(
		[]Middleware{Logged, LoginRequired, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			user, err := rc.DatabaseInterface.GetUserByName(rc.LoginInfo.UserName)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while retrieving user: %s\n", err), w, r)
				return
			}
			renderPrivacyPage(rc, w, user, nil, "")
		},
	))
	
	// The QR code of the pending setup. It is a route of its own instead of an
	// inline data: URI because html/template refuses data: URLs in src.
	http.HandleFunc("GET /setting/privacy/otp-qr", UseMiddleware(
		[]Middleware{Logged, LoginRequired, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			user, err := rc.DatabaseInterface.GetUserByName(rc.LoginInfo.UserName)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while retrieving user: %s", err), w, r)
				return
			}
			if !user.TFAConfig.OTP.Pending || len(user.TFAConfig.OTP.Secret) <= 0 {
				http.NotFound(w, r)
				return
			}
			typ := otp.TYPE_TOTP
			if !user.TFAConfig.OTPIsTOTP() { typ = otp.TYPE_HOTP }
			u := otp.ProvisioningURL(
				rc.Config.DepotName, user.Name, typ,
				user.TFAConfig.OTP.Secret,
				user.TFAConfig.OTP.Digits,
				user.TFAConfig.OTP.Period,
				user.TFAConfig.OTP.Counter,
			)
			png, err := qrcode.Encode(u, qrcode.Medium, 320)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to render the QR code: %s", err), w, r)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(png)
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
				case "otp-start":
					if user.TFAConfig.OTP.Enable {
						rc.ReportRedirect("/setting/privacy", 5, "Already Enabled", "An authenticator app is already configured; disable it first to set up a new one.", w, r)
						return
					}
					typ := strings.TrimSpace(r.Form.Get("otp-type"))
					if typ != otp.TYPE_HOTP { typ = otp.TYPE_TOTP }
					secret, err := otp.GenerateSecret()
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to generate a secret: %s", err), w, r)
						return
					}
					user.TFAConfig.OTP = model.GitusUser2FAOTPConfig{
						Enable: false,
						Pending: true,
						Type: typ,
						Secret: secret,
						Digits: otp.DefaultDigits,
						Period: otp.DefaultPeriod,
						Counter: 0,
					}
					if err = rc.DatabaseInterface.UpdateUserInfo(rc.LoginInfo.UserName, user); err != nil {
						rc.ReportInternalError(err.Error(), w, r)
						return
					}
					rc.ReportRedirect("/setting/privacy", 0, "Setup Started",
						"Add the secret below to your authenticator app, then enter the code it shows to finish the setup.", w, r)
					return
				case "otp-confirm":
					if !user.TFAConfig.OTP.Pending || len(user.TFAConfig.OTP.Secret) <= 0 {
						rc.ReportRedirect("/setting/privacy", 5, "No Pending Setup", "There is no authenticator setup to confirm; start one first.", w, r)
						return
					}
					counter, ok := verifyUserOTP(user, r.Form.Get("otp-code"), time.Now())
					if !ok {
						rc.ReportRedirect("/setting/privacy", 5, "Invalid Code", "The code does not match the secret; check that the app is set up correctly and try again.", w, r)
						return
					}
					user.TFAConfig.OTP.Pending = false
					user.TFAConfig.OTP.Enable = true
					if user.TFAConfig.OTPIsTOTP() {
						user.TFAConfig.OTP.Counter = 0
					} else {
						// the confirmed code is used up for HOTP.
						user.TFAConfig.OTP.Counter = counter + 1
					}
					// a fresh set of recovery codes goes with a fresh secret:
					// they are the way back in when the app is gone.
					plainCodes, hashes, err := otp.GenerateRecoveryCodes(otp.RecoveryCodeCount)
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to generate recovery codes: %s", err), w, r)
						return
					}
					user.TFAConfig.OTP.RecoveryCodes = hashes
					if err = rc.DatabaseInterface.UpdateUserInfo(rc.LoginInfo.UserName, user); err != nil {
						rc.ReportInternalError(err.Error(), w, r)
						return
					}
					renderPrivacyPage(rc, w, user, plainCodes,
						"Authenticator app two-factor authentication is now enabled. These recovery codes are shown only once: keep them somewhere safe, each one lets you in once if the app is gone.")
					return
				case "otp-cancel":
					user.TFAConfig.OTP = model.GitusUser2FAOTPConfig{}
					if err = rc.DatabaseInterface.UpdateUserInfo(rc.LoginInfo.UserName, user); err != nil {
						rc.ReportInternalError(err.Error(), w, r)
						return
					}
					rc.ReportRedirect("/setting/privacy", 5, "Cancelled", "The pending authenticator setup has been discarded.", w, r)
					return
				case "otp-recovery-generate":
					if !user.TFAConfig.OTP.Enable {
						rc.ReportRedirect("/setting/privacy", 5, "Not Enabled", "Enable an authenticator app first.", w, r)
						return
					}
					ok, err := checkUserPassword(rc, rc.LoginInfo.UserName, r.Form.Get("password"))
					if err != nil {
						rc.ReportInternalError(err.Error(), w, r)
						return
					}
					if !ok {
						rc.ReportRedirect("/setting/privacy", 5, "Invalid Password", "The password you entered is not correct.", w, r)
						return
					}
					plainCodes, hashes, err := otp.GenerateRecoveryCodes(otp.RecoveryCodeCount)
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to generate recovery codes: %s", err), w, r)
						return
					}
					user.TFAConfig.OTP.RecoveryCodes = hashes
					if err = rc.DatabaseInterface.UpdateUserInfo(rc.LoginInfo.UserName, user); err != nil {
						rc.ReportInternalError(err.Error(), w, r)
						return
					}
					renderPrivacyPage(rc, w, user, plainCodes,
						"New recovery codes. The previous ones no longer work. These are shown only once: keep them somewhere safe.")
					return
				case "otp-disable":
					ok, err := checkUserPassword(rc, rc.LoginInfo.UserName, r.Form.Get("password"))
					if err != nil {
						rc.ReportInternalError(err.Error(), w, r)
						return
					}
					if !ok {
						rc.ReportRedirect("/setting/privacy", 5, "Invalid Password", "The password you entered is not correct.", w, r)
						return
					}
					user.TFAConfig.OTP = model.GitusUser2FAOTPConfig{}
					if err = rc.DatabaseInterface.UpdateUserInfo(rc.LoginInfo.UserName, user); err != nil {
						rc.ReportInternalError(err.Error(), w, r)
						return
					}
					rc.ReportRedirect("/setting/privacy", 5, "Disabled", "Authenticator app two-factor authentication has been disabled.", w, r)
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

