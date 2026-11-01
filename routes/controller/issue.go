package controller

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

// canManageIssueLabels reports whether the visitor may attach or remove issue
// labels: administrators, the repository or namespace owner, and members with
// the setting privilege on the repository or namespace.
func canManageIssueLabels(rc *RouterContext, ns *model.Namespace, repo *model.Repository) bool {
	if rc.LoginInfo == nil || !rc.LoginInfo.LoggedIn { return false }
	if rc.LoginInfo.IsAdmin { return true }
	if repo.Owner == rc.LoginInfo.UserName { return true }
	if ns != nil && ns.Owner == rc.LoginInfo.UserName { return true }
	if p := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName); p != nil && p.HasSettingPrivilege() {
		return true
	}
	if ns != nil {
		if p := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName); p != nil && p.HasSettingPrivilege() {
			return true
		}
	}
	return false
}

// availableIssueLabels returns the labels of the repository's issue label
// vocabulary that the issue does not carry yet.
func availableIssueLabels(vocabulary []string, issue *model.Issue) []string {
	res := make([]string, 0)
	for _, l := range vocabulary {
		if slices.Contains(issue.LabelList, l) { continue }
		res = append(res, l)
	}
	return res
}

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
			nsPriv := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
			repoPriv := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
			isMember := nsPriv != nil || repoPriv != nil
			if (repo.Status == model.REPO_NORMAL_PRIVATE) && !rc.LoginInfo.IsAdmin && !rc.LoginInfo.IsOwner && !isMember {
				rc.ReportNotFound(repoName, "Repository", "", w, r)
				return
			}
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
			// the vocabulary of issue labels is separate from the
			// repository's own labels.
			issueLabelVocabulary, err := rc.DatabaseInterface.GetRepositoryIssueLabel(nsName, repoName)
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
				CanManageLabels: canManageIssueLabels(rc, ns, repo),
				IssueLabelVocabulary: issueLabelVocabulary,
				AvailableLabelList: availableIssueLabels(issueLabelVocabulary, issue),
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
			if formType == "add-label" || formType == "remove-label" {
				if !canManageIssueLabels(rc, ns, repo) {
					rc.ReportRedirect(fmt.Sprintf("/repo/%s/issue/%d", rfn, iid), 5, "Not Enough Privilege",
						"Your account doesn't have enough privilege to change the labels of this issue.", w, r)
					return
				}
				label := strings.TrimSpace(r.Form.Get("label"))
				if len(label) <= 0 {
					rc.ReportRedirect(fmt.Sprintf("/repo/%s/issue/%d", rfn, iid), 5, "Invalid Request",
						"No label was specified.", w, r)
					return
				}
				// the vocabulary is the repository's issue label list, so a
				// label that was not defined in the settings cannot be
				// attached.
				vocabulary, err := rc.DatabaseInterface.GetRepositoryIssueLabel(nsName, repoName)
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				if !slices.Contains(vocabulary, label) {
					rc.ReportRedirect(fmt.Sprintf("/repo/%s/issue/%d", rfn, iid), 5, "Unknown Label",
						fmt.Sprintf("The repository has no issue label %q.", label), w, r)
					return
				}
				if formType == "add-label" {
					err = rc.DatabaseInterface.AddIssueLabel(nsName, repoName, iid, label)
				} else {
					err = rc.DatabaseInterface.RemoveIssueLabel(nsName, repoName, iid, label)
				}
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				FoundAt(w, fmt.Sprintf("/repo/%s/issue/%d", rfn, iid))
				return
			}
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

