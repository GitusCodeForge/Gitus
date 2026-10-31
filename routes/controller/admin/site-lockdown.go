package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	. "github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
)

func bindAdminSiteLockdownController(ctx *RouterContext) {
	http.HandleFunc("GET /admin/site-lockdown", UseMiddleware(
		[]Middleware{Logged, LoginRequired, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,
		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			announcement := rc.Config.MaintenanceAnnouncement
			announcementMode := announcement.Mode
			if announcementMode != gitus.GLOBAL_VISIBILITY_MAINTENANCE {
				announcementMode = gitus.GLOBAL_VISIBILITY_SHUTDOWN
			}
			LogTemplateError(rc.LoadTemplate("admin/site-lockdown").Execute(w, &templates.AdminSiteLockdownTemplateModel{
				Config: rc.Config,
				LoginInfo: rc.LoginInfo,
				CurrentMode: rc.Config.EffectiveGlobalVisibility(),
				PrivateNoticeMessage: rc.Config.PrivateNoticeMessage,
				ShutdownNoticeMessage: rc.Config.ShutdownMessage,
				FullAccessUser: strings.Join(rc.Config.FullAccessUser, ","),
				MaintenanceNoticeMessage: rc.Config.MaintenanceMessage,
				AnnouncementEnabled: announcement.Enabled,
				AnnouncementMessage: announcement.Message,
				AnnouncementStartsAt: toDatetimeLocal(announcement.StartsAt),
				AnnouncementEndsAt: toDatetimeLocal(announcement.EndsAt),
				AnnouncementMode: announcementMode,
				AnnouncementAutoRestore: announcement.AutoRestore,
			}))
		},
	))
	
	http.HandleFunc("POST /admin/site-lockdown", UseMiddleware(
		[]Middleware{Logged, ValidPOSTRequestRequired,
			LoginRequired, CSRFCheck, AdminRequired,
			GlobalVisibility, ErrorGuard,
		}, ctx,

		func(rc *RouterContext, w http.ResponseWriter, r *http.Request) {
			rc.Config.LockForSync()
			defer rc.Config.Unlock()
			t := strings.TrimSpace(r.Form.Get("type"))
			switch t {
			case "public":
				rc.Config.GlobalVisibility = gitus.GLOBAL_VISIBILITY_PUBLIC
			case "private":
				rc.Config.GlobalVisibility = gitus.GLOBAL_VISIBILITY_PRIVATE
				rc.Config.PrivateNoticeMessage = r.Form.Get("private-notice-message")
			case "shutdown":
				rc.Config.GlobalVisibility = gitus.GLOBAL_VISIBILITY_SHUTDOWN
				ul := make([]string, 0)
				for k := range strings.SplitSeq(r.Form.Get("full-access-user"), ",") {
					ul = append(ul, strings.TrimSpace(k))
				}
				rc.Config.FullAccessUser = ul
				rc.Config.ShutdownMessage = r.Form.Get("shutdown-notice-message")
			case "maintenance":
				rc.Config.GlobalVisibility = gitus.GLOBAL_VISIBILITY_MAINTENANCE
				rc.Config.MaintenanceMessage = r.Form.Get("maintenance-notice-message")
			case "announcement":
				startsAt, err := parseDatetimeLocal(r.Form.Get("announcement-start"))
				if err != nil {
					rc.ReportRedirect("/admin/site-lockdown", 3, "Invalid Announcement", fmt.Sprintf("Invalid start time: %s", err.Error()), w, r)
					return
				}
				endsAt, err := parseDatetimeLocal(r.Form.Get("announcement-end"))
				if err != nil {
					rc.ReportRedirect("/admin/site-lockdown", 3, "Invalid Announcement", fmt.Sprintf("Invalid end time: %s", err.Error()), w, r)
					return
				}
				autoRestore := r.Form.Has("announcement-auto-restore")
				if autoRestore && (startsAt == "" || endsAt == "") {
					rc.ReportRedirect("/admin/site-lockdown", 3, "Invalid Announcement", "Automatic recovery needs both a start time and an end time.", w, r)
					return
				}
				if startsAt != "" && endsAt != "" {
					st, _ := time.Parse(time.RFC3339, startsAt)
					en, _ := time.Parse(time.RFC3339, endsAt)
					if !en.After(st) {
						rc.ReportRedirect("/admin/site-lockdown", 3, "Invalid Announcement", "The end time must be after the start time.", w, r)
						return
					}
				}
				mode := strings.TrimSpace(r.Form.Get("announcement-mode"))
				if mode != gitus.GLOBAL_VISIBILITY_MAINTENANCE {
					mode = gitus.GLOBAL_VISIBILITY_SHUTDOWN
				}
				rc.Config.MaintenanceAnnouncement = gitus.GitusMaintenanceAnnouncement{
					Enabled: r.Form.Has("announcement-enabled"),
					Message: strings.TrimSpace(r.Form.Get("announcement-message")),
					StartsAt: startsAt,
					EndsAt: endsAt,
					Mode: mode,
					AutoRestore: autoRestore,
				}
			}
			err := rc.Config.Sync()
			if err != nil {
				rc.ReportRedirect("/admin/site-lockdown", 0, "Internal Error", fmt.Sprintf("Failed to save config due to error: %s", err.Error()), w, r)
				return
			}
			rc.ReportRedirect("/admin/site-lockdown", 3, "Configuration Saved", "Configuration saved.", w, r)
		},
	))
}

func toDatetimeLocal(s string) string {
	s = strings.TrimSpace(s)
	if s == "" { return "" }
	t, err := time.Parse(time.RFC3339, s)
	if err != nil { return "" }
	return t.Local().Format("2006-01-02T15:04")
}

func parseDatetimeLocal(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" { return "", nil }
	layouts := []string{"2006-01-02T15:04", "2006-01-02T15:04:05"}
	var err error
	for _, layout := range layouts {
		var t time.Time
		t, err = time.ParseInLocation(layout, s, time.Local)
		if err == nil { return t.Format(time.RFC3339), nil }
	}
	return "", err
}
