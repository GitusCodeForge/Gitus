package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

// canEditRepositorySettings reports whether the visitor may change the
// repository's settings, i.e. its labels.
func canEditRepositorySettings(rc *RouterContext, ns *model.Namespace, repo *model.Repository) bool {
	if rc.LoginInfo == nil || !rc.LoginInfo.LoggedIn { return false }
	if rc.LoginInfo.IsAdmin { return true }
	if repo.Owner == rc.LoginInfo.UserName { return true }
	if ns != nil && ns.Owner == rc.LoginInfo.UserName { return true }
	if p := repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName); p != nil {
		if p.HasSettingPrivilege() || p.EditInfo { return true }
	}
	if ns != nil {
		if p := ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName); p != nil {
			if p.HasSettingPrivilege() || p.EditInfo { return true }
		}
	}
	return false
}

// bindRepositorySettingIssueLabelController manages the vocabulary of the
// issue labels of a repository, which is separate from the repository's own
// labels: a repository can be tagged "xorg" while its issues are tagged
// "bug".
func bindRepositorySettingIssueLabelController(ctx *RouterContext) {
	renderPage := func(rc *RouterContext, w http.ResponseWriter, r *http.Request, rfn string, nsName string, repoName string, ns *model.Namespace, repo *model.Repository) {
		labels, err := rc.DatabaseInterface.GetRepositoryIssueLabel(nsName, repoName)
		if err != nil {
			rc.ReportInternalError(fmt.Sprintf("Failed to retrieve the issue labels: %s", err.Error()), w, r)
			return
		}
		LogTemplateError(rc.LoadTemplate("repo-setting/edit-issue-label").Execute(w, &templates.RepositorySettingIssueLabelTemplateModel{
			Config: rc.Config,
			Repository: repo,
			RepoHeaderInfo: GenerateRepoHeader("", ""),
			RepoFullName: rfn,
			LoginInfo: rc.LoginInfo,
			IssueLabelList: labels,
		}))
	}

	http.HandleFunc("GET /repo/{repoName}/setting/issue-label", UseMiddleware(
		[]Middleware{Logged, LoginRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			if !canEditRepositorySettings(rc, ns, repo) {
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/setting", rfn), 0, "Not enough privilege",
					"Your user account seems to not have enough privilege for this action.", w, r)
				return
			}
			renderPage(rc, w, r, rfn, nsName, repoName, ns, repo)
		},
	))

	http.HandleFunc("POST /repo/{repoName}/setting/issue-label", UseMiddleware(
		[]Middleware{Logged, LoginRequired, ValidPOSTRequestRequired, CSRFCheck, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			returnPath := fmt.Sprintf("/repo/%s/setting/issue-label", rfn)
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			if !canEditRepositorySettings(rc, ns, repo) {
				rc.ReportRedirect(returnPath, 0, "Not enough privilege",
					"Your user account seems to not have enough privilege for this action.", w, r)
				return
			}
			label := strings.TrimSpace(r.Form.Get("label"))
			if len(label) <= 0 {
				rc.ReportRedirect(returnPath, 5, "Invalid Request", "Label text must not be empty.", w, r)
				return
			}
			if err := rc.DatabaseInterface.AddRepositoryIssueLabel(nsName, repoName, label); err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to add the issue label: %s", err.Error()), w, r)
				return
			}
			rc.ReportRedirect(returnPath, 0, "Added", "The issue label has been added to the repository.", w, r)
		},
	))

	http.HandleFunc("GET /repo/{repoName}/setting/issue-label/{label}/delete", UseMiddleware(
		[]Middleware{Logged, LoginRequired, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			label := r.PathValue("label")
			ctx.ReportSingleButtonCallback(
				fmt.Sprintf("/repo/%s/setting/issue-label/%s/delete", rfn, label),
				"Delete Issue Label",
				fmt.Sprintf("Click the following button to delete the issue label <code>%s</code> from repository <code>%s</code>. Issues that carry it keep their label until it is removed from them.", label, rfn),
				"Delete",
				nil,
				w, r,
			)
		},
	))

	http.HandleFunc("POST /repo/{repoName}/setting/issue-label/{label}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, ValidPOSTRequestRequired,
			CSRFCheck, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			returnPath := fmt.Sprintf("/repo/%s/setting/issue-label", rfn)
			nsName, repoName, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if repo == nil {
				ctx.ReportNotFound(repoName, "Repository", nsName, w, r)
				return
			}
			if !canEditRepositorySettings(rc, ns, repo) {
				rc.ReportRedirect(returnPath, 0, "Not enough privilege",
					"Your user account seems to not have enough privilege for this action.", w, r)
				return
			}
			label := r.PathValue("label")
			if err := rc.DatabaseInterface.RemoveRepositoryIssueLabel(nsName, repoName, label); err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to delete the issue label: %s", err.Error()), w, r)
				return
			}
			rc.ReportRedirect(returnPath, 0, "Deleted", "The issue label has been deleted from the repository.", w, r)
		},
	))
}
