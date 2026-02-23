//go:build ignore

package templates

import "github.com/GitusCodeForge/Gitus/pkg/gitus"

type LogoutTemplateModel struct {
	Config *gitus.GitusConfig
	ErrorMsg string
}

