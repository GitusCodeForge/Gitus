package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/routes"
	. "github.com/GitusCodeForge/Gitus/routes"
)

func printGitError(w io.Writer, s string) {
	ss, _ := gitlib.ToPktLine(fmt.Sprintf("ERR %s\n", s))
	fmt.Fprint(w, ss)
}


// info/refs
// HEAD
// objects/

// NOTE THAT this route handles public http read-only clone, thus it
// will report 404 for all private repositories.

func bindHttpCloneController(ctx *RouterContext) {
	http.HandleFunc("GET /repo/{repoName}/info/{p...}", UseMiddleware(
		[]Middleware{ Logged }, ctx,
		func(ctx *routes.RouterContext, w http.ResponseWriter, r *http.Request) {
			allowV2 := ctx.Config.GitConfig.HTTPCloneProtocol.V2
			allowV1Dumb := ctx.Config.GitConfig.HTTPCloneProtocol.V1Dumb
			if !allowV1Dumb && !allowV2 {
				w.WriteHeader(403)
				fmt.Fprint(w, "HTTP clone not supported on this instance")
				return
			}
			if ctx.Config.GlobalVisibility != gitus.GLOBAL_VISIBILITY_PUBLIC {
				ctx.ReportForbidden("", w, r)
				return
			}
			rfn := r.PathValue("repoName")
			if !model.ValidRepositoryName(rfn) {
				ctx.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			_, _, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err == routes.ErrNotFound {
				ctx.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if repo.Type != model.REPO_TYPE_GIT {
				ctx.ReportNormalError("The repository you have requested isn't a Git repository.", w, r)
				return
			}
			isNamespacePublic := ns.Status == model.NAMESPACE_NORMAL_PUBLIC
			isRepoPublic := repo.Status == model.REPO_NORMAL_PUBLIC
			isRepoArchived := repo.Status == model.REPO_ARCHIVED
			if !isNamespacePublic || !(isRepoPublic || isRepoArchived) {
				w.WriteHeader(404)
				fmt.Fprint(w, "404 Not Found")
				return
			}
			// see docs/http-clone.org.
			if (r.URL.Query().Has("service") && allowV2) {
				switch r.URL.Query().Get("service") {
				case "git-upload-pack":
					cmd := exec.Command("git", "upload-pack", repo.LocalPath, "--http-backend-info-refs")
					cmd.Dir = repo.LocalPath
					protocol := r.Header.Get("Git-Protocol")
					if protocol == "" { protocol = "version=2" }
					cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_PROTOCOL=%s", protocol))
					stdout := new(bytes.Buffer)
					cmd.Stdout = stdout
					err := cmd.Run()
					if err != nil {
						w.WriteHeader(500)
						printGitError(w, err.Error())
						return
					}
					w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
					w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
					w.WriteHeader(200)
					fmt.Fprint(w, "001e# service=git-upload-pack\n")
					fmt.Fprint(w, "0000")
					w.Write(stdout.Bytes())
				default:
					w.WriteHeader(403)
					printGitError(w, "Not supported service.")
				}
				return
			}
			// v1-dumb
			if !allowV1Dumb {
				w.WriteHeader(403)
				fmt.Fprint(w, "v1-dumb protocl not supported on this instance.")
				return
			}
			rr := repo.Repository.(*gitlib.LocalGitRepository)
			p := path.Clean(path.Join(rr.GitDirectoryPath, "info", r.PathValue("p")))
			chk, err := filepath.Rel(rr.GitDirectoryPath, p)
			if err != nil || strings.HasPrefix(chk, "..") {
				ctx.ReportInternalError("Failed to read info/refs", w, r)
				return
			}
			s, err := os.ReadFile(p)
			if err != nil {
				ctx.ReportInternalError("Failed to read info/refs", w, r)
				return
			}
			w.Write(s)
		}))
	http.HandleFunc("POST /repo/{repoName}/git-upload-pack", UseMiddleware(
		[]Middleware{ Logged }, ctx,
		func(ctx *RouterContext, w http.ResponseWriter, r *http.Request) {
			if !ctx.Config.GitConfig.HTTPCloneProtocol.V2 {
				w.WriteHeader(403)
				fmt.Fprint(w, "v2 protocl not supported on this instance.")
				return
			}
			if ctx.Config.GlobalVisibility != gitus.GLOBAL_VISIBILITY_PUBLIC {
				w.WriteHeader(403)
				w.Write([]byte("Service not available right now."))
				return
			}
			rfn := r.PathValue("repoName")
			if !model.ValidRepositoryName(rfn) {
				w.WriteHeader(404)
				w.Write([]byte("Repository not found."))
				return
			}
			_, _, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err == routes.ErrNotFound {
				w.WriteHeader(404)
				w.Write([]byte("Repository not found."))
				return
			}
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if repo.Type != model.REPO_TYPE_GIT {
				w.WriteHeader(403)
				w.Write([]byte("Repository not Git."))
				return
			}
			isNamespacePublic := ns.Status == model.NAMESPACE_NORMAL_PUBLIC
			isRepoPublic := repo.Status == model.REPO_NORMAL_PUBLIC
			isRepoArchived := repo.Status == model.REPO_ARCHIVED
			if !isNamespacePublic || !(isRepoPublic || isRepoArchived) {
				w.WriteHeader(404)
				w.Write([]byte("404 Not Found"))
				return
			}
			w.Header().Set("Content-Type", "application/x-git-upload-pack-response")
			w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
			w.WriteHeader(200)
			cmd := exec.Command("git", "upload-pack", repo.LocalPath, "--stateless-rpc")
			cmd.Stdin = r.Body
			protocol := r.Header.Get("Git-Protocol")
			if protocol == "" { protocol = "version=2" }
			cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_PROTOCOL=%s", protocol))
			buf := new(bytes.Buffer)
			cmd.Stdout = buf
			cmd.Run()
			io.Copy(w, buf)
		}))
	http.HandleFunc("GET /repo/{repoName}/HEAD", UseMiddleware(
		[]Middleware{ Logged }, ctx,
		func(ctx *RouterContext, w http.ResponseWriter, r *http.Request) {
			if ctx.Config.GlobalVisibility != gitus.GLOBAL_VISIBILITY_PUBLIC {
				ctx.ReportForbidden("", w, r)
				return
			}
			rfn := r.PathValue("repoName")
			if !model.ValidRepositoryName(rfn) {
				ctx.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			_, _, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err == routes.ErrNotFound {
				ctx.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if repo.Type != model.REPO_TYPE_GIT {
				ctx.ReportNormalError("The repository you have requested isn't a Git repository.", w, r)
				return
			}
			isNamespacePublic := ns.Status == model.NAMESPACE_NORMAL_PUBLIC
			isRepoPublic := repo.Status == model.REPO_NORMAL_PUBLIC
			isRepoArchived := repo.Status == model.REPO_ARCHIVED
			if !isNamespacePublic || !(isRepoPublic || isRepoArchived) {
				w.WriteHeader(404)
				w.Write([]byte("404 Not Found"))
				return
			}
			rr := repo.Repository.(*gitlib.LocalGitRepository)
			p := path.Join(rr.GitDirectoryPath, "HEAD")
			s, err := os.ReadFile(p)
			if err != nil {
				ctx.ReportInternalError("Fail to read info/refs", w, r)
				return
			}
			w.Write(s)
		}))
	http.HandleFunc("GET /repo/{repoName}/objects/{obj...}", UseMiddleware(
		[]Middleware{ Logged }, ctx,
		func(ctx *RouterContext, w http.ResponseWriter, r *http.Request) {
			if ctx.Config.GlobalVisibility != gitus.GLOBAL_VISIBILITY_PUBLIC {
				ctx.ReportForbidden("", w, r)
				return
			}
			rfn := r.PathValue("repoName")
			if !model.ValidRepositoryName(rfn) {
				ctx.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			_, _, ns, repo, err := ctx.ResolveRepositoryFullName(rfn)
			if err == routes.ErrNotFound {
				ctx.ReportNotFound(rfn, "Repository", "Depot", w, r)
				return
			}
			if err != nil {
				ctx.ReportInternalError(err.Error(), w, r)
				return
			}
			if repo.Type != model.REPO_TYPE_GIT {
				ctx.ReportNormalError("The repository you have requested isn't a Git repository.", w, r)
				return
			}
			isNamespacePublic := ns.Status == model.NAMESPACE_NORMAL_PUBLIC
			isRepoPublic := repo.Status == model.REPO_NORMAL_PUBLIC
			isRepoArchived := repo.Status == model.REPO_ARCHIVED
			if !isNamespacePublic || !(isRepoPublic || isRepoArchived) {
				w.WriteHeader(404)
				w.Write([]byte("404 Not Found"))
				return
			}
			obj := r.PathValue("obj")
			rr := repo.Repository.(*gitlib.LocalGitRepository)
			p := path.Clean(path.Join(rr.GitDirectoryPath, "objects", obj))
			chk, err := filepath.Rel(rr.GitDirectoryPath, p)
			if err != nil || strings.HasPrefix(chk, "..") {
				ctx.ReportNormalError("Failed to read object", w, r)
				return
			}
			s, err := os.ReadFile(p)
			if os.IsNotExist(err) {
				ctx.ReportNotFound(rfn, "object", ctx.Config.DepotName, w, r)
				return
			}
			if err != nil {
				ctx.ReportInternalError("Fail to read info/refs", w, r)
				return
			}
			w.Write(s)
		}))
}
