package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/receipt"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

// /admin/receipt/{{.Id}}/edit
// /admin/receipt/{{.Id}}/confirm
// /admin/receipt/{{.Id}}/delete
func bindAdminReceiptListController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/receipt-list", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			i, err := rc.DatabaseInterface.CountAllUser()
			p := r.URL.Query().Get("p")
			if len(p) <= 0 { p = "1" }
			s := r.URL.Query().Get("s")
			if len(s) <= 0 { s = "50" }
			q := strings.TrimSpace(r.URL.Query().Get("q"))
			pageNum, err := strconv.ParseInt(p, 10, 32)
			pageSize, err := strconv.ParseInt(s, 10, 32)
			totalPage := i / pageSize
			if i % pageSize != 0 { totalPage += 1 }
			if pageNum > totalPage { pageNum = totalPage }
			if pageNum <= 1 { pageNum = 1 }
			var receiptList []*receipt.Receipt
			if len(q) > 0 {
				receiptList, err = rc.ReceiptSystem.SearchReceipt(q, int(pageNum-1), int(pageSize))
			} else {
				receiptList, err = rc.ReceiptSystem.GetAllReceipt(int(pageNum-1), int(pageSize))
			}
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to get receipt: %s", err), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("admin/receipt-list").Execute(w, &templates.AdminReceiptListTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				ErrorMsg: "",
				ReceiptList: receiptList,
				Query: q,
				PageInfo: &templates.PageInfoModel{
					PageNum: pageNum,
					PageSize: pageSize,
					TotalPage: totalPage,
				},
			}))
		},
	))

	http.HandleFunc("GET /admin/receipt/{rid}/confirm", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rid := r.PathValue("rid")
			rc.ReportRedirect(fmt.Sprintf("/receipt?id=%s", rid), 0, "Confirming Receipt", "Please proceed to confirm the receipt.", w, r)
		},
	))

	http.HandleFunc("GET /admin/receipt/{rid}/delete", UseMiddleware(
		[]Middleware{
			Logged, LoginRequired, AdminRequired, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			ctx.ReportSingleButtonCallback(
				fmt.Sprintf("/admin/receipt/{rid}/delete", r.PathValue("rid")),
				"Delete User",
				fmt.Sprintf("Click the following button to delete receipt <code>%s</code>", r.PathValue("rid")),
				"Delete",
				nil,
				w, r,
			)
		},
	))
	http.HandleFunc("POST /admin/receipt/{rid}/delete", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			CSRFCheck, GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {		rid := r.PathValue("rid")
			err := rc.ReceiptSystem.CancelReceipt(rid)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to cancel receipt: %s", err), w, r)
				return
			}
			rc.ReportRedirect("/admin/receipt-list", 5, "Receipt Cancelled", "The receipt you've specified has been cancelled.", w, r)
		},
	))
}

