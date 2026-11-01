package controller

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
	"golang.org/x/crypto/bcrypt"
)


func bindLoginController(ctx *RouterContext) {
	http.HandleFunc("GET /login", UseMiddleware(
		[]Middleware{Logged, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			if ctx.Config.EffectiveGlobalVisibility() == gitus.GLOBAL_VISIBILITY_MAINTENANCE {
				FoundAt(w, "/maintenance-notice")
				return
			}
			if rc.LoginInfo == nil || rc.LoginInfo.LoggedIn { FoundAt(w, "/") }
			LogTemplateError(ctx.LoadTemplate("login").Execute(w, templates.LoginTemplateModel{
				Config: ctx.Config,
				LoginInfo: rc.LoginInfo,
				Callback: r.URL.Query().Get("callback"),
			}))
		},
	))

	http.HandleFunc("POST /login", UseMiddleware(
		[]Middleware{
			Logged, RateLimit, ValidPOSTRequestRequired, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			if rc.Config.EffectiveGlobalVisibility() == gitus.GLOBAL_VISIBILITY_MAINTENANCE {
				FoundAt(w, "/maintenance-notice")
				return
			}
			err := r.ParseForm()
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			un := r.Form.Get("username")
			ph := r.Form.Get("password")
			u, err := rc.DatabaseInterface.GetUserByName(un)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			switch u.Status {
			case model.BANNED: 
				LogTemplateError(rc.LoadTemplate("login").Execute(w, templates.LoginTemplateModel{
					Config: rc.Config,
					ErrorMsg: "User suspended.",
				}))
				return
			case model.NORMAL_USER_APPROVAL_NEEDED:
				LogTemplateError(rc.LoadTemplate("login").Execute(w, templates.LoginTemplateModel{
					Config: rc.Config,
					ErrorMsg: "User waiting for approval.",
				}))
				return
			case model.NORMAL_USER_CONFIRM_NEEDED:
				LogTemplateError(rc.LoadTemplate("login").Execute(w, templates.LoginTemplateModel{
					Config: rc.Config,
					ErrorMsg: "Confirmation needed.",
				}))
				return
			}
			
			err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(ph))
			if err == bcrypt.ErrMismatchedHashAndPassword {
				LogTemplateError(rc.LoadTemplate("login").Execute(w, templates.LoginTemplateModel{
					Config: rc.Config,
					ErrorMsg: "Invalid username or password.",
				}))
				return
			} else if err != nil {
				LogTemplateError(rc.LoadTemplate("login").Execute(w, templates.LoginTemplateModel{
					Config: rc.Config,
					ErrorMsg: "Internal error: " + err.Error(),
				}))
				return
			}

			if u.TFAConfig.AnySecondFactor() {
				if rc.ConfirmCodeManager == nil {
					rc.ReportInternalError("Confirm code manager not initialized. Please contact site owner to fix this problem... ", w, r)
					return
				}
				// The challenge binds this password check to the code check:
				// with email 2FA it is the code that is sent, with an
				// authenticator app it is a value only the server knows. It
				// expires with the entry in the confirm code manager.
				challenge, err := newConfirmCode()
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to generate a confirmation code: %s.", err), w, r)
					return
				}
				tempKey, err := bcrypt.GenerateFromPassword([]byte(u.PasswordHash+challenge), ctx.Config.PasswordHashStrength)
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to process generated confirmation code: %s.", err), w, r)
					return
				}
				// Remember which factors were required at this point, so that a
				// configuration change in another tab cannot turn the pending
				// login into a weaker one.
				factors := ""
				if u.TFAConfig.Email.Enable { factors += "e" }
				if u.TFAConfig.OTP.Enable { factors += "o" }
				rc.ConfirmCodeManager.Register(un, challenge + " " + factors, 10 * time.Minute)
				if u.TFAConfig.Email.Enable {
					if rc.Mailer == nil {
						rc.ReportInternalError("Mailer not initialized. Please contact site owner to fix this problem...", w, r)
						return
					}
					err = rc.Mailer.SendPlainTextMail(u.Email, fmt.Sprintf("Confirmation Code For Login - %s", rc.Config.DepotName), fmt.Sprintf(`Hello %s,

You're now trying to log in to %s. Since your account has set up email-based two-factor authentication, we have sent you this email.

At the login page you should see a prompt asking you to enter a confirmation code. The code is as follows:

    %s

If this isn't you, we advise you to change your password on %s and other platforms (if you have reused the same password) immediately.

%s
`, u.Name, rc.Config.DepotName, challenge, rc.Config.DepotName, rc.Config.DepotName))
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to send confirmation code email: %s.", err), w, r)
						return
					}
				}
				w.Header().Add("Set-Cookie", (&http.Cookie{
					Name: COOKIE_KEY_USERNAME,
					Value: u.Name,
					Path: "/",
					MaxAge: 600,
					HttpOnly: true,
					Secure: true,
					SameSite: http.SameSiteLaxMode,
				}).String())
				w.Header().Add("Set-Cookie", (&http.Cookie{
					Name: COOKIE_KEY_TEMP_KEY,
					Value: string(tempKey),
					Path: "/",
					MaxAge: 600,
					HttpOnly: true,
					Secure: true,
					SameSite: http.SameSiteLaxMode,
				}).String())
				FoundAt(w, "/login/confirm")
				return
			}
			
			ss, err := session.NewSessionString()
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			_, err = rc.SessionInterface.RegisterSession(un, ss, session.GitusSessionMetadata{
				IP: ResolveMostPossibleIP(w, r),
				UserAgent: r.Header.Get("User-Agent"),
			})
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			
			w.Header().Add("Set-Cookie", (&http.Cookie{
				Name: COOKIE_KEY_SESSION,
				Value: ss,
				Path: "/",
				MaxAge: 3600,
				HttpOnly: true,
				Secure: true,
				SameSite: http.SameSiteLaxMode,
			}).String())
			w.Header().Add("Set-Cookie", (&http.Cookie{
				Name: "username",
				Value: un,
				Path: "/",
				MaxAge: 3600,
				HttpOnly: true,
				Secure: true,
				SameSite: http.SameSiteLaxMode,
			}).String())
			callbackURL := strings.TrimSpace(r.Form.Get("login-callback"))
			if callbackURL == "" { callbackURL = "/" }
			target, err := getQueryPath(callbackURL)
			if err != nil { target = "/" }
			FoundAt(w, target)
		},
	))

	http.HandleFunc("GET /login/confirm", UseMiddleware(
		[]Middleware{Logged, RateLimit, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			if rc.Config.EffectiveGlobalVisibility() == gitus.GLOBAL_VISIBILITY_MAINTENANCE {
				FoundAt(w, "/maintenance-notice")
				return
			}
			username, err := r.Cookie(COOKIE_KEY_USERNAME)
			if err != nil && err != http.ErrNoCookie{
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			factors := ""
			errMsg := ""
			if rc.ConfirmCodeManager != nil {
				if stored, ok := rc.ConfirmCodeManager.Get(username.Value); ok {
					_, factors, _ = strings.Cut(stored, " ")
				} else {
					errMsg = "The confirmation has expired. Please log in again."
				}
			}
			renderLoginConfirm(rc, w, username.Value, nil, factors, errMsg)
		},
	))

	http.HandleFunc("POST /login/confirm", UseMiddleware(
		[]Middleware{Logged, RateLimit, ValidPOSTRequestRequired, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			key, err := r.Cookie(COOKIE_KEY_TEMP_KEY)
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			username := strings.TrimSpace(r.Form.Get("username"))
			user, err := rc.DatabaseInterface.GetUserByName(username)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to retrieve user: %s.", err), w, r)
				return
			}
			if rc.ConfirmCodeManager == nil {
				rc.ReportInternalError("Confirm code manager not initialized. Please contact site owner to fix this problem...", w, r)
				return
			}
			// the challenge proves that the password was checked: it was
			// stored server side when the login form was submitted, and the
			// cookie holds bcrypt(password hash + challenge).
			stored, ok := rc.ConfirmCodeManager.Get(username)
			if !ok || len(stored) <= 0 {
				renderLoginConfirm(rc, w, username, user, "", "The confirmation has expired. Please log in again.")
				return
			}
			challenge, factors, _ := strings.Cut(stored, " ")
			err = bcrypt.CompareHashAndPassword([]byte(key.Value), []byte(user.PasswordHash+challenge))
			if err != nil {
				renderLoginConfirm(rc, w, username, user, factors, "Invalid or expired confirmation code.")
				return
			}
			if strings.Contains(factors, "e") {
				code := strings.ToUpper(strings.TrimSpace(r.Form.Get("confirmation-code")))
				if subtle.ConstantTimeCompare([]byte(code), []byte(challenge)) != 1 {
					renderLoginConfirm(rc, w, username, user, factors, "Invalid confirmation code.")
					return
				}
			}
			if strings.Contains(factors, "o") {
				counter, ok := verifyUserOTP(user, r.Form.Get("otp-code"), time.Now())
				if !ok {
					renderLoginConfirm(rc, w, username, user, factors, "Invalid authentication code.")
					return
				}
				// HOTP codes are tied to a counter, so remember the next one;
				// otherwise the same code could be replayed.
				if !user.TFAConfig.OTPIsTOTP() {
					user.TFAConfig.OTP.Counter = counter + 1
					err = rc.DatabaseInterface.UpdateUserInfo(username, user)
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to update the authentication counter: %s.", err), w, r)
						return
					}
				}
			}

			ss, err := session.NewSessionString()
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			_, err = rc.SessionInterface.RegisterSession(username, ss, session.GitusSessionMetadata{
				IP: ResolveMostPossibleIP(w, r),
				UserAgent: r.Header.Get("User-Agent"),
			})
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			
			w.Header().Add("Set-Cookie", (&http.Cookie{
				Name: COOKIE_KEY_SESSION,
				Value: ss,
				Path: "/",
				MaxAge: 3600,
				HttpOnly: true,
				Secure: true,
				SameSite: http.SameSiteLaxMode,
			}).String())
			w.Header().Add("Set-Cookie", (&http.Cookie{
				Name: "username",
				Value: username,
				Path: "/",
				MaxAge: 3600,
				HttpOnly: true,
				Secure: true,
				SameSite: http.SameSiteLaxMode,
			}).String())
			FoundAt(w, "/")
		},
	))
}


