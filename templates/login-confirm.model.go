//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitus"

type LoginConfirmTemplateModel struct {
	Config *gitus.GitusConfig
	ErrorMsg string
	LoginInfo *LoginInfoModel
	Username string
	// EmailRequired is whether a code was sent by email.
	EmailRequired bool
	// OTPRequired is whether a code from the authenticator app is required.
	OTPRequired bool
	// OTPType is "totp" or "hotp"; it only changes the wording.
	OTPType string
}
