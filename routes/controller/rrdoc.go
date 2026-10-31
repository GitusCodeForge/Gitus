package controller

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
	"github.com/gomarkdown/markdown"

	// "github.com/gomarkdown/markdown"
	"github.com/niklasfasching/go-org/org"
)

func bindRRDocController(ctx *RouterContext) {
	http.HandleFunc("GET /rrdoc/{p}", UseMiddleware(
		[]Middleware{ Logged, UseLoginInfo }, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			switch ctx.Config.GlobalVisibility {
			case gitus.GLOBAL_VISIBILITY_MAINTENANCE:
				FoundAt(w, "/maintenance-notice")
			case gitus.GLOBAL_VISIBILITY_PRIVATE:
				if !rc.LoginInfo.LoggedIn {
					FoundAt(w, "/login")
				}
			case gitus.GLOBAL_VISIBILITY_SHUTDOWN:
				FoundAt(w, "/shutdown-notice")
			}
			found := false
			for _, k := range rc.Config.ReadingRequiredDocument {
				if k.Path == r.PathValue("p") {
					found = true
				}
			}
			// we don't handle http:// and https:// here because we are to render
			// them as direct links to those addresses.
			if strings.HasPrefix(r.PathValue("p"), "http://") ||
				strings.HasPrefix(r.PathValue("p"), "https://") ||
				!found {
				rc.ReportNotFound(r.PathValue("p"), "document", "instance", w, r)
				return
			}
			fp := path.Join(rc.Config.StaticAssetDirectory, "_rrdoc", r.PathValue("p"))
			e := path.Ext(fp)
			var sourceString string
			source, err := os.ReadFile(fp)
			if errors.Is(err, os.ErrNotExist) {
				sourceString = "(file not found)"
				goto _sourceStringRendered
			}
			if err != nil {
				rc.ReportInternalError(err.Error(), w, r)
				return
			}
			switch e {
			case ".md":
				sourceString = string(markdown.ToHTML(source, nil, nil))
				sourceString = ugcSanitizer.Sanitize(sourceString)
			case ".org":
				doc := org.New().Parse(bytes.NewReader(source), "")
				out, err := doc.Write(org.NewHTMLWriter())
				if err != nil {
					sourceString = ugcSanitizer.Sanitize(string(source))
					sourceString = fmt.Sprintf("<pre>%s</pre>", sourceString)
				} else {
					sourceString = ugcSanitizer.Sanitize(out)
				}
			case ".htm": fallthrough
			case ".html":
				sourceString = ugcSanitizer.Sanitize(string(source))
			default:
				sourceString = ugcSanitizer.Sanitize(string(source))
				sourceString = fmt.Sprintf("<pre>%s</pre>", sourceString)
			}
		_sourceStringRendered:
			w.Header().Add("Content-Type", "text/html")
			w.WriteHeader(200)
			LogTemplateError(rc.LoadTemplate("rrdoc").Execute(w, &templates.RRDocTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				Title: rc.Config.GetRRDocTitle(r.PathValue("p")),
				DocumentContent: sourceString,
			}))
		},
	))
}
	
