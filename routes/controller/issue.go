package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindIssueController(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/issue", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, GlobalVisibility, ErrorGuard,
		},
		ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := rc.ResolveRepositoryFullName(rfn)
			rc.LoginInfo.IsOwner = ns.Owner == rc.LoginInfo.UserName || repo.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsStrictOwner = repo.Owner == rc.LoginInfo.UserName
			q := strings.TrimSpace(r.URL.Query().Get("q"))
			pStr := strings.TrimSpace(r.URL.Query().Get("p"))
			sStr := strings.TrimSpace(r.URL.Query().Get("s"))
			fStr := strings.TrimSpace(r.URL.Query().Get("f"))
			p64, err := strconv.ParseInt(pStr, 10, 64)
			if err != nil { p64 = 1 }
			s, err := strconv.ParseInt(sStr, 10, 64)
			if err != nil { s = 30 }
			f, err := strconv.ParseInt(fStr, 10, 32)
			if err != nil { f = 0 }
			count, err := rc.DatabaseInterface.CountIssue(q, nsName, repoName, int(f))
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			pageCount := count / s
			if (count % s) > 0 { pageCount += 1 }
			p := p64
			if p < 1 { p = 1 }
			if p > pageCount { p = pageCount }
			pageInfo := &templates.PageInfoModel{
				PageNum: p,
				PageSize: s,
				TotalPage: pageCount,
			}
			issueList, err := rc.DatabaseInterface.SearchIssuePaginated(q, nsName, repoName, int(f), p-1, s)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("issue/issue-list").Execute(w, &templates.RepositoryIssueListTemplateModel{
				Config: rc.Config,
				Repository: repo,
				RepoHeaderInfo: GenerateRepoHeader("", ""),
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "",
				IssueList: issueList,
				PageInfo: pageInfo,
				FilterType: int(f),
				Query: q,
			}))

		},
	))

	http.HandleFunc("GET /repo/{repoName}/issue/new", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo,
			GlobalVisibility, ValidRepositoryNameRequired("repoName"),
			ErrorGuard,
		},
		ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			_, _, ns, repo, err := rc.ResolveRepositoryFullName(rfn)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to resolve repository: %s", err), w, r)
				return
			}
			rc.LoginInfo.IsOwner = ns.Owner == rc.LoginInfo.UserName || repo.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsStrictOwner = repo.Owner == rc.LoginInfo.UserName
			LogTemplateError(rc.LoadTemplate("issue/new-issue").Execute(w, &templates.RepositoryNewIssueTemplateModel{
				Config: rc.Config,
				Repository: repo,
				RepoHeaderInfo: GenerateRepoHeader("", ""),
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "",
			}))
		},
	))

	http.HandleFunc("POST /repo/{repoName}/issue/new", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			LoginRequired, CSRFCheck, ValidPOSTRequestRequired,
			UseLoginInfo, GlobalVisibility,
			ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := rc.ResolveRepositoryFullName(rfn)
			rc.LoginInfo.IsOwner = ns.Owner == rc.LoginInfo.UserName || repo.Owner == rc.LoginInfo.UserName
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			isMember := nsPriv != nil || repoPriv != nil
			if (repo.Status == model.REPO_NORMAL_PRIVATE) && !rc.LoginInfo.IsAdmin && !rc.LoginInfo.IsOwner && !isMember {
				rc.ReportNotFound(repoName, "Repository", "", w, r)
				return
			}
			err = r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			title := r.Form.Get("title")
			content := r.Form.Get("content")
			iid, err := rc.DatabaseInterface.NewRepositoryIssue(nsName, repoName, rc.LoginInfo.UserName, title, content)
			FoundAt(w, fmt.Sprintf("/repo/%s/issue/%d", rfn, iid))
		},
	))
	
	http.HandleFunc("GET /repo/{repoName}/issue/{id}", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, GlobalVisibility,
			ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := rc.ResolveRepositoryFullName(rfn)
			rc.LoginInfo.IsOwner = ns.Owner == rc.LoginInfo.UserName || repo.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsStrictOwner = repo.Owner == rc.LoginInfo.UserName
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			isMember := nsPriv != nil || repoPriv != nil
			if (repo.Status == model.REPO_NORMAL_PRIVATE) && !rc.LoginInfo.IsAdmin && !rc.LoginInfo.IsOwner && !isMember {
				rc.ReportNotFound(repoName, "Repository", "", w, r)
				return
			}
			iid, err := strconv.Atoi(r.PathValue("id"))
			if err != nil {
				rc.ReportNormalError(err.Error(), w, r)
				return
			}
			issue, err := rc.DatabaseInterface.GetRepositoryIssue(nsName, repoName, iid)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			eventList, err := rc.DatabaseInterface.GetAllIssueEvent(nsName, repoName, iid)
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("issue/single-issue").Execute(w, &templates.RepositorySingleIssueTemplateModel{
				Config: rc.Config,
				Repository: repo,
				RepoHeaderInfo: GenerateRepoHeader("", ""),
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "",
				Issue: issue,
				IssueEventList: eventList,
			}))
			
		},
	))
	
	http.HandleFunc("POST /repo/{repoName}/issue/{id}", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			ValidPOSTRequestRequired,
			UseLoginInfo, CSRFCheck, GlobalVisibility,
			ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := rc.ResolveRepositoryFullName(rfn)
			iid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
			if err != nil {
				rc.ReportNormalError(err.Error(), w, r)
				return
			}
			if !rc.LoginInfo.LoggedIn {
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/issue/%d", rfn, iid), 3, "Not Logged In", "You must login before commenting on an issue.", w, r)
				return
			}
			rc.LoginInfo.IsStrictOwner = repo.Owner == rc.LoginInfo.UserName
			rc.LoginInfo.IsOwner = ns.Owner == rc.LoginInfo.UserName || repo.Owner == rc.LoginInfo.UserName
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			isMember := nsPriv != nil || repoPriv != nil
			if (repo.Status == model.REPO_NORMAL_PRIVATE) && !rc.LoginInfo.IsAdmin && !rc.LoginInfo.IsOwner && !isMember {
				rc.ReportNotFound(repoName, "Repository", "", w, r)
				return
			}
			err = r.ParseForm()
			if err != nil {
				rc.ReportNormalError(err.Error(), w, r)
				return
			}
			formType := strings.TrimSpace(r.Form.Get("type"))
			if formType == "unpin" || formType == "pin" {
				switch formType {
				case "unpin":
					err = rc.DatabaseInterface.SetIssuePriority(nsName, repoName, iid, 0)
				case "pin":
					err = rc.DatabaseInterface.SetIssuePriority(nsName, repoName, iid, 100)
				}
			} else {
				eType := model.EVENT_COMMENT
				author := rc.LoginInfo.UserName
				content := ""
				switch formType {
				case "comment":
					content = r.Form.Get("content")
				case "discarded":
					eType = model.EVENT_CLOSED_AS_DISCARDED
				case "solved":
					eType = model.EVENT_CLOSED_AS_SOLVED
				case "reopen":
					eType = model.EVENT_REOPENED
				}
				err = rc.DatabaseInterface.NewRepositoryIssueEvent(nsName, repoName, iid, eType, author, content)
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			FoundAt(w, fmt.Sprintf("/repo/%s/issue/%d", rfn, iid))
		},
	))
}

