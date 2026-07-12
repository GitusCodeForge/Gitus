package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

// /admin/repo-list?p={pagenum}&s={pagesize}&q={query}
func bindAdminRepositoryListController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/repo-list", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			i, err := rc.DatabaseInterface.CountAllRepositories()
			p := r.URL.Query().Get("p")
			if len(p) <= 0 { p = "1" }
			s := r.URL.Query().Get("s")
			if len(s) <= 0 { s = "50" }
			q := strings.TrimSpace(r.URL.Query().Get("q"))
			pageNum, err := strconv.ParseInt(p, 10, 64)
			pageSize, err := strconv.ParseInt(s, 10, 64)
			totalPage := i / pageSize
			if i % pageSize != 0 { totalPage += 1 }
			if pageNum > totalPage { pageNum = totalPage }
			if pageNum <= 1 { pageNum = 1 }
			var repoList []*model.Repository
			if len(q) > 0 {
				repoList, err = rc.DatabaseInterface.SearchForRepository(q, pageNum-1, pageSize)
			} else {
				repoList, err = rc.DatabaseInterface.GetAllRepositories(pageNum-1, pageSize)
			}
			if err != nil {
				LogTemplateError(rc.LoadTemplate("admin/repo-list").Execute(w, &templates.AdminRepositoryListTemplateModel{
					Config: rc.Config,
					LoginInfo: rc.LoginInfo,
					Query: q,
					ErrorMsg: fmt.Sprintf("Failed to load repository list: %s", err.Error()),
					RepositoryList: nil,
				}))
				return
			}
			LogTemplateError(rc.LoadTemplate("admin/repo-list").Execute(w, &templates.AdminRepositoryListTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "",
				RepositoryList: repoList,
				Query: q,
				PageInfo: &templates.PageInfoModel{
					PageNum: pageNum,
					PageSize: pageSize,
					TotalPage: totalPage,
				},
			}))

		},
	))

}

