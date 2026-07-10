package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindSettingGPGController(ctx *RouterContext) {
	http.HandleFunc("GET /setting/gpg", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired,
			GlobalVisibility,
			ErrorGuard,
		}, ctx, 
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := rc.LoginInfo.UserName
			s, err := rc.DatabaseInterface.GetAllSignKeyByUsername(un)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("setting/gpg-key").Execute(w, templates.SettingGPGKeyTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				KeyList: s,
			}))
		},
	))
	
	http.HandleFunc("POST /setting/gpg", UseMiddleware(
		[]Middleware{
			Logged, ValidPOSTRequestRequired, LoginRequired, CSRFCheck,
			GlobalVisibility,
			ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := rc.LoginInfo.UserName
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			confirmPassword := strings.TrimSpace(r.Form.Get("password"))
			chkres, err := checkUserPassword(ctx, un, confirmPassword)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			if !chkres {
				rc.ReportRedirect("/setting/gpg", 3, "Password Mismatch", "The password you've provided does not match. Please try again.", w, r)
				return
			}
			keyText := strings.TrimSpace(r.Form.Get("key-text"))
			if len(keyText) <= 0 {
				rc.ReportRedirect("/setting/gpg", 3, "Invalid Key Text", "Key text cannot be empty.", w, r)
				return
			}
			s := strings.Split(keyText, " ")
			keyName := ""
			if len(s) < 3 {
				keyName = "key_" + auxfuncs.GenSym(8)
			} else {
				keyName = s[2]
			}
			err = rc.DatabaseInterface.RegisterSignKey(un, keyName, keyText)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			FoundAt(w, "/setting/gpg")
		},
	))
	
	http.HandleFunc("GET /setting/gpg/{keyName}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			ctx.ReportSingleButtonCallback(
				fmt.Sprintf("/setting/gpg/{keyName}/delete", r.PathValue("keyName")),
				"Delete GPG Key",
				fmt.Sprintf("Click the following button to delete GPG key <code>%s</code>", r.PathValue("keyName")),
				"Delete",
				nil,
				w, r,
			)
		},
	))
	http.HandleFunc("POST /setting/gpg/{keyName}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired,
			CSRFCheck, GlobalVisibility,
			ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request){
			un := rc.LoginInfo.UserName
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			err := rc.DatabaseInterface.RemoveSignKey(un, r.PathValue("keyName"))
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			FoundAt(w, "/setting/gpg")
		},
	))
	
	http.HandleFunc("GET /setting/gpg/{keyName}/edit", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired,
			GlobalVisibility,
			ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request){
			un := rc.LoginInfo.UserName
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			k, err := rc.DatabaseInterface.GetSignKeyByName(un, r.PathValue("keyName"))
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("setting/edit-gpg-key").Execute(w, &templates.SettingEditGPGKeyTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				Key: k,
			}))
		},
	))
	
	http.HandleFunc("POST /setting/gpg/{keyName}/edit", UseMiddleware(
		[]Middleware{
			Logged, ValidPOSTRequestRequired, LoginRequired, CSRFCheck,
			GlobalVisibility,
			ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request){
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
				rc.ReportRedirect(fmt.Sprintf("/setting/gpg/%s/edit", keyName), 3, "Password Mismatch", "The password you've provided does not match. Please try again.", w, r)
				return
			}
			keyText := r.Form.Get("key-text")
			err = rc.DatabaseInterface.UpdateSignKey(un, keyName, keyText)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/setting/gpg/%s/edit", keyName), 3, "Updated", "Updated.", w, r)
		},
	))
}

