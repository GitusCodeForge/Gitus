package rrdoc

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"slices"
	"strconv"

	. "github.com/GitusCodeForge/Gitus/routes"
)

func bindAdminRRDocDeleteController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/rrdoc/{n}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			n, err := strconv.ParseInt(r.PathValue("n"), 10, 32)
			if err != nil {
				rc.ReportNotFound("Required", "document", "instance", w, r)
				return
			}
			if len(rc.Config.ReadingRequiredDocument) < int(n) {
				rc.ReportNotFound("Required", "document", "instance", w, r)
				return
			}
			p := rc.Config.ReadingRequiredDocument[int(n)-1]
			ctx.ReportSingleButtonCallback(
				fmt.Sprintf("/admin/rrdoc/{n}/delete", r.PathValue("n")),
				"Delete RRDoc",
				fmt.Sprintf("Click the following button to delete user registration document number %s: <code>%s</code>", r.PathValue("n"), p),
				"Delete",
				nil,
				w, r,
			)
		},
	))
	http.HandleFunc("POST /admin/rrdoc/{n}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired,
			CSRFCheck, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			n, err := strconv.ParseInt(r.PathValue("n"), 10, 32)
			if err != nil {
				rc.ReportNotFound("Required", "document", "instance", w, r)
				return
			}
			if len(rc.Config.ReadingRequiredDocument) < int(n) {
				rc.ReportNotFound("Required", "document", "instance", w, r)
				return
			}
			p := rc.Config.ReadingRequiredDocument[int(n)-1]
			tp := path.Join(rc.Config.StaticAssetDirectory, "_rrdoc", p.Path)
			err = os.Remove(tp)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to delete file %s: %s", p, err), w, r)
				return
			}
			rc.Config.ReadingRequiredDocument = slices.Delete(rc.Config.ReadingRequiredDocument, int(n)-1, int(n))
			// we should probably turn this into a transaction...
			rc.Config.Sync()
			rc.ReportRedirect("/admin/rrdoc", 3, "Deleted", "The document you've specified has been deleted.", w, r)
		},
	))
}

