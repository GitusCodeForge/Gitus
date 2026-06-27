package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	auxfuncs "github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindNamespaceSettingController(ctx *RouterContext) {
	http.HandleFunc("GET /s/{namespace}/setting", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, LoginRequired, GlobalVisibility, ErrorGuard},
		ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			namespaceName := r.PathValue("namespace")
			if !model.ValidNamespaceName(namespaceName) {
				rc.ReportNotFound(namespaceName, "Repository", "Depot", w, r)
				return
			}
			namespacePath := fmt.Sprintf("/s/%s", namespaceName)
			// NOTE: we don't support editing namespace from web ui when in plain mode.
			if rc.Config.IsInPlainMode() { FoundAt(w, namespacePath); return }
			if !rc.LoginInfo.LoggedIn { FoundAt(w, namespacePath); return }
			ns, err := rc.DatabaseInterface.GetNamespaceByName(namespaceName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			isOwner := ns.Owner == rc.LoginInfo.UserName
			priv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			isMember := priv != nil
			isSettingMember := isMember && priv.HasSettingPrivilege()
			if !rc.LoginInfo.IsAdmin && !isOwner && !isSettingMember {
				rc.ReportRedirect(fmt.Sprintf("/s/%s", namespaceName), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			rc.LoginInfo.IsOwner = isOwner
			LogTemplateError(rc.LoadTemplate("namespace-setting/change-info").Execute(w, templates.NamespaceSettingTemplateModel{
				Namespace: ns,
				LoginInfo: rc.LoginInfo,
				Config: rc.Config,
			}))
		},
	))

	http.HandleFunc("POST /s/{namespace}/setting", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			namespaceName := r.PathValue("namespace")
			if !model.ValidNamespaceName(namespaceName) {
				rc.ReportNotFound(namespaceName, "Repository", "Depot", w, r)
				return
			}
			namespacePath := fmt.Sprintf("/s/%s", namespaceName)
			if rc.Config.IsInPlainMode() { FoundAt(w, namespacePath); return }
			ns, err := rc.DatabaseInterface.GetNamespaceByName(namespaceName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			isOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isOwner
			priv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			canEditInfo := priv != nil && priv.EditInfo
			if !rc.LoginInfo.IsAdmin && !isOwner && !canEditInfo {
				rc.ReportRedirect(fmt.Sprintf("/s/%s", namespaceName), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			rc.LoginInfo.IsSettingMember = rc.LoginInfo.IsAdmin || isOwner || priv.HasSettingPrivilege()
			err = r.ParseForm()
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			newOwner := strings.TrimSpace(r.Form.Get("owner"))
			if ns.Owner != newOwner {
				if !isOwner && !rc.LoginInfo.IsAdmin {
					rc.ReportRedirect(fmt.Sprintf("/s/%s/setting", namespaceName), 0,
						"Not enough privilege",
						"You are neither owner nor an admin, thus cannot change this namespace's ownership.",
						w, r,
					)
					return
				}
				ns.Owner = newOwner
			}
			ns.Title = r.Form.Get("title")
			ns.Description = r.Form.Get("description")
			ns.Email = r.Form.Get("email")
			i, err := strconv.Atoi(r.Form.Get("status"))
			if err != nil {
				rc.ReportRedirect(
					fmt.Sprintf("/s/%s/setting", namespaceName), 0,
					"Invalid Request",
					"Invalid status value. Please try again.",
					w, r,
				)
				return
			}
			ns.Status = model.GitusNamespaceStatus(i)
			err = rc.DatabaseInterface.UpdateNamespaceInfo(namespaceName, ns)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/s/%s/setting", namespaceName), 3,
				"Updated",
				"Namespace info updated.",
				w, r,
			)
		},
	))

	http.HandleFunc("GET /s/{namespace}/delete", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, LoginRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			namespaceName := r.PathValue("namespace")
			if !model.ValidNamespaceName(namespaceName) {
				rc.ReportNotFound(namespaceName, "Repository", "Depot", w, r)
				return
			}
			namespacePath := fmt.Sprintf("/s/%s", namespaceName)
			if rc.Config.IsInPlainMode() { FoundAt(w, namespacePath); return }
			ns, err := rc.DatabaseInterface.GetNamespaceByName(namespaceName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			if !rc.LoginInfo.IsAdmin &&  ns.Owner != rc.LoginInfo.UserName {
				rc.ReportRedirect(fmt.Sprintf("/s/%s/setting", namespaceName), 3,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			err = rc.DatabaseInterface.HardDeleteNamespaceByName(namespaceName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			rc.ReportRedirect("/", 3,
				"Deleted",
				"Namespace deleted..",
				w, r,
			)
		},
	))

	http.HandleFunc("GET /s/{namespace}/member", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, LoginRequired, GlobalVisibility, ErrorGuard},
		ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			namespaceName := r.PathValue("namespace")
			if !model.ValidNamespaceName(namespaceName) {
				rc.ReportNotFound(namespaceName, "Repository", "Depot", w, r)
				return
			}
			namespacePath := fmt.Sprintf("/s/%s", namespaceName)
			if rc.Config.IsInPlainMode() { FoundAt(w, namespacePath); return }
			ns, err := rc.DatabaseInterface.GetNamespaceByName(namespaceName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			isOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isOwner
			priv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			isMember := priv != nil
			isSettingMember := isMember && priv.HasSettingPrivilege()
			if !rc.LoginInfo.IsAdmin && !isOwner && !isSettingMember {
				rc.ReportRedirect(fmt.Sprintf("/s/%s/setting", ns.Name), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			var userList map[string]*model.ACLTuple
			if ns.ACL == nil {
				userList = nil
			} else {
				userList = ns.ACL.ACL
			}
			totalMemberCount := int64(len(userList))
			pageInfo, err := GeneratePageInfo(r, totalMemberCount)
			// the reason we do this is the fact that maps in go does not
			// guarantee the order of keys when doing a range over them.
			k := auxfuncs.SortedKeys(userList)
			stidx := (pageInfo.PageNum-1)*pageInfo.PageSize
			eidx := min(stidx+pageInfo.PageSize, totalMemberCount)
			k = k[stidx:eidx]
			page := make(map[string]*model.ACLTuple, 4)
			if eidx - stidx > 0 {
				for _, item := range k {
					page[item] = userList[item]
				}
				userList = page
			}
			LogTemplateError(rc.LoadTemplate("namespace-setting/member-list").Execute(w, templates.NamespaceSettingMemberListTemplateModel{
				Namespace: ns,
				LoginInfo: rc.LoginInfo,
				Config: rc.Config,
				ACL: userList,
				PageInfo: pageInfo,
			}))
			
		},
	))
	
	http.HandleFunc("POST /s/{namespace}/member", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			namespaceName := r.PathValue("namespace")
			if !model.ValidNamespaceName(namespaceName) {
				rc.ReportNotFound(namespaceName, "Repository", "Depot", w, r)
				return
			}
			namespacePath := fmt.Sprintf("/s/%s", namespaceName)
			if rc.Config.IsInPlainMode() { FoundAt(w, namespacePath); return }
			ns, err := rc.DatabaseInterface.GetNamespaceByName(namespaceName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			isOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isOwner
			priv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			canAddMember := priv != nil && priv.AddMember
			if !rc.LoginInfo.IsAdmin && !isOwner && !canAddMember {
				rc.ReportRedirect(fmt.Sprintf("/s/%s/setting", ns.Name), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			username := strings.TrimSpace(r.Form.Get("username"))
			if len(username) <= 0 {
				rc.ReportRedirect(fmt.Sprintf("/s/%s/setting", ns.Name), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			t := &model.ACLTuple{
				AddMember: len(r.Form.Get("addMember")) > 0,
				DeleteMember: len(r.Form.Get("deleteMember")) > 0,
				EditMember: len(r.Form.Get("editMember")) > 0,
				AddRepository: len(r.Form.Get("addRepo")) > 0,
				EditInfo: len(r.Form.Get("editInfo")) > 0,
				PushToRepository: len(r.Form.Get("pushToRepo")) > 0,
				ArchiveRepository: len(r.Form.Get("archiveRepo")) > 0,
				DeleteRepository: len(r.Form.Get("deleteRepo")) > 0,
				EditHooks: len(r.Form.Get("editHooks")) > 0,
			}
			err = rc.DatabaseInterface.SetNamespaceACL(namespaceName, username, t)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/s/%s/member", ns.Name), 3,
				"Updated",
				"Member list updated.",
				w, r,
			)
		},
	))

	http.HandleFunc("GET /s/{namespace}/member/{username}/delete", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, LoginRequired,
			GlobalVisibility, ErrorGuard,
		},
		ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {

			namespaceName := r.PathValue("namespace")
			if !model.ValidNamespaceName(namespaceName) {
				rc.ReportNotFound(namespaceName, "Repository", "Depot", w, r)
				return
			}
			namespacePath := fmt.Sprintf("/s/%s", namespaceName)
			if rc.Config.IsInPlainMode() { FoundAt(w, namespacePath); return }
			ns, err := rc.DatabaseInterface.GetNamespaceByName(namespaceName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			isOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isOwner
			priv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			privSufficient := isOwner || rc.LoginInfo.IsAdmin || (priv != nil && priv.DeleteMember)
			if !privSufficient {
				rc.ReportRedirect(fmt.Sprintf("/s/%s/member", ns.Name), 0,
					"Not enough privilege",
					"You seem to not have not enough privilege for this action.",
					w, r,
				)
				return
			}
			targetUsername := r.PathValue("username")
			err = rc.DatabaseInterface.SetNamespaceACL(namespaceName, targetUsername, nil)
			if err != nil {
				LogTemplateError(rc.LoadTemplate("namespace-setting/_redirect-with-message").Execute(w, templates.RedirectWithMessageModel{
					Config: rc.Config,
					LoginInfo: rc.LoginInfo,
					Timeout: 3,
					RedirectUrl: fmt.Sprintf("/s/%s/member", namespaceName),
					MessageTitle: "Failed to delete member",
					MessageText: fmt.Sprintf("Error: %s", err.Error()),
				}))
				return
			}
			FoundAt(w, fmt.Sprintf("/s/%s/member", namespaceName))
		},
	))
	
	http.HandleFunc("GET /s/{namespace}/member/{username}/edit", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, LoginRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			namespaceName := r.PathValue("namespace")
			if !model.ValidNamespaceName(namespaceName) {
				rc.ReportNotFound(namespaceName, "Repository", "Depot", w, r)
				return
			}
			namespacePath := fmt.Sprintf("/s/%s", namespaceName)
			if rc.Config.IsInPlainMode() { FoundAt(w, namespacePath); return }
			ns, err := rc.DatabaseInterface.GetNamespaceByName(namespaceName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			isOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isOwner
			priv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			canEditMember := priv != nil && priv.EditMember
			if !rc.LoginInfo.IsAdmin && !isOwner && !canEditMember {
				rc.ReportRedirect(fmt.Sprintf("/s/%s/member", namespaceName), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			targetUsername := r.PathValue("username")
			userPriv := ns.ACL.GetUserPrivilege(targetUsername)
			LogTemplateError(rc.LoadTemplate("namespace-setting/edit-member").Execute(w, templates.NamespaceSettingEditMemberTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				Namespace: ns,
				Username: targetUsername,
				ACLTuple: userPriv,
			}))
		},
	))

	http.HandleFunc("POST /s/{namespace}/member/{username}/edit", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			UseLoginInfo, LoginRequired, CSRFCheck, GlobalVisibility,
			ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			namespaceName := r.PathValue("namespace")
			if !model.ValidNamespaceName(namespaceName) {
				rc.ReportNotFound(namespaceName, "Repository", "Depot", w, r)
				return
			}
			namespacePath := fmt.Sprintf("/s/%s", namespaceName)
			if rc.Config.IsInPlainMode() { FoundAt(w, namespacePath); return }
			if !rc.LoginInfo.LoggedIn { FoundAt(w, namespacePath); return }
			ns, err := rc.DatabaseInterface.GetNamespaceByName(namespaceName)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			isOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isOwner
			priv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			canEditMember := priv != nil && priv.EditMember
			if !rc.LoginInfo.IsAdmin && !isOwner && !canEditMember {
				rc.ReportRedirect(fmt.Sprintf("/s/%s/member", namespaceName), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			targetUsername := r.PathValue("username")
			err = r.ParseForm()
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			t := &model.ACLTuple{
				AddMember: len(r.Form.Get("addMember")) > 0,
				DeleteMember: len(r.Form.Get("deleteMember")) > 0,
				EditMember: len(r.Form.Get("editMember")) > 0,
				AddRepository: len(r.Form.Get("addRepo")) > 0,
				EditInfo: len(r.Form.Get("editInfo")) > 0,
				PushToRepository: len(r.Form.Get("pushToRepo")) > 0,
				ArchiveRepository: len(r.Form.Get("archiveRepo")) > 0,
				DeleteRepository: len(r.Form.Get("deleteRepo")) > 0,
				EditWebHooks: len(r.Form.Get("editWebHooks")) > 0,
			}
			err = rc.DatabaseInterface.SetNamespaceACL(namespaceName, targetUsername, t)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			FoundAt(w, fmt.Sprintf("/s/%s/member", namespaceName))
		},
	))
}

