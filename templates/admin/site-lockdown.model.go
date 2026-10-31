//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitus"
import "github.com/GitusCodeForge/Gitus/pkg/gitus/model"

type AdminSiteLockdownTemplateModel struct {
	Config *gitus.GitusConfig
	LoginInfo *LoginInfoModel
	CurrentMode string
	PrivateNoticeMessage string
	ShutdownNoticeMessage string
	FullAccessUser string
	MaintenanceNoticeMessage string
	AnnouncementEnabled bool
	AnnouncementMessage string
	AnnouncementStartsAt string
	AnnouncementEndsAt string
	AnnouncementMode string
	AnnouncementAutoRestore bool
}

