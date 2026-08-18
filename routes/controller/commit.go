package controller

import (
	"fmt"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/routes"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func handleCommitSnapshotRequest(repo *model.Repository, commitId string, obj gitlib.GitObject, w http.ResponseWriter) {
	filename := fmt.Sprintf(
		"%s-%s-commit-%s",
		repo.Namespace, repo.Name, commitId,
	)
	responseWithTreeZip(repo.Repository.(*gitlib.LocalGitRepository), obj, filename, w)
}

func bindCommitController(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/commit/{commitId}/{treePath...}", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, GlobalVisibility, ErrorGuard},
		ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			if !model.ValidRepositoryName(rfn) {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			_, _, ns, repo, err := rc.ResolveRepositoryFullName(rfn)
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
			if !rc.Config.IsInBrowseOnlyMode() {
				rc.LoginInfo.IsOwner = (repo.Owner == rc.LoginInfo.UserName) || (ns.Owner == rc.LoginInfo.UserName)
			}

			// reject visit if repo is private & user not logged in or not member.
			if !ctx.Config.IsInBrowseOnlyMode() && repo.Status == model.REPO_NORMAL_PRIVATE {
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
			
			commitId := r.PathValue("commitId")
			treePath := r.PathValue("treePath")

			repoHeaderInfo := GenerateRepoHeader("commit", commitId)

			rr := repo.Repository.(*gitlib.LocalGitRepository)
			gobj, err := rr.ReadObject(commitId)
			if err != nil {
				rc.ReportObjectReadFailure(commitId, err.Error(), w, r)
				return
			}
			if gobj.Type() != gitlib.COMMIT {
				rc.ReportObjectTypeMismatch(gobj.ObjectId(), "COMMIT", gobj.Type().String(), w, r)
				return
			}

			cobj := gobj.(*gitlib.CommitObject)
			m := make(map[string]string, 0)
			if ctx.Config.IsInForgeMode() {
				m[cobj.AuthorInfo.AuthorEmail] = ""
				m[cobj.CommitterInfo.AuthorEmail] = ""
				rc.DatabaseInterface.ResolveMultipleEmailToUsername(m)
			}
			// NOTE: we don't check, we just assume the emails are not verified
			// to anyone if an error occur.
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
			
			isSnapshotRequest :=  r.URL.Query().Has("snapshot")
			if isSnapshotRequest {
				if isTargetBlob {
					mime := mime.TypeByExtension(path.Ext(treePath))
					if len(mime) <= 0 { mime = "application/octet-stream" }
					w.Header().Add("Content-Type", mime)
					w.Write((target.(*gitlib.BlobObject)).Data)
					return
				} else {
					handleCommitSnapshotRequest(repo, commitId, target, w)
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
			rootFullName := fmt.Sprintf("%s@%s:%s", rfn, "commit", commitId)
			repoPath := fmt.Sprintf("/repo/%s", rfn)
			rootPath := fmt.Sprintf("/repo/%s/%s/%s", rfn, "commit", commitId)
			permaLink := fmt.Sprintf("/repo/%s/commit/%s/%s", rfn, commitId, treePath)
			treePathModelValue := &templates.TreePathTemplateModel{
				RootFullName: rootFullName,
				RootPath: rootPath,
				TreePath: treePath,
				TreePathSegmentList: treePathSegmentList,
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
							RootPath: fmt.Sprintf("/repo/%s/%s/%s", rfn, "commit", cobj.Id),
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
					PermaLink: permaLink,
					ParentTreeFileList: parentTreeFileList,
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
}

