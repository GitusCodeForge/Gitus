package rrdoc

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindAdminRRDocNewController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/rrdoc/new", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "text/html")
			w.WriteHeader(200)
			LogTemplateError(rc.LoadTemplate("admin/rrdoc/edit").Execute(w, &templates.AdminRRDocEditTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				DocumentNumber: -1,
				Title: "",
				Path: "",
				Content: "",
			}))
		},
	))

	http.HandleFunc("POST /admin/rrdoc/new", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, CSRFCheck, AdminRequired,
			ValidPOSTRequestRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
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
			rc.Config.ReadingRequiredDocument = append(rc.Config.ReadingRequiredDocument, struct{Title string; Path string}{
				Title: title,
				Path: p,
			})
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
			rc.ReportRedirect("/admin/rrdoc/new", 3, "Updated", "The document you've specified has been updated.", w, r)
		},
	))
}

