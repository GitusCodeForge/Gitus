package rrdoc

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindAdminRRDocEditController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/rrdoc/{n}/edit", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			n, err := strconv.ParseInt(r.PathValue("n"), 10, 32)
			if err != nil {
				rc.ReportNotFound("Required", "document", "instance", w, r)
				return
			}
			if n < 1 || n > int64(len(rc.Config.ReadingRequiredDocument)) {
				rc.ReportNotFound("Required", "document", "instance", w, r)
				return
			}
			p := rc.Config.ReadingRequiredDocument[int(n)-1]
			content := ""
			if !(strings.HasPrefix(p.Path, "http://") || strings.HasPrefix(p.Path, "https://")) {
				os.MkdirAll(RRDocDirectory(rc.Config.StaticAssetDirectory), os.ModeDir|0755)
				fp, err := ResolveRRDocPath(rc.Config.StaticAssetDirectory, p.Path)
				if err != nil {
					rc.ReportNotFound(p.Path, "document", "instance", w, r)
					return
				}
				f, err := os.ReadFile(fp)
				if err != nil {
					if !errors.Is(err, os.ErrNotExist) {
						rc.ReportInternalError(fmt.Sprintf("Cannot read file %s: %s", p.Path, err.Error()), w, r)
					}
				} else {
					content = string(f)
				}
			}
			w.Header().Add("Content-Type", "text/html")
			w.WriteHeader(200)
			LogTemplateError(rc.LoadTemplate("admin/rrdoc/edit").Execute(w, &templates.AdminRRDocEditTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				DocumentNumber: int(n),
				Title: p.Title,
				Path: p.Path,
				Content: content,
			}))
		},
	))

	http.HandleFunc("POST /admin/rrdoc/{n}/edit", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, CSRFCheck, AdminRequired,
			ValidPOSTRequestRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			n, err := strconv.ParseInt(r.PathValue("n"), 10, 32)
			if err != nil {
				rc.ReportNotFound("Required", "document", "instance", w, r)
				return
			}
			if n < 1 || n > int64(len(rc.Config.ReadingRequiredDocument)) {
				rc.ReportNotFound("Required", "document", "instance", w, r)
				return
			}
			title := r.Form.Get("title")
			p := strings.TrimSpace(r.Form.Get("path"))
			content := r.Form.Get("content")
			isRemote := strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://")
			fp := ""
			if !isRemote {
				var err error
				fp, err = ResolveRRDocPath(rc.Config.StaticAssetDirectory, p)
				if err != nil {
					rc.ReportNormalError("The document path must stay inside the document directory.", w, r)
					return
				}
			}
			rc.Config.ReadingRequiredDocument[n-1].Title = title
			rc.Config.ReadingRequiredDocument[n-1].Path = p
			// don't sync until we have properly edited the file.
			if !isRemote {
				os.MkdirAll(RRDocDirectory(rc.Config.StaticAssetDirectory), os.ModeDir|0755)
				f, err := os.OpenFile(fp, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to open file %s: %s", p, err), w, r)
					return
				}
				_, err = f.Write([]byte(content))
				if err != nil {
					rc.ReportInternalError(fmt.Sprintf("Failed to write to file: %s", err), w, r)
					return
				}
				f.Close()
			}
			// we should probably turn this into a transaction...
			rc.Config.Sync()
			rc.ReportRedirect(fmt.Sprintf("/admin/rrdoc/%d/edit", n), 3, "Updated", "The document you've specified has been updated.", w, r)
		},
	))
}

