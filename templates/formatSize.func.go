//go:build ignore
package templates

import "github.com/GitusCodeForge/Gitus/pkg/auxfuncs"

func(n int64) string {
	return auxfuncs.FormatSize(n)
}
