package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/session"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

// buildSessionList turns the stored sessions into what the settings page
// shows: newest first, with the current one marked and the session id cut
// down to a prefix (the full id is a credential).
func buildSessionList(list []*session.GitusSession, currentId string) []templates.SessionInfoModel {
	sorted := make([]*session.GitusSession, len(list))
	copy(sorted, list)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Timestamp == sorted[j].Timestamp { return sorted[i].Id < sorted[j].Id }
		return sorted[i].Timestamp > sorted[j].Timestamp
	})
	res := make([]templates.SessionInfoModel, 0, len(sorted))
	for _, s := range sorted {
		prefix := s.Id
		if len(prefix) > 8 { prefix = prefix[:8] + "..." }
		res = append(res, templates.SessionInfoModel{
			Id: s.Id,
			IdPrefix: prefix,
			CreatedAt: s.Timestamp,
			ExpiresAt: s.ExpireTimestamp,
			IP: s.IP,
			Device: session.DescribeUserAgent(s.UserAgent),
			Current: len(currentId) > 0 && s.Id == currentId,
		})
	}
	return res
}

func bindSettingSessionController(ctx *RouterContext) {
	http.HandleFunc("GET /setting/session", UseMiddleware(
		[]Middleware{Logged, LoginRequired, GlobalVisibility, ErrorGuard}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := rc.LoginInfo.UserName
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			list, err := rc.SessionInterface.RetrieveSession(un)
			if err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to retrieve sessions: %s", err.Error()), w, r)
				return
			}
			LogTemplateError(rc.LoadTemplate("setting/session").Execute(w, &templates.SettingSessionTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				SessionList: buildSessionList(list, rc.LoginInfo.UserSessionId),
			}))
		},
	))

	http.HandleFunc("POST /setting/session", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			UseLoginInfo, LoginRequired, CSRFCheck,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			un := rc.LoginInfo.UserName
			if !model.ValidUserName(un) {
				rc.ReportNotFound(un, "User", "Depot", w, r)
				return
			}
			action := strings.TrimSpace(r.Form.Get("type"))
			if action != "revoke" {
				rc.ReportNormalError("Invalid request", w, r)
				return
			}
			id := strings.TrimSpace(r.Form.Get("id"))
			if len(id) <= 0 {
				rc.ReportRedirect("/setting/session", 5, "Invalid Request",
					"No session was specified.", w, r)
				return
			}
			if id == rc.LoginInfo.UserSessionId {
				rc.ReportRedirect("/setting/session", 5, "Not Allowed",
					"This is the session you are using right now; use the log out link instead.", w, r)
				return
			}
			// RevokeSession only touches rows of this user, so a forged id
			// cannot end somebody else's session.
			if err := rc.SessionInterface.RevokeSession(un, id); err != nil {
				rc.ReportInternalError(fmt.Sprintf("Failed to revoke the session: %s", err.Error()), w, r)
				return
			}
			rc.ReportRedirect("/setting/session", 5, "Done", "The session has been revoked.", w, r)
		},
	))
}
