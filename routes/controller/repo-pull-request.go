package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindRepositoryPullRequestController(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/pull-request", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			if rc.Config.IsInPlainMode() {
				FoundAt(w, fmt.Sprintf("/repo/%s", rfn))
				return
			}
			_, _, _, s, err := rc.ResolveRepositoryFullName(rfn)
			if err == ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			q := strings.TrimSpace(r.URL.Query().Get("q"))
			pStr := strings.TrimSpace(r.URL.Query().Get("p"))
			sStr := strings.TrimSpace(r.URL.Query().Get("s"))
			fStr := strings.TrimSpace(r.URL.Query().Get("f"))
			p, err := strconv.ParseInt(pStr, 10, 64)
			if err != nil { p = 1 }
			ps, err := strconv.ParseInt(sStr, 10, 64)
			if err != nil { ps = 30 }
			f, err := strconv.ParseInt(fStr, 10, 32)
			if err != nil { f = 0 }
			count, err := rc.DatabaseInterface.CountPullRequest(q, s.Namespace, s.Name, int(f))
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to count pull request: %s", err.Error()), w, r)
				return
			}
			pageCount := count / ps
			if (count % ps) > 0 { pageCount += 1 }
			if p < 1 { p = 1 }
			if p > pageCount { p = pageCount }
			pageInfo := &templates.PageInfoModel{
				PageNum: p,
				PageSize: ps,
				TotalPage: pageCount,
			}
			prList, err := rc.DatabaseInterface.SearchPullRequestPaginated(q, s.Namespace, s.Name, int(f), p-1, ps)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to search for pull request: %s", err.Error()), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("pull-request/pull-request-list").Execute(w, &templates.RepositoryPullRequestListTemplateModel{
				Config: rc.Config,
				Repository: s,
				RepoHeaderInfo: &templates.RepoHeaderTemplateModel{
					TypeStr: "", NodeName: "",
				},
				LoginInfo: rc.LoginInfo,
				PullRequestList: prList,
				PageInfo: pageInfo,
				Query: q,
				FilterType: int(f),
			}))
		},
	))
	
	http.HandleFunc("GET /repo/{repoName}/pull-request/{prid}", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			if rc.Config.IsInPlainMode() {
				FoundAt(w, fmt.Sprintf("/repo/%s", rfn))
				return
			}
			_, _, _, s, err := rc.ResolveRepositoryFullName(rfn)
			if err == ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			pridStr := r.PathValue("prid")
			prid, err := strconv.ParseInt(pridStr, 10, 64)
			if err != nil {
				rc.ReportNotFound(pridStr, "Pull request", rfn, w, r)
				return
			}
			pr, err := rc.DatabaseInterface.GetPullRequest(s.Namespace, s.Name, prid)
			if err != nil {
				if err == db.ErrEntityNotFound {
					rc.ReportRedirect(fmt.Sprintf("/repo/%s/pull-request", rfn), 5, "Not Found", "The pull request you've specified does not exist in this repository.", w, r)
					return
				}
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			pnstr := r.URL.Query().Get("p")
			pn, err := strconv.ParseInt(pnstr, 10, 64)
			if err != nil { pn = 0 }
			preList, err := rc.DatabaseInterface.GetAllPullRequestEventPaginated(pr.PRAbsId, pn, 30)
			LogTemplateError(rc.LoadTemplate("pull-request/single-pull-request").Execute(w, &templates.RepositorySinglePullRequestTemplateModel{
				Config: rc.Config,
				Repository: s,
				RepoHeaderInfo: &templates.RepoHeaderTemplateModel{
					TypeStr: "", NodeName: "",
				},
				LoginInfo: rc.LoginInfo,
				PullRequest: pr,
				PullRequestEventList: preList,
				PageNum: pn,
			}))
		},
	))

	http.HandleFunc("POST /repo/{repoName}/pull-request/{prid}", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			ValidRepositoryNameRequired("repoName"), UseLoginInfo,
			LoginRequired, CSRFCheck, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			if rc.Config.IsInPlainMode() {
				FoundAt(w, fmt.Sprintf("/repo/%s", rfn))
				return
			}
			_, _, _, s, err := rc.ResolveRepositoryFullName(rfn)
			if err == ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			pridStr := r.PathValue("prid")
			if !rc.LoginInfo.LoggedIn {
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/pull-request/%s", rfn, pridStr), 5, "Not Logged In", "You must log in before performing this action.", w, r)
				return
			}
			prid, err := strconv.ParseInt(pridStr, 10, 64)
			if err != nil {
				rc.ReportNotFound(pridStr, "Pull request", rfn, w, r)
				return
			}
			pr, err := rc.DatabaseInterface.GetPullRequest(s.Namespace, s.Name, prid)
			if err != nil {
				if err == db.ErrEntityNotFound {
					rc.ReportRedirect(fmt.Sprintf("/repo/%s/pull-request", rfn), 5, "Not Found", "The pull request you've specified does not exist in this repository.", w, r)
					return
				}
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			err = r.ParseForm()
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			rt := r.Form.Get("type")
			returnPath := fmt.Sprintf("/repo/%s/pull-request/%d", rfn, prid)
			switch rt {
			case "comment":
				_, err = rc.DatabaseInterface.CommentOnPullRequest(pr.PRAbsId, rc.LoginInfo.UserName, r.Form.Get("content"))
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				FoundAt(w, returnPath)
			case "merge-check":
				e, err := rc.DatabaseInterface.CheckPullRequestMergeConflict(pr.PRAbsId)
				fmt.Println("ee", e)
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				FoundAt(w, returnPath)
			case "close-as-merged":
				err = rc.DatabaseInterface.CheckAndMergePullRequest(pr.PRAbsId, rc.LoginInfo.UserName)
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				FoundAt(w, returnPath)
			case "close-as-not-merged":
				err = rc.DatabaseInterface.ClosePullRequestAsNotMerged(pr.PRAbsId, rc.LoginInfo.UserName)
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				FoundAt(w, returnPath)
			case "reopen":
				err = rc.DatabaseInterface.ReopenPullRequest(pr.PRAbsId, rc.LoginInfo.UserName)
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				FoundAt(w, returnPath)
			default:
				rc.ReportNormalError("Invalid Request", w, r)
				return
			}

			FoundAt(w, returnPath)
		},
	))
	
	http.HandleFunc("GET /repo/{repoName}/pull-request/new", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, LoginRequired, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			if rc.Config.IsInPlainMode() {
				FoundAt(w, fmt.Sprintf("/repo/%s", rfn))
				return
			}
			_, _, _, s, err := rc.ResolveRepositoryFullName(rfn)
			if err == ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			err = s.Repository.(*gitlib.LocalGitRepository).SyncAllBranchList()
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to sync all branch list: %s", err), w, r)
				return
			}
			receiverBranch := strings.TrimSpace(r.URL.Query().Get("recv-br"))
			if len(receiverBranch) > 0 {
				_, ok := s.Repository.(*gitlib.LocalGitRepository).BranchIndex[receiverBranch]
				if !ok {
					rc.ReportRedirect(fmt.Sprintf("/repo/%s/pull-request/new", rfn), 5, "Not Found", fmt.Sprintf("Branch \"%s\" does not exist in repository %s. Please choose an existing branch.", receiverBranch, rfn), w, r)
					return
				}
			}
			providerRepositoryName := strings.TrimSpace(r.URL.Query().Get("repo"))
			if !model.ValidRepositoryName(providerRepositoryName) {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			if len(providerRepositoryName) <= 0 {
				fr, err := rc.DatabaseInterface.GetForkRepositoryOfUser(rc.LoginInfo.UserName, s.Namespace, s.Name)
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				LogTemplateError(rc.LoadTemplate("pull-request/new-pull-request").Execute(w, &templates.RepositoryNewPullRequestTemplateModel{
					Config: rc.Config,
					Repository: s,
					LoginInfo: rc.LoginInfo,
					ProviderRepository: fr,
					Stage: "repo",
				}))
			} else {
				_, _, _, provider, err := rc.ResolveRepositoryFullName(providerRepositoryName)
				if err == ErrNotFound {
					rc.ReportNotFound(provider.FullName(), "Repository", "Depot", w, r)
					return
				}
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				branchNameList := make([]string, 0)
				err = provider.Repository.(*gitlib.LocalGitRepository).SyncAllBranchList()
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				for k := range provider.Repository.(*gitlib.LocalGitRepository).BranchIndex {
					branchNameList = append(branchNameList, k)
				}
				LogTemplateError(rc.LoadTemplate("pull-request/new-pull-request").Execute(w, &templates.RepositoryNewPullRequestTemplateModel{
					Config: rc.Config,
					Repository: s,
					LoginInfo: rc.LoginInfo,
					ReceiverBranch: receiverBranch,
					ChosenProviderRepository: provider,
					ProviderBranchList: branchNameList,
					Stage: "branch",
				}))
			}
		},
	))
	
	http.HandleFunc("POST /repo/{repoName}/pull-request/new", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			if rc.Config.IsInPlainMode() {
				FoundAt(w, fmt.Sprintf("/repo/%s", rfn))
				return
			}
			_, _, _, s, err := rc.ResolveRepositoryFullName(rfn)
			if err == ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			err = r.ParseForm()
			if err != nil {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			title := r.Form.Get("title")
			receiverBranch := r.Form.Get("receiver-branch")
			providerNamespace := r.Form.Get("provider-namespace")
			providerName := r.Form.Get("provider-name")
			providerBranch := r.Form.Get("provider-branch")
			resId, err := rc.DatabaseInterface.NewPullRequest(rc.LoginInfo.UserName, title, s.Namespace, s.Name, receiverBranch, providerNamespace, providerName, providerBranch)
			if err != nil {
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/pull-request/new", rfn), 0, "Internal Error", fmt.Sprintf("Failed to create pull request: %s", err.Error()), w, r)
				return
			}
			FoundAt(w, fmt.Sprintf("/repo/%s/pull-request/%d", rfn, resId))
		},
	))
}

