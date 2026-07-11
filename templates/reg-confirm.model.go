//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitus"

type RegConfirmTemplateModel struct {
	Config *gitus.GitusConfig
	ErrorMsg string
	LoginInfo *LoginInfoModel
	ReceiptID string
	Username string
}

