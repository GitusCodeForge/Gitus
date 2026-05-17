package controller

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"os/exec"
	"path"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/routes"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func handleBranchSnapshotRequest(repo *model.Repository, branchName string, obj gitlib.GitObject, w http.ResponseWriter) {
	filename := fmt.Sprintf(
		"%s-%s-branch-%s",
		repo.Namespace, repo.Name, branchName,
	)
	responseWithTreeZip(repo.Repository.(*gitlib.LocalGitRepository), obj, filename, w)
}

func bindBranchController(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/branch/{branchName}/{treePath...}", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			if !model.ValidRepositoryName(rfn) {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			_, repoName, ns, repo, err := rc.ResolveRepositoryFullName(rfn)
			if err == routes.ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			if repo.Type != model.REPO_TYPE_GIT {
				rc.ReportNormalError("The repository you have requested isn't a Git repository.", w, r)
				return
			}
			if !rc.Config.IsInPlainMode() {
				rc.LoginInfo.IsOwner = (repo.Owner == rc.LoginInfo.UserName) || (ns.Owner == rc.LoginInfo.UserName)
			}
			// reject visit if repo is private & user not logged in or not member.
			if !ctx.Config.IsInPlainMode() && repo.Status == model.REPO_NORMAL_PRIVATE {
				chk := rc.LoginInfo.IsAdmin || rc.LoginInfo.IsOwner
				if !chk {
					chk = repo.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName) != nil
				}
				if !chk {
					chk = ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName) != nil
				}
				if !chk {
					rc.ReportNotFound(repo.FullName(), "Repository", "Depot", w, r)
					return
				}
			}
			
			branchName := r.PathValue("branchName")
			repoHeaderInfo := GenerateRepoHeader("branch", branchName)
			
			treePath := r.PathValue("treePath")

			rr := repo.Repository.(*gitlib.LocalGitRepository)
			err = rr.SyncAllBranchList()
			if err != nil {
				rc.ReportInternalError(
					fmt.Sprintf(
						"Cannot sync branch list for %s: %s",
						repoName,
						err.Error(),
					), w, r,
				)
				return
			}
			br, ok := rr.BranchIndex[branchName]
			if !ok {
				rc.ReportNotFound(branchName, "Branch", repoName, w, r)
				return
			}
			gobj, err := rr.ReadObject(br.HeadId)
			if err != nil {
				rc.ReportObjectReadFailure(br.HeadId, err.Error(), w, r)
				return
			}
			if gobj.Type() != gitlib.COMMIT {
				rc.ReportObjectTypeMismatch(gobj.ObjectId(), "COMMIT", gobj.Type().String(), w, r)
				return
			}

			cobj := gobj.(*gitlib.CommitObject)
			m := make(map[string]string, 0)
			if ctx.Config.OperationMode == gitus.OP_MODE_NORMAL {
				m[cobj.AuthorInfo.AuthorEmail] = ""
				m[cobj.CommitterInfo.AuthorEmail] = ""
				rc.DatabaseInterface.ResolveMultipleEmailToUsername(m)
			}
			commitInfo := &templates.CommitInfoTemplateModel{
				RootPath: fmt.Sprintf("/repo/%s", rfn),
				Commit: cobj,
				EmailUserMapping: m,
			}
			gobj, err = rr.ReadObject(cobj.TreeObjId)
			if err != nil { rc.ReportInternalError(err.Error(), w, r) }
			target, err := rr.ResolveTreePath(gobj.(*gitlib.TreeObject), treePath)
			if err != nil {
				if err == gitlib.ErrObjectNotFound {
					rc.ReportNotFound(treePath, "Path", fmt.Sprintf("repository %s", repo.FullName()), w, r)
					return
				}
				rc.ReportInternalError(err.Error(), w, r)
				return
			}

			isTargetBlob := target.Type() == gitlib.BLOB
			isTargetTree := target.Type() == gitlib.TREE

			// if it's a query for snapshot of a tree we directly output
			// the tree object as a .zip file
			isSnapshotRequest :=  r.URL.Query().Has("snapshot")
			if isSnapshotRequest {
				if isTargetBlob {
					mime := mime.TypeByExtension(path.Ext(treePath))
					if len(mime) <= 0 { mime = "application/octet-stream" }
					w.Header().Add("Content-Type", mime)
					w.Write((target.(*gitlib.BlobObject)).Data)
					return
				} else {
					handleBranchSnapshotRequest(repo, branchName, target, w)
					return
				}
			}

			tp1 := make([]string, 0)
			treePathSegmentList := make([]struct{Name string;RelPath string}, 0)
			for item := range strings.SplitSeq(treePath, "/") {
				if len(item) <= 0 { continue }
				tp1 = append(tp1, item)
				treePathSegmentList = append(treePathSegmentList, struct{
					Name string; RelPath string
				}{
					Name: item, RelPath: strings.Join(tp1, "/"),
				})
			}
			
			rootFullName := fmt.Sprintf("%s@%s:%s", rfn, "branch", branchName)
			repoPath := fmt.Sprintf("/repo/%s", rfn)
			rootPath := fmt.Sprintf("/repo/%s/%s/%s", rfn, "branch", branchName)
			treePathModelValue := &templates.TreePathTemplateModel{
				RootFullName: rootFullName,
				RootPath: rootPath,
				TreePath: treePath,
				TreePathSegmentList: treePathSegmentList,
			}
			permaLink := fmt.Sprintf("/repo/%s/commit/%s/%s", rfn, cobj.Id, treePath)
			
			isNewFileRequest := r.URL.Query().Has("new-file")
			if isNewFileRequest && isTargetTree {
				if !rc.LoginInfo.LoggedIn {
					rc.ReportRedirect("/login", 0, "Login Required", "The action you requested requires you to log in. Please log in and try again.", w, r)
				return
				}
				LogTemplateError(rc.LoadTemplate("new-file").Execute(w, &templates.NewFileTemplateModel{
					Config: rc.Config,
					Repository: repo,
					RepoHeaderInfo: *repoHeaderInfo,
					PermaLink: permaLink,
					TargetFilePath: treePath,
					CommitInfo: commitInfo,
					LoginInfo: rc.LoginInfo,
				}))
				return
			}
			
			isEditRequest := r.URL.Query().Has("edit")
			if isEditRequest && isTargetBlob {
				if !rc.LoginInfo.LoggedIn {
					rc.ReportRedirect("/login", 0, "Login Required", "The action you requested requires you to log in. Please log in and try again.", w, r)
				return
				}
				mime := mime.TypeByExtension(path.Ext(treePath))
				if len(mime) <= 0 { mime = "application/octet-stream" }
				if !strings.HasPrefix(mime, "image/") {
					LogTemplateError(rc.LoadTemplate("edit-file").Execute(w, &templates.EditFileTemplateModel{
						Config: rc.Config,
						Repository: repo,
						RepoHeaderInfo: *repoHeaderInfo,
						PermaLink: permaLink,
						FullTreePath: treePath,
						FileContent: string(target.(*gitlib.BlobObject).Data),
						CommitInfo: commitInfo,
						TagInfo: nil,
						LoginInfo: rc.LoginInfo,
					}))
				} else {
					LogTemplateError(rc.LoadTemplate("upload-file").Execute(w, &templates.UploadFileTemplateModel{
						Config: rc.Config,
						Repository: repo,
						RepoHeaderInfo: *repoHeaderInfo,
						PermaLink: permaLink,
						FullTreePath: treePath,
						CommitInfo: commitInfo,
						TagInfo: nil,
						LoginInfo: rc.LoginInfo,
					}))
				}
				return
			}

			var upstream model.LocalRepository
			var compareInfo *gitlib.BranchComparisonInfo = nil
			if len(repo.ForkOriginName) > 0 || len(repo.ForkOriginNamespace) > 0 {
				upstreamPath := path.Join(rc.Config.GitRoot, repo.ForkOriginNamespace, repo.ForkOriginName)
				upstream, _ = model.CreateLocalRepository(model.REPO_TYPE_GIT, repo.ForkOriginNamespace, repo.ForkOriginName, upstreamPath)
				remoteName := fmt.Sprintf("%s/%s", repo.Namespace, repo.Name)
				compareInfo, err = upstream.(*gitlib.LocalGitRepository).CompareBranchWithRemote(branchName, remoteName)
			}
			
			isFastForwardRequest := r.URL.Query().Has("ff")
			if isFastForwardRequest && compareInfo != nil {
				if !rc.LoginInfo.IsOwner {
					ctx.ReportRedirect(fmt.Sprintf("/repo/%s", rfn), 0,
						"Not enouhg privilege",
						"Your user account seems to not have enough privilege for this action.",
						w, r,
					)
					return
				}
				if len(compareInfo.ARevList) > 0 && len(compareInfo.BRevList) <= 0 {
					err = rr.FetchRemote("origin")
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to sync repository: %s", err), w, r)
						return
					}
					err = rr.UpdateRef(branchName, compareInfo.ARevList[0])
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to sync repository: %s", err), w, r)
						return
					}
					rc.ReportRedirect(fmt.Sprintf("/repo/%s/branch/%s", rfn, branchName), 3, "Repository Synced", "The repository has been successfully fast-forwarded with its upstream.", w, r)
					return
				}
			}
			
			isBlameRequest := r.URL.Query().Has("blame")
			if isBlameRequest && isTargetBlob {
				mime := mime.TypeByExtension(path.Ext(treePath))
				if len(mime) <= 0 { mime = "application/octet-stream" }
				if !strings.HasPrefix(mime, "image/") {
					dirPath := path.Dir(treePath) + "/"
					dirObj, err := rr.ResolveTreePath(gobj.(*gitlib.TreeObject), dirPath)
					if err != nil {
						rc.ReportInternalError(err.Error(), w, r)
						return
					}
					blame, err := rr.Blame(cobj, treePath)
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to run git-blame: %s.", err), w, r)
						return
					}
					LogTemplateError(rc.LoadTemplate("git-blame").Execute(w, &templates.GitBlameTemplateModel{
						Repository: repo,
						RepoHeaderInfo: *repoHeaderInfo,
						TreeFileList: &templates.TreeFileListTemplateModel{
							ShouldHaveParentLink: len(treePath) > 0,
							RepoPath: fmt.Sprintf("/repo/%s", rfn),
							RootPath: fmt.Sprintf("/repo/%s/%s/%s", rfn, "branch", branchName),
							TreePath: dirPath,
							FileList: dirObj.(*gitlib.TreeObject).ObjectList,
						},
						TreePath: treePathModelValue,
						PermaLink: permaLink,
						Blame: blame,
						CommitInfo: commitInfo,
						TagInfo: nil,
						LoginInfo: rc.LoginInfo,
						Config: rc.Config,
					}))
					return
				}
			}
			
			switch target.Type() {
			case gitlib.TREE:
				if len(treePath) > 0 && !strings.HasSuffix(treePath, "/") {
					FoundAt(w, fmt.Sprintf("%s/%s/", rootPath, treePath))
					return
				}
				// NOTE: this is intentional. by the time we've reached here
				// `treePath` would end with a slash `/`, and the first `path.Dir`
				// call would only remove that slash, whose result is not the path
				// of the parent directory.
				var parentTreeFileList *templates.TreeFileListTemplateModel = nil
				if treePath != "" {
					dirPath := path.Dir(path.Dir(treePath)) + "/"
					dirObj, err := rr.ResolveTreePath(gobj.(*gitlib.TreeObject), dirPath)
					if err != nil {
						if err == gitlib.ErrObjectNotFound {
							rc.ReportNotFound(treePath, "Directory", "Repository", w, r)
							return
						}
						rc.ReportInternalError(err.Error(), w, r)
						return
					}
					parentTreeFileList = &templates.TreeFileListTemplateModel{
						ShouldHaveParentLink: len(treePath) > 0,
						RepoPath: repoPath,
						RootPath: rootPath,
						TreePath: dirPath,
						FileList: dirObj.(*gitlib.TreeObject).ObjectList,
					}
				}
				// TODO: find a better way to do this...
				for i, k := range target.(*gitlib.TreeObject).ObjectList {
					cid, err := rr.ResolvePathLastCommitId(cobj, treePath + k.Name)
					if err != nil { continue }
					o, err := rr.ReadObject(strings.TrimSpace(cid))
					if err != nil { continue }
					target.(*gitlib.TreeObject).ObjectList[i].LastCommit = o.(*gitlib.CommitObject)
				}
				LogTemplateError(rc.LoadTemplate("tree").Execute(w, templates.TreeTemplateModel{
					Repository: repo,
					RepoHeaderInfo: *repoHeaderInfo,
					TreeFileList: &templates.TreeFileListTemplateModel{
						ShouldHaveParentLink: len(treePath) > 0,
						RepoPath: repoPath,
						RootPath: rootPath,
						TreePath: treePath,
						FileList: target.(*gitlib.TreeObject).ObjectList,
					},
					ComparisonInfo: compareInfo,
					ParentTreeFileList: parentTreeFileList,
					PermaLink: permaLink,
					TreePath: treePathModelValue,
					CommitInfo: commitInfo,
					TagInfo: nil,
					LoginInfo: rc.LoginInfo,
					Config: rc.Config,
				}))
			case gitlib.BLOB:
				dirPath := path.Dir(treePath) + "/"
				dirObj, err := rr.ResolveTreePath(gobj.(*gitlib.TreeObject), dirPath)
				if err != nil {
					if err == gitlib.ErrObjectNotFound {
						rc.ReportNotFound(treePath, "File", "Repository", w, r)
						return
					}
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				mime := mime.TypeByExtension(path.Ext(treePath))
				if len(mime) <= 0 { mime = "application/octet-stream" }
				templateType := "file-text"
				if strings.HasPrefix(mime, "image/") {
					templateType = "file-image"
				}
				bobj := target.(*gitlib.BlobObject)
				if r.URL.Query().Has("raw") {
					w.Header().Add("Content-Type", mime)
					w.Write(bobj.Data)
					return
				}
				str := string(bobj.Data)
				filename := path.Base(treePath)
				coloredStr, err := colorSyntax(filename, str)
				if err == nil { str = coloredStr }
				LogTemplateError(rc.LoadTemplate(templateType).Execute(w, templates.FileTemplateModel{
					Repository: repo,
					RepoHeaderInfo: *repoHeaderInfo,
					File: templates.BlobTextTemplateModel{
						FileLineCount: strings.Count(str, "\n"),
						FileContent: str,
					},
					PermaLink: permaLink,
					TreeFileList: &templates.TreeFileListTemplateModel{
						ShouldHaveParentLink: len(treePath) > 0,
						RepoPath: repoPath,
						RootPath: rootPath,
						TreePath: dirPath,
						FileList: dirObj.(*gitlib.TreeObject).ObjectList,
					},
					ComparisonInfo: compareInfo,
					AllowBlame: !strings.HasPrefix(mime, "image/"),
					TreePath: treePathModelValue,
					CommitInfo: commitInfo,
					TagInfo: nil,
					LoginInfo: rc.LoginInfo,
					Config: rc.Config,
				}))
			default:
				rc.ReportInternalError("", w, r)
			}
		},
	))

	http.HandleFunc("POST /repo/{repoName}/branch/{branchName}/{treePath...}", UseMiddleware(
		[]Middleware{
			Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, GlobalVisibility,
			ValidRepositoryNameRequired("repoName"),
			ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			_, _, _, repo, err := rc.ResolveRepositoryFullName(rfn)
			if err == routes.ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			if repo.Type != model.REPO_TYPE_GIT {
				rc.ReportNormalError("The repository you have requested isn't a Git repository.", w, r)
				return
			}
			if rc.Config.IsInPlainMode() {
				FoundAt(w, fmt.Sprintf("/repo/%s/branch/%s/%s", rfn, r.PathValue("branchName"), r.PathValue("treePath")))
				return
			}
			if repo.Owner != rc.LoginInfo.UserName {
				rc.ReportRedirect(
					fmt.Sprintf("/repo/%s/branch/%s/%s", rfn, r.PathValue("branchName"), r.PathValue("treePath")),
					5,
					"Not Enough Privilege",
					"Your account doesn't have enough privilege to perform this action.",
					w, r,
				)
			}
			// we have to handle the upload-file case carefully since the
			// file could be big and i do not wish to read a big file into
			// the memory.
			action := strings.TrimSpace(r.Form.Get("action"))
			if len(action) <= 0 {
				r.ParseMultipartForm(32*1024*1024)
				action = strings.TrimSpace(r.MultipartForm.Value["action"][0])
			}
			branchName := r.PathValue("branchName")
			commitMessage := r.Form.Get("commit-message")
			var commitId string
			var treePath string
			switch action {
			case "edit":
				treePath = r.PathValue("treePath")
				content := r.Form.Get("content")
				commitId, err = model.AddFileToRepoString(repo.Repository, branchName, treePath, rc.LoginInfo.UserFullName, rc.LoginInfo.UserEmail, rc.LoginInfo.UserFullName, rc.LoginInfo.UserEmail, commitMessage, content)
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed while adding file to repo: %s", err), w, r)
					return
				}
			case "new":
				treePath = strings.TrimSpace(r.Form.Get("new-file-path"))
				if len(r.Form.Get("use-upload-file")) > 0 {
					f, e, err := r.FormFile("file-upload")
					commitId, err = model.AddFileToRepoReader(repo.Repository, branchName, treePath, rc.LoginInfo.UserFullName, rc.LoginInfo.UserEmail, rc.LoginInfo.UserFullName, rc.LoginInfo.UserEmail, commitMessage, f, e.Size)
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed while adding file to repo: %s", err), w, r)
						return
					}
				} else {
					content := r.Form.Get("content")
					commitId, err = model.AddFileToRepoString(repo.Repository, branchName, treePath, rc.LoginInfo.UserFullName, rc.LoginInfo.UserEmail, rc.LoginInfo.UserFullName, rc.LoginInfo.UserEmail, commitMessage, content)
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed while adding file to repo: %s", err), w, r)
						return
					}
				}
			case "replace":
				treePath = r.PathValue("treePath")
				f, e, err := r.FormFile("file-upload")
				commitId, err = model.AddFileToRepoReader(repo.Repository, branchName, treePath, rc.LoginInfo.UserFullName, rc.LoginInfo.UserEmail, rc.LoginInfo.UserFullName, rc.LoginInfo.UserEmail, commitMessage, f, e.Size)
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed while adding file to repo: %s", err), w, r)
					return
				}
			}
			commitId = strings.TrimSpace(commitId)
			cmd2 := exec.Command("git", "update-ref", fmt.Sprintf("refs/heads/%s", branchName), commitId)
			cmd2.Dir = repo.LocalPath
			stderrBuf := new(bytes.Buffer)
			stderrBuf.Reset()
			cmd2.Stderr = stderrBuf
			err = cmd2.Run()
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to update ref: %s; %s", err.Error(), stderrBuf.String()), w, r)
				return
			}
			rc.ReportRedirect(fmt.Sprintf("/repo/%s/branch/%s/%s", rfn, branchName, r.PathValue("treePath")), 5, "Updated", "Your edit has been saved to the repository.", w, r)
		},
	))
}


