package controller

import (
	"fmt"
	"maps"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindSnippetController(ctx *RouterContext) {
	http.HandleFunc("GET /snippet/{username}/{name}", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			username := r.PathValue("username")
			name := r.PathValue("name")
			sn, err := rc.DatabaseInterface.GetSnippet(username, name)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			if rc.LoginInfo == nil || !rc.LoginInfo.IsAdmin {
				switch sn.Status {
				case model.SNIPPET_PUBLIC:
					// intentionally left blank
				case model.SNIPPET_INTERNAL:
					if rc.LoginInfo == nil || !rc.LoginInfo.LoggedIn {
						rc.ReportNotFound(fmt.Sprintf("%s:%s", username, name), "Snippet", "Depot", w, r)
						return
					}
				case model.SNIPPET_SHARED_LINK:
					// intentionally left blank
				case model.SNIPPET_SHARED_USER:
					if rc.LoginInfo == nil || !rc.LoginInfo.LoggedIn {
						rc.ReportNotFound(fmt.Sprintf("%s:%s", username, name), "Snippet", "Depot", w, r)
						return
					}
					_, ok := sn.SharedUser[rc.LoginInfo.UserName]
					if !ok {
						rc.ReportNotFound(fmt.Sprintf("%s:%s", username, name), "Snippet", "Depot", w, r)
						return
					}
				case model.SNIPPET_PRIVATE:
					if rc.LoginInfo == nil || !rc.LoginInfo.LoggedIn || sn.BelongingUser != rc.LoginInfo.UserName {
						rc.ReportNotFound(fmt.Sprintf("%s:%s", username, name), "Snippet", "Depot", w, r)
						return
					}
				case model.SNIPPET_SHARED_LINK_INTERNAL:
					if rc.LoginInfo == nil || !rc.LoginInfo.LoggedIn {
						rc.ReportNotFound(fmt.Sprintf("%s:%s", username, name), "Snippet", "Depot", w, r)
						return
					}
				}
			}
			err = sn.RetrieveAllFile(rc.Config.SnippetRoot)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			disp := make(map[string]string, 0)
			for k, v := range sn.FileList {
				filename := path.Base(k)
				coloredStr, err := colorSyntax(filename, v)
				if err != nil {
					disp[k] = v
				} else {
					disp[k] = coloredStr
				}
			}
			LogTemplateError(rc.LoadTemplate("snippet/all-file").Execute(w, &templates.SnippetAllFileTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				Snippet: sn,
				DisplayingFileList: disp,
			}))
			
		},
	))
	
	http.HandleFunc("GET /snippet/{username}/{name}/raw/{path}", UseMiddleware(
		[]Middleware{Logged, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			username := r.PathValue("username")
			name := r.PathValue("name")
			p := r.PathValue("path")
			sn, err := rc.DatabaseInterface.GetSnippet(username, name)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			validUser := rc.LoginInfo.UserName == sn.BelongingUser
			if !validUser {
				_, ok := sn.SharedUser[rc.LoginInfo.UserName]
				validUser = ok
			}
			if !validUser {
				rc.ReportForbidden("Not enough privilege", w, r)
				return
			}
			err = sn.RetrieveAllFile(rc.Config.SnippetRoot)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			res, ok := sn.FileList[p]
			if !ok { res = "" }
			w.Write([]byte(res))
		},
	))
	
	http.HandleFunc("GET /snippet/{username}/{name}/setting", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, LoginRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			username := r.PathValue("username")
			name := r.PathValue("name")
			if rc.LoginInfo.UserName != username && !rc.LoginInfo.IsAdmin {
				rc.ReportRedirect(fmt.Sprintf("/snippet/%s/%s", username, name), 5, "Not Enough Privilege", "You don't have the permission required to perform this action.", w, r)
				return
			}
			sn, err := rc.DatabaseInterface.GetSnippet(username, name)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			err = sn.CalculateFileList(rc.Config.SnippetRoot)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("snippet/setting").Execute(w, &templates.SnippetSettingTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				Snippet: sn,
			}))
		},
	))
	
	http.HandleFunc("POST /snippet/{username}/{name}/setting", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			username := r.PathValue("username")
			name := r.PathValue("name")
			if rc.LoginInfo.UserName != username && !rc.LoginInfo.IsAdmin {
				rc.ReportRedirect(fmt.Sprintf("/snippet/%s/%s", username, name), 5, "Not Enough Privilege", "You don't have the permission required to perform this action.", w, r)
				return
			}
			visibility := r.Form.Get("status")
			description := r.Form.Get("description")
			s, err := strconv.ParseInt(visibility, 10, 8)
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			sharedUserString := r.Form.Get("shared-user")
			sn, err := rc.DatabaseInterface.GetSnippet(username, name)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			sn.Status = uint8(s)
			sn.Description = description
			if sn.SharedUser == nil { sn.SharedUser = make(map[string]bool, 0) }
			for k := range strings.SplitSeq(sharedUserString, ",") {
				sn.SharedUser[strings.TrimSpace(k)] = true
			}
			err = rc.DatabaseInterface.SaveSnippetInfo(sn)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/snippet/%s/%s/setting", username, name), 5, "Updated", "The info of the specified snippet has been updated.", w, r)
		},
	))
	
	http.HandleFunc("GET /snippet/{username}/{name}/setting/new", UseMiddleware(
		[]Middleware{Logged, LoginRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			username := r.PathValue("username")
			name := r.PathValue("name")
			isOwner := rc.LoginInfo.UserName == username
			if !isOwner && !rc.LoginInfo.IsAdmin {
				rc.ReportRedirect(fmt.Sprintf("/snippet/%s/%s", username, name), 5, "Not Enough Privilege", "You don't have the permission required to perform this action.", w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("snippet/new-file").Execute(w, &templates.SnippetNewFileTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				BelongingUser: username,
				Name: name,
			}))
		},
	))

	http.HandleFunc("POST /snippet/{username}/{name}/setting/new", UseMiddleware(
		[]Middleware{Logged, LoginRequired, CSRFCheck, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			username := r.PathValue("username")
			name := r.PathValue("name")
			isOwner := rc.LoginInfo.UserName == username
			if !isOwner && !rc.LoginInfo.IsAdmin {
				rc.ReportRedirect(fmt.Sprintf("/snippet/%s/%s", username, name), 5, "Not Enough Privilege", "You don't have the permission required to perform this action.", w, r)
				return
			}
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError(err.Error(), w, r)
				return
			}
			filename := r.Form.Get("filename")
			content := r.Form.Get("content")
			sn, err := rc.DatabaseInterface.GetSnippet(username, name)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			sn.SetFile(filename, content)
			err = sn.SyncFile(rc.Config.SnippetRoot, filename)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to save snippet file: %s", err), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/snippet/%s/%s", username, name), 5, "File Created", "The specified file has been created for this snippet.", w, r)
		},
	))
	
	http.HandleFunc("GET /snippet/{username}/{name}/setting/edit/{filePath}", UseMiddleware(
		[]Middleware{Logged, LoginRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			username := r.PathValue("username")
			name := r.PathValue("name")
			filePath := r.PathValue("filePath")
			isOwner := rc.LoginInfo.UserName == username
			if !isOwner && !rc.LoginInfo.IsAdmin {
				rc.ReportRedirect(fmt.Sprintf("/snippet/%s/%s", username, name), 5, "Not Enough Privilege", "You don't have the permission required to perform this action.", w, r)
				return
			}
			sn, err := rc.DatabaseInterface.GetSnippet(username, name)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			err = sn.Retrieve(rc.Config.SnippetRoot, filePath)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("snippet/edit-file").Execute(w, &templates.SnippetEditFileTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				BelongingUser: username,
				Name: name,
				FileName: filePath,
				FileContent: sn.FileList[filePath],
			}))
		},
	))
	
	http.HandleFunc("POST /snippet/{username}/{name}/setting/edit/{filePath}", UseMiddleware(
		[]Middleware{Logged, LoginRequired, CSRFCheck, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			username := r.PathValue("username")
			name := r.PathValue("name")
			filePath := r.PathValue("filePath")
			isOwner := rc.LoginInfo.UserName == username
			if !isOwner && !rc.LoginInfo.IsAdmin {
				rc.ReportRedirect(fmt.Sprintf("/snippet/%s/%s", username, name), 5, "Not Enough Privilege", "You don't have the permission required to perform this action.", w, r)
				return
			}
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			sn, err := rc.DatabaseInterface.GetSnippet(username, name)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			content := r.Form.Get("content")
			sn.SetFile(filePath, content)
			err = sn.SyncFile(rc.Config.SnippetRoot, filePath)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to save file to snippet: %s", err), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/snippet/%s/%s", username, name), 5, "File Updated", "The update has been made to the specified file of the specified snippet.", w, r)
		},
	))

	http.HandleFunc("GET /snippet/{username}/{name}/setting/delete/{filePath}", UseMiddleware(
		[]Middleware{Logged, LoginRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			username := r.PathValue("username")
			name := r.PathValue("name")
			filePath := r.PathValue("filePath")
			isOwner := rc.LoginInfo.UserName == username
			if !isOwner && !rc.LoginInfo.IsAdmin {
				rc.ReportRedirect(fmt.Sprintf("/snippet/%s/%s", username, name), 5, "Not Enough Privilege", "You don't have the permission required to perform this action.", w, r)
				return
			}
			sn, err := rc.DatabaseInterface.GetSnippet(username, name)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get snippet: %s", err), w, r)
				return
			}
			err = sn.DeleteFile(rc.Config.SnippetRoot, filePath)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to delete file: %s", err), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/snippet/%s/%s", username, name), 5, "File Deleted", "The specified file has been deleted from the snippet.", w, r)
		},
	))
}

