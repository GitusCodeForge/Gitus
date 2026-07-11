package controller

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindRepositorySettingController(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/setting", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, GlobalVisibility, ErrorGuard,
			ValidRepositoryNameRequired("repoName"),
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if ctx.Config.UseNamespace && ns == nil {
				ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			isRepoOwner := repo.Owner == rc.LoginInfo.UserName
			isNsOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			isSettingMember := repoPriv.HasSettingPrivilege() || nsPriv.HasSettingPrivilege()
			if !rc.LoginInfo.IsAdmin && !isRepoOwner && !isNsOwner && !isSettingMember {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s", rfn), 0,
					"Not enouhg privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			rc.LoginInfo.IsSettingMember = true
			LogTemplateError(ctx.LoadTemplate("repo-setting/change-info").Execute(w, templates.RepositorySettingTemplateModel{
				Config: ctx.Config,
				Repository: repo,
				RepoFullName: rfn,
				LoginInfo: rc.LoginInfo,
			}))
		},
	))

	http.HandleFunc("POST /repo/{repoName}/setting", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, CSRFCheck, GlobalVisibility, ErrorGuard,
			ValidRepositoryNameRequired("repoName"),
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if ctx.Config.UseNamespace && ns == nil {
				ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			isRepoOwner := repo.Owner == rc.LoginInfo.UserName
			isNsOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
			isOwner := isRepoOwner || isNsOwner
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			isSettingMember := repoPriv.HasSettingPrivilege() || nsPriv.HasSettingPrivilege()
			if !rc.LoginInfo.IsAdmin && !isOwner && !isSettingMember {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s", rfn), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			rc.LoginInfo.IsSettingMember = isSettingMember
			err = r.ParseForm()
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			anythingChanged := false
			newOwner := strings.TrimSpace(r.Form.Get("owner"))
			if repo.Owner != newOwner {
				if !isOwner && !isNsOwner && !rc.LoginInfo.IsAdmin && newOwner != repo.Owner {
					ctx.ReportRedirect(fmt.Sprintf("/repo/%s/setting", rfn), 0,
						"Not enough privilege",
						"Your user account seems to not have enough privilege for this action.",
						w, r,
					)
					return
				}
				repo.Owner = newOwner
				anythingChanged = true
			}
			newDescription := strings.TrimSpace(r.Form.Get("description"))
			if repo.Description != newDescription {
				anythingChanged = true
			}
			i, err := strconv.Atoi(r.Form.Get("status"))
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			hasEditInfoPriv := rc.LoginInfo.IsAdmin || isOwner || (repoPriv != nil && repoPriv.EditInfo) || (nsPriv != nil && nsPriv.EditInfo)
			if anythingChanged && !hasEditInfoPriv {
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/setting", rfn), 5, "Not Enough Privilege", "You don't have enough privilege to perform this action.", w, r)
				return
			}
			repo.Description = newDescription
			repo.Status = model.GitusRepositoryStatus(i)
			err = ctx.DatabaseInterface.UpdateRepositoryInfo(repo.Namespace, repo.Name, repo)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to update repository info: %s", err), w, r)
				return
			}
			LogTemplateError(ctx.LoadTemplate("repo-setting/change-info").Execute(w, &templates.RepositorySettingTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				Repository: repo,
				RepoFullName: rfn,
				ErrorMsg: "Updated.",
			}))
		},
	))

	http.HandleFunc("GET /repo/{repoName}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			ctx.ReportSingleButtonCallback(
				fmt.Sprintf("/repo/{repoName}/delete", r.PathValue("repoName")),
				"Delete Repository",
				fmt.Sprintf("Click the following button to delete repository <code>%s</code>", r.PathValue("repoName")),
				"Delete",
				nil,
				w, r,
			)
		},
	))
	http.HandleFunc("POST /repo/{repoName}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, CSRFCheck, GlobalVisibility, ErrorGuard,
			ValidRepositoryNameRequired("repoName"),
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if ctx.Config.UseNamespace && ns == nil {
				ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			isRepoOwner := repo.Owner == rc.LoginInfo.UserName
			isNsOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			canDeleteRepo := (repoPriv != nil && repoPriv.DeleteRepository) || (nsPriv != nil && nsPriv.DeleteRepository)
			if !rc.LoginInfo.IsAdmin && !isRepoOwner && !isNsOwner && !canDeleteRepo {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s/setting", rfn), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}

			err = ctx.DatabaseInterface.HardDeleteRepository(repo.Namespace, repo.Name)
			if err != nil {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s/setting", rfn), 0,
					"Internal error",
					fmt.Sprintf("Failed to delete repository: %s.", err.Error()),
					w, r,
				)
				return
			}
			redirectTarget := "/"
			if ctx.Config.UseNamespace { redirectTarget = fmt.Sprintf("/s/%s", ns.Name) }
			ctx.ReportRedirect(redirectTarget, 3, "Deleted.", "The specified repository is deleted.", w, r)
		},
	))

	http.HandleFunc("GET /repo/{repoName}/setting/member", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, GlobalVisibility, ErrorGuard,
			ValidRepositoryNameRequired("repoName"),
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if ctx.Config.UseNamespace && ns == nil {
				ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			isRepoOwner := repo.Owner == rc.LoginInfo.UserName
			isNsOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
			isOwner := isRepoOwner || isNsOwner
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			isSettingMember := repoPriv.HasSettingPrivilege() || nsPriv.HasSettingPrivilege()
			if !rc.LoginInfo.IsAdmin && !isOwner && !isSettingMember {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s/setting", rfn), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			rc.LoginInfo.IsSettingMember = isSettingMember

			var userList map[string]*model.ACLTuple
			if repo.AccessControlList == nil {
				userList = nil
			} else {
				userList = repo.AccessControlList.ACL
			}
			totalMemberCount := int64(len(userList))
			pageInfo, err := GeneratePageInfo(r, totalMemberCount)
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
			
			LogTemplateError(ctx.LoadTemplate("repo-setting/member-list").Execute(w, templates.RepositorySettingMemberListTemplateModel{
				Config: ctx.Config,
				Repository: repo,
				RepoFullName: rfn,
				LoginInfo: rc.LoginInfo,
				ACL: page,
				PageInfo: pageInfo,
			}))

		},
	))
	
	http.HandleFunc("POST /repo/{repoName}/setting/member", UseMiddleware(
		[]Middleware{Logged, LoginRequired, CSRFCheck, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			if !model.ValidRepositoryName(rfn) {
				ctx.ReportNotFound(rfn, "Repository", "Namespace", w, r)
				return
			}
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if ctx.Config.UseNamespace && ns == nil {
				ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			repoPath := fmt.Sprintf("/repo/%s", rfn)
			if !rc.LoginInfo.LoggedIn { FoundAt(w, repoPath); return }
			
			isRepoOwner := repo.Owner == rc.LoginInfo.UserName
			isNsOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
			isOwner := isRepoOwner || isNsOwner
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			hasAddMemberPriv := (nsPriv != nil && nsPriv.AddMember) || (repoPriv != nil && repoPriv.AddMember)
			if !rc.LoginInfo.IsAdmin && !isOwner && !hasAddMemberPriv {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s/setting", rfn), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}

			err = r.ParseForm()
			if err != nil {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s/member", rfn), 0,
					"Invalid request",
					"Failed to parse request.",
					w, r,
				)
				return
			}
			username := strings.TrimSpace(r.Form.Get("username"))
			if len(username) <= 0 {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s/member", rfn), 0,
					"Invalid request",
					"User name cannot be empty.",
					w, r,
				)
				return
			}
			t := &model.ACLTuple{
				AddMember: len(r.Form.Get("addMember")) > 0,
				DeleteMember: len(r.Form.Get("deleteMember")) > 0,
				EditMember: len(r.Form.Get("editMember")) > 0,
				AddRepository: false,
				EditInfo: len(r.Form.Get("editInfo")) > 0,
				PushToRepository: len(r.Form.Get("pushToRepo")) > 0,
				ArchiveRepository: len(r.Form.Get("archiveRepo")) > 0,
				DeleteRepository: len(r.Form.Get("deleteRepo")) > 0,
				EditWebHooks: len(r.Form.Get("editWebHooks")) > 0,
			}
			err = ctx.DatabaseInterface.SetRepositoryACL(repo.Namespace, repo.Name, username, t)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			FoundAt(w, fmt.Sprintf("/repo/%s/setting/member", rfn))

		},
	))

	http.HandleFunc("GET /repo/{repoName}/setting/member/{userName}/edit", UseMiddleware(
		[]Middleware{Logged, LoginRequired, ValidRepositoryNameRequired("repoName"),
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
		rfn := r.PathValue("repoName")
		nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
		if err != nil {
			ctx.ReportInternalError(err.Error(), w, r)
			return
		}
		if ctx.Config.UseNamespace && ns == nil {
			ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
			return
		}
		if repo == nil {
			ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
			return
		}
		repoPath := fmt.Sprintf("/repo/%s", rfn)
		if !rc.LoginInfo.LoggedIn { FoundAt(w, repoPath); return }
		
		isRepoOwner := repo.Owner == rc.LoginInfo.UserName
		isNsOwner := ns.Owner == rc.LoginInfo.UserName
		rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
		isOwner := isRepoOwner || isNsOwner
		repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
		nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
		hasEditMemberPriv := (nsPriv != nil && nsPriv.EditMember) || (repoPriv != nil && repoPriv.EditMember)
		if !rc.LoginInfo.IsAdmin && !isOwner && !hasEditMemberPriv {
			ctx.ReportRedirect(fmt.Sprintf("/repo/%s/setting/member", rfn), 0,
				"Not enough privilege",
				"Your user account seems to not have enough privilege for this action.",
				w, r,
			)
			return
		}		
		targetUsername := r.PathValue("userName")
		userPriv := repo.AccessControlList.GetUserPrivilege(targetUsername)
		fmt.Println("userpriv", userPriv)
		LogTemplateError(ctx.LoadTemplate("repo-setting/edit-member").Execute(w, templates.RepositorySettingEditMemberTemplateModel{
			Config: ctx.Config,
			LoginInfo: rc.LoginInfo,
			Repository: repo,
			RepoFullName: rfn,
			Username: targetUsername,
			ACLTuple: userPriv,
		}))
		},
	))

	http.HandleFunc("POST /repo/{repoName}/setting/member/{userName}/edit", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {

			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if ctx.Config.UseNamespace && ns == nil {
				ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			isRepoOwner := repo.Owner == rc.LoginInfo.UserName
			isNsOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
			isOwner := isRepoOwner || isNsOwner
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			hasEditMemberPriv := (nsPriv != nil && nsPriv.EditMember) || (repoPriv != nil && repoPriv.EditMember)
			if !rc.LoginInfo.IsAdmin && !isOwner && !hasEditMemberPriv {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s/setting/member", rfn), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			
			targetUsername := r.PathValue("userName")
			t := &model.ACLTuple{
				AddMember: len(r.Form.Get("addMember")) > 0,
				DeleteMember: len(r.Form.Get("deleteMember")) > 0,
				EditMember: len(r.Form.Get("editMember")) > 0,
				AddRepository: false,
				EditInfo: len(r.Form.Get("editInfo")) > 0,
				PushToRepository: len(r.Form.Get("pushToRepo")) > 0,
				ArchiveRepository: len(r.Form.Get("archiveRepo")) > 0,
				DeleteRepository: len(r.Form.Get("deleteRepo")) > 0,
				EditHooks: len(r.Form.Get("editHooks")) > 0,
			}
			err = ctx.DatabaseInterface.SetRepositoryACL(repo.Namespace, repo.Name, targetUsername, t)
			if err != nil {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s/setting/member", rfn), 0,
					"Failed to update member privilege",
					fmt.Sprintf("Failed to update member privilege: %s. Please contact site owner.", err.Error()),
					w, r,
				)
				return
			}
			FoundAt(w, fmt.Sprintf("/repo/%s/setting/member", rfn))
		},
	))
	
	http.HandleFunc("GET /repo/{repoName}/setting/member/{username}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			ctx.ReportSingleButtonCallback(
				fmt.Sprintf("/repo/%s/setting/member/%s/delete",
					r.PathValue("repoName"),
					r.PathValue("username"),
				),
				"Delete Repository Member",
				fmt.Sprintf("Click the following button to delete user <code>%s</code> from repository <code>%s</code>", r.PathValue("username"), r.PathValue("repoName")),
				"Delete",
				nil,
				w, r,
			)
		},
	))
	http.HandleFunc("POST /repo/{repoName}/setting/member/{username}/delete", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"), ValidPOSTRequestRequired,
			UseLoginInfo, LoginRequired, CSRFCheck, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			isRepoOwner := repo.Owner == rc.LoginInfo.UserName
			isNsOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			privSufficient := rc.LoginInfo.IsOwner || rc.LoginInfo.IsAdmin || (nsPriv != nil && nsPriv.DeleteMember) || (repoPriv != nil && repoPriv.DeleteMember)
			if !privSufficient {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s/setting/member", rfn), 0,
					"Not enough privilege",
					"You seem to not have not enough privilege for this action.",
					w, r,
				)
				return
			}
			targetUsername := r.PathValue("username")
			err = ctx.DatabaseInterface.SetRepositoryACL(nsName, repoName, targetUsername, nil)
			if err != nil {
				ctx.ReportInternalError(fmt.Sprintf("Failed to delete member: %s.", err), w, r)
				return
			}
			FoundAt(w, fmt.Sprintf("/repo/%s/setting/member", rfn))
		},
	))

	http.HandleFunc("GET /repo/{repoName}/setting/label", UseMiddleware(
		[]Middleware{Logged, LoginRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if ctx.Config.UseNamespace && ns == nil {
				ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			isRepoOwner := repo.Owner == rc.LoginInfo.UserName
			isNsOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
			isOwner := isRepoOwner || isNsOwner
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			isSettingMember := repoPriv.HasSettingPrivilege() || nsPriv.HasSettingPrivilege()
			if !rc.LoginInfo.IsAdmin && !isOwner && !isSettingMember {
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/setting", rfn), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			rc.LoginInfo.IsSettingMember = isSettingMember

			LogTemplateError(rc.LoadTemplate("repo-setting/edit-label").Execute(w, templates.RepositorySettingMemberListTemplateModel{
				Config: rc.Config,
				Repository: repo,
				RepoFullName: rfn,
				LoginInfo: rc.LoginInfo,
			}))
		},
	))
	
	http.HandleFunc("GET /repo/{repoName}/setting/label/{label}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			// TODO: add owner/member guard
			ctx.ReportSingleButtonCallback(
				fmt.Sprintf("/repo/%s/setting/label/%s/delete",
					r.PathValue("repoName"),
					r.PathValue("label"),
				),
				"Delete Label",
				fmt.Sprintf("Click the following button to delete label <code>%s</code> from repository <code>%s</code>", r.PathValue("label"), r.PathValue("repoName")),
				"Delete",
				nil,
				w, r,
			)
		},
	))
	http.HandleFunc("POST /repo/{repoName}/setting/label/{label}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, ValidPOSTRequestRequired,
			CSRFCheck, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if ctx.Config.UseNamespace && ns == nil {
				ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			isRepoOwner := repo.Owner == rc.LoginInfo.UserName
			isNsOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
			isOwner := isRepoOwner || isNsOwner
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			isSettingMember := repoPriv.HasSettingPrivilege() || nsPriv.HasSettingPrivilege()
			canEditInfo := (repoPriv != nil && repoPriv.EditInfo) || (nsPriv != nil && nsPriv.EditInfo)
			if !rc.LoginInfo.IsAdmin && !isOwner && !isSettingMember && !canEditInfo {
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/setting", rfn), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			rc.LoginInfo.IsSettingMember = isSettingMember

			label := r.PathValue("label")
			err = rc.DatabaseInterface.RemoveRepositoryLabel(nsName, repoName, label)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to delete label from repository: %s.", err), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/repo/%s/setting/label", rfn), 0, "Deleted", "The specified label has been deleted from the repository.", w, r)
		},
	))

	http.HandleFunc("POST /repo/{repoName}/setting/label", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if ctx.Config.UseNamespace && ns == nil {
				ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			isRepoOwner := repo.Owner == rc.LoginInfo.UserName
			isNsOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
			isOwner := isRepoOwner || isNsOwner
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			isSettingMember := repoPriv.HasSettingPrivilege() || nsPriv.HasSettingPrivilege()
			canEditInfo := (repoPriv != nil && repoPriv.EditInfo) || (nsPriv != nil && nsPriv.EditInfo)
			if !rc.LoginInfo.IsAdmin && !isOwner && !isSettingMember && !canEditInfo {
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/setting", rfn), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			rc.LoginInfo.IsSettingMember = isSettingMember
			err = r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			label := strings.TrimSpace(r.Form.Get("label"))
			if len(label) <= 0 {
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/setting/label", rfn), 5, "Invalid Request", "Label text must not be empty.", w, r)
				return
			}
			err = rc.DatabaseInterface.AddRepositoryLabel(nsName, repoName, label)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to add label to repository: %s.", err), w, r)
				return
			}

			rc.ReportRedirect(fmt.Sprintf("/repo/%s/setting/label", rfn), 0, "Added", "The specific tag has been added to the repository.", w, r)
		},
	))
	
	http.HandleFunc("GET /repo/{repoName}/setting/webhook",
		UseMiddleware(
			[]Middleware{Logged, LoginRequired, GlobalVisibility, ErrorGuard}, ctx,
			func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {		rfn := r.PathValue("repoName")
				if !model.ValidRepositoryName(rfn) {
					ctx.ReportNotFound(rfn, "Repository", "Namespace", w, r)
					return
				}
				nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
				if err != nil {
					ctx.ReportInternalError(err.Error(), w, r)
					return
				}
				if ctx.Config.UseNamespace && ns == nil {
					ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
					return
				}
				if repo == nil {
					ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
					return
				}
				repoPath := fmt.Sprintf("/repo/%s", repo.FullName())
				if !rc.LoginInfo.LoggedIn { FoundAt(w, repoPath); return }
				// NOTE: we don't support editing namespace from web ui when in plain mode.
				if rc.Config.IsInPlainMode() { FoundAt(w, repoPath); return }
				isRepoOwner := repo.Owner == rc.LoginInfo.UserName
				isNsOwner := ns.Owner == rc.LoginInfo.UserName
				rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
				repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
				nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
				allowEdit := (repoPriv != nil && repoPriv.EditWebHooks) || (nsPriv != nil && nsPriv.EditWebHooks)
				if !rc.LoginInfo.IsAdmin && !isRepoOwner && !isNsOwner && !allowEdit {
					ctx.ReportRedirect(fmt.Sprintf("/repo/%s", rfn), 0,
						"Not enouhg privilege",
						"Your user account seems to not have enough privilege for this action.",
						w, r,
					)
					return
				}
				rc.LoginInfo.IsSettingMember = true
				LogTemplateError(ctx.LoadTemplate("repo-setting/edit-webhook").Execute(w, templates.RepositorySettingEditWebHookTemplateModel{
					Config: ctx.Config,
					Repository: repo,
					RepoFullName: rfn,
					LoginInfo: rc.LoginInfo,
				}))

			},
		),
	)
	
	http.HandleFunc("POST /repo/{repoName}/setting/webhook", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			if !model.ValidRepositoryName(rfn) {
				ctx.ReportNotFound(rfn, "Repository", "Namespace", w, r)
				return
			}
			err := r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if ctx.Config.UseNamespace && ns == nil {
				ctx.ReportNotFound(repo.Namespace, "Namespace", "depot", w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			lgr, ok := repo.Repository.(*gitlib.LocalGitRepository)
			if !ok {
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/setting", rfn), 5, "Unsupported", "Webhooks only supports Git repositories.", w, r)
				return
			}
			repoPath := fmt.Sprintf("/repo/%s", repo.FullName())
			if !rc.LoginInfo.LoggedIn { FoundAt(w, repoPath); return }
			if rc.Config.IsInPlainMode() { FoundAt(w, repoPath); return }
			isRepoOwner := repo.Owner == rc.LoginInfo.UserName
			isNsOwner := ns.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = isRepoOwner || isNsOwner
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			allowEdit := (repoPriv != nil && repoPriv.EditWebHooks) || (nsPriv != nil && nsPriv.EditWebHooks)
			if !rc.LoginInfo.IsAdmin && !isRepoOwner && !isNsOwner && !allowEdit {
				ctx.ReportRedirect(fmt.Sprintf("/repo/%s", rfn), 0,
					"Not enough privilege",
					"Your user account seems to not have enough privilege for this action.",
					w, r,
				)
				return
			}
			rc.LoginInfo.IsSettingMember = true
			enableWebhook := len(r.Form.Get("enable")) > 0
			repo.WebHookConfig.Enable = enableWebhook
			repo.WebHookConfig.PayloadType = "json"
			repo.WebHookConfig.Secret = strings.TrimSpace(repo.WebHookConfig.Secret)
			if len(repo.WebHookConfig.Secret) <= 16 {
				repo.WebHookConfig.Secret = auxfuncs.CryptoGenSym(24)
			}
			{
				u, e := url.Parse(strings.TrimSpace(r.Form.Get("target-url")))
				if e != nil {
					ctx.ReportRedirect(
						fmt.Sprintf("/repo/%s/setting/webhook", rfn), 0,
						"Invalid webhook URL",
						"You've entered an invalid URL for webhook. Please try again.",
						w, r,
					)
					return
				}
				if u.Scheme != "http" && u.Scheme != "https" {
					ctx.ReportRedirect(
						fmt.Sprintf("/repo/%s/setting/webhook", rfn), 0,
						"Invalid webhook URL",
						"You've entered an invalid URL for webhook. Please try again.",
						w, r,
					)
					return
				}
				ips, e := net.LookupIP(u.Hostname())
				if e != nil {
					ctx.ReportRedirect(
						fmt.Sprintf("/repo/%s/setting/webhook", rfn), 0,
						"Cannot resolve webhook URL host",
						fmt.Sprintf("The server failed to resolve host %s. Please choose a different address.", u.Hostname()),
						w, r,
					)
					return
				}
				for _, ip := range ips {
					if auxfuncs.IsPotentiallyLocalAddress(ip.String()) {
						if !ctx.LoginInfo.IsSuperAdmin {
							ctx.ReportRedirect(
								fmt.Sprintf("/repo/%s/setting/webhook", rfn), 0,
								"Not enough privilege",
								"Your user account seems to not have enough privilege for this action.",
								w, r,
							)
							return
						}
					}
				}
				repo.WebHookConfig.TargetURL = u.String()
			}
			err = rc.DatabaseInterface.UpdateRepositoryInfo(repo.Namespace, repo.Name, repo)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to update repository info: %s", err), w, r)
				return
			}
			if enableWebhook {
				err = lgr.EnableWebHook(repo.FullName())
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to setup webhook: %s", err), w, r)
					return
				}
			} else {
				err = lgr.DisableWebHook()
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to disable webhook: %s", err), w, r)
					return
				}
			}
			rc.ReportRedirect(fmt.Sprintf("/repo/%s/setting/webhook", repo.FullName()), 5, "Updated", "Your configuration of webhooks has been saved.", w, r)
		},
	))
}

