//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitus"

type SettingSessionTemplateModel struct {
	Config *gitus.GitusConfig
	LoginInfo *LoginInfoModel
	SessionList []SessionInfoModel
	ErrorMsg string
}

// SessionInfoModel is one row of the active session list. Id is the session
// id as stored, which is used for the revoke form; IdPrefix is what is shown.
type SessionInfoModel struct {
	Id string
	IdPrefix string
	CreatedAt int64
	ExpiresAt int64
	IP string
	Device string
	Current bool
}
