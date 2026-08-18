package controller

import (
	"net/http"
	"slices"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

// list of namespace if enabled,
// list of repo if namespace is not used,
func bindAllController(ctx *routes.RouterContext) {
	if ctx.Config.UseNamespace {
		http.HandleFunc("GET /all/namespace", routes.UseMiddleware(
			[]routes.Middleware{
				routes.Logged, routes.UseLoginInfo,
				routes.GlobalVisibility,
			}, ctx,
			func(ctx *routes.RouterContext, w http.ResponseWriter, r *http.Request) {
				var err error
				q := strings.TrimSpace(r.URL.Query().Get("q"))
				var nsl map[string]*model.Namespace
				var nslCount int64
				var pageInfo *templates.PageInfoModel
				if ctx.Config.IsInForgeMode() {
					if len(q) > 0 {
						nsl, err = ctx.Config.SearchAllNamespacePlain(q)
					} else {
						nsl, err = ctx.Config.GetAllNamespacePlain()
					}
					if err != nil {
						ctx.ReportInternalError(err.Error(), w, r)
						return
					}
					nslCount = int64(len(nsl))
					pageInfo, err = routes.GeneratePageInfo(r, nslCount)
					if err != nil {
						ctx.ReportInternalError(err.Error(), w, r)
						return
					}
				} else {
					var nslCountv int64
					if len(q) > 0 {
						nslCountv, err = ctx.DatabaseInterface.CountAllVisibleNamespaceSearchResult(ctx.LoginInfo.UserName, q)
						nslCount = nslCountv
					} else {
						nslCountv, err = ctx.DatabaseInterface.CountAllVisibleNamespace(ctx.LoginInfo.UserName)
						nslCount = nslCountv
					}
					if err != nil {
						ctx.ReportInternalError(err.Error(), w, r)
						return
					}
					
					pageInfo, err = routes.GeneratePageInfo(r, nslCount)
					if err != nil {
						ctx.ReportInternalError(err.Error(), w, r)
						return
					}
					if len(q) > 0 {
						nsl, err = ctx.DatabaseInterface.SearchAllVisibleNamespacePaginated(ctx.LoginInfo.UserName, q, pageInfo.PageNum-1, pageInfo.PageSize)
					} else {
						nsl, err = ctx.DatabaseInterface.GetAllVisibleNamespacePaginated(ctx.LoginInfo.UserName, pageInfo.PageNum-1, pageInfo.PageSize)
					}
				}
				routes.LogTemplateError(ctx.LoadTemplate("all/namespace-list").Execute(w, templates.AllNamespaceListModel{
					DepotName: ctx.Config.DepotName,
					NamespaceList: nsl,
					Config: ctx.Config,
					LoginInfo: ctx.LoginInfo,
					PageInfo: pageInfo,
					Query: q,
				}))
			},
		))
	}
	
	http.HandleFunc("GET /all/repo", routes.UseMiddleware(
		[]routes.Middleware{
			routes.Logged,
			routes.UseLoginInfo,
			routes.GlobalVisibility,
		}, ctx,
		func(ctx *routes.RouterContext, w http.ResponseWriter, r *http.Request) {
			var err error
			q := strings.TrimSpace(r.URL.Query().Get("q"))
			var repol []*model.Repository
			var repolCount int64
			var pageInfo *templates.PageInfoModel
			if ctx.Config.IsInForgeMode() {
				if len(q) > 0 {
					repol, err = ctx.Config.SearchAllRepositoryPlain(q)
				} else {
					repol, err = ctx.Config.GetAllRepositoryPlain()
				}
				if err != nil {
					ctx.ReportInternalError(err.Error(), w, r)
					return
				}
				repolCount = int64(len(repol))
				pageInfo, err = routes.GeneratePageInfo(r, repolCount)
				if err != nil {
					ctx.ReportInternalError(err.Error(), w, r)
					return
				}
				slices.SortFunc(repol, func(a, b *model.Repository) int {
					if a.Namespace < b.Namespace { return -1 }
					if a.Namespace > b.Namespace { return 1 }
					if a.Name < b.Name { return -1 }
					if a.Name > b.Name { return 1 }
					return 0
				})
			} else {
				var repolCount int64
				if ctx.LoginInfo.IsAdmin {
					if len(q) > 0 {
						repolCount, err = ctx.DatabaseInterface.CountAllRepositoriesSearchResult(q)
					} else {
						repolCount, err = ctx.DatabaseInterface.CountAllRepositories()
					}
				} else {
					if len(q) > 0 {
						repolCount, err = ctx.DatabaseInterface.CountAllVisibleRepositoriesSearchResult(ctx.LoginInfo.UserName, q)
					} else {
						repolCount, err = ctx.DatabaseInterface.CountAllVisibleRepositories(ctx.LoginInfo.UserName)
					}
				}
				if err != nil {
					ctx.ReportInternalError(err.Error(), w, r)
					return
				}
				pageInfo, err = routes.GeneratePageInfo(r, repolCount)
				if err != nil {
					ctx.ReportInternalError(err.Error(), w, r)
					return
				}
				if ctx.LoginInfo.IsAdmin {
					if len(q) > 0 {
						repol, err = ctx.DatabaseInterface.SearchForRepository(q, pageInfo.PageNum - 1, pageInfo.PageSize)
					} else {
						repol, err = ctx.DatabaseInterface.GetAllRepositories(pageInfo.PageNum - 1, pageInfo.PageSize)
					}
				} else {
					if len(q) > 0 {
						repol, err = ctx.DatabaseInterface.SearchAllVisibleRepositoryPaginated(ctx.LoginInfo.UserName, q, pageInfo.PageNum - 1, pageInfo.PageSize)
					} else {
						repol, err = ctx.DatabaseInterface.GetAllVisibleRepositoryPaginated(ctx.LoginInfo.UserName, pageInfo.PageNum-1, pageInfo.PageSize)
					}
				}
			}
			routes.LogTemplateError(ctx.LoadTemplate("all/repository-list").Execute(w, templates.AllRepositoryListModel{
				RepositoryList: repol,
				DepotName: ctx.Config.DepotName,
				Config: ctx.Config,
				LoginInfo: ctx.LoginInfo,
				PageInfo: pageInfo,
				Query: q,
			}))
		},
	))
}

