package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

// canModifyRepositoryRefs reports whether the visitor may create or delete
// branches and tags: the people who can push to the repository, plus the ones
// who can change its settings.
func canModifyRepositoryRefs(rc *RouterContext, ns *model.Namespace, repo *model.Repository) bool {
	if rc.LoginInfo == nil || !rc.LoginInfo.LoggedIn { return false }
	if rc.LoginInfo.IsAdmin { return true }
	if repo.Owner == rc.LoginInfo.UserName { return true }
	if ns != nil && ns.Owner == rc.LoginInfo.UserName { return true }
	if p := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName); p != nil {
		if p.HasSettingPrivilege() || p.PushToRepository { return true }
	}
	if ns != nil {
		if p := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName); p != nil {
			if p.HasSettingPrivilege() || p.PushToRepository { return true }
		}
	}
	return false
}

// refShortNameProblem returns a description of why name cannot be used from
// the web interface, or an empty string when it can. Git accepts hierarchical
// names like "feature/x", but the branch and tag routes only take a single
// path segment, so such a ref could not be visited afterwards.
func refShortNameProblem(name string) string {
	if len(name) <= 0 { return "the name is empty" }
	if strings.Contains(name, "/") {
		return "names with a slash are not supported by the web interface yet"
	}
	return ""
}

func bindRepositoryRefsController(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/refs", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			_, _, ns, repo, err := rc.ResolveRepositoryFullName(rfn)
			if err == ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			if !rc.Config.IsInForgeMode() {
				FoundAt(w, fmt.Sprintf("/repo/%s", rfn))
				return
			}
			if repo.Type != model.REPO_TYPE_GIT {
				rc.ReportNormalError("The repository you have requested isn't a Git repository.", w, r)
				return
			}
			if repo.Status == model.REPO_NORMAL_PRIVATE {
				chk := rc.LoginInfo.IsAdmin || repo.Owner == rc.LoginInfo.UserName ||
					(ns != nil && ns.Owner == rc.LoginInfo.UserName)
				if !chk {
					chk = repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName) != nil ||
						(ns != nil && ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName) != nil)
				}
				if !chk {
					rc.ReportNotFound(repo.FullName(), "Repository", "Depot", w, r)
					return
				}
			}
			rr := repo.Repository.(*gitlib.LocalGitRepository)
			if err := rr.SyncAllBranchList(); err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to read the branch list: %s", err.Error()), w, r)
				return
			}
			if err := rr.SyncAllTagList(); err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to read the tag list: %s", err.Error()), w, r)
				return
			}
			head, err := rr.HeadBranch()
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to read HEAD: %s", err.Error()), w, r)
				return
			}
			branches := make([]*gitlib.Branch, 0, len(rr.BranchIndex))
			for _, b := range rr.BranchIndex {
				branches = append(branches, b)
			}
			sort.Slice(branches, func(i, j int) bool { return branches[i].Name < branches[j].Name })
			tags := make([]*gitlib.Tag, 0, len(rr.TagIndex))
			for _, t := range rr.TagIndex {
				tags = append(tags, t)
			}
			sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })

			LogTemplateError(rc.LoadTemplate("repo-refs").Execute(w, &templates.RepositoryRefsTemplateModel{
				Config: rc.Config,
				Repository: repo,
				RepoHeaderInfo: &templates.RepoHeaderTemplateModel{TypeStr: "", NodeName: ""},
				LoginInfo: rc.LoginInfo,
				BranchList: branches,
				TagList: tags,
				HeadBranch: head,
				CanModify: canModifyRepositoryRefs(rc, ns, repo),
			}))
		},
	))

	http.HandleFunc("POST /repo/{repoName}/refs", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, LoginRequired, CSRFCheck, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			returnPath := fmt.Sprintf("/repo/%s/refs", rfn)
			_, _, ns, repo, err := rc.ResolveRepositoryFullName(rfn)
			if err == ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			if !rc.Config.IsInForgeMode() {
				FoundAt(w, returnPath)
				return
			}
			if repo.Type != model.REPO_TYPE_GIT {
				rc.ReportNormalError("The repository you have requested isn't a Git repository.", w, r)
				return
			}
			if !canModifyRepositoryRefs(rc, ns, repo) {
				rc.ReportRedirect(returnPath, 5, "Not Enough Privilege",
					"Your account doesn't have enough privilege to change the branches or tags of this repository.", w, r)
				return
			}
			rr := repo.Repository.(*gitlib.LocalGitRepository)
			action := strings.TrimSpace(r.Form.Get("type"))
			name := strings.TrimSpace(r.Form.Get("name"))
			target := strings.TrimSpace(r.Form.Get("target"))
			if len(target) <= 0 {
				target, err = rr.HeadBranch()
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to read HEAD: %s", err.Error()), w, r)
					return
				}
			}
			switch action {
			case "create-branch", "create-tag":
				if problem := refShortNameProblem(name); len(problem) > 0 {
					rc.ReportRedirect(returnPath, 5, "Invalid Name", problem, w, r)
					return
				}
			}
			switch action {
			case "create-branch":
				err = rr.CreateBranch(name, target)
			case "delete-branch":
				err = rr.DeleteBranch(name)
			case "create-tag":
				err = rr.CreateTag(name, target)
			case "delete-tag":
				err = rr.DeleteTag(name)
			default:
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			if err != nil {
				rc.ReportRedirect(returnPath, 10, "Failed", err.Error(), w, r)
				return
			}
			message := fmt.Sprintf("Branch %q has been created.", name)
			switch action {
			case "delete-branch": message = fmt.Sprintf("Branch %q has been deleted.", name)
			case "create-tag": message = fmt.Sprintf("Tag %q has been created.", name)
			case "delete-tag": message = fmt.Sprintf("Tag %q has been deleted.", name)
			}
			rc.ReportRedirect(returnPath, 5, "Done", message, w, r)
		},
	))
}
