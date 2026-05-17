package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
	"golang.org/x/crypto/bcrypt"
)


func bindSettingSSHController(ctx *RouterContext) {
	http.HandleFunc("GET /setting/ssh", UseMiddleware(
		[]Middleware{Logged, LoginRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
		un := rc.LoginInfo.UserName
		if !model.ValidUserName(un) {
			rc.ReportNotFound(un, "User", "Depot", w, r)
			return
		}
		s, err := rc.DatabaseInterface.GetAllAuthKeyByUsername(un)
		if err != nil {
			rc.ReportInternalError(fmt.Sprintf("Failed to retrieve authentication key: %s", err), w, r)
			return
		}
		LogTemplateError(rc.LoadTemplate("setting/ssh-key").Execute(w, templates.SettingSSHKeyTemplateModel{
			Config: rc.Config,
			LoginInfo: rc.LoginInfo,
			KeyList: s,
		}))
		},
	))
	
	http.HandleFunc("POST /setting/ssh", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := rc.LoginInfo.UserName
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			keyList, err := rc.DatabaseInterface.GetAllAuthKeyByUsername(un)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to retrieve authentication key: %s", err), w, r)
				return
			}
			confirmPassword := strings.TrimSpace(r.Form.Get("password"))
			u, err := rc.DatabaseInterface.GetUserByName(un)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get user: %s", err), w, r)
				return
			}
			err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(confirmPassword))
			if err == bcrypt.ErrMismatchedHashAndPassword {
				LogTemplateError(rc.LoadTemplate("setting/ssh-key").Execute(w, templates.SettingSSHKeyTemplateModel{
					Config: rc.Config,
					LoginInfo: rc.LoginInfo,
					KeyList: keyList,
					ErrorMsg: struct{Type string; Message string}{
						Type: "",
						Message: "Invalid confirmation password",
					},
				}))
				return
			}
			keyText := strings.TrimSpace(r.Form.Get("key-text"))
			if len(strings.TrimSpace(keyText)) <= 0 {
				LogTemplateError(rc.LoadTemplate("setting/ssh-key").Execute(w, templates.SettingSSHKeyTemplateModel{
					Config: rc.Config,
					LoginInfo: rc.LoginInfo,
					KeyList: keyList,
					ErrorMsg: struct{Type string; Message string}{
						Type: "",
						Message: "Invalid key text",
					},
				}))
				return
			}
			s := strings.Split(keyText, " ")
			keyName := ""
			if len(s) < 3 {
				keyName = "key_" + auxfuncs.GenSym(8)
			} else {
				keyName = s[2]
			}
			keyName = strings.TrimSpace(keyName)
			err = rc.DatabaseInterface.RegisterAuthKey(un, keyName, keyText)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			rc.SSHKeyManagingContext.AddAuthorizedKey(un, keyName, keyText)
			err = rc.SSHKeyManagingContext.Sync()
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			FoundAt(w, "/setting/ssh")
		},
	))
	
	http.HandleFunc("GET /setting/ssh/{keyName}/delete", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, LoginRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := rc.LoginInfo.UserName
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			keyName := r.PathValue("keyName")
			err := rc.DatabaseInterface.RemoveAuthKey(un, keyName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			rc.SSHKeyManagingContext.RemoveAuthorizedKey(un, keyName)
			err = rc.SSHKeyManagingContext.Sync()
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			FoundAt(w, "/setting/ssh")
		},
	))
	
	http.HandleFunc("GET /setting/ssh/{keyName}/edit", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, LoginRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := rc.LoginInfo.UserName
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			k, err := rc.DatabaseInterface.GetAuthKeyByName(un, r.PathValue("keyName"))
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("setting/edit-ssh-key").Execute(w, &templates.SettingEditSSHKeyTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				Key: k,
			}))
		},
	))
	
	http.HandleFunc("POST /setting/ssh/{keyName}/edit", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := rc.LoginInfo.UserName
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			keyName := r.PathValue("keyName")
			chkres, err := checkUserPassword(ctx, un, r.Form.Get("password"))
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
			}
			if !chkres {
				rc.ReportRedirect(fmt.Sprintf("/setting/ssh/%s/edit", keyName), 3, "Password Mismatch", "The password you've provided does not match. Please try again.", w, r)
				return
			}
			keyText := r.Form.Get("key-text")
			err = rc.DatabaseInterface.UpdateAuthKey(un, keyName, keyText)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/setting/ssh/%s/edit", keyName), 3, "Updated", "Updated.", w, r)
		},
	))
}

