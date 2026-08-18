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

func handleTagSnapshotRequest(repo *model.Repository, branchName string, obj gitlib.GitObject, w http.ResponseWriter) error {
	// would resolve tags that point to tags.
	subj := obj
	var err error = nil
	rr := repo.Repository.(*gitlib.LocalGitRepository)
	for subj.Type() == gitlib.TAG {
		subj, err = rr.ReadObject((subj.(*gitlib.TagObject)).TaggedObjId)
		if err != nil { return err }
	}
	filename := fmt.Sprintf(
		"%s-%s-branch-%s",
		repo.Namespace, repo.Name, branchName,
	)
	if subj.Type() == gitlib.TREE {
		return responseWithTreeZip(rr, obj, filename, w)
	} else {
		w.Write(subj.RawData())
		return nil
	}
}

func bindTagController(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/tag/{tagId}/{treePath...}", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
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
				rc.LoginInfo.IsOwner = repo.Owner == rc.LoginInfo.UserName || ns.Owner == rc.LoginInfo.UserName
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
			rr := repo.Repository.(*gitlib.LocalGitRepository)
			tagName := r.PathValue("tagId")
			treePath := r.PathValue("treePath")

			m := make(map[string]string, 0)
			
			// the logic goes as follows:
			// + the subject for resolving would be a tag.
			// + resolve suject into tagInfo w/ one of commit / tree / blob / tag
			// + if the subject is commit, resolves it to commitInfo w/ tree.
			// + if the subject is tree and len(treePath) > 0, resolve tree with treePath
			//   into a tree or a blob.
			// + by now we have a tagInfo/nil, a commitInfo/nil and a tree/blob/tag.
			//   we thus display them accordingly.
			repoHeaderInfo := GenerateRepoHeader("tag", tagName)
			
			err = rr.SyncAllTagList()
			if err != nil {
				rc.ReportInternalError(
					fmt.Sprintf("Failed to sync tag list: %s", err.Error()),
					w, r,
				)
				return
			}
			t, ok := rr.TagIndex[tagName]
			if !ok {
				rc.ReportNotFound(tagName, "Tag", rfn, w, r)
				return
			}
			tobj, err := rr.ReadObject(t.HeadId)
			if err != nil {
				rc.ReportObjectReadFailure(t.HeadId, err.Error(), w, r)
				return
			}

			if r.URL.Query().Has("snapshot") {
				handleTagSnapshotRequest(repo, tagName, tobj, w)
				return
			}

			// NOTE: the part about permalink is slightly tricky.
			// + if a tag points to a tag, then the permalink is the same as the link.
			// + if a tag points to a commit, then the permalink should be that of
			//   the commit.
			// + if a tag points to anything else then the permalink should be that
			//   of those.
			permaLink := ""
			
			var subject gitlib.GitObject = nil
			var tagInfo *templates.TagInfoTemplateModel = nil

			if tobj.Type() == gitlib.TAG {
				to := tobj.(*gitlib.TagObject)
				m[to.TaggerInfo.AuthorEmail] = ""
				tagInfo = &templates.TagInfoTemplateModel{
					Annotated: true,
					RepoName: rfn,
					Tag: to,
				}
				subject, err = rr.ReadObject(to.TaggedObjId)
				if err != nil {
					rc.ReportObjectReadFailure(to.TaggedObjId, err.Error(), w, r)
					return
				}
			} else {
				subject = tobj
			}
			

			var commitInfo *templates.CommitInfoTemplateModel = nil

			if subject.Type() == gitlib.COMMIT {
				cobj, ok := subject.(*gitlib.CommitObject)
				if !ok {
					rc.ReportInternalError(
						fmt.Sprintf(
							"Shouldn't happen - object with COMMIT type but not parsed as commit object. ObjId: %s", subject.ObjectId(),
						),
						w, r,
					)
					return
				}
				m[cobj.AuthorInfo.AuthorEmail] = ""
				// NOTE: we don't resolve here since there could be
				// other emails getting registered down below.
				commitInfo = &templates.CommitInfoTemplateModel{
					RootPath: fmt.Sprintf("/repo/%s", rfn),
					Commit: cobj,
					EmailUserMapping: m,
				}
				subject, err = rr.ReadObject(cobj.TreeObjId)
				if err != nil {
					rc.ReportObjectReadFailure(
						cobj.TreeObjId,
						fmt.Sprintf("%s (commit %s)", err.Error(), cobj.Id),
						w, r,
					)
					return
				}
				permaLink = fmt.Sprintf("/repo/%s/commit/%s/%s", rfn, cobj.Id, treePath)
			}

			if subject.Type() == gitlib.TREE {
				subject, err = rr.ResolveTreePath(subject.(*gitlib.TreeObject), treePath)
				if err != nil {
					if err == gitlib.ErrObjectNotFound {
						rc.ReportNotFound(treePath, "Path", fmt.Sprintf("tag %s of repository %s", tagName, repo.FullName()), w, r)
						return
					}
					rc.ReportInternalError(err.Error(), w, r)
					return
				}
				if subject.Type() == gitlib.TREE && len(treePath) > 0 && !strings.HasSuffix(treePath, "/") {
					FoundAt(w, fmt.Sprintf("/repo/%s/tag/%s/%s/", rfn, tagName, treePath))
					return
				}
				if len(permaLink) <= 0 {
					permaLink = fmt.Sprintf("/repo/%s/tree/%s/%s", rfn, subject.ObjectId(), treePath)
				}
			}

			switch subject.Type() {
			case gitlib.TAG:
				tobj, ok := subject.(*gitlib.TagObject)
				if !ok {
					rc.ReportInternalError(
						fmt.Sprintf(
							"Shouldn't happen - object with TAG type but not parsed as tag object. ObjId: %s", subject.ObjectId(),
						),
						w, r,
					)
					return
				}
				if rc.Config.IsInForgeMode() {
					m, _ = rc.DatabaseInterface.ResolveMultipleEmailToUsername(m)
				}
				tagInfo.EmailUserMapping = m
				LogTemplateError(rc.LoadTemplate("tag").Execute(w, templates.TagTemplateModel{
					Repository: repo,
					RepoHeaderInfo: *repoHeaderInfo,
					Tag: tobj,
					TagInfo: tagInfo,
					LoginInfo: rc.LoginInfo,
				}))
				return
			case gitlib.BLOB:
				mime := mime.TypeByExtension(path.Ext(treePath))
				if len(mime) <= 0 { mime = "application/octet-stream" }
				templateType := "file-text"
				if strings.HasPrefix(mime, "image/") {
					templateType = "file-image"
				}
				bobj := subject.(*gitlib.BlobObject)
				if r.URL.Query().Has("raw") {
					w.Header().Add("Content-Type", mime)
					w.Write(bobj.Data)
					return
				}
				if len(permaLink) <= 0 {
					permaLink = fmt.Sprintf("/repo/%s/blob/%s", rfn, bobj.Id)
				}
				str := string(bobj.Data)
				coloredStr, err := colorSyntax("", str)
				if err == nil { str = coloredStr }
				if rc.Config.IsInForgeMode() {
					m, _ = rc.DatabaseInterface.ResolveMultipleEmailToUsername(m)
				}
				tagInfo.EmailUserMapping = m
				LogTemplateError(rc.LoadTemplate(templateType).Execute(w, templates.FileTemplateModel{
					Repository: repo,
					RepoHeaderInfo: *repoHeaderInfo,
					File: templates.BlobTextTemplateModel{
						FileLineCount: strings.Count(str, "\n"),
						FileContent: str,
					},
					PermaLink: permaLink,
					TreePath: nil,
					CommitInfo: commitInfo,
					TagInfo: tagInfo,
					LoginInfo: rc.LoginInfo,
					Config: rc.Config,
				}))

			case gitlib.TREE:
				rootFullName := fmt.Sprintf("%s@%s:%s", rfn, "tag", tagName)
				rootPath := fmt.Sprintf("/repo/%s/%s/%s", rfn, "tag", tagName)
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
				target, err := rr.ResolveTreePath(subject.(*gitlib.TreeObject), treePath)
				if err != nil {
					rc.ReportInternalError(err.Error(), w, r)
				}
				treePathModelValue := &templates.TreePathTemplateModel{
					RootFullName: rootFullName,
					RootPath: rootPath,
					TreePath: treePath,
					TreePathSegmentList: treePathSegmentList,
				}
				if rc.Config.IsInForgeMode() {
					m, _ = rc.DatabaseInterface.ResolveMultipleEmailToUsername(m)
				}
				commitInfo.EmailUserMapping = m
				LogTemplateError(rc.LoadTemplate("tree").Execute(w, templates.TreeTemplateModel{
					Repository: repo,
					RepoHeaderInfo: *repoHeaderInfo,
					TreeFileList: &templates.TreeFileListTemplateModel{
						ShouldHaveParentLink: len(treePath) > 0,
						RepoPath: fmt.Sprintf("/repo/%s", rfn),
						RootPath: rootPath,
						TreePath: treePath,
						FileList: target.(*gitlib.TreeObject).ObjectList,
					},
					PermaLink: permaLink,
					TreePath: treePathModelValue,
					CommitInfo: commitInfo,
					TagInfo: nil,
					LoginInfo: rc.LoginInfo,
					Config: rc.Config,
				}))
				return
			default:
				rc.ReportInternalError(
					fmt.Sprintf("Shouldn't happen: object type expected to be one of tag/blob/tree but it's %s instead.", subject.Type().String()),
					w, r,
				)
			}
		},
	))
}

