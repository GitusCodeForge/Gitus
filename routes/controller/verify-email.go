package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/receipt"
	"github.com/GitusCodeForge/Gitus/routes"
	. "github.com/GitusCodeForge/Gitus/routes"
)

func bindVerifyEmailController(ctx *routes.RouterContext) {
	http.HandleFunc("GET /verify-email", UseMiddleware(
		[]Middleware{
			Logged, GlobalVisibility,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rid := strings.TrimSpace(r.URL.Query().Get("id"))
			if len(rid) <= 0 { routes.FoundAt(w, "/setting/email"); return }
			re, err := rc.ReceiptSystem.RetrieveReceipt(rid)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while retrieving receipt: %s\n", err), w, r)
				return
			}
			if re.Expired() {
				rc.ReceiptSystem.CancelReceipt(rid)
				rc.ReportRedirect("/", 5, "Receipt Expired", "The receipt you've received has passed its validity time limit. Please go through the process again.", w, r)
				return
			}
			if len(re.Command) != 3 || re.Command[0] != receipt.VERIFY_EMAIL {
				rc.ReceiptSystem.CancelReceipt(rid)
				rc.ReportRedirect("/", 5, "Invalid Receipt", "The receipt you've provided is invalid. Please try again.", w, r)
				return
			}
			err = rc.DatabaseInterface.VerifyRegisteredEmail(re.Command[1], re.Command[2])
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed while retrieving receipt: %s\n", err), w, r)
				return
			}
			rc.ReportRedirect("/", 0, "Email Verified", "The request email is verified successfully.", w, r)
		},
	))
}

