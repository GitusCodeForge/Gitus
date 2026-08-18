//go:build ignore

package templates

type SingleButtonCallbackModel struct {
	Config *gitus.GitusConfig
	LoginInfo *LoginInfoModel
	ErrorMsg string
	TargetUrl string
	AccompanyingData map[string]string
	ActionTitle string
	ActionText string
	ButtonText string
}

