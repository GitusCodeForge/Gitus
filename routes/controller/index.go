package controller

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"

	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
	"github.com/gomarkdown/markdown"
	"github.com/microcosm-cc/bluemonday"
	"github.com/niklasfasching/go-org/org"
)

func bindIndexController(ctx *RouterContext) {
	http.HandleFunc("GET /", UseMiddleware(
		[]Middleware{Logged, UseLoginInfo, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(rc.Config.FrontPage.Type, "static/") {
				frontPageContentType := strings.TrimPrefix(rc.Config.FrontPage.Type, "static/")
				f := rc.Config.FrontPage.FileContent
				var frontPageHtml string
				switch frontPageContentType {
				case "text":
					frontPageHtml = fmt.Sprintf("<pre>%s</pre>", html.EscapeString(f))
				case "markdown":
					frontPageHtml = string(markdown.ToHTML([]byte(f), nil, nil))
				case "org":
					out, err := org.New().Parse(strings.NewReader(f), "").Write(org.NewHTMLWriter())
					if err != nil {
						frontPageHtml = fmt.Sprintf("<pre>%s</pre>", f)
					} else {
						frontPageHtml = out
					}
				case "html":
					frontPageHtml = f
				}
				frontPageHtml = bluemonday.UGCPolicy().Sanitize(frontPageHtml)
				LogTemplateError(rc.LoadTemplate("index-static").Execute(w, templates.IndexStaticTemplateModel{
					Config: rc.Config,
					LoginInfo: rc.LoginInfo,
					FrontPage: frontPageHtml,
				}))
				return
			} else if rc.Config.FrontPage.Type == "all/namespace" {
				if rc.Config.UseNamespace {
					FoundAt(w, "/all/namespace")
				} else {
					log.Println("Inconsistency: all/namespace front page but instance does not support namespaces. config changed to all/repository.")
					rc.Config.FrontPage.Type = "all/repository"
					rc.Config.Sync()
					FoundAt(w, "/all/repo")
				}
			} else if rc.Config.FrontPage.Type == "all/repository" {
				FoundAt(w, "/all/repo")
			} else if rc.Config.FrontPage.Type == "namespace" {
				if !rc.Config.UseNamespace {
					frontPageHtml := "<p>Misconfiguration: a namespace is used for the front page, but the depot itself is configured to not support namespaces. Please contact the site owner about this issue.</p>"
					LogTemplateError(rc.LoadTemplate("index-static").Execute(w, templates.IndexStaticTemplateModel{
						Config: rc.Config,
						LoginInfo: rc.LoginInfo,
						FrontPage: frontPageHtml,
					}))
					return
				}
				FoundAt(w, fmt.Sprintf("/s/%s", rc.Config.FrontPage.Namespace))
			} else if rc.Config.FrontPage.Type == "repository" {
				FoundAt(w, fmt.Sprintf("/repo/%s", rc.Config.FrontPage.Repository))
			} else {
				if rc.Config.UseNamespace {
					FoundAt(w, "/all/namespace")
				} else {
					FoundAt(w, "/all/repo")
				}
			}
		},
	))
}

