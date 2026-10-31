package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/receipt"
	. "github.com/GitusCodeForge/Gitus/routes"
)

func bindReceiptController(ctx *RouterContext) {
	//     /receipt?id={receipt id}
	http.HandleFunc("GET /receipt", UseMiddleware(
		[]Middleware{Logged, RateLimit, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			switch rc.Config.GlobalVisibility {
			case gitus.GLOBAL_VISIBILITY_MAINTENANCE:
				FoundAt(w, "/maintenance-notice")
				return
			case gitus.GLOBAL_VISIBILITY_SHUTDOWN:
				FoundAt(w, "/shutdown-notice")
				return
			}
			rid := r.URL.Query().Get("id")
			re, err := rc.ReceiptSystem.RetrieveReceipt(rid)
			if err != nil { FoundAt(w, "/"); return }
			if (time.Now().Unix() - re.IssueTime) >= re.TimeoutMinute*60 {
				rc.ReceiptSystem.CancelReceipt(rid)
				FoundAt(w, "/")
				return
			}
			if len(re.Command) <= 0 {
				// invalid receipt command...
				FoundAt(w, "/")
				return
			}
			switch re.Command[0] {
			case receipt.CONFIRM_REGISTRATION:
				FoundAt(w, fmt.Sprintf("/confirm-registration?id=%s", rid))
				return
			case receipt.RESET_PASSWORD:
				FoundAt(w, fmt.Sprintf("/reset-password/update-password?id=%s", rid))
				return
			case receipt.VERIFY_EMAIL:
				FoundAt(w, fmt.Sprintf("/verify-email?id=%s", rid))
				return
			}
			rc.ReportNormalError("Invalid receipt", w, r)
		},
	))
}

