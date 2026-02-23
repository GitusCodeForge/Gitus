//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitus"

type LogoutTemplateModel struct {
	LoginInfo *LoginInfoModel
	Config *gitus.GitusConfig
	ErrorMsg string
}

