package controller

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
	"github.com/gomarkdown/markdown"
	"github.com/microcosm-cc/bluemonday"
	"github.com/niklasfasching/go-org/org"
)

func bindRepositoryController(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/", UseMiddleware(
		[]Middleware{Logged, ValidRepositoryNameRequired("repoName"),
			UseLoginInfo, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rfn := r.PathValue("repoName")
			branch := strings.TrimSpace(r.URL.Query().Get("branch"))
			if len(branch) > 0 {
				FoundAt(w, fmt.Sprintf("/repo/%s/branch/%s", rfn, branch))
				return
			}
			tag := strings.TrimSpace(r.URL.Query().Get("tag"))
			if len(tag) > 0 {
				FoundAt(w, fmt.Sprintf("/repo/%s/tag/%s", rfn, tag))
				return
			}
			_, _, ns, s, err := rc.ResolveRepositoryFullName(rfn)
			if err == ErrNotFound {
				rc.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			
			if rc.Config.OperationMode == gitus.OP_MODE_NORMAL {
				rc.LoginInfo.IsOwner = s.Owner == rc.LoginInfo.UserName || ns.Owner == rc.LoginInfo.UserName
			}
			if !rc.Config.IsInPlainMode() && s.Status == model.REPO_NORMAL_PRIVATE {
				t := s.AccessControlList.GetUserPrivilege(rc.LoginInfo.UserName)
				if t == nil {
					t = ns.ACL.GetUserPrivilege(rc.LoginInfo.UserName)
				}
				if t == nil {
					rc.ReportForbidden("Not enough privilege", w, r)
					return
				}
			}

			rr := s.Repository.(*gitlib.LocalGitRepository)
			err = rr.SyncAllBranchList()
			if err != nil {
				LogTemplateError(rc.LoadTemplate("error").Execute(w, templates.ErrorTemplateModel{
					ErrorCode: 500,
					ErrorMessage: fmt.Sprintf("Failed to sync branch list: %s", err.Error()),
				}))
				return
			}
			err = rr.SyncAllTagList()
			if err != nil {
				LogTemplateError(rc.LoadTemplate("error").Execute(w, templates.ErrorTemplateModel{
					ErrorCode: 500,
					ErrorMessage: fmt.Sprintf("Failed to sync tag list: %s", err.Error()),
				}))
				return
			}

			var repoHeaderInfo *templates.RepoHeaderTemplateModel
			var treeFileList *templates.TreeFileListTemplateModel
			var commitInfo *templates.CommitInfoTemplateModel
			var cobj *gitlib.CommitObject
			var treeObjList []gitlib.TreeObjectItem
			var permaLink string = ""
			var isFork bool = false
			var upstream model.LocalRepository
			var compareInfo *gitlib.BranchComparisonInfo = nil
			if len(s.ForkOriginName) > 0 || len(s.ForkOriginNamespace) > 0 {
				isFork = true
			}
			
			emailUserMap := make(map[string]string, 0)

			repoHeaderInfo = GenerateRepoHeader("", "")
			
			readmeString := ""
			readmeType := ""
			readmeTier := 0
			// now we try to read the "major" branch.
			// first we check for "master", then we check for "main",
			// if both of those branches do not exists we find the first branch
			// alphabetically.
			var obj gitlib.GitObject
			br, ok := rr.BranchIndex["master"]
			if !ok { br, ok = rr.BranchIndex["main"] }
			if !ok {
				k := auxfuncs.SortedKeys(rr.BranchIndex)
				if len(k) <= 0 {
					goto findingMajorBranchDone;
				}
				br = rr.BranchIndex[k[0]];
			}
			repoHeaderInfo.TypeStr = "branch"
			repoHeaderInfo.NodeName = br.Name
			// now we try to read the README file.
			// the order would be: README - README.txt
			//                     - any file that starts with "README."
			// if any of the two cannot be found, it's considered without a readme.
			obj, err = rr.ReadObject(br.HeadId)
			if err != nil { goto findingMajorBranchDone; }
			// i don't know if it would ever happen that a branch head would point to
			// anything that's not a commit, but if we can't find it we treat it as
			// no readme.
			if !gitlib.IsCommitObject(obj) { goto findingMajorBranchDone; }
			cobj = obj.(*gitlib.CommitObject)
			if ctx.Config.OperationMode == gitus.OP_MODE_NORMAL {
				emailUserMap[cobj.AuthorInfo.AuthorEmail] = ""
				emailUserMap[cobj.CommitterInfo.AuthorEmail] = ""
				rc.DatabaseInterface.ResolveMultipleEmailToUsername(emailUserMap)
			}
			commitInfo = &templates.CommitInfoTemplateModel{
				RootPath: fmt.Sprintf("/repo/%s", s.FullName()),
				Commit: cobj,
				EmailUserMapping: emailUserMap,
			}
			permaLink = fmt.Sprintf("/repo/%s/commit/%s/%s", rfn, cobj.Id, "")
			obj, err = rr.ReadObject(cobj.TreeObjId)
			if err != nil { goto findingMajorBranchDone; }
			treeObjList = obj.(*gitlib.TreeObject).ObjectList
			treeFileList = &templates.TreeFileListTemplateModel{
				ShouldHaveParentLink: false,
				RepoPath: fmt.Sprintf("/repo/%s", s.FullName()),
				RootPath: fmt.Sprintf("/repo/%s/branch/%s", s.FullName(), br.Name),
				TreePath: "/",
				FileList: treeObjList,
			}
			for i, item := range treeObjList {
				// TODO: find a better way to do this...
				cid, _ := rr.ResolvePathLastCommitId(cobj, item.Name)
				o, err := rr.ReadObject(strings.TrimSpace(cid))
				treeObjList[i].LastCommit = o.(*gitlib.CommitObject)
				if item.Name == "README" || strings.HasPrefix(item.Name, "README.") {
					// NOTE: this is to make sure that README.md and the like will
					// always have a higher priority than other README file; some repo
					// put platform-specific README in files like `README.{plat}.md`
					// and you can't have them getting selected when a more general
					// readme exists.
					thisTier := strings.Count(item.Name, ".")
					if readmeTier > 0 && thisTier >= readmeTier { continue }
					readmeTier = thisTier
					obj, err = rr.ReadObject(item.Hash)
					if err != nil { continue }
					if !gitlib.IsBlobObject(obj) { continue }
					obj, err = rr.ReadObject(item.Hash)
					readmeType = path.Ext(item.Name)
					readmeString = string(obj.(*gitlib.BlobObject).Data)
				}
			}
			switch readmeType {
			case ".md":
				// NOTE: markdown is tricky. because people uses html in
				// markdown file for things markdown does not support
				// (e.g. <detail>) so it's a good idea to allow them
				// instead of escaping all html (and also we're going to
				// embed raw html anyway doe to how we currently do
				// things), but if we allow arbitrary html then people are
				// gonna do XSS attacks. i'd like to keep dependencies as
				// minimal as possible, but i'm not ready to make a whole
				// markdown-to-html renderer, at least not yet.
				readmeString = string(markdown.ToHTML([]byte(readmeString), nil, nil))
				readmeString = bluemonday.StrictPolicy().Sanitize(readmeString)
			case ".org":
				// NOTE: due to go-org having no documentations and I can't see
				// a way to inject prefix into the rendering process, we might
				// have to either make a better form or create an org-mode
				// parser on our own.
				doc := org.New().Parse(strings.NewReader(readmeString), "")
				out, err := doc.Write(org.NewHTMLWriter())
				if err != nil {
					readmeString = bluemonday.StrictPolicy().Sanitize(readmeString)
					readmeString = fmt.Sprintf("<pre class=\"repo-readme\">>%s</pre>", readmeString)
				} else {
					readmeString = bluemonday.StrictPolicy().Sanitize(out)
				}
			default:
				readmeString = bluemonday.StrictPolicy().Sanitize(readmeString)
				readmeString = fmt.Sprintf("<pre>%s</pre>", readmeString)
			}
			// resolve branch comparison info...
			if isFork {
				upstreamPath := path.Join(ctx.Config.GitRoot, s.ForkOriginNamespace, s.ForkOriginName)
				upstream, _ = model.CreateLocalRepository(model.REPO_TYPE_GIT, s.ForkOriginNamespace, s.ForkOriginName, upstreamPath)
				remoteName := fmt.Sprintf("%s/%s", s.Namespace, s.Name)
				compareInfo, err = upstream.(*gitlib.LocalGitRepository).CompareBranchWithRemote(br.Name, remoteName)
			}
			
		findingMajorBranchDone:
			isFastForwardRequest := r.URL.Query().Has("ff")
			if isFastForwardRequest {
				if !rc.LoginInfo.IsOwner {
					ctx.ReportRedirect(fmt.Sprintf("/repo/%s", rfn), 0,
						"Not enouhg privilege",
						"Your user account seems to not have enough privilege for this action.",
						w, r,
					)
					return
				}
				if compareInfo != nil {
					err = rr.FetchRemote("origin")
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to sync repository: %s", err), w, r)
						return
					}
					if len(compareInfo.ARevList) > 0 && len(compareInfo.BRevList) <= 0 {
						err = rr.UpdateRef(br.Name, compareInfo.ARevList[0])
						if err != nil {
							rc.ReportInternalError(fmt.Sprintf("Failed to sync repository: %s", err), w, r)
							return
						}
					}
				} else {
					err = rr.SyncEmptyRepositoryFromRemote("origin")
					if err != nil {
						rc.ReportInternalError(fmt.Sprintf("Failed to sync repository: %s", err), w, r)
						return
					}
				}
				rc.ReportRedirect(fmt.Sprintf("/repo/%s/", rfn), 3, "Repository Synced", "The repository has been successfully fast-forwarded with its upstream.", w, r)
				return
			}
			
			LogTemplateError(rc.LoadTemplate("repository").Execute(w, templates.RepositoryModel{
				Config: rc.Config,
				Repository: s,
				RepoHeaderInfo: *repoHeaderInfo,
				BranchList: rr.BranchIndex,
				TagList: rr.TagIndex,
				ReadmeString: readmeString,
				LoginInfo: rc.LoginInfo,
				MajorBranchPermaLink: permaLink,
				TreeFileList: treeFileList,
				CommitInfo: commitInfo,
				ComparisonInfo: compareInfo,
			}))
		},
	))

	if ctx.Config.OperationMode == gitus.OP_MODE_NORMAL {
		bindRepositoryForkController(ctx)
		bindRepositoryPullRequestController(ctx)
	}
}

